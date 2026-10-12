// Package openai is the client for the openai protocol: OpenAI and any
// OpenAI-compatible server reached through the family baseUrl.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/trace"
)

type Client struct {
	client sdk.Client
	key    string
}

// New builds a client for baseURL. The SDK does not retry, because the router
// owns retries and fallback. Environment credentials are never read. A baseURL
// pasted with the /chat/completions endpoint is accepted, because the SDK adds it.
func New(baseURL, key string) *Client {
	opts := []option.RequestOption{
		option.WithMaxRetries(0),
		option.WithHeaderDel("OpenAI-Organization"),
		option.WithHeaderDel("OpenAI-Project"),
		option.WithAPIKey(key),
	}
	baseURL = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/chat/completions")
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
	params := sdk.ChatCompletionNewParams{
		Model:               shared.ChatModel(req.Model),
		Messages:            messages(req),
		MaxCompletionTokens: sdk.Int(int64(maxTokens)),
	}
	for _, tool := range req.Tools {
		def := shared.FunctionDefinitionParam{Name: tool.Name, Parameters: shared.FunctionParameters(schema(tool.Parameters))}
		if tool.Description != "" {
			def.Description = sdk.String(tool.Description)
		}
		params.Tools = append(params.Tools, sdk.ChatCompletionFunctionTool(def))
	}
	if req.Effort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(req.Effort)
	}
	var opts []option.RequestOption
	// OpenAI has no thinking switch; GLM, DeepSeek, Kimi, and Qwen read this field.
	if req.Thinking == "enabled" || req.Thinking == "disabled" {
		opts = append(opts, option.WithJSONSet("thinking", map[string]string{"type": req.Thinking}))
	}
	completion, err := c.client.Chat.Completions.New(ctx, params, opts...)
	if err != nil {
		return providers.Response{}, c.wrap(err)
	}
	if len(completion.Choices) == 0 {
		return providers.Response{}, errors.New("openai: response has no choices")
	}
	choice := completion.Choices[0]
	out := providers.Response{
		Text:         choice.Message.Content,
		InputTokens:  int(completion.Usage.PromptTokens),
		OutputTokens: int(completion.Usage.CompletionTokens),
		StopReason:   choice.FinishReason,
	}
	if field, ok := choice.Message.JSON.ExtraFields["reasoning_content"]; ok {
		var text string
		if json.Unmarshal([]byte(field.Raw()), &text) == nil && text != "" {
			out.Reasoning = []providers.Reasoning{{Model: req.Model, Kind: "reasoning_content", Text: text}}
		}
	}
	for _, call := range choice.Message.ToolCalls {
		if call.Function.Name == "" {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, providers.ToolCall{
			ID:   call.ID,
			Name: call.Function.Name,
			Args: decodeArgs(call.Function.Arguments),
		})
	}
	return out, nil
}

func messages(req providers.Request) []sdk.ChatCompletionMessageParamUnion {
	out := []sdk.ChatCompletionMessageParamUnion{sdk.SystemMessage(req.System), sdk.UserMessage(req.User)}
	for _, turn := range req.Turns {
		assistant := sdk.ChatCompletionAssistantMessageParam{}
		if turn.Text != "" {
			assistant.Content.OfString = sdk.String(turn.Text)
		}
		var reasoning string
		for _, r := range providers.Replay(turn, req.Model) {
			reasoning += r.Text
		}
		if reasoning != "" {
			assistant.SetExtraFields(map[string]any{"reasoning_content": reasoning})
		}
		for _, call := range turn.Calls {
			args, _ := json.Marshal(call.Args)
			assistant.ToolCalls = append(assistant.ToolCalls, sdk.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &sdk.ChatCompletionMessageFunctionToolCallParam{
					ID:       call.ID,
					Function: sdk.ChatCompletionMessageFunctionToolCallFunctionParam{Name: call.Name, Arguments: string(args)},
				},
			})
		}
		out = append(out, sdk.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
		for i, call := range turn.Calls {
			result := ""
			if i < len(turn.Results) {
				result = turn.Results[i]
			}
			out = append(out, sdk.ToolMessage(result, call.ID))
		}
	}
	return out
}

func (c *Client) wrap(err error) error {
	var apiErr *sdk.Error
	msg := err.Error()
	if errors.As(err, &apiErr) {
		msg = fmt.Sprintf("openai: status %d", apiErr.StatusCode)
		if apiErr.Message != "" {
			msg += ": " + apiErr.Message
		}
		if apiErr.StatusCode == http.StatusTooManyRequests && apiErr.Response != nil {
			return &providers.RateLimited{Message: trace.Redact(msg, []string{c.key}), Wait: providers.RetryWait(apiErr.Response.Header)}
		}
	}
	return errors.New(trace.Redact(msg, []string{c.key}))
}

func schema(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return params
}

func decodeArgs(raw string) map[string]any {
	args := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return args
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return map[string]any{}
	}
	return args
}
