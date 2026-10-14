package httpapi_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/httpapi"
)

func agentServer(t *testing.T) (*httpapi.Server, string, string) {
	t.Helper()
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	if err := os.WriteFile(filepath.Join(srv.Data, "secrets.json"), []byte(`{"databaseUrl":"postgres://kept","tokenKey":"kept-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	admin := loginToken(t, srv, "ada@example.com", "password-long-1")
	lead := loginToken(t, srv, "lead@example.com", "password-long-2")
	families := map[string]any{"families": []map[string]any{{
		"id": "deepseek", "protocol": "anthropic", "baseUrl": "https://api.deepseek.com/anthropic",
		"models": []string{"deepseek-chat"}, "apiKey": modelKey,
	}}}
	if rec := putJSON(t, srv, "/api/settings/llm", admin, families); rec.Code != 200 {
		t.Fatalf("llm put = %d %s", rec.Code, rec.Body.String())
	}
	return srv, admin, lead
}

var claudeAgent = map[string]any{
	"kind":       "claude_code",
	"claudeCode": map[string]any{"family": "deepseek", "model": "deepseek-chat", "auth": "auto"},
}

func TestAgentSettingsAdminOnly(t *testing.T) {
	srv, admin, lead := agentServer(t)
	if rec := getAuth(t, srv, "/api/settings/agent", lead); rec.Code != 403 {
		t.Fatalf("lead get = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, "/api/settings/agent", lead, claudeAgent); rec.Code != 403 {
		t.Fatalf("lead put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/settings/agent/test", lead, map[string]any{}); rec.Code != 403 {
		t.Fatalf("lead test = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, srv, "/api/settings/agent", admin); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"kind":"builtin"`) {
		t.Fatalf("admin get = %d %s", rec.Code, rec.Body.String())
	}
	bad := map[string]any{"kind": "claude_code", "claudeCode": map[string]any{"family": "deepseek", "model": "gpt-x", "binary": "claude"}}
	rec := putJSON(t, srv, "/api/settings/agent", admin, bad)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"claudeCode.model"`) || !strings.Contains(rec.Body.String(), `"claudeCode.binary"`) {
		t.Fatalf("bad put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/settings/agent/test", admin, map[string]any{}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("admin test = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAgentSettingsNeverReturnsKey(t *testing.T) {
	srv, admin, _ := agentServer(t)
	put := putJSON(t, srv, "/api/settings/agent", admin, claudeAgent)
	if put.Code != 200 || !strings.Contains(put.Body.String(), `"kind":"claude_code"`) || strings.Contains(put.Body.String(), modelKey) {
		t.Fatalf("put = %d %s", put.Code, put.Body.String())
	}
	get := getAuth(t, srv, "/api/settings/agent", admin)
	if get.Code != 200 || !strings.Contains(get.Body.String(), `"family":"deepseek"`) || strings.Contains(get.Body.String(), modelKey) {
		t.Fatalf("get = %d %s", get.Code, get.Body.String())
	}
	runners := getAuth(t, srv, "/api/runners", admin)
	if runners.Code != 200 || !strings.Contains(runners.Body.String(), `"agent":"claude_code"`) || strings.Contains(runners.Body.String(), modelKey) {
		t.Fatalf("runners = %d %s", runners.Code, runners.Body.String())
	}
	var changed int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM events WHERE kind = 'config_changed' AND payload_json LIKE '%"agent"%'`).Scan(&changed); err != nil {
		t.Fatal(err)
	}
	var leaks int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM events WHERE payload_json LIKE '%' || $1 || '%'`, modelKey).Scan(&leaks); err != nil {
		t.Fatal(err)
	}
	if changed != 1 || leaks != 0 {
		t.Fatalf("config_changed agent events = %d, events holding the key = %d", changed, leaks)
	}
	raw, err := os.ReadFile(filepath.Join(srv.Data, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), modelKey) || !strings.Contains(string(raw), "claude_code") {
		t.Fatalf("config.yaml:\n%s", raw)
	}
}
