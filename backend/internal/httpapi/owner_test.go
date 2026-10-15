package httpapi_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
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
		"creator":  {creator.ID},
		"outsider": {outsider.ID},
		"unknown":  {"usr_missing"},
	} {
		if rec := putJSON(t, srv, path, ada, map[string]any{"userIds": ids}); rec.Code != 422 {
			t.Fatalf("%s = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec := putJSON(t, srv, path, ada, map[string]any{"userIds": []string{plain.ID}}); rec.Code != 200 {
		t.Fatalf("unverified member = %d %s", rec.Code, rec.Body.String())
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

func TestReviewerOptionsUnverified(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	creator := seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	plain := seedUser(t, srv, "plain@example.com", "password-long-2", "developer", false)
	gone := seedUser(t, srv, "gone@example.com", "password-long-3", "developer", false)
	outsider := seedUser(t, srv, "out@example.com", "password-long-4", "developer", false)
	grace := verifiedMember(t, srv, admin, "grace@example.com", "grace", 21)
	for _, user := range []app.User{creator, plain, gone} {
		if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.DB.Exec(`UPDATE users SET disabled = true WHERE id = $1`, gone.ID); err != nil {
		t.Fatal(err)
	}
	rec := getAuth(t, srv, "/api/projects/proj_default/intake", loginToken(t, srv, "ada@example.com", "password-long-1"))
	if rec.Code != 200 {
		t.Fatalf("intake = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ReviewerOptions []struct {
			ID          string `json:"id"`
			GitHubLogin string `json:"githubLogin"`
		} `json:"reviewerOptions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	logins := map[string]string{}
	for _, option := range body.ReviewerOptions {
		logins[option.ID] = option.GitHubLogin
	}
	if login, ok := logins[plain.ID]; !ok || login != "" {
		t.Fatalf("unverified member option = %q %v in %s", login, ok, rec.Body.String())
	}
	if logins[grace.ID] != "grace" {
		t.Fatalf("verified member option = %q", logins[grace.ID])
	}
	for name, id := range map[string]string{"disabled": gone.ID, "outsider": outsider.ID} {
		if _, ok := logins[id]; ok {
			t.Fatalf("reviewerOptions lists the %s user: %s", name, rec.Body.String())
		}
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

func TestSoloOwnerApproves(t *testing.T) {
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	owner := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	if err := identity.SaveGitHub(ctx, srv.DB, owner.ID, "github.com", "acme-bot", nil, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE workspace_github SET bot_login = 'acme-bot' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	card := reviewCard(t, srv, "proj_default")
	if _, err := srv.DB.Exec(`UPDATE cards SET owner_id = $2 WHERE id = $1`, card.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/proj_default/cards/" + card.ID
	ownerToken := loginToken(t, srv, "lead@example.com", "password-long-1")
	if denied := postJSON(t, srv, path+"/approve", ownerToken, map[string]string{"sha": headSHA}); denied.Code != 403 {
		t.Fatalf("approve with solo off = %d %s", denied.Code, denied.Body.String())
	}

	adminToken := loginToken(t, srv, "admin@example.com", "password-long-0")
	saved := putJSON(t, srv, "/api/projects/proj_default/settings/workflow", adminToken, map[string]any{"soloApproval": true})
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"soloApproval":true`) {
		t.Fatalf("save solo = %d %s", saved.Code, saved.Body.String())
	}
	if view := getAuth(t, srv, path, ownerToken); !strings.Contains(view.Body.String(), `"approve"`) {
		t.Fatalf("owner view with solo on = %s", view.Body.String())
	}
	approved := postJSON(t, srv, path+"/approve", ownerToken, map[string]string{"sha": headSHA})
	if approved.Code != 200 || !strings.Contains(approved.Body.String(), `"status":"approved"`) {
		t.Fatalf("solo approve = %d %s", approved.Code, approved.Body.String())
	}
	var approver string
	var independent bool
	if err := srv.DB.QueryRow(`SELECT approver, independent FROM approvals WHERE card_id = $1`, card.ID).Scan(&approver, &independent); err != nil {
		t.Fatal(err)
	}
	if approver != "acme-bot" || independent {
		t.Fatalf("approval = %q independent=%v", approver, independent)
	}
}

func TestSelfReviewNeedsLead(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	dev := seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	lead := seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	for _, user := range []app.User{admin, dev, lead} {
		if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	add := "/api/projects/proj_default/requirements"
	draft := func(id string, self bool) map[string]any {
		return map[string]any{"id": id, "statement": "Export the report", "acceptance": []string{"A CSV downloads"}, "selfReview": self}
	}
	stored := func(id string) bool {
		t.Helper()
		var on bool
		if err := srv.DB.QueryRow(`SELECT self_review FROM requirements WHERE project_id = 'proj_default' AND id = $1`, id).Scan(&on); err != nil {
			t.Fatal(err)
		}
		return on
	}

	ada := loginToken(t, srv, "ada@example.com", "password-long-1")
	if rec := postJSON(t, srv, add, ada, draft("REQ-001", true)); rec.Code != 403 {
		t.Fatalf("developer with self review = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, add, ada, draft("REQ-001", false)); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"canSetSelfReview":false`) {
		t.Fatalf("developer add = %d %s", rec.Code, rec.Body.String())
	}
	self := add + "/REQ-001/self-review"
	if rec := putJSON(t, srv, self, ada, map[string]any{"selfReview": true}); rec.Code != 403 || stored("REQ-001") {
		t.Fatalf("creator toggle = %d %s", rec.Code, rec.Body.String())
	}

	leadToken := loginToken(t, srv, "lead@example.com", "password-long-2")
	if rec := postJSON(t, srv, add, leadToken, draft("REQ-002", true)); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"canSetSelfReview":true`) {
		t.Fatalf("lead add = %d %s", rec.Code, rec.Body.String())
	}
	if !stored("REQ-002") {
		t.Fatal("lead's Self review was not stored")
	}

	adminToken := loginToken(t, srv, "admin@example.com", "password-long-0")
	if rec := putJSON(t, srv, self, adminToken, map[string]any{"selfReview": true}); rec.Code != 200 || !stored("REQ-001") {
		t.Fatalf("admin on = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, self, adminToken, map[string]any{"selfReview": false}); rec.Code != 200 || stored("REQ-001") {
		t.Fatalf("admin off = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, add+"/REQ-404/self-review", adminToken, map[string]any{"selfReview": true}); rec.Code != 404 {
		t.Fatalf("unknown requirement = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAIReviewModeNeedsLead(t *testing.T) {
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	dev := seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	lead := seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	for _, user := range []app.User{admin, dev, lead} {
		if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	add := "/api/projects/proj_default/requirements"
	draft := func(id, mode string) map[string]any {
		return map[string]any{"id": id, "statement": "Export the report", "acceptance": []string{"A CSV downloads"}, "aiReviewMode": mode}
	}
	stored := func(id string) string {
		t.Helper()
		var mode string
		if err := srv.DB.QueryRow(`SELECT ai_review_mode FROM requirements WHERE project_id = 'proj_default' AND id = $1`, id).Scan(&mode); err != nil {
			t.Fatal(err)
		}
		return mode
	}

	ada := loginToken(t, srv, "ada@example.com", "password-long-1")
	if rec := postJSON(t, srv, add, ada, draft("REQ-001", "manual")); rec.Code != 403 {
		t.Fatalf("developer asking for manual = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, add, ada, map[string]any{"id": "REQ-001", "statement": "Export the report"}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"aiReviewMode":"continue"`) {
		t.Fatalf("developer add = %d %s", rec.Code, rec.Body.String())
	}
	mode := add + "/REQ-001/ai-review-mode"
	if rec := putJSON(t, srv, mode, ada, map[string]any{"aiReviewMode": "manual"}); rec.Code != 403 || stored("REQ-001") != "continue" {
		t.Fatalf("developer change = %d %s", rec.Code, rec.Body.String())
	}

	leadToken := loginToken(t, srv, "lead@example.com", "password-long-2")
	if rec := postJSON(t, srv, add, leadToken, draft("REQ-002", "sometimes")); rec.Code != 422 || !strings.Contains(rec.Body.String(), "aiReviewMode") {
		t.Fatalf("unknown mode on add = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, mode, leadToken, map[string]any{"aiReviewMode": "sometimes"}); rec.Code != 422 || stored("REQ-001") != "continue" {
		t.Fatalf("unknown mode = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, add, leadToken, draft("REQ-002", "manual")); rec.Code != 200 || stored("REQ-002") != "manual" {
		t.Fatalf("lead add = %d %s", rec.Code, rec.Body.String())
	}
	rec := putJSON(t, srv, mode, leadToken, map[string]any{"aiReviewMode": "manual"})
	if rec.Code != 200 || stored("REQ-001") != "manual" || strings.Contains(rec.Body.String(), `"aiReviewMode":"continue"`) {
		t.Fatalf("lead change = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, add+"/REQ-404/ai-review-mode", leadToken, map[string]any{"aiReviewMode": "manual"}); rec.Code != 404 {
		t.Fatalf("unknown requirement = %d %s", rec.Code, rec.Body.String())
	}

	card := reviewCard(t, srv, "proj_default")
	path := "/api/projects/proj_default/cards/" + card.ID
	if view := getAuth(t, srv, path, leadToken); !strings.Contains(view.Body.String(), `"aiReviewMode":"continue"`) {
		t.Fatalf("card without requirements = %s", view.Body.String())
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET requirement_ids_json = '["REQ-001"]' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	if view := getAuth(t, srv, path, leadToken); !strings.Contains(view.Body.String(), `"aiReviewMode":"manual"`) {
		t.Fatalf("card with a manual requirement = %s", view.Body.String())
	}
}

func TestSelfReviewOwnerApproves(t *testing.T) {
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	owner := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	if err := identity.SaveGitHub(ctx, srv.DB, owner.ID, "github.com", "lead", nil, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", owner.ID); err != nil {
		t.Fatal(err)
	}
	for id, self := range map[string]bool{"REQ-001": true, "REQ-002": false} {
		if _, err := srv.DB.Exec(`
			INSERT INTO requirements (
				id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
				content_hash, project_id, created_by, self_review
			) VALUES ($1, 'prd', 'docs/prd/a.md', $1, 'statement', '[]', '[]', $1, 'proj_default', $2, $3)`,
			id, owner.ID, self); err != nil {
			t.Fatal(err)
		}
	}
	card := reviewCard(t, srv, "proj_default")
	if _, err := srv.DB.Exec(`UPDATE cards SET owner_id = $2, requirement_ids_json = '["REQ-001","REQ-002"]' WHERE id = $1`, card.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/proj_default/cards/" + card.ID
	ownerToken := loginToken(t, srv, "lead@example.com", "password-long-1")
	if view := getAuth(t, srv, path, ownerToken); strings.Contains(view.Body.String(), `"approve"`) || !strings.Contains(view.Body.String(), `"selfReview":false`) {
		t.Fatalf("owner view with one unflagged requirement = %s", view.Body.String())
	}
	if denied := postJSON(t, srv, path+"/approve", ownerToken, map[string]string{"sha": headSHA}); denied.Code != 403 {
		t.Fatalf("approve with one unflagged requirement = %d %s", denied.Code, denied.Body.String())
	}

	if _, err := srv.DB.Exec(`UPDATE cards SET requirement_ids_json = '["REQ-001"]' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	if view := getAuth(t, srv, path, ownerToken); !strings.Contains(view.Body.String(), `"approve"`) || !strings.Contains(view.Body.String(), `"selfReview":true`) {
		t.Fatalf("owner view with self review = %s", view.Body.String())
	}
	approved := postJSON(t, srv, path+"/approve", ownerToken, map[string]string{"sha": headSHA})
	if approved.Code != 200 || !strings.Contains(approved.Body.String(), `"status":"approved"`) {
		t.Fatalf("self review approve = %d %s", approved.Code, approved.Body.String())
	}
	var approver string
	var independent bool
	if err := srv.DB.QueryRow(`SELECT approver, independent FROM approvals WHERE card_id = $1`, card.ID).Scan(&approver, &independent); err != nil {
		t.Fatal(err)
	}
	if approver != "lead" || independent {
		t.Fatalf("approval = %q independent=%v", approver, independent)
	}
}

func TestCardReviewers(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	adminToken := loginToken(t, srv, "admin@example.com", "password-long-0")
	owner := verifiedMember(t, srv, admin, "ada@example.com", "ada", 31)
	grace := verifiedMember(t, srv, admin, "grace@example.com", "grace", 32)
	lin := seedUser(t, srv, "lin@example.com", "password-long-8", "developer", false)
	if err := app.AddMember(context.Background(), srv.DB, admin, "tribe_default", lin.ID); err != nil {
		t.Fatal(err)
	}
	gadget := newProject(t, srv, adminToken, "tribe_default", "Gadget", "gadget")
	for _, project := range []string{"proj_default", gadget} {
		if _, err := srv.DB.Exec(`
			INSERT INTO requirements (
				id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
				content_hash, project_id, created_by
			) VALUES ('REQ-001', 'prd', 'docs/prd/a.md', 'Export', 'statement', '[]', '[]', 'h1', $1, $2)`,
			project, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, reviewer := range []string{grace.ID, lin.ID, owner.ID} {
		if _, err := srv.DB.Exec(`
			INSERT INTO requirement_reviewers (project_id, requirement_id, user_id, added_by, created_at)
			VALUES ('proj_default', 'REQ-001', $1, $2, 'now')`, reviewer, admin.ID); err != nil {
			t.Fatal(err)
		}
	}
	reviewed := reviewerCard(t, srv, "proj_default", owner.ID)
	elsewhere := reviewerCard(t, srv, gadget, owner.ID)
	token := loginToken(t, srv, "ada@example.com", "password-long-9")

	var list struct {
		Cards []struct {
			ID        string `json:"id"`
			Reviewers []struct {
				ID          string `json:"id"`
				Email       string `json:"email"`
				GitHubLogin string `json:"githubLogin"`
			} `json:"reviewers"`
		} `json:"cards"`
	}
	rec := getAuth(t, srv, "/api/projects/proj_default/cards", token)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Cards) != 1 || list.Cards[0].ID != reviewed {
		t.Fatalf("cards = %d %s", rec.Code, rec.Body.String())
	}
	got := list.Cards[0].Reviewers
	if len(got) != 2 || got[0].Email != "grace@example.com" || got[0].GitHubLogin != "grace" || got[1].Email != "lin@example.com" || got[1].GitHubLogin != "" {
		t.Fatalf("reviewers = %+v", got)
	}
	one := getAuth(t, srv, "/api/projects/proj_default/cards/"+reviewed, token)
	var card struct {
		Reviewers []struct {
			ID string `json:"id"`
		} `json:"reviewers"`
	}
	if err := json.Unmarshal(one.Body.Bytes(), &card); err != nil || len(card.Reviewers) != 2 || card.Reviewers[0].ID != grace.ID || card.Reviewers[1].ID != lin.ID {
		t.Fatalf("card = %s", one.Body.String())
	}
	other := getAuth(t, srv, "/api/projects/"+gadget+"/cards/"+elsewhere, token)
	if !strings.Contains(other.Body.String(), `"reviewers":[]`) {
		t.Fatalf("reviewers leaked across projects: %s", other.Body.String())
	}
}

func reviewerCard(t *testing.T, srv *httpapi.Server, project, ownerID string) string {
	t.Helper()
	card, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: "backlog", RequirementIDs: []string{"REQ-001"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET project_id = $2, owner_id = $3 WHERE id = $1`, card.ID, project, ownerID); err != nil {
		t.Fatal(err)
	}
	return card.ID
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
