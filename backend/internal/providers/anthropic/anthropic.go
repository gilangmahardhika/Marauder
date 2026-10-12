// Package anthropic is the client for the anthropic protocol: Anthropic and any
// Anthropic-compatible server reached through the family baseUrl.
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/trace"
)

type Client struct {
	client sdk.Client
	key    string
}

// New builds a client for baseURL, or the Anthropic API when baseURL is empty.
// The SDK does not retry, because the router owns retries and fallback. The SDK
// adds /v1/messages, so a baseURL pasted with either part is accepted.
func New(baseURL, key string) *Client {
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithMaxRetries(0),
		option.WithAPIKey(key),
	}
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(strings.TrimSuffix(baseURL, "/messages"), "/v1")
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &Client{client: sdk.NewClient(opts...), key: key}
}

func (c *Client) Complete(ctx context.Context, req providers.Request) (providers.Response, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = providers.DefaultMaxTokens
	}
	params := sdk.MessageNewParams{
		Model:    sdk.Model(req.Model),
		Messages: messages(req),
	}
	switch req.Thinking {
	case "enabled":
		budget := req.ThinkingBudget
		if budget <= 0 {
			budget = providers.DefaultThinkingBudget
		}
		// budget_tokens must stay under max_tokens, and the reply keeps its own limit.
		maxTokens += budget
		params.Thinking = sdk.ThinkingConfigParamUnion{OfEnabled: &sdk.ThinkingConfigEnabledParam{BudgetTokens: int64(budget)}}
	case "adaptive":
		params.Thinking = sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}}
	case "disabled":
		params.Thinking = sdk.ThinkingConfigParamUnion{OfDisabled: &sdk.ThinkingConfigDisabledParam{}}
	}
	if req.Effort != "" {
		params.OutputConfig = sdk.OutputConfigParam{Effort: sdk.OutputConfigEffort(req.Effort)}
	}
	params.MaxTokens = int64(maxTokens)
	if req.System != "" {
		params.System = []sdk.TextBlockParam{{Text: req.System}}
	}
	for _, tool := range req.Tools {
		param := sdk.ToolParam{Name: tool.Name, InputSchema: inputSchema(tool.Parameters)}
		if tool.Description != "" {
			param.Description = sdk.String(tool.Description)
		}
		params.Tools = append(params.Tools, sdk.ToolUnionParam{OfTool: &param})
	}
	msg, err := c.client.Messages.New(ctx, params, option.WithRequestTimeout(requestTimeout(maxTokens)))
	if err != nil {
		return providers.Response{}, c.wrap(err)
	}
	out := providers.Response{
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
		StopReason:   string(msg.StopReason),
	}
	for _, block := range msg.Content {
		switch block.Type {
		case "thinking":
			out.Reasoning = append(out.Reasoning, providers.Reasoning{Model: req.Model, Kind: "thinking", Text: block.Thinking, Signature: block.Signature})
		case "redacted_thinking":
			out.Reasoning = append(out.Reasoning, providers.Reasoning{Model: req.Model, Kind: "redacted_thinking", Data: block.Data})
		case "text":
			out.Text += block.Text
		case "tool_use":
			args := map[string]any{}
			if len(block.Input) > 0 {
				if err := json.Unmarshal(block.Input, &args); err != nil {
					args = map[string]any{}
				}
			}
			out.ToolCalls = append(out.ToolCalls, providers.ToolCall{ID: block.ID, Name: block.Name, Args: args})
		}
	}
	return out, nil
}

// requestTimeout is the SDK's own estimate for a non-streaming reply of
// maxTokens, at least 10 minutes. Without an explicit timeout the SDK refuses,
// before sending, any request it estimates past 10 minutes, such as a reviewer
// request with 32,768 tokens.
func requestTimeout(maxTokens int) time.Duration {
	return max(10*time.Minute, time.Duration(float64(time.Hour)*float64(maxTokens)/128000))
}

func messages(req providers.Request) []sdk.MessageParam {
	out := []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock(req.User))}
	for _, turn := range req.Turns {
		var said []sdk.ContentBlockParamUnion
		// With thinking on, the API refuses a tool turn whose signed thinking
		// blocks do not come back first.
		for _, r := range providers.Replay(turn, req.Model) {
			switch r.Kind {
			case "thinking":
				said = append(said, sdk.NewThinkingBlock(r.Signature, r.Text))
			case "redacted_thinking":
				said = append(said, sdk.NewRedactedThinkingBlock(r.Data))
			}
		}
		if turn.Text != "" {
			said = append(said, sdk.NewTextBlock(turn.Text))
		}
		var results []sdk.ContentBlockParamUnion
		for i, call := range turn.Calls {
			args := call.Args
			if args == nil {
				args = map[string]any{}
			}
			said = append(said, sdk.NewToolUseBlock(call.ID, args, call.Name))
			result := ""
			if i < len(turn.Results) {
				result = turn.Results[i]
			}
			results = append(results, sdk.NewToolResultBlock(call.ID, result, false))
		}
		if len(said) > 0 {
			out = append(out, sdk.NewAssistantMessage(said...))
		}
		if len(results) > 0 {
			out = append(out, sdk.NewUserMessage(results...))
		}
	}
	return out
}

func (c *Client) wrap(err error) error {
	var apiErr *sdk.Error
	msg := err.Error()
	if errors.As(err, &apiErr) {
		msg = fmt.Sprintf("anthropic: status %d", apiErr.StatusCode)
		if body := apiErr.RawJSON(); body != "" {
			msg += ": " + body
		}
		if apiErr.StatusCode == http.StatusTooManyRequests && apiErr.Response != nil {
			return &providers.RateLimited{Message: trace.Redact(msg, []string{c.key}), Wait: providers.RetryWait(apiErr.Response.Header)}
		}
	}
	return errors.New(trace.Redact(msg, []string{c.key}))
}

func inputSchema(params map[string]any) sdk.ToolInputSchemaParam {
	out := sdk.ToolInputSchemaParam{Properties: map[string]any{}}
	if params == nil {
		return out
	}
	if props, ok := params["properties"]; ok {
		out.Properties = props
	}
	switch required := params["required"].(type) {
	case []string:
		out.Required = required
	case []any:
		for _, item := range required {
			if name, ok := item.(string); ok {
				out.Required = append(out.Required, name)
			}
		}
	}
	return out
}
