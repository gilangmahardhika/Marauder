package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/trace"
)

// ExtraTools are the tribe's MCP tools for one attempt (RFC-094). Tools is
// read once when the attempt starts and does not change after.
type ExtraTools interface {
	Tools() []providers.Tool
	Call(ctx context.Context, name string, args map[string]any) (mcp.Result, error)
	// Describe is the mcp_context event's servers: slug, transport, host or
	// image, and tool names with schema hashes.
	Describe() []map[string]any
}

// Limits of MCP calls in one attempt (RFC-094).
const (
	MaxMCPCalls      = 30
	MaxMCPCallsReply = 8
	MCPCallTimeout   = 60 * time.Second
	MaxMCPArgBytes   = 4 << 10
	MaxMCPResult     = 64 << 10
)

// MCPToolName is an exposed name: mcp_<slug>_<tool>. A slug has no '_', so
// the first '_' after the prefix ends it.
var MCPToolName = regexp.MustCompile(`^mcp_[a-z][a-z0-9]{1,15}_[A-Za-z0-9_-]{1,40}$`)

// SplitMCPName returns the server slug and tool of an exposed name.
func SplitMCPName(name string) (string, string, bool) {
	if !MCPToolName.MatchString(name) {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, "mcp_")
	slug, tool, ok := strings.Cut(rest, "_")
	return slug, tool, ok
}

// IsMCP reports whether name is an exposed MCP tool name.
func IsMCP(name string) bool {
	return MCPToolName.MatchString(name)
}

// ProfileTools is base plus the extras whose names are valid MCP names and
// collide with nothing before them.
func ProfileTools(base []providers.Tool, extra ExtraTools) []providers.Tool {
	tools := append([]providers.Tool(nil), base...)
	if extra == nil {
		return tools
	}
	seen := map[string]bool{}
	for _, tool := range base {
		seen[tool.Name] = true
	}
	for _, tool := range extra.Tools() {
		if !IsMCP(tool.Name) || seen[tool.Name] {
			continue
		}
		seen[tool.Name] = true
		tools = append(tools, tool)
	}
	return tools
}

// CheckMCPArgs refuses arguments over MaxMCPArgBytes and a missing required key.
func CheckMCPArgs(tool providers.Tool, args map[string]any) error {
	raw, err := json.Marshal(args)
	if err != nil {
		return errors.New("the arguments are not JSON")
	}
	if len(raw) > MaxMCPArgBytes {
		return fmt.Errorf("the arguments are over %d bytes", MaxMCPArgBytes)
	}
	var required []string
	switch list := tool.Parameters["required"].(type) {
	case []string:
		required = list
	case []any:
		for _, item := range list {
			if name, ok := item.(string); ok {
				required = append(required, name)
			}
		}
	}
	for _, key := range required {
		if _, ok := args[key]; !ok {
			return fmt.Errorf("the argument %s is required", key)
		}
	}
	return nil
}

// CallMCP runs one MCP call with the per-call timeout and returns the text
// the model sees: redacted, cut at MaxMCPResult, and wrapped as data.
func CallMCP(ctx context.Context, extra ExtraTools, tool providers.Tool, args map[string]any, secrets []string) (string, error) {
	slug, name, _ := SplitMCPName(tool.Name)
	if err := CheckMCPArgs(tool, args); err != nil {
		return "", err
	}
	if args == nil {
		args = map[string]any{}
	}
	ctx, cancel := context.WithTimeout(ctx, MCPCallTimeout)
	defer cancel()
	res, err := extra.Call(ctx, tool.Name, args)
	if err != nil {
		return "", fmt.Errorf("mcp %s: %s", slug, mcp.Failure(err))
	}
	text := cutResult(trace.Redact(res.Text, secrets), MaxMCPResult, false)
	wrapped := contextpack.MCPResult(slug, name, text)
	if res.IsError {
		return "", errors.New(wrapped)
	}
	return wrapped, nil
}

// MCPEventFields are the tool_call fields of an MCP result: the result
// clipped to trace.TextCap, its byte count, and its SHA-256.
func MCPEventFields(summary string, secrets []string) map[string]any {
	return map[string]any{
		"summary": trace.Clip(summary, secrets, trace.TextCap),
		"bytes":   len(summary),
		"sha256":  hashText(summary),
	}
}

// MCPArgs are the recorded arguments of an MCP call: the server, the tool,
// and each argument's name with its size in bytes.
func MCPArgs(name string, args map[string]any) map[string]any {
	slug, tool, _ := SplitMCPName(name)
	sizes := map[string]any{}
	for key, value := range args {
		raw, _ := json.Marshal(value)
		sizes[trace.Clip(key, nil, 64)] = len(raw)
	}
	return map[string]any{"server": slug, "tool": tool, "argBytes": sizes}
}

func (s *session) callMCP(ctx context.Context, call providers.ToolCall) (string, error) {
	tool, ok := s.allowed[call.Name]
	if !ok || s.in.Extra == nil {
		return "", errors.New("not in profile")
	}
	if s.mcpCalls >= MaxMCPCalls {
		return "", fmt.Errorf("MCP tools have run %d times in this attempt, the most they may", MaxMCPCalls)
	}
	if s.replyMCP >= MaxMCPCallsReply {
		return "", fmt.Errorf("one reply may make at most %d MCP calls", MaxMCPCallsReply)
	}
	s.mcpCalls++
	s.replyMCP++
	return CallMCP(ctx, s.in.Extra, tool, call.Args, s.in.Secrets)
}

func mentionedServers(scope *MCPContext) []string {
	if scope == nil || (scope.Scope != "mentioned" && scope.Scope != "card") {
		return nil
	}
	return scope.Offered
}

// mcpContextEvent is the mcp_context tool_call payload (RFC-094, RFC-102).
func mcpContextEvent(extra ExtraTools, scope *MCPContext, tools int) map[string]any {
	servers := []map[string]any{}
	if extra != nil {
		servers = extra.Describe()
	}
	payload := map[string]any{
		"name": "mcp_context", "ok": true, "rejected": false,
		"summary": fmt.Sprintf("%d MCP servers, %d tools", len(servers), tools),
		"servers": servers,
	}
	if scope == nil {
		return payload
	}
	missing := scope.Missing
	if missing == nil {
		missing = []string{}
	}
	payload["scope"] = scope.Scope
	payload["missing"] = missing
	if len(scope.Attached) > 0 || len(scope.Detached) > 0 {
		payload["attached"] = nonNilSlugs(scope.Attached)
		payload["detached"] = nonNilSlugs(scope.Detached)
	}
	switch scope.Scope {
	case "mentioned", "card":
		payload["summary"] = fmt.Sprintf("%d of %d MCP servers, %d tools", len(servers), scope.Total, tools)
	case "none":
		payload["summary"] = "No MCP servers: " + scope.Reason
		payload["reason"] = scope.Reason
	}
	return payload
}

func nonNilSlugs(slugs []string) []string {
	if slugs == nil {
		return []string{}
	}
	return slugs
}
