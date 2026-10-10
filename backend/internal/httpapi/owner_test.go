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

func TestGitHubLoginVerified(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	seedUser(t, srv, "eve@example.com", "password-long-2", "developer", false)
	rec := &recorder{reply: func(execx.Child) execx.Result {
		return execx.Result{Out: []byte("real-login\n12345\n")}
	}}
	srv.Tools = app.Tools{GH: "/usr/bin/gh", Exec: rec.run}
	ada := loginToken(t, srv, "ada@example.com", "password-long-1")

	put := putJSON(t, srv, "/api/me/github", ada, map[string]any{"host": "github.com", "login": "typed-login", "token": "ada-token-1"})
	if put.Code != 200 || !strings.Contains(put.Body.String(), `"verified":false`) {
		t.Fatalf("put = %d %s", put.Code, put.Body.String())
	}
	tested := postJSON(t, srv, "/api/me/github/test", ada, map[string]any{})
	if tested.Code != 200 || !strings.Contains(tested.Body.String(), `"ok":true`) || !strings.Contains(tested.Body.String(), `"login":"real-login"`) {
		t.Fatalf("test = %d %s", tested.Code, tested.Body.String())
	}
	for _, call := range rec.all() {
		if strings.Contains(strings.Join(call.Argv, " "), "ada-token-1") || !contains(call.Env, "GH_TOKEN=ada-token-1") {
			t.Fatalf("token handling: argv=%v", call.Argv)
		}
	}
	got := getAuth(t, srv, "/api/me/github", ada)
	if !strings.Contains(got.Body.String(), `"login":"real-login"`) || !strings.Contains(got.Body.String(), `"verified":true`) {
		t.Fatalf("get = %s", got.Body.String())
	}
	if me := getAuth(t, srv, "/api/me", ada); !strings.Contains(me.Body.String(), `"githubVerified":true`) {
		t.Fatalf("me = %s", me.Body.String())
	}

	putJSON(t, srv, "/api/me/github", ada, map[string]any{"host": "github.com", "login": "real-login", "token": "ada-token-2"})
	if got := getAuth(t, srv, "/api/me/github", ada); !strings.Contains(got.Body.String(), `"verified":false`) {
		t.Fatalf("new token kept verification: %s", got.Body.String())
	}
	postJSON(t, srv, "/api/me/github/test", ada, map[string]any{})

	eve := loginToken(t, srv, "eve@example.com", "password-long-2")
	putJSON(t, srv, "/api/me/github", eve, map[string]any{"host": "github.com", "login": "real-login", "token": "eve-token"})
	taken := postJSON(t, srv, "/api/me/github/test", eve, map[string]any{})
	if taken.Code != 200 || !strings.Contains(taken.Body.String(), `"ok":false`) {
		t.Fatalf("second verify = %d %s", taken.Code, taken.Body.String())
	}
	if got := getAuth(t, srv, "/api/me/github", eve); !strings.Contains(got.Body.String(), `"verified":false`) {
		t.Fatalf("second user verified: %s", got.Body.String())
	}
}

