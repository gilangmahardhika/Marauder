package app_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/settings"
)

const okChat = `{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"fallback answer"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`

func TestFallbackAfterRetry(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/primary/") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":{"message":"overloaded","type":"server_error"}}`)
			return
		}
		_, _ = io.WriteString(w, okChat)
	}))
	defer srv.Close()

	var cfg settings.Config
	cfg.LLM.Families = []settings.Family{
		{ID: "primary", Protocol: "openai", BaseURL: srv.URL + "/primary", Models: []settings.Model{{ID: "big"}}},
		{ID: "backup", Protocol: "openai", BaseURL: srv.URL + "/backup", Models: []settings.Model{{ID: "small"}}},
	}
	secrets := settings.Secrets{LLMKeys: map[string]string{"primary": "k-primary-1", "backup": "k-backup-1"}}

	resp, err := app.RoutedProvider(cfg, secrets, "big", "small").Complete(context.Background(), providers.Request{User: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "fallback answer" {
		t.Fatalf("text = %q", resp.Text)
	}
	if calls["/primary/chat/completions"] != 2 || calls["/backup/chat/completions"] != 1 {
		t.Fatalf("calls = %v", calls)
	}

	_, err = app.RoutedProvider(cfg, secrets, "big", "").Complete(context.Background(), providers.Request{User: "hi"})
	if !errors.Is(err, app.ErrProvider) || strings.Contains(err.Error(), "k-primary-1") {
		t.Fatalf("err = %v", err)
	}
}

func TestRateLimitWaitsThenRetries(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		n := calls[r.URL.Path]
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.Header().Set("Retry-After-Ms", "50")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"Rate limit reached on tokens per minute","type":"tokens"}}`)
			return
		}
		_, _ = io.WriteString(w, okChat)
	}))
	defer srv.Close()

	var cfg settings.Config
	cfg.LLM.Families = []settings.Family{
		{ID: "primary", Protocol: "openai", BaseURL: srv.URL + "/primary", Models: []settings.Model{{ID: "big"}}},
	}
	secrets := settings.Secrets{LLMKeys: map[string]string{"primary": "k-primary-1"}}

	start := time.Now()
	if _, err := app.RoutedProvider(cfg, secrets, "big", "").Complete(context.Background(), providers.Request{User: "hi"}); err != nil {
		t.Fatal(err)
	}
	if calls["/primary/chat/completions"] != 2 {
		t.Fatalf("calls = %v", calls)
	}
	if waited := time.Since(start); waited < 50*time.Millisecond {
		t.Fatalf("did not wait for retry-after: %s", waited)
	}
}
