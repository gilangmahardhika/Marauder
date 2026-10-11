package httpapi_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/identity"
)

const companyToken = "company-token-value"

func TestCompanyGitHubTokenNeverReturned(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "lead_developer", true)
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	put := putJSON(t, srv, "/api/settings/github", token, map[string]any{"host": "github.com", "org": "acme", "token": companyToken})
	if put.Code != 200 || strings.Contains(put.Body.String(), companyToken) || !strings.Contains(put.Body.String(), `"tokenSet":true`) {
		t.Fatalf("put = %d %s", put.Code, put.Body.String())
	}
	got := getAuth(t, srv, "/api/settings/github", token)
	if got.Code != 200 || strings.Contains(got.Body.String(), companyToken) || !strings.Contains(got.Body.String(), `"org":"acme"`) {
		t.Fatalf("get = %d %s", got.Code, got.Body.String())
	}
	var cipher string
	if err := srv.DB.QueryRow(`SELECT token_ciphertext FROM workspace_github WHERE id = 1`).Scan(&cipher); err != nil {
		t.Fatal(err)
	}
	if cipher == "" || strings.Contains(cipher, companyToken) {
		t.Fatalf("stored = %q", cipher)
	}
	var events string
	if err := srv.DB.QueryRow(`SELECT string_agg(payload_json, ' ') FROM events WHERE kind = 'config_changed'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(events, companyToken) {
		t.Fatalf("event leaked token: %s", events)
	}
}

func TestCompanyGitHubAdminOnly(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	token := loginToken(t, srv, "lead@example.com", "password-long-1")
	if rec := putJSON(t, srv, "/api/settings/github", token, map[string]any{"org": "acme", "token": companyToken}); rec.Code != 403 {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, srv, "/api/settings/github", token); rec.Code != 403 {
		t.Fatalf("get = %d", rec.Code)
	}
	if rec := postJSON(t, srv, "/api/settings/github/test", token, map[string]any{}); rec.Code != 403 {
		t.Fatalf("test = %d", rec.Code)
	}
}

func TestRepoListUsesCompanyToken(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	seedUser(t, srv, "dev@example.com", "password-long-3", "developer", false)
	saveCompany(t, srv, admin)
	rec := &recorder{reply: func(child execx.Child) execx.Result {
		if !contains(child.Argv, "orgs/acme/repos?type=all&per_page=100") {
			return execx.Result{}
		}
		return execx.Result{Out: []byte(`{"name":"widget","defaultBranch":"develop","archived":false}` + "\n" +
			`{"name":"api","defaultBranch":"main","archived":true}` + "\n")}
	}}
	srv.Tools = app.Tools{GH: "/usr/bin/gh", Exec: rec.run}
	lead := loginToken(t, srv, "lead@example.com", "password-long-2")
	got := getAuth(t, srv, "/api/settings/github/repos", lead)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"name":"widget"`) || !strings.Contains(got.Body.String(), `"defaultBranch":"develop"`) {
		t.Fatalf("repos = %d %s", got.Code, got.Body.String())
	}
	calls := rec.all()
	if len(calls) != 2 || !contains(calls[0].Argv, "orgs/acme/repos?type=all&per_page=100") ||
		!contains(calls[1].Argv, "user/repos?affiliation=owner,collaborator,organization_member&per_page=100") {
		t.Fatalf("calls = %+v", calls)
	}
	for _, call := range calls {
		if !contains(call.Env, "GH_TOKEN="+companyToken) || strings.Contains(strings.Join(call.Argv, " "), companyToken) {
			t.Fatalf("token handling: env has token %v, argv %v", contains(call.Env, "GH_TOKEN="+companyToken), call.Argv)
		}
	}
	dev := loginToken(t, srv, "dev@example.com", "password-long-3")
	if denied := getAuth(t, srv, "/api/settings/github/repos", dev); denied.Code != 403 {
		t.Fatalf("developer = %d", denied.Code)
	}
}

