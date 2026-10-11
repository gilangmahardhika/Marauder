package llm_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/llm"
	"orchestrator/backend/internal/settings"
)

const (
	chatReply    = `{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"OK"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	messageReply = `{"id":"m","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
)

func TestForModelPicksProtocol(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/groq/chat/completions" {
			_, _ = io.WriteString(w, chatReply)
			return
		}
		_, _ = io.WriteString(w, messageReply)
	}))
	defer srv.Close()

	var cfg settings.Config
	cfg.LLM.Families = []settings.Family{
		{ID: "groq", Protocol: "openai", BaseURL: srv.URL + "/groq", Models: []settings.Model{{ID: "llama"}}},
		{ID: "deepseek", Protocol: "anthropic", BaseURL: srv.URL + "/deepseek", Models: []settings.Model{{ID: "Pro"}}},
		{ID: "nokey", Protocol: "openai", BaseURL: "https://api.example.com/v1", Models: []settings.Model{{ID: "far"}}},
		{ID: "local", Protocol: "openai", BaseURL: "http://127.0.0.1:1/v1", Models: []settings.Model{{ID: "near"}}},
	}
	secrets := settings.Secrets{LLMKeys: map[string]string{"groq": "gsk-1", "deepseek": "sk-2"}}

	for _, model := range []string{"llama", "Pro"} {
		p, family, key, err := llm.ForModel(cfg, secrets, model)
		if err != nil {
			t.Fatalf("%s: %v", model, err)
		}
		if key != secrets.LLMKeys[family.ID] {
			t.Fatalf("%s key from %s", model, family.ID)
		}
		if _, err := p.Complete(context.Background(), providers.Request{Model: model, User: "ping"}); err != nil {
			t.Fatalf("%s complete: %v", model, err)
		}
	}
	if len(hits) != 2 || hits[0] != "/groq/chat/completions" || hits[1] != "/deepseek/v1/messages" {
		t.Fatalf("hits = %v", hits)
	}
	if _, _, _, err := llm.ForModel(cfg, secrets, "far"); !errors.Is(err, llm.ErrNoKey) {
		t.Fatalf("remote family without key = %v", err)
	}
	if _, _, _, err := llm.ForModel(cfg, secrets, "near"); err != nil {
		t.Fatalf("loopback family without key = %v", err)
	}
	if _, _, _, err := llm.ForModel(cfg, secrets, "missing"); !errors.Is(err, llm.ErrUnknownModel) {
		t.Fatalf("unknown model = %v", err)
	}
}
