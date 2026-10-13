package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/mcp/mcptest"
	"orchestrator/backend/internal/netguard"
	"orchestrator/backend/internal/tenancy"
)

const refineBearer = "tok-refine-0123456789"

// refineMCP gives tribe_default a "tickets" server with a lookup tool.
func refineMCP(t *testing.T, srv *httpapi.Server, enabled bool) *mcptest.Server {
	t.Helper()
	schema := map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []any{"id"}}
	mcpSrv := mcptest.New([]mcp.Tool{{Name: "lookup", Schema: schema}})
	t.Cleanup(mcpSrv.Close)
	mcpSrv.Call = func(_ string, args map[string]any) (string, bool) {
		return "Ticket " + args["id"].(string) + ": members need a CSV export. " + refineBearer, false
	}
	srv.MCP = app.MCPDialer{Policy: netguard.Policy{AllowLoopbackHTTP: true}}
	var adminID string
	if err := srv.DB.QueryRow(`SELECT id FROM users WHERE email = 'ada@example.com'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	_, _, err := app.CreateMCPServer(context.Background(), srv.DB, app.User{ID: adminID, Admin: true}, srv.TokenKey, srv.MCP, "tribe_default", app.MCPServerInput{
		Slug: "tickets", Transport: "http", URL: mcpSrv.URL + "/mcp", Enabled: &enabled,
		Secrets: map[string]string{"Authorization": "Bearer " + refineBearer},
		Tools:   []tenancy.MCPTool{{Name: "lookup", Description: "Find a ticket by id.", Schema: schema}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return mcpSrv
}

func projectToolCalls(t *testing.T, srv *httpapi.Server) []map[string]any {
	t.Helper()
	rows, err := srv.DB.Query(`SELECT payload_json FROM events WHERE kind = 'tool_call' AND card_id IS NULL AND project_id = 'proj_default' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatal(err)
		}
		out = append(out, payload)
	}
	return out
}

// shownText is the field text a client shows: deltas, cleared by reset.
func shownText(events []refineEvent) string {
	var b strings.Builder
	for _, ev := range events {
		switch ev.Kind {
		case "delta":
			b.WriteString(ev.Data["text"].(string))
		case "reset":
			b.Reset()
		}
	}
	return b.String()
}

func kinds(events []refineEvent) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.Kind
	}
	return out
}

func TestRefineNoServersNoTools(t *testing.T) {
	for _, withServer := range []bool{false, true} {
		srv, provider, dev, _ := refineServer(t, refineRouter, refineReply)
		var mcpSrv *mcptest.Server
		if withServer {
			mcpSrv = refineMCP(t, srv, false)
		}
		_, events := refineStream(t, srv, dev, map[string]any{"field": "statement", "statement": "Export the board."})
		prompts := provider.Prompts()
		if len(prompts) != 1 || len(prompts[0].Tools) != 0 || prompts[0].ToolChoice != "" {
			t.Fatalf("disabled server %v: prompts = %+v", withServer, prompts)
		}
		for _, ev := range events {
			if ev.Kind == "reset" || ev.Kind == "tool" {
				t.Fatalf("events = %v", kinds(events))
			}
		}
		if mcpSrv != nil && len(mcpSrv.Headers) != 0 {
			t.Fatal("a disabled server was opened")
		}
		if calls := projectToolCalls(t, srv); len(calls) != 0 {
			t.Fatalf("tool_call events = %v", calls)
		}
	}
}

