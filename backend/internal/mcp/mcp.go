// Package mcp is a small Model Context Protocol client: initialize, list
// tools, and call a tool (RFC-093). It declares no client capabilities and
// answers every server request with method-not-found.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	// MaxResponse is the most bytes read for one response.
	MaxResponse     = 256 << 10
	ProtocolVersion = "2025-06-18"
	NonText         = "[non-text content omitted]"
	maxPages        = 10
)

var (
	ErrUnreachable = errors.New("unreachable")
	ErrBlocked     = errors.New("address refused")
	ErrTooLarge    = errors.New("result over 256 KiB")
	ErrProtocol    = errors.New("bad response")
	ErrServer      = errors.New("server error")
)

// Tool is one tool a server lists. Schema is its inputSchema.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"inputSchema"`
}

// Result is a tool's text. IsError is the server's isError flag.
type Result struct {
	Text    string
	IsError bool
}

type Client interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (Result, error)
	Close() error
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// wire is any message read from a server.
type wire struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code int `json:"code"`
	} `json:"error,omitempty"`
}

func (w wire) isResponse(id int64) bool {
	return w.Method == "" && strings.TrimSpace(string(w.ID)) == strconv.FormatInt(id, 10)
}

func (w wire) isRequest() bool {
	return w.Method != "" && len(w.ID) > 0 && string(w.ID) != "null"
}

// methodNotFound is the reply to a server request; sampling, elicitation,
// and roots are never offered.
func methodNotFound(id json.RawMessage) []byte {
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": -32601, "message": "method not found"},
	})
	return raw
}

// transport sends one request and returns the response with that id, or
// sends one notification.
type transport interface {
	call(ctx context.Context, id int64, body []byte) (wire, error)
	notify(ctx context.Context, body []byte) error
	close() error
}

type client struct {
	t  transport
	id atomic.Int64
}

func newClient(ctx context.Context, t transport) (*client, error) {
	c := &client{t: t}
	params := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "orchestrator", "version": "1"},
	}
	if _, err := c.call(ctx, "initialize", params); err != nil {
		_ = t.close()
		return nil, err
	}
	body, _ := json.Marshal(request{JSONRPC: "2.0", Method: "notifications/initialized"})
	if err := t.notify(ctx, body); err != nil {
		_ = t.close()
		return nil, err
	}
	return c, nil
}

func (c *client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.id.Add(1)
	body, err := json.Marshal(request{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	resp, err := c.t.call(ctx, id, body)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("%w %d", ErrServer, resp.Error.Code)
	}
	if len(resp.Result) == 0 {
		return nil, ErrProtocol
	}
	return resp.Result, nil
}

func (c *client) ListTools(ctx context.Context) ([]Tool, error) {
	var tools []Tool
	cursor := ""
	for page := 0; page < maxPages; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.call(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		var out struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, ErrProtocol
		}
		tools = append(tools, out.Tools...)
		if out.NextCursor == "" {
			return tools, nil
		}
		cursor = out.NextCursor
	}
	return tools, nil
}

func (c *client) CallTool(ctx context.Context, name string, args map[string]any) (Result, error) {
	if args == nil {
		args = map[string]any{}
	}
	raw, err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return Result{}, err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Result{}, ErrProtocol
	}
	parts := make([]string, 0, len(out.Content))
	for _, item := range out.Content {
		if item.Type == "text" {
			parts = append(parts, item.Text)
		} else {
			parts = append(parts, NonText)
		}
	}
	if len(parts) == 0 && len(out.StructuredContent) > 0 && string(out.StructuredContent) != "null" {
		parts = append(parts, string(out.StructuredContent))
	}
	return Result{Text: strings.Join(parts, "\n"), IsError: out.IsError}, nil
}

func (c *client) Close() error {
	return c.t.close()
}

// Failure is a fixed reason for an MCP error. It never carries the server's
// response text.
func Failure(err error) string {
	switch {
	case errors.Is(err, ErrBlocked):
		return "the server address was refused"
	case errors.Is(err, ErrTooLarge):
		return "the response was over 256 KiB"
	case errors.Is(err, ErrProtocol):
		return "the server sent a response that is not MCP"
	case errors.Is(err, ErrServer):
		return "the server returned an error"
	case errors.Is(err, context.DeadlineExceeded):
		return "the server did not answer in time"
	default:
		return "the server is unreachable"
	}
}
