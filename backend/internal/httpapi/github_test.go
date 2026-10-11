package httpapi_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/identity"
)

// fakeGitHub answers the company repository and branch lookups. A name that
// starts with "missing" is a 404, "archived" is archived, and "moved" belongs
// to another organization.
func fakeGitHub(_ context.Context, child execx.Child) (execx.Result, error) {
	notFound := execx.Result{Exit: 1, Out: []byte("{\"message\":\"Not Found\"}\ngh: Not Found (HTTP 404)\n")}
	for _, arg := range child.Argv {
		path, ok := strings.CutPrefix(arg, "repos/acme/")
		if !ok {
			continue
		}
		name, branch, isBranch := strings.Cut(path, "/branches/")
		switch {
		case strings.HasPrefix(name, "missing"):
			return notFound, nil
		case isBranch && strings.HasPrefix(branch, "missing"):
			return notFound, nil
		case isBranch:
			return execx.Result{Out: []byte(branch + "\n")}, nil
		}
		owner, archived := "acme", "false"
		if strings.HasPrefix(name, "moved") {
			owner = "other"
		}
		if strings.HasPrefix(name, "archived") {
			archived = "true"
		}
		return execx.Result{Out: []byte(`{"name":"` + name + `","owner":"` + owner + `","archived":` + archived + "}\n")}, nil
	}
	return execx.Result{}, nil
}

func withCompanyGitHub(t *testing.T, srv *httpapi.Server) {
	t.Helper()
	token := companyToken
	if _, err := app.SaveCompany(context.Background(), srv.DB, identity.User{Admin: true}, "github.com", "acme", &token, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	srv.Tools = app.Tools{GH: "/usr/bin/gh", Exec: fakeGitHub}
}

func TestProjectRepoCheckedOnGitHub(t *testing.T) {
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, token, "Platform")
	create := func(repo, base string) (int, string) {
		rec := postJSON(t, srv, "/api/projects", token, map[string]string{"tribeId": tribe, "name": repo, "repo": repo, "baseBranch": base})
		return rec.Code, rec.Body.String()
	}
	for _, c := range []struct{ repo, base, field string }{
		{"missing-repo", "main", `"repo"`},
		{"archived-repo", "main", `"repo"`},
		{"moved-repo", "main", `"repo"`},
		{"widget", "missing-branch", `"baseBranch"`},
	} {
		if code, body := create(c.repo, c.base); code != 422 || !strings.Contains(body, c.field) {
			t.Fatalf("create %s@%s = %d %s, want 422 on %s", c.repo, c.base, code, body, c.field)
		}
	}
	var n int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM projects WHERE github_repo <> ''`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("stored %d projects for rejected repositories", n)
	}
	if code, body := create("widget", "release/1.0"); code != 201 {
		t.Fatalf("create widget = %d %s", code, body)
	}
	project := newProject(t, srv, token, tribe, "Gadget", "gadget")
	path := "/api/projects/" + project + "/settings/repository"
	if rec := putJSON(t, srv, path, token, map[string]string{"repo": "missing-repo", "baseBranch": "main"}); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"repo"`) {
		t.Fatalf("save missing = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, path, token, map[string]string{"repo": "gadget-v2", "baseBranch": "main"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"repo":"gadget-v2"`) {
		t.Fatalf("save gadget-v2 = %d %s", rec.Code, rec.Body.String())
	}
}

func TestProjectRepoNeedsCompanyToken(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	clear := ""
	if _, err := app.SaveCompany(context.Background(), srv.DB, admin, "github.com", "acme", &clear, true, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, token, "Platform")
	rec := postJSON(t, srv, "/api/projects", token, map[string]string{"tribeId": tribe, "name": "Widget", "repo": "widget"})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "company GitHub token") {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
}

func TestProjectRepoGitHubDown(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	srv.Tools.Exec = func(context.Context, execx.Child) (execx.Result, error) {
		return execx.Result{Exit: 1, Out: []byte("gh: Bad Gateway (HTTP 502)\n")}, nil
	}
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, token, "Platform")
	rec := postJSON(t, srv, "/api/projects", token, map[string]string{"tribeId": tribe, "name": "Widget", "repo": "widget"})
	if rec.Code != 502 {
		t.Fatalf("create = %d %s, want 502", rec.Code, rec.Body.String())
	}
}

func TestRepoListIncludesCollaboratorRepos(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	srv.Tools.Exec = func(_ context.Context, child execx.Child) (execx.Result, error) {
		if !contains(child.Argv, "user/repos?affiliation=owner,collaborator,organization_member&per_page=100") {
			return execx.Result{}, nil
		}
		filter := child.Argv[len(child.Argv)-1]
		if !strings.Contains(filter, `ascii_downcase == "acme"`) {
			return execx.Result{Exit: 1, Out: []byte("unexpected filter " + filter)}, nil
		}
		return execx.Result{Out: []byte(`{"name":"scbd-solid-web","defaultBranch":"main","archived":false,"private":true}` + "\n")}, nil
	}
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	got := getAuth(t, srv, "/api/settings/github/repos", token)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"name":"scbd-solid-web"`) || !strings.Contains(got.Body.String(), `"private":true`) {
		t.Fatalf("repos = %d %s", got.Code, got.Body.String())
	}
}

func TestCompanyTestReportsAccess(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	classic := "ghp_classic-token-value"
	if _, err := app.SaveCompany(context.Background(), srv.DB, admin, "github.com", "acme", &classic, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	var argv []string
	srv.Tools.Exec = func(_ context.Context, child execx.Child) (execx.Result, error) {
		argv = append(argv, child.Argv...)
		if contains(child.Argv, "--include") {
			return execx.Result{Out: []byte("HTTP/2.0 200 OK\r\nX-Oauth-Scopes: public_repo, workflow\r\n\r\n{\"login\":\"acme-bot\"}")}, nil
		}
		return execx.Result{}, nil
	}
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	rec := postJSON(t, srv, "/api/settings/github/test", token, map[string]any{})
	body := rec.Body.String()
	for _, want := range []string{`"ok":true`, `"login":"acme-bot"`, `"tokenKind":"classic"`, `"repoCount":0`, `"public_repo"`, "no repo scope"} {
		if rec.Code != 200 || !strings.Contains(body, want) {
			t.Fatalf("test = %d %s, want %s", rec.Code, body, want)
		}
	}
	if strings.Contains(body, classic) || strings.Contains(strings.Join(argv, " "), classic) {
		t.Fatal("token leaked into the response or argv")
	}
}