func TestRefineMCPLoop(t *testing.T) {
	script := `{"refine_requirement":[{"tool":"mcp_tickets_lookup","id":"T-1","say":"Looking it up."},{"text":"` + refineStatement + `"}]}`
	srv, provider, dev, _ := refineServer(t, refineRouter, script)
	mcpSrv := refineMCP(t, srv, true)
	rec, events := refineStream(t, srv, dev, map[string]any{"field": "statement", "statement": "Export the board."})

	done := events[len(events)-1]
	if done.Kind != "done" || done.Data["text"] != refineStatement {
		t.Fatalf("events = %+v", events)
	}
	var tool *refineEvent
	for i := range events {
		if events[i].Kind == "tool" {
			tool = &events[i]
		}
	}
	if tool == nil || tool.Data["server"] != "tickets" || tool.Data["tool"] != "lookup" {
		t.Fatalf("tool event = %+v", tool)
	}
	prompts := provider.Prompts()
	if len(prompts) != 2 || strings.Join(prompts[0].Tools, ",") != "mcp_tickets_lookup" || prompts[0].ToolChoice != "" {
		t.Fatalf("prompts = %+v", prompts)
	}
	result := prompts[1].Turns[0].Results[0]
	if !strings.Contains(result, "MCP RESULT tickets/lookup\nTicket T-1") || strings.Contains(result, refineBearer) {
		t.Fatalf("result = %q", result)
	}
	if got := mcpSrv.Headers[0].Get("Authorization"); got != "Bearer "+refineBearer {
		t.Fatalf("server header = %q", got)
	}
	if strings.Contains(rec.Body.String(), refineBearer) {
		t.Fatal("the secret reached the stream")
	}

	payloads := refinedPayloads(t, srv)
	if len(payloads) != 1 || !strings.Contains(payloads[0], `"inputTokens":600`) || !strings.Contains(payloads[0], `"outputTokens":240`) {
		t.Fatalf("requirement_refined = %v", payloads)
	}
	calls := projectToolCalls(t, srv)
	if len(calls) != 1 || calls[0]["name"] != "mcp_tickets_lookup" || calls[0]["ok"] != true || calls[0]["sha256"] == nil || calls[0]["bytes"] == nil {
		t.Fatalf("tool_call = %v", calls)
	}
	raw, _ := json.Marshal(calls[0])
	if _, ok := calls[0]["summary"]; ok || strings.Contains(string(raw), "Ticket") || strings.Contains(string(raw), refineBearer) {
		t.Fatalf("tool_call carries text: %s", raw)
	}
	var leaks int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM events WHERE payload_json LIKE '%' || $1 || '%'`, refineBearer).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatalf("%d events hold the secret (%v)", leaks, err)
	}
}

func TestRefineMCPMentions(t *testing.T) {
	cases := []struct {
		statement  string
		acceptance []string
		tools      string
	}{
		{"Export the board.", nil, "mcp_tickets_lookup"},
		{"Export the board named in @tickets.", nil, "mcp_tickets_lookup"},
		{"Export the board.", []string{"Columns match @tickets."}, "mcp_tickets_lookup"},
		{"Export the board named in @docs.", nil, ""},
		{"Export the board named in `@tickets`, see @docs.", nil, ""},
	}
	for _, tc := range cases {
		srv, provider, dev, _ := refineServer(t, refineRouter, refineReply)
		mcpSrv := refineMCP(t, srv, true)
		refineStream(t, srv, dev, map[string]any{"field": "statement", "statement": tc.statement, "acceptance": tc.acceptance})
		prompts := provider.Prompts()
		if len(prompts) != 1 || strings.Join(prompts[0].Tools, ",") != tc.tools {
			t.Fatalf("%q: prompts = %+v", tc.statement, prompts)
		}
		if !strings.Contains(prompts[0].System, "Keep each @name exactly as written.") {
			t.Fatalf("system = %q", prompts[0].System)
		}
		if tc.tools == "" && len(mcpSrv.Headers) != 0 {
			t.Fatalf("%q: an unmentioned server was opened", tc.statement)
		}
	}
}

func TestRefineMCPResetEvent(t *testing.T) {
	script := `{"refine_requirement":[
		{"tool":"mcp_tickets_lookup","id":"T-1","say":"Checking the ticket for who asked for the export and which columns the report needs."},
		{"tool":"mcp_tickets_lookup","id":"T-2","say":"Checking another ticket that names the people who read the report every week."},
		{"text":"` + refineStatement + `"}]}`
	srv, _, dev, _ := refineServer(t, refineRouter, script)
	refineMCP(t, srv, true)
	_, events := refineStream(t, srv, dev, map[string]any{"field": "statement", "statement": "Export the board."})
	resets := 0
	for i, ev := range events {
		if ev.Kind == "reset" {
			resets++
			if i == 0 || events[i-1].Kind != "delta" {
				t.Fatalf("reset does not follow the turn's text: %v", kinds(events))
			}
		}
	}
	if resets != 2 || events[0].Kind != "start" {
		t.Fatalf("events = %v", kinds(events))
	}
	if got := shownText(events); got != refineStatement {
		t.Fatalf("shown text = %q", got)
	}
}

func TestRefineLastTurnNoTools(t *testing.T) {
	t.Run("third turn", func(t *testing.T) {
		script := `{"refine_requirement":[
			{"tool":"mcp_tickets_lookup","id":"T-1"},
			{"tool":"mcp_tickets_lookup","id":"T-2"},
			{"tool":"mcp_tickets_lookup","id":"T-3","say":"` + refineStatement + `"}]}`
		srv, provider, dev, _ := refineServer(t, refineRouter, script)
		mcpSrv := refineMCP(t, srv, true)
		_, events := refineStream(t, srv, dev, map[string]any{"field": "statement", "statement": "Export the board."})
		prompts := provider.Prompts()
		if len(prompts) != 3 || prompts[0].ToolChoice != "" || prompts[1].ToolChoice != "" || prompts[2].ToolChoice != "none" {
			t.Fatalf("prompts = %d", len(prompts))
		}
		if len(prompts[2].Tools) != 1 {
			t.Fatalf("last turn tools = %v", prompts[2].Tools)
		}
		if mcpSrv.CallCount() != 2 {
			t.Fatalf("server calls = %d", mcpSrv.CallCount())
		}
		if done := events[len(events)-1]; done.Kind != "done" || done.Data["text"] != refineStatement {
			t.Fatalf("done = %+v", done)
		}
	})
	t.Run("call cap", func(t *testing.T) {
		script := `{"refine_requirement":[
			{"calls":[{"tool":"mcp_tickets_lookup","id":"T-1"},{"tool":"mcp_tickets_lookup","id":"T-2"},{"tool":"mcp_tickets_lookup","id":"T-3"},{"tool":"mcp_tickets_lookup","id":"T-4"}]},
			{"text":"` + refineStatement + `"}]}`
		srv, provider, dev, _ := refineServer(t, refineRouter, script)
		mcpSrv := refineMCP(t, srv, true)
		refineStream(t, srv, dev, map[string]any{"field": "statement", "statement": "Export the board."})
		prompts := provider.Prompts()
		if len(prompts) != 2 || prompts[1].ToolChoice != "none" || mcpSrv.CallCount() != 3 {
			t.Fatalf("prompts = %d, server calls %d", len(prompts), mcpSrv.CallCount())
		}
		if last := prompts[1].Turns[0].Results[3]; !strings.Contains(last, "at most 3 times") {
			t.Fatalf("fourth result = %q", last)
		}
		calls := projectToolCalls(t, srv)
		if len(calls) != 4 || calls[3]["rejected"] != true {
			t.Fatalf("tool_call = %v", calls)
		}
	})
}

func TestRefineMCPCancel(t *testing.T) {
	srv, provider, dev, _ := refineServer(t, refineRouter, `{"refine_requirement":[{"tool":"mcp_tickets_lookup","id":"T-1"},{"text":"`+refineStatement+`"}]}`)
	mcpSrv := refineMCP(t, srv, true)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	mcpSrv.Call = func(string, map[string]any) (string, bool) {
		select {
		case <-release:
		case <-time.After(20 * time.Second):
		}
		return "late", false
	}
	live := httptest.NewServer(srv.Handler())
	defer live.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	raw, _ := json.Marshal(map[string]any{"field": "statement", "statement": "Export the board."})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, live.URL+refinePath, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Authorization", "Bearer "+dev)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended before the tool event: %v", err)
		}
		if strings.HasPrefix(line, "event: tool") {
			break
		}
	}
	cancel()

	deadline := time.Now().Add(5 * time.Second)
	for {
		calls := projectToolCalls(t, srv)
		if len(calls) == 1 && calls[0]["ok"] == false {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("closing the connection did not stop the MCP call: %v", calls)
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(provider.Prompts()); n != 1 {
		t.Fatalf("a canceled refine sent %d model requests", n)
	}
	if payloads := refinedPayloads(t, srv); len(payloads) != 0 {
		t.Fatalf("a canceled refine wrote requirement_refined: %v", payloads)
	}
}
