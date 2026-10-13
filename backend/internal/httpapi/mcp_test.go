package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/mcp/mcptest"
	"orchestrator/backend/internal/netguard"
)

const mcpSecret = "Bearer mcp-secret-value-42"

func mcpBody(url string) map[string]any {
	return map[string]any{
		"slug": "tickets", "transport": "http", "url": url,
		"secrets": map[string]string{"Authorization": mcpSecret},
		"tools": []map[string]any{{
			"name": "lookup", "description": "Find a ticket by id",
			"schema": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}},
		}},
	}
}

func createMCP(t *testing.T, f tribeFixture, token string, body map[string]any) map[string]any {
	t.Helper()
	rec := postJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers", token, body)
	if rec.Code != 201 {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPServerSecretsNeverReturned(t *testing.T) {
	f := newTribeFixture(t)
	created := createMCP(t, f, f.chiefToken, mcpBody("https://mcp.example.com/v1"))
	id := created["id"].(string)
	rec := getAuth(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers", f.chiefToken)
	if rec.Code != 200 {
		t.Fatalf("list = %d", rec.Code)
	}
	for _, body := range []string{rec.Body.String(), mustJSON(t, created)} {
		if strings.Contains(body, "mcp-secret-value-42") {
			t.Fatalf("secret returned: %s", body)
		}
		if !strings.Contains(body, `"secretsSet":["Authorization"]`) {
			t.Fatalf("secretsSet missing: %s", body)
		}
	}
	tool := created["tools"].([]any)[0].(map[string]any)
	if tool["schemaSha256"] == "" || tool["description"] != "Find a ticket by id" {
		t.Fatalf("tool = %v", tool)
	}

	// An empty value keeps the secret; clear deletes it.
	keep := mcpBody("https://mcp.example.com/v1")
	keep["secrets"] = map[string]string{"Authorization": ""}
	rec = putJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers/"+id, f.chiefToken, keep)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"secretsSet":["Authorization"]`) {
		t.Fatalf("keep = %d %s", rec.Code, rec.Body.String())
	}
	var cipher string
	if err := f.srv.DB.QueryRow(`SELECT secrets_ciphertext FROM tribe_mcp_servers WHERE id = $1`, id).Scan(&cipher); err != nil {
		t.Fatal(err)
	}
	if cipher == "" || strings.Contains(cipher, "mcp-secret") {
		t.Fatalf("ciphertext = %q", cipher)
	}
	short := mcpBody("https://mcp.example.com/v1")
	short["secrets"] = map[string]string{"Authorization": "short"}
	if rec := putJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers/"+id, f.chiefToken, short); rec.Code != 422 {
		t.Fatalf("short secret = %d", rec.Code)
	}
	clear := mcpBody("https://mcp.example.com/v1")
	delete(clear, "secrets")
	clear["clear"] = []string{"Authorization"}
	rec = putJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers/"+id, f.chiefToken, clear)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"secretsSet":[]`) {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body.String())
	}

	rows, err := f.srv.DB.Query(`SELECT payload_json FROM events WHERE kind = 'config_changed'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payload, "mcp-secret") || !strings.Contains(payload, `"section":"mcp"`) {
			t.Fatalf("config_changed = %s", payload)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("config_changed count = %d", n)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestMCPServerAccess(t *testing.T) {
	f := newTribeFixture(t)
	created := createMCP(t, f, f.adminToken, mcpBody("https://mcp.example.com/v1"))
	id := created["id"].(string)
	memberToken := loginToken(t, f.srv, "member@example.com", "password-long-3")
	outsiderToken := loginToken(t, f.srv, "out@example.com", "password-long-4")
	base := "/api/tribes/" + f.tribe + "/mcp-servers"
	for name, token := range map[string]string{"member": memberToken, "outsider": outsiderToken} {
		if rec := getAuth(t, f.srv, base, token); rec.Code != 403 {
			t.Errorf("%s list = %d", name, rec.Code)
		}
		if rec := putJSON(t, f.srv, base+"/"+id, token, mcpBody("https://mcp.example.com/v1")); rec.Code != 403 {
			t.Errorf("%s save = %d", name, rec.Code)
		}
		if rec := sendJSON(t, f.srv, http.MethodDelete, base+"/"+id, token, nil); rec.Code != 403 {
			t.Errorf("%s delete = %d", name, rec.Code)
		}
	}
	// The tribe admin of one tribe cannot reach another tribe's servers, and
	// a server id from one tribe is not found under another.
	if rec := getAuth(t, f.srv, "/api/tribes/"+f.other+"/mcp-servers", f.chiefToken); rec.Code != 403 {
		t.Errorf("chief other tribe = %d", rec.Code)
	}
	if rec := putJSON(t, f.srv, "/api/tribes/"+f.other+"/mcp-servers/"+id, f.adminToken, mcpBody("https://mcp.example.com/v1")); rec.Code != 404 {
		t.Errorf("server under other tribe = %d", rec.Code)
	}
	if rec := getAuth(t, f.srv, "/api/tribes/tribe_missing/mcp-servers", f.adminToken); rec.Code != 404 {
		t.Errorf("missing tribe = %d", rec.Code)
	}
	if rec := getAuth(t, f.srv, base, f.chiefToken); rec.Code != 200 {
		t.Errorf("chief list = %d", rec.Code)
	}
	if rec := sendJSON(t, f.srv, http.MethodDelete, base+"/"+id, f.chiefToken, nil); rec.Code != 204 {
		t.Errorf("chief delete = %d", rec.Code)
	}
	if rec := sendJSON(t, f.srv, http.MethodDelete, base+"/"+id, f.chiefToken, nil); rec.Code != 404 {
		t.Errorf("delete again = %d", rec.Code)
	}
}

func TestMCPSlugAndToolNames(t *testing.T) {
	f := newTribeFixture(t)
	base := "/api/tribes/" + f.tribe + "/mcp-servers"
	cases := map[string]func(map[string]any){
		"slug upper":      func(b map[string]any) { b["slug"] = "Tickets" },
		"slug underscore": func(b map[string]any) { b["slug"] = "tick_ets" },
		"slug too long":   func(b map[string]any) { b["slug"] = "abcdefghijklmnopq" },
		"http url":        func(b map[string]any) { b["url"] = "http://mcp.example.com" },
		"query url":       func(b map[string]any) { b["url"] = "https://mcp.example.com/?key=x" },
		"private url":     func(b map[string]any) { b["url"] = "https://10.1.2.3/mcp" },
		"bad transport":   func(b map[string]any) { b["transport"] = "ws" },
		"reserved header": func(b map[string]any) { b["secrets"] = map[string]string{"Mcp-Session-Id": "aaaaaaaaaa"} },
		"tool name dot":   func(b map[string]any) { b["tools"] = []map[string]any{{"name": "a.b"}} },
		"tool name long":  func(b map[string]any) { b["tools"] = []map[string]any{{"name": strings.Repeat("a", 41)}} },
		"tool twice":      func(b map[string]any) { b["tools"] = []map[string]any{{"name": "a"}, {"name": "a"}} },
		"tool long desc": func(b map[string]any) {
			b["tools"] = []map[string]any{{"name": "a", "description": strings.Repeat("d", 513)}}
		},
		"schema not obj": func(b map[string]any) {
			b["tools"] = []map[string]any{{"name": "a", "schema": map[string]any{"type": "string"}}}
		},
		"stdio no digest": func(b map[string]any) {
			b["transport"] = "stdio"
			b["url"] = ""
			b["image"] = "node:22"
			b["argv"] = []string{"mcp-server"}
		},
		"stdio shell argv": func(b map[string]any) {
			b["transport"] = "stdio"
			b["url"] = ""
			b["image"] = pinned
			b["argv"] = []string{"sh", "-c", "x"}
		},
		"stdio bad env": func(b map[string]any) {
			b["transport"] = "stdio"
			b["url"] = ""
			b["image"] = pinned
			b["argv"] = []string{"srv"}
			b["secrets"] = map[string]string{"BAD-NAME": "aaaaaaaaaa"}
		},
	}
	for name, mutate := range cases {
		body := mcpBody("https://mcp.example.com/v1")
		mutate(body)
		if rec := postJSON(t, f.srv, base, f.chiefToken, body); rec.Code != 422 {
			t.Errorf("%s = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	stdio := mcpBody("")
	stdio["transport"] = "stdio"
	stdio["image"] = pinned
	stdio["argv"] = []string{"mcp-docs", "--read-only"}
	stdio["secrets"] = map[string]string{"DOCS_TOKEN": "aaaaaaaaaa"}
	created := createMCP(t, f, f.chiefToken, stdio)
	if created["transport"] != "stdio" || mustJSON(t, created["secretsSet"]) != `["DOCS_TOKEN"]` {
		t.Fatalf("stdio = %v", created)
	}
}

const pinned = "ghcr.io/acme/mcp-docs@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestMCPNameCollision(t *testing.T) {
	f := newTribeFixture(t)
	createMCP(t, f, f.chiefToken, mcpBody("https://mcp.example.com/v1"))
	body := mcpBody("https://other.example.com/v1")
	if rec := postJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers", f.chiefToken, body); rec.Code != 422 || !strings.Contains(rec.Body.String(), "slug") {
		t.Fatalf("duplicate slug = %d %s", rec.Code, rec.Body.String())
	}
	// The same slug in another tribe is a different server.
	rec := postJSON(t, f.srv, "/api/tribes/"+f.other+"/mcp-servers", f.adminToken, body)
	if rec.Code != 201 {
		t.Fatalf("other tribe = %d %s", rec.Code, rec.Body.String())
	}
}

func TestMCPDiscoverNoBody(t *testing.T) {
	f := newTribeFixture(t)
	f.srv.MCP.Policy = netguard.Policy{AllowLoopbackHTTP: true}
	fake := mcptest.New([]mcp.Tool{
		{Name: "lookup", Description: "server text", Schema: map[string]any{"type": "object"}},
		{Name: "bad.name", Schema: map[string]any{"type": "object"}},
	})
	defer fake.Close()
	created := createMCP(t, f, f.chiefToken, mcpBody(strings.Replace(fake.URL, "localhost", "127.0.0.1", 1)+"/mcp"))
	id := created["id"].(string)
	rec := postJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers/"+id+"/discover", f.chiefToken, map[string]any{})
	if rec.Code != 200 {
		t.Fatalf("discover = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		OK    bool `json:"ok"`
		Tools []struct {
			Name         string `json:"name"`
			SchemaSHA256 string `json:"schemaSha256"`
			Valid        bool   `json:"valid"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || len(out.Tools) != 2 || !out.Tools[0].Valid || out.Tools[1].Valid || out.Tools[0].SchemaSHA256 == "" {
		t.Fatalf("discover = %s", rec.Body.String())
	}
	if got := fake.Headers[0].Get("Authorization"); got != mcpSecret {
		t.Fatalf("authorization = %q", got)
	}

	// A server that answers with text that is not MCP shows a fixed message,
	// never its body.
	leak := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("internal secret page"))
	})
	fake.Config.Handler = leak
	rec = postJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers/"+id+"/discover", f.chiefToken, map[string]any{})
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "internal secret page") || !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("failed discover = %d %s", rec.Code, rec.Body.String())
	}

	// Without the loopback policy, the same server is refused by address.
	f.srv.MCP.Policy = netguard.Policy{}
	rec = postJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers/"+id+"/discover", f.chiefToken, map[string]any{})
	if !strings.Contains(rec.Body.String(), "address was refused") {
		t.Fatalf("blocked discover = %s", rec.Body.String())
	}
	memberToken := loginToken(t, f.srv, "member@example.com", "password-long-3")
	if rec := postJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-servers/"+id+"/discover", memberToken, map[string]any{}); rec.Code != 403 {
		t.Fatalf("member discover = %d", rec.Code)
	}
}
