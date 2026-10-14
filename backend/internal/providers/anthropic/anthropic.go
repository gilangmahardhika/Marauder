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
	if err := providers.CheckTools(req.Tools); err != nil {
		return providers.Response{}, err
	}
	params, maxTokens := newParams(req)
	msg, err := c.client.Messages.New(ctx, params, option.WithRequestTimeout(requestTimeout(maxTokens)))
	if err != nil {
		return providers.Response{}, c.wrap(err)
	}
	return response(msg, req.Model), nil
}

// Stream sends req as a streaming request and calls onText with each piece of
// reply text as it arrives. The returned Response is the whole reply, as
// Complete would return it.
func (c *Client) Stream(ctx context.Context, req providers.Request, onText func(string)) (providers.Response, error) {
	if err := providers.CheckTools(req.Tools); err != nil {
		return providers.Response{}, err
	}
	params, maxTokens := newParams(req)
	stream := c.client.Messages.NewStreaming(ctx, params, option.WithRequestTimeout(requestTimeout(maxTokens)))
	defer stream.Close()
	var msg sdk.Message
	for stream.Next() {
		event := stream.Current()
		if err := msg.Accumulate(event); err != nil {
			return providers.Response{}, c.wrap(err)
		}
		if event.Type == "content_block_delta" {
			if delta := event.AsContentBlockDelta().Delta; delta.Type == "text_delta" && delta.Text != "" {
				onText(delta.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return providers.Response{}, c.wrap(err)
	}
	return response(&msg, req.Model), nil
}

func newParams(req providers.Request) (sdk.MessageNewParams, int) {
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
		params.System = []sdk.TextBlockParam{{Text: req.System, CacheControl: sdk.NewCacheControlEphemeralParam()}}
	}
	for i, tool := range req.Tools {
		param := sdk.ToolParam{Name: tool.Name, InputSchema: inputSchema(tool.Parameters)}
		if tool.Description != "" {
			param.Description = sdk.String(tool.Description)
		}
		if i == len(req.Tools)-1 {
			param.CacheControl = sdk.NewCacheControlEphemeralParam()
		}
		params.Tools = append(params.Tools, sdk.ToolUnionParam{OfTool: &param})
	}
	if req.ToolChoice == "none" && len(req.Tools) > 0 {
		none := sdk.NewToolChoiceNoneParam()
		params.ToolChoice = sdk.ToolChoiceUnionParam{OfNone: &none}
	}
	return params, maxTokens
}

func response(msg *sdk.Message, model string) providers.Response {
	out := providers.Response{
		InputTokens:       int(msg.Usage.InputTokens + msg.Usage.CacheCreationInputTokens),
		CachedInputTokens: int(msg.Usage.CacheReadInputTokens),
		OutputTokens:      int(msg.Usage.OutputTokens),
		StopReason:        string(msg.StopReason),
	}
	for _, block := range msg.Content {
		switch block.Type {
		case "thinking":
			out.Reasoning = append(out.Reasoning, providers.Reasoning{Model: model, Kind: "thinking", Text: block.Thinking, Signature: block.Signature})
		case "redacted_thinking":
			out.Reasoning = append(out.Reasoning, providers.Reasoning{Model: model, Kind: "redacted_thinking", Data: block.Data})
		case "text":
			out.Text += block.Text
		case "tool_use":
			call := providers.ToolCall{ID: block.ID, Name: block.Name, Args: map[string]any{}}
			if len(block.Input) > 0 {
				if err := json.Unmarshal(block.Input, &call.Args); err != nil {
					call.Args, call.BadArgs = map[string]any{}, err.Error()
				}
			}
			out.ToolCalls = append(out.ToolCalls, call)
		}
	}
	return out
}

// requestTimeout is the SDK's own estimate for a non-streaming reply of
// maxTokens, at least 10 minutes. Without an explicit timeout the SDK refuses,
// before sending, any request it estimates past 10 minutes, such as a reviewer
// request with 32,768 tokens.
func requestTimeout(maxTokens int) time.Duration {
	return max(10*time.Minute, time.Duration(float64(time.Hour)*float64(maxTokens)/128000))
}

// messages builds the conversation with one cache breakpoint on the last
// block of the newest turn's results, or on the pack before the first turn
// (RFC-083). The newest turn's Note comes after that block, so the todo list
// and the turns-left note do not move the cached prefix.
func messages(req providers.Request) []sdk.MessageParam {
	pack := sdk.NewTextBlock(req.User)
	if len(req.Turns) == 0 {
		pack.OfText.CacheControl = sdk.NewCacheControlEphemeralParam()
	}
	out := []sdk.MessageParam{sdk.NewUserMessage(pack)}
	for t, turn := range req.Turns {
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
		} else if len(turn.Calls) == 0 {
			said = append(said, sdk.NewTextBlock(noReply))
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
			results = append(results, sdk.NewToolResultBlock(call.ID, result, strings.HasPrefix(result, providers.ErrorPrefix)))
		}
		if len(said) > 0 {
			out = append(out, sdk.NewAssistantMessage(said...))
		}
		if t == len(req.Turns)-1 && len(results) > 0 {
			results[len(results)-1].OfToolResult.CacheControl = sdk.NewCacheControlEphemeralParam()
		}
		if turn.Note != "" {
			results = append(results, sdk.NewTextBlock(turn.Note))
		}
		if len(results) > 0 {
			out = append(out, sdk.NewUserMessage(results...))
		}
	}
	return out
}

// noReply stands in for a reply with no text and no tool call, because an
// assistant message may not be empty.
const noReply = "(no reply)"

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
		if providers.ContextFullStatus(apiErr.StatusCode, msg) {
			return fmt.Errorf("%w: %s", providers.ErrContextFull, trace.Redact(msg, []string{c.key}))
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
	for key, value := range params {
		switch key {
		case "type", "properties", "required":
		default:
			if out.ExtraFields == nil {
				out.ExtraFields = map[string]any{}
			}
			out.ExtraFields[key] = value
		}
	}
	return out
}
