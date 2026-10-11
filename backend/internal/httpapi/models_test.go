package httpapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	modelKey = "model-key-value-1"
	badKey   = "bad-key-value-1"
)

// modelServer answers the Anthropic messages route. A bad key gets 401 with
// the key echoed in the body, so tests can check redaction.
func modelServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		if seen != nil {
			*seen = append(*seen, r.URL.Path+" "+key)
		}
		w.Header().Set("Content-Type", "application/json")
		if key == badKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key `+key+`"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func llmAdmin(t *testing.T, baseURL, key string) func(path string) (int, string) {
	t.Helper()
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	admin := loginToken(t, srv, "ada@example.com", "password-long-1")
	families := map[string]any{"families": []map[string]any{{
		"id": "deepseek", "protocol": "anthropic", "baseUrl": baseURL,
		"models": []string{"Pro"}, "apiKey": key,
	}}}
	if rec := putJSON(t, srv, "/api/settings/llm", admin, families); rec.Code != 200 {
		t.Fatalf("llm put = %d %s", rec.Code, rec.Body.String())
	}
	return func(path string) (int, string) {
		rec := postJSON(t, srv, path, admin, map[string]any{})
		return rec.Code, rec.Body.String()
	}
}

func TestLLMTestCallsProvider(t *testing.T) {
	var seen []string
	model := modelServer(t, &seen)
	post := llmAdmin(t, model.URL+"/anthropic", modelKey)
	code, body := post("/api/settings/llm/deepseek/test")
	if code != 200 || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("test = %d %s", code, body)
	}
	if len(seen) != 1 || seen[0] != "/anthropic/v1/messages "+modelKey {
		t.Fatalf("requests = %v", seen)
	}
	if code, body := post("/api/settings/llm/missing/test"); code != 404 {
		t.Fatalf("unknown family = %d %s", code, body)
	}
}

func TestLLMTestNoKey(t *testing.T) {
	post := llmAdmin(t, "https://api.example.com/v1", "")
	if code, body := post("/api/settings/llm/deepseek/test"); code != 422 || !strings.Contains(body, `"apiKey"`) {
		t.Fatalf("no key = %d %s", code, body)
	}
}

func TestLLMTestBadKey(t *testing.T) {
	model := modelServer(t, nil)
	post := llmAdmin(t, model.URL, badKey)
	code, body := post("/api/settings/llm/deepseek/test")
	if code != 200 || !strings.Contains(body, `"ok":false`) || !strings.Contains(body, "401") {
		t.Fatalf("test = %d %s", code, body)
	}
	if strings.Contains(body, badKey) {
		t.Fatalf("key in response: %s", body)
	}
}

func TestModelSettingsAdminOnly(t *testing.T) {
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	secretsPath := filepath.Join(srv.Data, "secrets.json")
	if err := os.WriteFile(secretsPath, []byte(`{"databaseUrl":"postgres://kept","tokenKey":"kept-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	admin := loginToken(t, srv, "ada@example.com", "password-long-1")
	lead := loginToken(t, srv, "lead@example.com", "password-long-2")
	model := modelServer(t, nil)
	families := map[string]any{"families": []map[string]any{{
		"id": "anthropic", "protocol": "anthropic", "baseUrl": model.URL,
		"models": []string{"claude-example"}, "apiKey": modelKey,
	}}}
	routes := map[string]any{
		"routes":  map[string]any{"implement": map[string]string{"model": "claude-example", "fallback": ""}},
		"budgets": map[string]int{"maxAttempts": 3, "maxModelTurnsPerAttempt": 12, "maxTokensPerCard": 200000, "maxWallMinutesPerAttempt": 30},
	}

	if rec := putJSON(t, srv, "/api/settings/llm", lead, families); rec.Code != 403 {
		t.Fatalf("lead llm put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, "/api/settings/router", lead, routes); rec.Code != 403 {
		t.Fatalf("lead router put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/settings/llm/anthropic/test", lead, map[string]any{}); rec.Code != 403 {
		t.Fatalf("lead llm test = %d %s", rec.Code, rec.Body.String())
	}

	saved := putJSON(t, srv, "/api/settings/llm", admin, families)
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"keySet":true`) || strings.Contains(saved.Body.String(), modelKey) {
		t.Fatalf("admin llm put = %d %s", saved.Code, saved.Body.String())
	}
	if rec := putJSON(t, srv, "/api/settings/router", admin, routes); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"claude-example"`) {
		t.Fatalf("admin router put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/settings/llm/anthropic/test", admin, map[string]any{}); rec.Code != 200 {
		t.Fatalf("admin llm test = %d %s", rec.Code, rec.Body.String())
	}

	llm := getAuth(t, srv, "/api/settings/llm", lead)
	if llm.Code != 200 || !strings.Contains(llm.Body.String(), `"claude-example"`) || strings.Contains(llm.Body.String(), modelKey) {
		t.Fatalf("lead llm get = %d %s", llm.Code, llm.Body.String())
	}
	if rec := getAuth(t, srv, "/api/settings/router", lead); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"claude-example"`) {
		t.Fatalf("lead router get = %d %s", rec.Code, rec.Body.String())
	}

	raw, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{modelKey, "postgres://kept", "kept-key"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("service secrets lost %q", want)
		}
	}
	var events string
	if err := srv.DB.QueryRow(`SELECT coalesce(string_agg(payload_json, ' '), '') FROM events WHERE kind = 'config_changed'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(events, `"llm"`) || !strings.Contains(events, `"router"`) || strings.Contains(events, modelKey) {
		t.Fatalf("config_changed events = %s", events)
	}
}
