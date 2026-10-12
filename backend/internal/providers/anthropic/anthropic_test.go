package anthropic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/anthropic"
)

const key = "sk-ant-test-key-1"

func messageServer(t *testing.T, got *map[string]any, path, apiKey *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*path, *apiKey = r.URL.Path, r.Header.Get("X-Api-Key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"m",
			"content":[{"type":"text","text":"editing"},{"type":"tool_use","id":"t1","name":"edit_file","input":{"path":"a.txt","text":"hi\n"}}],
			"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":7,"output_tokens":3}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnthropicToolCall(t *testing.T) {
	var got map[string]any
	var path, apiKey string
	srv := messageServer(t, &got, &path, &apiKey)

	resp, err := anthropic.New(srv.URL, key).Complete(context.Background(), providers.Request{
		Model: "claude-test", System: "be brief", User: "hello",
		Tools: []providers.Tool{{
			Name: "edit_file", Description: "Write a file",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"}},
				"required":   []string{"path", "text"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if apiKey != key {
		t.Fatalf("x-api-key = %q", apiKey)
	}
	if got["model"] != "claude-test" || got["max_tokens"] != float64(providers.DefaultMaxTokens) {
		t.Fatalf("body = %v", got)
	}
	tools, _ := json.Marshal(got["tools"])
	if !strings.Contains(string(tools), `"input_schema"`) || !strings.Contains(string(tools), `"required":["path","text"]`) {
		t.Fatalf("tools = %s", tools)
	}
	system, _ := json.Marshal(got["system"])
	if !strings.Contains(string(system), "be brief") {
		t.Fatalf("system = %s", system)
	}
	if resp.Text != "editing" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Args["text"] != "hi\n" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.InputTokens != 7 || resp.OutputTokens != 3 || resp.StopReason != "tool_use" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestAnthropicSendsTurns(t *testing.T) {
	var got map[string]any
	var path, apiKey string
	srv := messageServer(t, &got, &path, &apiKey)

	_, err := anthropic.New(srv.URL, key).Complete(context.Background(), providers.Request{
		Model: "claude-test", User: "card",
		Turns: []providers.Turn{{
			Text:    "looking",
			Calls:   []providers.ToolCall{{ID: "toolu_1", Name: "read_file", Args: map[string]any{"path": "a.txt"}}},
			Results: []string{"hello"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	messages, _ := got["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %v", messages)
	}
	assistant, _ := json.Marshal(messages[1])
	result, _ := json.Marshal(messages[2])
	if !strings.Contains(string(assistant), `"role":"assistant"`) || !strings.Contains(string(assistant), `"type":"tool_use"`) || !strings.Contains(string(assistant), `"id":"toolu_1"`) {
		t.Fatalf("assistant = %s", assistant)
	}
	if !strings.Contains(string(result), `"role":"user"`) || !strings.Contains(string(result), `"tool_use_id":"toolu_1"`) || !strings.Contains(string(result), "hello") {
		t.Fatalf("result = %s", result)
	}
}

func TestAnthropicReviewMaxTokens(t *testing.T) {
	var got map[string]any
	var path, apiKey string
	srv := messageServer(t, &got, &path, &apiKey)

	for _, model := range []string{"deepseek-flash", "claude-opus-4-0"} {
		got = nil
		_, err := anthropic.New(srv.URL, key).Complete(context.Background(), providers.Request{Model: model, User: "review", MaxTokens: 32 << 10})
		if err != nil {
			t.Fatalf("%s: %v", model, err)
		}
		if got["max_tokens"] != float64(32<<10) {
			t.Fatalf("%s: body = %v", model, got)
		}
	}
}

func TestAnthropicBaseURL(t *testing.T) {
	var got map[string]any
	var path, apiKey string
	srv := messageServer(t, &got, &path, &apiKey)

	for _, base := range []string{"/anthropic", "/anthropic/", "/anthropic/v1", "/anthropic/v1/messages"} {
		path = ""
		if _, err := anthropic.New(srv.URL+base, key).Complete(context.Background(), providers.Request{Model: "Pro", User: "hi"}); err != nil {
			t.Fatal(err)
		}
		if path != "/anthropic/v1/messages" {
			t.Fatalf("base %q: path = %q", base, path)
		}
	}
}