func TestProjectRepoUnique(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, token, "Platform")
	first := postJSON(t, srv, "/api/projects", token, map[string]string{"tribeId": tribe, "name": "Widget", "repo": "widget", "baseBranch": "main"})
	if first.Code != 201 || !strings.Contains(first.Body.String(), `"repo":"widget"`) || strings.Contains(first.Body.String(), "repoPath") {
		t.Fatalf("first = %d %s", first.Code, first.Body.String())
	}
	second := postJSON(t, srv, "/api/projects", token, map[string]string{"tribeId": tribe, "name": "Widget again", "repo": "widget"})
	if second.Code != 422 || !strings.Contains(second.Body.String(), `"repo"`) {
		t.Fatalf("second = %d %s", second.Code, second.Body.String())
	}
}

func TestCloneTokenNotInArgv(t *testing.T) {
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	lead := seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	saveCompany(t, srv, admin)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, adminToken, "Platform")
	project := newProject(t, srv, adminToken, tribe, "Widget", "widget")
	if err := app.AddMember(context.Background(), srv.DB, admin, tribe, lead.ID); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{reply: func(child execx.Child) execx.Result {
		if contains(child.Argv, "rev-parse") {
			return execx.Result{Out: []byte(headSHA + "\n")}
		}
		return execx.Result{}
	}}
	srv.Tools = app.Tools{Git: "/usr/bin/git", Exec: rec.run}
	leadToken := loginToken(t, srv, "lead@example.com", "password-long-2")
	synced := postJSON(t, srv, "/api/projects/"+project+"/sync", leadToken, map[string]any{})
	if synced.Code != 200 {
		t.Fatalf("sync = %d %s", synced.Code, synced.Body.String())
	}
	var clone *execx.Child
	for _, call := range rec.all() {
		if strings.Contains(strings.Join(call.Argv, " "), companyToken) {
			t.Fatalf("token in argv: %v", call.Argv)
		}
		if contains(call.Argv, "clone") {
			c := call
			clone = &c
		}
	}
	if clone == nil {
		t.Fatal("no clone call")
	}
	if !contains(clone.Argv, "https://github.com/acme/widget.git") {
		t.Fatalf("clone argv = %v", clone.Argv)
	}
	askpass := filepath.Join(srv.Data, "projects", project, "askpass", "askpass.sh")
	if !contains(clone.Env, "GH_TOKEN="+companyToken) || !contains(clone.Env, "GIT_ASKPASS="+askpass) {
		t.Fatalf("clone env = %v", clone.Env)
	}
	if !contains(clone.Argv, filepath.Join(srv.Data, "projects", project, "repo")) {
		t.Fatalf("clone target = %v", clone.Argv)
	}
}

func TestBotApprover(t *testing.T) {
	srv := accountServer(t)
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", true)
	if err := identity.SaveGitHub(context.Background(), srv.DB, lead.ID, "github.com", "acme-bot", nil, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE workspace_github SET bot_login = 'acme-bot' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMember(context.Background(), srv.DB, lead, "tribe_default", lead.ID); err != nil {
		t.Fatal(err)
	}
	card := reviewCard(t, srv, "proj_default")
	token := loginToken(t, srv, "lead@example.com", "password-long-1")
	rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/approve", token, map[string]string{"sha": headSHA})
	if rec.Code != 422 {
		t.Fatalf("bot approve = %d %s", rec.Code, rec.Body.String())
	}
}

type recorder struct {
	mu    sync.Mutex
	calls []execx.Child
	reply func(execx.Child) execx.Result
}

func (r *recorder) run(_ context.Context, child execx.Child) (execx.Result, error) {
	r.mu.Lock()
	r.calls = append(r.calls, child)
	r.mu.Unlock()
	return r.reply(child), nil
}

func (r *recorder) all() []execx.Child {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]execx.Child(nil), r.calls...)
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func saveCompany(t *testing.T, srv *httpapi.Server, admin app.User) {
	t.Helper()
	token := companyToken
	if _, err := app.SaveCompany(context.Background(), srv.DB, admin, "github.com", "acme", &token, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
}

func dataRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newTribe(t *testing.T, srv *httpapi.Server, token, name string) string {
	t.Helper()
	rec := postJSON(t, srv, "/api/tribes", token, map[string]string{"name": name})
	if rec.Code != 201 {
		t.Fatalf("tribe = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.ID
}

func newProject(t *testing.T, srv *httpapi.Server, token, tribe, name, repo string) string {
	t.Helper()
	rec := postJSON(t, srv, "/api/projects", token, map[string]string{"tribeId": tribe, "name": name, "repo": repo})
	if rec.Code != 201 {
		t.Fatalf("project = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.ID
}
