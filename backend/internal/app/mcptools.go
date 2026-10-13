package app

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/trace"
)

const mcpConnectTimeout = 30 * time.Second

// MCPToolset is the tribe's MCP tools for one attempt or refine, frozen when
// it opens (RFC-094). It satisfies loop.ExtraTools.
type MCPToolset struct {
	tools   []providers.Tool
	routes  map[string]mcpRoute
	servers []map[string]any
	clients []mcp.Client
}

type mcpRoute struct {
	client mcp.Client
	tool   string
}

// MCPScope is where an MCP event goes: a card's project and card, or a
// project alone for refine.
type MCPScope struct {
	ProjectID string
	CardID    string
}

// MCPSecrets are the values a job redacts for its tribe's servers: every
// header and environment value, and a token after "Bearer ".
func MCPSecrets(conns []MCPConn) []string {
	var out []string
	for _, conn := range conns {
		for _, value := range conn.Secrets {
			out = append(out, value)
			if token, ok := strings.CutPrefix(value, "Bearer "); ok && len(token) >= 8 {
				out = append(out, token)
			}
		}
	}
	return out
}

// LoadProjectMCP returns the enabled servers of the project's tribe with
// their secrets.
func LoadProjectMCP(ctx context.Context, db *sql.DB, key []byte, projectID string) ([]MCPConn, error) {
	return tenancy.ProjectMCPServers(ctx, db, key, projectID)
}

// OpenMCPToolset connects to each server and offers each allowlisted tool
// whose schema hash still matches. A changed tool is left out with an
// assertion_failed mcp_changed event. A server that cannot be reached offers
// nothing and is described with its error. It returns nil when the tribe has
// no enabled server.
func OpenMCPToolset(ctx context.Context, db *sql.DB, d MCPDialer, conns []MCPConn, scope MCPScope, redact []string) (*MCPToolset, error) {
	if len(conns) == 0 {
		return nil, nil
	}
	set := &MCPToolset{routes: map[string]mcpRoute{}}
	for _, conn := range conns {
		desc := map[string]any{"slug": conn.Slug, "transport": conn.Transport}
		switch {
		case conn.Transport == "http":
			if u, err := url.Parse(conn.URL); err == nil {
				desc["host"] = u.Host
			}
		case conn.Config != nil:
			desc["host"] = net.JoinHostPort(conn.Config.Host, strconv.Itoa(conn.Config.Port))
			desc["database"] = conn.Config.Database
		default:
			desc["image"] = conn.Image
		}
		set.servers = append(set.servers, desc)
		if len(conn.Tools) == 0 {
			desc["tools"] = []any{}
			continue
		}
		dialCtx, cancel := context.WithTimeout(ctx, mcpConnectTimeout)
		client, err := d.Open(dialCtx, conn)
		var live []mcp.Tool
		if err == nil {
			live, err = client.ListTools(dialCtx)
			if err != nil {
				client.Close()
			}
		}
		cancel()
		if err != nil {
			desc["error"] = MCPFailure(err)
			desc["tools"] = []any{}
			continue
		}
		listed := map[string]mcp.Tool{}
		for _, t := range live {
			listed[t.Name] = t
		}
		var offered []map[string]any
		for _, tool := range conn.Tools {
			current, ok := listed[tool.Name]
			if !ok || tenancy.SchemaHash(current.Schema) != tool.SchemaSHA256 {
				if _, err := trace.Append(ctx, db, trace.Event{
					CardID: scope.CardID, ProjectID: scope.ProjectID, Kind: "assertion_failed",
					Payload: map[string]any{"name": "mcp_changed", "server": conn.Slug, "tool": tool.Name, "listed": ok},
				}, redact); err != nil {
					client.Close()
					set.Close()
					return nil, err
				}
				continue
			}
			name := "mcp_" + conn.Slug + "_" + tool.Name
			description := tool.Description
			if description == "" {
				description = "Tool " + tool.Name + " of the " + conn.Slug + " MCP server."
			}
			set.tools = append(set.tools, providers.Tool{Name: name, Description: description, Parameters: tool.Schema})
			set.routes[name] = mcpRoute{client: client, tool: tool.Name}
			offered = append(offered, map[string]any{"name": tool.Name, "schemaSha256": tool.SchemaSHA256})
		}
		if len(offered) == 0 {
			client.Close()
			desc["tools"] = []any{}
			continue
		}
		desc["tools"] = offered
		set.clients = append(set.clients, client)
	}
	return set, nil
}

func (s *MCPToolset) Tools() []providers.Tool {
	return append([]providers.Tool(nil), s.tools...)
}

func (s *MCPToolset) Call(ctx context.Context, name string, args map[string]any) (mcp.Result, error) {
	route, ok := s.routes[name]
	if !ok {
		return mcp.Result{}, mcp.ErrProtocol
	}
	return route.client.CallTool(ctx, route.tool, args)
}

func (s *MCPToolset) Describe() []map[string]any {
	return s.servers
}

// Close closes every open server.
func (s *MCPToolset) Close() {
	if s == nil {
		return
	}
	for _, client := range s.clients {
		_ = client.Close()
	}
	s.clients = nil
}