func TestReviewersSetByCreator(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	creator := seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	other := seedUser(t, srv, "eve@example.com", "password-long-2", "developer", false)
	lead := seedUser(t, srv, "lead@example.com", "password-long-3", "lead_developer", false)
	grace := verifiedMember(t, srv, admin, "grace@example.com", "grace", 21)
	plain := seedUser(t, srv, "plain@example.com", "password-long-5", "developer", false)
	outsider := seedUser(t, srv, "out@example.com", "password-long-6", "developer", false)
	if err := identity.SaveGitHub(ctx, srv.DB, outsider.ID, "github.com", "outsider", nil, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if err := identity.VerifyGitHub(ctx, srv.DB, outsider.ID, "outsider", 22); err != nil {
		t.Fatal(err)
	}
	for _, user := range []app.User{creator, other, lead, plain} {
		if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.DB.Exec(`
		INSERT INTO requirements (
			id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
			content_hash, project_id, created_by
		) VALUES ('REQ-001', 'prd', 'docs/prd/a.md', 'Export', 'statement', '[]', '[]', 'h1', 'proj_default', $1)`,
		creator.ID); err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/proj_default/requirements/REQ-001/reviewers"

	eve := loginToken(t, srv, "eve@example.com", "password-long-2")
	if rec := putJSON(t, srv, path, eve, map[string]any{"userIds": []string{grace.ID}}); rec.Code != 403 {
		t.Fatalf("other user = %d %s", rec.Code, rec.Body.String())
	}

	ada := loginToken(t, srv, "ada@example.com", "password-long-1")
	for name, ids := range map[string][]string{
		"creator":    {creator.ID},
		"unverified": {plain.ID},
		"outsider":   {outsider.ID},
		"unknown":    {"usr_missing"},
	} {
		if rec := putJSON(t, srv, path, ada, map[string]any{"userIds": ids}); rec.Code != 422 {
			t.Fatalf("%s = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec := putJSON(t, srv, path, ada, map[string]any{"userIds": []string{grace.ID, grace.ID}}); rec.Code != 200 {
		t.Fatalf("creator = %d %s", rec.Code, rec.Body.String())
	}
	intake := getAuth(t, srv, "/api/projects/proj_default/intake", ada)
	body := intake.Body.String()
	if intake.Code != 200 || !strings.Contains(body, `"githubLogin":"grace"`) || !strings.Contains(body, `"canEditReviewers":true`) || !strings.Contains(body, `"reviewerOptions"`) {
		t.Fatalf("intake = %d %s", intake.Code, body)
	}
	if strings.Contains(body, "outsider") {
		t.Fatalf("reviewerOptions lists a non-member: %s", body)
	}
	if eveIntake := getAuth(t, srv, "/api/projects/proj_default/intake", eve); !strings.Contains(eveIntake.Body.String(), `"canEditReviewers":false`) {
		t.Fatalf("eve intake = %s", eveIntake.Body.String())
	}

	leadToken := loginToken(t, srv, "lead@example.com", "password-long-3")
	if rec := putJSON(t, srv, path, leadToken, map[string]any{"userIds": []string{}}); rec.Code != 200 {
		t.Fatalf("lead = %d %s", rec.Code, rec.Body.String())
	}
	var n int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM requirement_reviewers WHERE project_id = 'proj_default'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("reviewers = %d after the lead cleared them", n)
	}
}

func TestOwnerCannotApprove(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	owner := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	other := seedUser(t, srv, "lead2@example.com", "password-long-2", "lead_developer", false)
	for _, user := range []app.User{owner, other} {
		if err := identity.SaveGitHub(ctx, srv.DB, user.ID, "github.com", strings.Split(user.Email, "@")[0], nil, false, srv.TokenKey); err != nil {
			t.Fatal(err)
		}
		if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	card := reviewCard(t, srv, "proj_default")
	if _, err := srv.DB.Exec(`UPDATE cards SET owner_id = $2 WHERE id = $1`, card.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/proj_default/cards/" + card.ID

	ownerToken := loginToken(t, srv, "lead@example.com", "password-long-1")
	view := getAuth(t, srv, path, ownerToken)
	if view.Code != 200 || strings.Contains(view.Body.String(), `"approve"`) || !strings.Contains(view.Body.String(), `"owner":{`) {
		t.Fatalf("owner view = %d %s", view.Code, view.Body.String())
	}
	denied := postJSON(t, srv, path+"/approve", ownerToken, map[string]string{"sha": headSHA})
	if denied.Code != 403 {
		t.Fatalf("owner approve = %d %s", denied.Code, denied.Body.String())
	}

	otherToken := loginToken(t, srv, "lead2@example.com", "password-long-2")
	if view := getAuth(t, srv, path, otherToken); !strings.Contains(view.Body.String(), `"approve"`) {
		t.Fatalf("other lead view = %s", view.Body.String())
	}
	approved := postJSON(t, srv, path+"/approve", otherToken, map[string]string{"sha": headSHA})
	if approved.Code != 200 || !strings.Contains(approved.Body.String(), `"status":"approved"`) {
		t.Fatalf("other approve = %d %s", approved.Code, approved.Body.String())
	}
}

func verifiedMember(t *testing.T, srv *httpapi.Server, admin app.User, email, login string, id int64) app.User {
	t.Helper()
	ctx := context.Background()
	user := seedUser(t, srv, email, "password-long-9", "developer", false)
	token := login + "-token"
	if err := identity.SaveGitHub(ctx, srv.DB, user.ID, "github.com", login, &token, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if err := identity.VerifyGitHub(ctx, srv.DB, user.ID, login, id); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", user.ID); err != nil {
		t.Fatal(err)
	}
	return user
}
