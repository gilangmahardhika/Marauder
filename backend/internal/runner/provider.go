package runner

import (
	"context"
	"errors"
	"sync"

	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runnerapi"
)

// Provider sends each model request of a leased attempt to the control plane,
// which runs it with the lease's routed model and budgets and publishes it to
// the card's live trace. Turns are numbered in call order.
type Provider struct {
	Client *Client
	Board  *Board

	mu   sync.Mutex
	turn int
}

func (p *Provider) Complete(ctx context.Context, req providers.Request) (providers.Response, error) {
	attemptID := p.Board.AttemptID()
	if attemptID == "" {
		return providers.Response{}, errors.New("model call before the attempt was recorded")
	}
	p.mu.Lock()
	turn := p.turn
	p.turn++
	p.mu.Unlock()
	reply, err := p.Client.Model(ctx, p.Board.LeaseID, runnerapi.ModelCall{AttemptID: attemptID, Turn: turn, Request: req})
	return reply.Response, err
}

// MCPProxy is the loop.ExtraTools of a leased attempt. The control plane
// opens the servers with its own credentials and network guard; this side
// sees tool names, schemas, and redacted results only.
type MCPProxy struct {
	Client  *Client
	LeaseID string
	tools   []providers.Tool
	servers []map[string]any
}

// OpenMCP lists the lease's MCP tools. It returns nil when the lease has no
// server, so the loop offers none.
func OpenMCP(ctx context.Context, c *Client, leaseID string) (*MCPProxy, error) {
	list, err := c.MCPTools(ctx, leaseID)
	if err != nil {
		return nil, err
	}
	if len(list.Tools) == 0 && len(list.Servers) == 0 {
		return nil, nil
	}
	return &MCPProxy{Client: c, LeaseID: leaseID, tools: list.Tools, servers: list.Servers}, nil
}

func (m *MCPProxy) Tools() []providers.Tool {
	return m.tools
}

func (m *MCPProxy) Describe() []map[string]any {
	return m.servers
}

func (m *MCPProxy) Call(ctx context.Context, name string, args map[string]any) (mcp.Result, error) {
	res, err := m.Client.MCPCall(ctx, m.LeaseID, runnerapi.MCPCall{Name: name, Args: args})
	if err != nil {
		return mcp.Result{}, err
	}
	return mcp.Result{Text: res.Text, IsError: res.IsError}, nil
}
