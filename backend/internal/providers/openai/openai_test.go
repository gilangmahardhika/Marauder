package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/openai"
)

const key = "sk-openai-test-key-1"

func TestOpenAIToolCall(t *testing.T) {
	var got map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c","object":"chat.completion","created":1,"model":"m",
			"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,
			"tool_calls":[{"id":"t1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]}}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer srv.Close()

	client := openai.New(srv.URL+"/openai/v1", key)
	resp, err := client.Complete(context.Background(), providers.Request{
		Model: "llama-test", System: "be brief", User: "hello",
		Tools: []providers.Tool{{
			Name: "read_file", Description: "Read a file",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/openai/v1/chat/completions" || auth != "Bearer "+key {
		t.Fatalf("path %q auth %q", path, auth)
	}
	if got["model"] != "llama-test" {
		t.Fatalf("model = %v", got["model"])
	}
	tools, _ := got["tools"].([]any)
	if len(tools) != 1 || !strings.Contains(mustJSON(t, tools[0]), `"required":["path"]`) {
		t.Fatalf("tools = %v", got["tools"])
	}
	messages, _ := got["messages"].([]any)
	if len(messages) != 2 || !strings.Contains(mustJSON(t, messages[0]), "be brief") {
		t.Fatalf("messages = %v", got["messages"])
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "read_file" || resp.ToolCalls[0].Args["path"] != "a.txt" {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	if resp.InputTokens != 10 || resp.OutputTokens != 5 || resp.StopReason != "tool_calls" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestOpenAISendsTurns(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c","object":"chat.completion","created":1,"model":"m",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	_, err := openai.New(srv.URL, key).Complete(context.Background(), providers.Request{
		Model: "m", System: "sys", User: "card",
		Turns: []providers.Turn{{
			Text:    "looking",
			Calls:   []providers.ToolCall{{ID: "call_1", Name: "read_file", Args: map[string]any{"path": "a.txt"}}},
			Results: []string{"hello"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	messages, _ := got["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("messages = %s", mustJSON(t, messages))
	}
	assistant, tool := mustJSON(t, messages[2]), mustJSON(t, messages[3])
	if !strings.Contains(assistant, `"role":"assistant"`) || !strings.Contains(assistant, `"id":"call_1"`) || !strings.Contains(assistant, `\"path\":\"a.txt\"`) {
		t.Fatalf("assistant = %s", assistant)
	}
	if !strings.Contains(tool, `"role":"tool"`) || !strings.Contains(tool, `"tool_call_id":"call_1"`) || !strings.Contains(tool, `"hello"`) {
		t.Fatalf("tool = %s", tool)
	}
}

func TestOpenAIEndpointBaseURL(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"c","object":"chat.completion","created":1,"model":"m",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	for _, base := range []string{"/openai/v1/", "/openai/v1/chat/completions", "/openai/v1/chat/completions/"} {
		path = ""
		if _, err := openai.New(srv.URL+base, key).Complete(context.Background(), providers.Request{Model: "m", User: "hi"}); err != nil {
			t.Fatal(err)
		}
		if path != "/openai/v1/chat/completions" {
			t.Fatalf("base %q: path = %q", base, path)
		}
	}
}

func TestOpenAIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Invalid API Key `+key+`","type":"invalid_request_error","code":"invalid_api_key"}}`)
	}))
	defer srv.Close()

	_, err := openai.New(srv.URL, key).Complete(context.Background(), providers.Request{Model: "m", User: "hi"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Invalid API Key") {
		t.Fatalf("err = %v", err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
