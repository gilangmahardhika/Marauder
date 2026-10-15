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
	"orchestrator/backend/internal/settings"
)

const (
	mergeSHA  = "fedcba9876543210fedcba9876543210fedcba98"
	leadToken = "lead-user-token-value"
)

// mergeGH records gh calls like ghWrites and answers with out and exit.
type mergeGH struct {
	ghWrites
	out  string
	exit int
}

func (m *mergeGH) run(ctx context.Context, child execx.Child) (execx.Result, error) {
	_, _ = m.ghWrites.run(ctx, child)
	return execx.Result{Exit: m.exit, Out: []byte(m.out)}, nil
}

// mergeServer has an approved card with a pull request, and a lead whose
// GitHub token is verified when withToken is set.
func mergeServer(t *testing.T, withToken bool) (*httpapi.Server, *mergeGH, board.Card, string) {
	t.Helper()
	srv, _, card, token := approvalServer(t)
	gh := &mergeGH{out: mergeSHA + "\n"}
	srv.Tools = app.Tools{GH: "/usr/bin/gh", Exec: gh.run}
	if _, err := srv.DB.Exec(`UPDATE cards SET status = 'approved', approved_sha = head_sha WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	if withToken {
		lead := emailUserID(t, srv, "lead@example.com")
		value := leadToken
		ctx := context.Background()
		if err := identity.SaveGitHub(ctx, srv.DB, lead, "github.com", "lead", &value, false, srv.TokenKey); err != nil {
			t.Fatal(err)
		}
		if err := identity.VerifyGitHub(ctx, srv.DB, lead, "lead", 42); err != nil {
			t.Fatal(err)
		}
	}
	return srv, gh, card, token
}

func setDeploy(t *testing.T, srv *httpapi.Server, on, solo bool) {
	t.Helper()
	d := settings.Deploy{DeployEnabled: &on}
	if _, _, err := settings.SaveProjectDeploy(srv.Data, "proj_default", settings.ProjectPaths{Data: srv.Data}, d, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := settings.SaveProjectWorkflow(srv.Data, "proj_default", nil, &solo); err != nil {
		t.Fatal(err)
	}
}

func emailUserID(t *testing.T, srv *httpapi.Server, email string) string {
	t.Helper()
	var id string
	if err := srv.DB.QueryRow(`SELECT id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func mergeActions(t *testing.T, srv *httpapi.Server, cardID, token string) string {
	t.Helper()
	rec := getAuth(t, srv, "/api/projects/proj_default/cards/"+cardID, token)
	if rec.Code != 200 {
		t.Fatalf("card = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Actions []string `json:"actions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return " " + strings.Join(body.Actions, " ") + " "
}

func storedStatus(t *testing.T, srv *httpapi.Server, cardID string) string {
	t.Helper()
	var status string
	if err := srv.DB.QueryRow(`SELECT status FROM cards WHERE id = $1`, cardID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestActionsDeployOff(t *testing.T) {
	srv, _, card, lead := mergeServer(t, true)
	ctx := context.Background()
	leadUser := app.User{ID: emailUserID(t, srv, "lead@example.com"), Role: "lead_developer", Admin: true}
	qa := seedUser(t, srv, "qa@example.com", "password-long-2", "qa_lead", false)
	owner := seedUser(t, srv, "owner@example.com", "password-long-3", "developer", false)
	for _, u := range []app.User{qa, owner} {
		if err := app.AddMember(ctx, srv.DB, leadUser, "tribe_default", u.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET owner_id = $2 WHERE id = $1`, card.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	qaToken := loginToken(t, srv, "qa@example.com", "password-long-2")
	ownerToken := loginToken(t, srv, "owner@example.com", "password-long-3")

	if got := mergeActions(t, srv, card.ID, lead); !strings.Contains(got, " deploy ") || strings.Contains(got, " merge ") {
		t.Fatalf("deploy on, lead actions =%s", got)
	}
	setDeploy(t, srv, false, false)
	if got := mergeActions(t, srv, card.ID, lead); strings.Contains(got, " deploy ") || !strings.Contains(got, " merge ") {
		t.Fatalf("deploy off, lead actions =%s", got)
	}
	if got := mergeActions(t, srv, card.ID, qaToken); strings.Contains(got, " deploy ") || strings.Contains(got, " merge ") {
		t.Fatalf("deploy off, QA lead actions =%s", got)
	}
	if got := mergeActions(t, srv, card.ID, ownerToken); strings.Contains(got, " merge ") {
		t.Fatalf("owner without solo approval actions =%s", got)
	}
	setDeploy(t, srv, false, true)
	if got := mergeActions(t, srv, card.ID, ownerToken); !strings.Contains(got, " merge ") {
		t.Fatalf("owner with solo approval actions =%s", got)
	}
}

func TestSquashMerge(t *testing.T) {
	srv, gh, card, lead := mergeServer(t, true)
	setDeploy(t, srv, false, false)
	rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/merge", lead, map[string]string{})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"done"`) {
		t.Fatalf("merge = %d %s", rec.Code, rec.Body.String())
	}
	posts := gh.all()
	if len(posts) != 1 {
		t.Fatalf("gh calls = %+v", posts)
	}
	put := posts[0]
	argv := strings.Join(put.argv, " ")
	if put.path != "repos/acme/widget/pulls/7/merge" || !strings.Contains(argv, "--method PUT") ||
		!strings.Contains(put.input, `"merge_method":"squash"`) || !strings.Contains(put.input, `"sha":"`+headSHA+`"`) {
		t.Fatalf("merge call = %+v", put)
	}
	if !contains(put.env, "GH_TOKEN="+leadToken) || strings.Contains(argv, leadToken) {
		t.Fatalf("token handling: env has user token = %v, argv = %s", contains(put.env, "GH_TOKEN="+leadToken), argv)
	}
	var payload string
	if err := srv.DB.QueryRow(`SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'merge'`, card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, mergeSHA) || !strings.Contains(payload, `"login":"lead"`) || strings.Contains(payload, leadToken) {
		t.Fatalf("merge event = %s", payload)
	}
}

func TestSquashMergeNoToken(t *testing.T) {
	srv, gh, card, lead := mergeServer(t, false)
	setDeploy(t, srv, false, false)
	rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/merge", lead, map[string]string{})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "connect your GitHub account") {
		t.Fatalf("merge = %d %s", rec.Code, rec.Body.String())
	}
	if status := storedStatus(t, srv, card.ID); status != "approved" {
		t.Fatalf("status = %s, want approved", status)
	}
	if calls := gh.all(); len(calls) != 0 {
		t.Fatalf("gh calls = %+v", calls)
	}
}

func TestSquashMergeRefused(t *testing.T) {
	srv, gh, card, lead := mergeServer(t, true)
	setDeploy(t, srv, false, false)
	gh.exit, gh.out = 1, "gh: Pull Request is not mergeable (HTTP 405)\n"
	rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/merge", lead, map[string]string{})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "not mergeable") || strings.Contains(rec.Body.String(), leadToken) {
		t.Fatalf("merge = %d %s", rec.Code, rec.Body.String())
	}
	if status := storedStatus(t, srv, card.ID); status != "approved" {
		t.Fatalf("status = %s, want approved", status)
	}
}

func TestDeployRefusedWhenOff(t *testing.T) {
	srv, gh, card, lead := mergeServer(t, true)
	path := "/api/projects/proj_default/cards/" + card.ID
	if rec := postJSON(t, srv, path+"/merge", lead, map[string]string{}); rec.Code != 422 {
		t.Fatalf("merge with deploy on = %d %s", rec.Code, rec.Body.String())
	}
	setDeploy(t, srv, false, false)
	if rec := postJSON(t, srv, path+"/deploy", lead, map[string]string{}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "deploy is off") {
		t.Fatalf("deploy with deploy off = %d %s", rec.Code, rec.Body.String())
	}
	if status := storedStatus(t, srv, card.ID); status != "approved" {
		t.Fatalf("status = %s, want approved", status)
	}
	if calls := gh.all(); len(calls) != 0 {
		t.Fatalf("gh calls = %+v", calls)
	}
}
