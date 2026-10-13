// Package mcptest is a Streamable HTTP MCP server for tests.
package mcptest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"orchestrator/backend/internal/mcp"
)

// Server answers initialize, tools/list, and tools/call. Call returns a
// tool's text and isError; nil echoes the arguments.
type Server struct {
	*httptest.Server
	Tools []mcp.Tool
	Call  func(name string, args map[string]any) (string, bool)
	// SSE answers requests as server-sent events, after a sampling request.
	SSE bool
	// Raw, when set, is written as the tools/call response body.
	Raw []byte

	mu      sync.Mutex
	Headers []http.Header
	Calls   []string
	Replies []json.RawMessage
}

func New(tools []mcp.Tool) *Server {
	s := &Server{Tools: tools}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.Headers = append(s.Headers, r.Header.Clone())
	s.mu.Unlock()
	if r.Method == http.MethodDelete {
		w.WriteHeader(200)
		return
	}
	var msg struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		w.WriteHeader(400)
		return
	}
	if msg.Method == "" {
		s.mu.Lock()
		s.Replies = append(s.Replies, msg.Error)
		s.mu.Unlock()
		w.WriteHeader(202)
		return
	}
	if len(msg.ID) == 0 {
		w.WriteHeader(202)
		return
	}
	var result any
	switch msg.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "sess-1")
		result = map[string]any{"protocolVersion": mcp.ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "test", "version": "1"}}
	case "tools/list":
		result = map[string]any{"tools": s.Tools}
	case "tools/call":
		s.mu.Lock()
		s.Calls = append(s.Calls, msg.Params.Name)
		s.mu.Unlock()
		if s.Raw != nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(s.Raw)
			return
		}
		text, isErr := "", false
		if s.Call != nil {
			text, isErr = s.Call(msg.Params.Name, msg.Params.Arguments)
		} else {
			raw, _ := json.Marshal(msg.Params.Arguments)
			text = string(raw)
		}
		result = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}, map[string]any{"type": "image", "data": "AAAA"}}, "isError": isErr}
	default:
		writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": map[string]any{"code": -32601, "message": "no"}})
		return
	}
	resp := map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result}
	if s.SSE {
		w.Header().Set("Content-Type", "text/event-stream")
		sampling, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "sampling/createMessage", "params": map[string]any{}})
		raw, _ := json.Marshal(resp)
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", sampling)
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", raw)
		return
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// CallCount is the number of tools/call requests served.
func (s *Server) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Calls)
}
