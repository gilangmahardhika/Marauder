package mcp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/mcp/mcptest"
	"orchestrator/backend/internal/netguard"
)

var lookup = mcp.Tool{Name: "lookup", Description: "find a ticket", Schema: map[string]any{
	"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []any{"id"},
}}

func client(t *testing.T, srv *mcptest.Server, headers map[string]string) mcp.Client {
	t.Helper()
	c, err := mcp.DialHTTP(context.Background(), srv.URL+"/mcp", headers, netguard.Client(netguard.Policy{AllowLoopbackHTTP: true}, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestMCPHTTPListAndCall(t *testing.T) {
	for _, sse := range []bool{false, true} {
		srv := mcptest.New([]mcp.Tool{lookup})
		srv.SSE = sse
		c := client(t, srv, map[string]string{"Authorization": "Bearer secret-token"})
		tools, err := c.ListTools(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(tools) != 1 || tools[0].Name != "lookup" || tools[0].Schema["type"] != "object" {
			t.Fatalf("tools = %+v", tools)
		}
		res, err := c.CallTool(context.Background(), "lookup", map[string]any{"id": "T-1"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != `{"id":"T-1"}`+"\n"+mcp.NonText || res.IsError {
			t.Fatalf("result = %+v", res)
		}
		last := srv.Headers[len(srv.Headers)-1]
		if last.Get("Authorization") != "Bearer secret-token" || last.Get("Mcp-Session-Id") != "sess-1" || last.Get("MCP-Protocol-Version") != mcp.ProtocolVersion {
			t.Fatalf("headers = %v", last)
		}
		if sse && len(srv.Replies) == 0 {
			t.Fatal("sampling request was not answered")
		}
		for _, reply := range srv.Replies {
			if !strings.Contains(string(reply), "-32601") {
				t.Fatalf("server request answered with %s", reply)
			}
		}
		srv.Close()
	}
}

func TestMCPResultCap(t *testing.T) {
	srv := mcptest.New([]mcp.Tool{lookup})
	defer srv.Close()
	big := strings.Repeat("x", mcp.MaxResponse+10)
	srv.Raw = []byte(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"` + big + `"}]}}`)
	c := client(t, srv, nil)
	if _, err := c.CallTool(context.Background(), "lookup", nil); !errors.Is(err, mcp.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}

	blocked, err := mcp.DialHTTP(context.Background(), srv.URL, nil, netguard.Client(netguard.Policy{}, nil))
	if !errors.Is(err, mcp.ErrBlocked) || blocked != nil {
		t.Fatalf("loopback dial err = %v", err)
	}
}
