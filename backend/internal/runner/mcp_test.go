package runner_test

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/mcp/mcptest"
	"orchestrator/backend/internal/netguard"
	"orchestrator/backend/internal/runner"
	"orchestrator/backend/internal/tenancy"
)

const mcpBearer = "tok-abcdef123456"

// resolver answers each host with one fixed address.
type resolver map[string]string

func (r resolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	if addr, ok := r[host]; ok {
		return []netip.Addr{netip.MustParseAddr(addr)}, nil
	}
	return net.DefaultResolver.LookupNetIP(ctx, network, host)
}

func TestMCPProxyNetguard(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	ctx := context.Background()
	schema := map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []any{"id"}}
	tickets := mcptest.New([]mcp.Tool{{Name: "lookup", Description: "server text, never sent", Schema: schema}})
	t.Cleanup(tickets.Close)
	tickets.Call = func(string, map[string]any) (string, bool) {
		return "ticket T-1, Bearer " + mcpBearer, false
	}
	policy := netguard.Policy{AllowLoopbackHTTP: true}
	saved := app.MCPDialer{Policy: policy, Resolver: resolver{"intranet.example": "93.184.216.34"}}
	h.srv.MCP = app.MCPDialer{Policy: policy, Resolver: resolver{"intranet.example": "10.0.0.5"}}
	admin := app.User{ID: h.admin.ID, Admin: true}
	for _, in := range []app.MCPServerInput{
		{Slug: "tickets", Transport: "http", URL: tickets.URL + "/mcp", Secrets: map[string]string{"Authorization": "Bearer " + mcpBearer},
			Tools: []tenancy.MCPTool{{Name: "lookup", Description: "Find a ticket by id.", Schema: schema}}},
		{Slug: "intranet", Transport: "http", URL: "https://intranet.example/mcp",
			Tools: []tenancy.MCPTool{{Name: "search", Description: "Search the intranet.", Schema: schema}}},
	} {
		if _, _, err := app.CreateMCPServer(ctx, h.db, admin, tokenKey, saved, "tribe_default", in); err != nil {
			t.Fatalf("create %s: %v", in.Slug, err)
		}
	}
	c := h.register(t, "agent-1")
	_, leaseID := startAttempt(t, h, c, "att_mcpproxy0001")
	proxy, err := runner.OpenMCP(ctx, c, leaseID)
	if err != nil || proxy == nil {
		t.Fatalf("open = %v %v", proxy, err)
	}
	var names []string
	for _, tool := range proxy.Tools() {
		names = append(names, tool.Name)
	}
	if strings.Join(names, ",") != "mcp_tickets_lookup" {
		t.Fatalf("tools = %v, want only the reachable server's", names)
	}
	var blocked bool
	for _, server := range proxy.Describe() {
		if server["slug"] == "intranet" {
			text, _ := server["error"].(string)
			blocked = strings.Contains(text, "refused")
		}
	}
	if !blocked {
		t.Fatalf("servers = %v, want intranet blocked on the control plane", proxy.Describe())
	}
	res, err := proxy.Call(ctx, "mcp_tickets_lookup", map[string]any{"id": "T-1"})
	if err != nil || res.IsError {
		t.Fatalf("call = %+v %v", res, err)
	}
	if !strings.Contains(res.Text, "ticket T-1") || strings.Contains(res.Text, mcpBearer) {
		t.Fatalf("result = %q", res.Text)
	}
	if len(tickets.Headers) == 0 || tickets.Headers[0].Get("Authorization") != "Bearer "+mcpBearer {
		t.Fatal("the control plane did not open the server with its credential")
	}
	for route, list := range h.tap.all() {
		for _, body := range list {
			for _, leak := range []string{mcpBearer, tickets.URL, "intranet.example", "10.0.0.5"} {
				if strings.Contains(body, leak) {
					t.Fatalf("%s reply carries %q:\n%s", route, leak, body)
				}
			}
		}
	}
}
