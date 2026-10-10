package httpapi_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modelKey = "model-key-value-1"

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
	families := map[string]any{"families": []map[string]any{{
		"id": "anthropic", "protocol": "anthropic", "baseUrl": "https://api.anthropic.com",
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
