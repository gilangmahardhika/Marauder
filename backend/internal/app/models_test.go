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

func TestRoutedProviderModelOptions(t *testing.T) {
	var mu sync.Mutex
	bodies := map[string][]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies[r.URL.Path] = append(bodies[r.URL.Path], string(body))
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
		{ID: "primary", Protocol: "openai", BaseURL: srv.URL + "/primary", Models: []settings.Model{{ID: "big", ModelOptions: settings.ModelOptions{Thinking: "enabled", Effort: "high"}}}},
		{ID: "backup", Protocol: "openai", BaseURL: srv.URL + "/backup", Models: []settings.Model{{ID: "small", ModelOptions: settings.ModelOptions{Effort: "low"}}}},
	}
	secrets := settings.Secrets{LLMKeys: map[string]string{"primary": "k-primary-1", "backup": "k-backup-1"}}

	if _, err := app.RoutedProvider(cfg, secrets, "big", "small").Complete(context.Background(), providers.Request{User: "hi"}); err != nil {
		t.Fatal(err)
	}
	primary, backup := bodies["/primary/chat/completions"], bodies["/backup/chat/completions"]
	if len(primary) != 2 || len(backup) != 1 {
		t.Fatalf("bodies = %v", bodies)
	}
	for _, body := range primary {
		if !strings.Contains(body, `"reasoning_effort":"high"`) || !strings.Contains(body, `"thinking":{"type":"enabled"}`) {
			t.Fatalf("primary body = %s", body)
		}
	}
	if !strings.Contains(backup[0], `"reasoning_effort":"low"`) || strings.Contains(backup[0], `"thinking"`) {
		t.Fatalf("fallback body = %s", backup[0])
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

func streamChunk(text string) string {
	return `data: {"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"` + text + `"},"finish_reason":null}]}` + "\n\n"
}

func streamFamilies(url string) (settings.Config, settings.Secrets) {
	var cfg settings.Config
	cfg.LLM.Families = []settings.Family{
		{ID: "primary", Protocol: "openai", BaseURL: url + "/primary", Models: []settings.Model{{ID: "big"}}},
		{ID: "backup", Protocol: "openai", BaseURL: url + "/backup", Models: []settings.Model{{ID: "small"}}},
	}
	return cfg, settings.Secrets{LLMKeys: map[string]string{"primary": "k-primary-1", "backup": "k-backup-1"}}
}

func TestRoutedStreamFallsBackBeforeFirstToken(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/primary/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":{"message":"overloaded","type":"server_error"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, streamChunk("fallback ")+streamChunk("answer")+"data: [DONE]\n\n")
	}))
	defer srv.Close()
	cfg, secrets := streamFamilies(srv.URL)

	var deltas []string
	streamer := app.RoutedProvider(cfg, secrets, "big", "small").(providers.Streamer)
	resp, err := streamer.Stream(context.Background(), providers.Request{User: "hi"}, func(text string) {
		deltas = append(deltas, text)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deltas, "|") != "fallback |answer" || resp.Text != "fallback answer" || resp.Model != "small" {
		t.Fatalf("deltas %q resp %+v", deltas, resp)
	}
	if calls["/primary/chat/completions"] != 2 || calls["/backup/chat/completions"] != 1 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestRoutedStreamNoRetryAfterToken(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, streamChunk("partial ")+`data: {"error":{"message":"connection reset by k-primary-1"}}`+"\n\n")
	}))
	defer srv.Close()
	cfg, secrets := streamFamilies(srv.URL)

	var deltas []string
	streamer := app.RoutedProvider(cfg, secrets, "big", "small").(providers.Streamer)
	_, err := streamer.Stream(context.Background(), providers.Request{User: "hi"}, func(text string) {
		deltas = append(deltas, text)
	})
	if !errors.Is(err, app.ErrProvider) {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(deltas, "|") != "partial " {
		t.Fatalf("deltas = %q", deltas)
	}
	if calls["/primary/chat/completions"] != 1 || calls["/backup/chat/completions"] != 0 {
		t.Fatalf("calls = %v", calls)
	}
}
