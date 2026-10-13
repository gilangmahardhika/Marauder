package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/mcpcatalog"
	"orchestrator/backend/internal/netguard"
)

func TestMCPCatalogAccess(t *testing.T) {
	f := newTribeFixture(t)
	var regHits atomic.Int32
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		regHits.Add(1)
		_, _ = w.Write([]byte(`{"servers":[{"server":{"name":"io.example/tickets","version":"1.0.0",
			"remotes":[{"type":"streamable-http","url":"https://mcp.tickets.example.com/mcp"}]}}],"metadata":{}}`))
	}))
	t.Cleanup(reg.Close)
	f.srv.Catalog = mcpcatalog.New(reg.URL, netguard.Policy{AllowLoopbackHTTP: true})
	base := "/api/tribes/" + f.tribe + "/mcp-catalog"
	memberToken := loginToken(t, f.srv, "member@example.com", "password-long-3")
	outsiderToken := loginToken(t, f.srv, "out@example.com", "password-long-4")
	for name, token := range map[string]string{"member": memberToken, "outsider": outsiderToken} {
		if rec := getAuth(t, f.srv, base+"?q=pg", token); rec.Code != 403 {
			t.Errorf("%s catalog = %d", name, rec.Code)
		}
		if rec := postJSON(t, f.srv, "/api/tribes/"+f.tribe+"/mcp-images/pull", token, map[string]string{"image": "ghcr.io/a/b:1"}); rec.Code != 403 {
			t.Errorf("%s pull = %d", name, rec.Code)
		}
	}
	if rec := getAuth(t, f.srv, "/api/tribes/"+f.other+"/mcp-catalog", f.chiefToken); rec.Code != 403 {
		t.Errorf("chief other tribe = %d", rec.Code)
	}
	if rec := getAuth(t, f.srv, "/api/tribes/tribe_missing/mcp-catalog", f.adminToken); rec.Code != 404 {
		t.Errorf("missing tribe = %d", rec.Code)
	}

	rec := getAuth(t, f.srv, base, f.chiefToken)
	if rec.Code != 200 {
		t.Fatalf("catalog = %d %s", rec.Code, rec.Body.String())
	}
	var page app.MCPCatalogPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	local := 2 + len(mcpcatalog.Featured())
	if len(page.Entries) != local+1 || page.Entries[0].ID != "builtin:postgres" || page.Entries[local].ID != "registry:io.example/tickets@1.0.0" {
		t.Fatalf("entries = %+v", page.Entries)
	}

	hits := regHits.Load()
	rec = getAuth(t, f.srv, base+"?source=local&q=github", f.chiefToken)
	page = app.MCPCatalogPage{}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if rec.Code != 200 || len(page.Entries) == 0 || page.Entries[0].ID != "featured:github" || regHits.Load() != hits {
		t.Fatalf("local = %d %s", rec.Code, rec.Body.String())
	}
	rec = getAuth(t, f.srv, base+"?source=registry&q=tickets", f.chiefToken)
	page = app.MCPCatalogPage{}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if rec.Code != 200 || len(page.Entries) != 1 || page.Entries[0].Source != "registry" {
		t.Fatalf("registry = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, f.srv, base+"?source=everything", f.chiefToken); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"source"`) {
		t.Fatalf("bad source = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, f.srv, base+"?source=everything", memberToken); rec.Code != 403 {
		t.Fatalf("member bad source = %d", rec.Code)
	}

	rec = getAuth(t, f.srv, base+"?id=featured%3Agithub", f.chiefToken)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"url":"https://api.githubcopilot.com/mcp/readonly"`) {
		t.Fatalf("featured entry = %d %s", rec.Code, rec.Body.String())
	}
	rec = getAuth(t, f.srv, base+"?id=builtin%3Asqlserver", f.chiefToken)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"transport":"sqlserver"`) {
		t.Fatalf("entry = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, f.srv, base+"?id=builtin%3Aoracle", f.chiefToken); rec.Code != 404 {
		t.Fatalf("missing entry = %d", rec.Code)
	}

	// With no catalog the built-in and featured entries still list, with the notice.
	f.srv.Catalog = nil
	rec = getAuth(t, f.srv, base, f.chiefToken)
	if !strings.Contains(rec.Body.String(), mcpcatalog.RegistryError) || !strings.Contains(rec.Body.String(), "builtin:sqlserver") {
		t.Fatalf("no catalog = %s", rec.Body.String())
	}
	if rec := getAuth(t, f.srv, base+"?source=local", f.chiefToken); strings.Contains(rec.Body.String(), "registryError") || !strings.Contains(rec.Body.String(), "featured:deepwiki") {
		t.Fatalf("no catalog local = %s", rec.Body.String())
	}
	if rec := getAuth(t, f.srv, base+"?source=registry", f.chiefToken); !strings.Contains(rec.Body.String(), mcpcatalog.RegistryError) || strings.Contains(rec.Body.String(), "builtin:") {
		t.Fatalf("no catalog registry = %s", rec.Body.String())
	}
	if rec := getAuth(t, f.srv, base+"?id=featured%3Agithub", f.chiefToken); rec.Code != 200 {
		t.Fatalf("no catalog featured entry = %d", rec.Code)
	}
}

func TestMCPImagePull(t *testing.T) {
	f := newTribeFixture(t)
	digest := "ghcr.io/acme/docs-mcp@sha256:" + strings.Repeat("b", 64)
	var argvs [][]string
	pullExit := 0
	f.srv.Images = app.ImagePuller{Engine: "/usr/bin/podman", Run: func(_ context.Context, c execx.Child) (execx.Result, error) {
		argvs = append(argvs, c.Argv)
		if c.Argv[1] == "pull" {
			return execx.Result{Exit: pullExit}, nil
		}
		return execx.Result{Out: []byte(`["other.io/mirror@sha256:` + strings.Repeat("c", 64) + `","` + digest + `"]` + "\n")}, nil
	}}
	path := "/api/tribes/" + f.tribe + "/mcp-images/pull"
	rec := postJSON(t, f.srv, path, f.chiefToken, map[string]string{"image": "ghcr.io/acme/docs-mcp:0.3.1"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"image":"`+digest+`"`) {
		t.Fatalf("pull = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Join(argvs[0], " ") != "/usr/bin/podman pull ghcr.io/acme/docs-mcp:0.3.1" || argvs[1][1] != "image" {
		t.Fatalf("argv = %v", argvs)
	}

	for _, bad := range []string{"--privileged", "ghcr.io/acme/docs mcp", "Upper/Case", "a;b", ""} {
		argvs = nil
		rec := postJSON(t, f.srv, path, f.chiefToken, map[string]string{"image": bad})
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"image"`) || len(argvs) != 0 {
			t.Fatalf("bad ref %q = %d %s, ran %v", bad, rec.Code, rec.Body.String(), argvs)
		}
	}

	pullExit = 1
	if rec := postJSON(t, f.srv, path, f.chiefToken, map[string]string{"image": "ghcr.io/acme/docs-mcp:9"}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "could not pull") {
		t.Fatalf("failed pull = %d %s", rec.Code, rec.Body.String())
	}

	argvs = nil
	f.srv.Images.Fixture = true
	if rec := postJSON(t, f.srv, path, f.chiefToken, map[string]string{"image": "ghcr.io/acme/docs-mcp:0.3.1"}); rec.Code != 422 || len(argvs) != 0 {
		t.Fatalf("fixture pull = %d, ran %v", rec.Code, argvs)
	}
	f.srv.Images = app.ImagePuller{}
	if rec := postJSON(t, f.srv, path, f.chiefToken, map[string]string{"image": "ghcr.io/acme/docs-mcp:0.3.1"}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "sandbox") {
		t.Fatalf("no engine pull = %d %s", rec.Code, rec.Body.String())
	}
}
