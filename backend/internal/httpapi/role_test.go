package httpapi_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/identity"
)

const headSHA = "0123456789abcdef0123456789abcdef01234567"

func TestLeadDeploy(t *testing.T) {
	srv := accountServer(t)
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", true)
	if err := identity.SaveGitHub(context.Background(), srv.DB, lead.ID, "github.com", "ada", nil, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMember(context.Background(), srv.DB, lead, "tribe_default", lead.ID); err != nil {
		t.Fatal(err)
	}
	card := reviewCard(t, srv, "proj_default")
	token := loginToken(t, srv, "lead@example.com", "password-long-1")
	approved := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/approve", token, map[string]string{"sha": headSHA})
	if approved.Code != 200 || !strings.Contains(approved.Body.String(), `"status":"approved"`) {
		t.Fatalf("approve = %d %s", approved.Code, approved.Body.String())
	}
	deployed := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/deploy", token, map[string]string{})
	if deployed.Code != 200 || !strings.Contains(deployed.Body.String(), "deploying_dev") {
		t.Fatalf("deploy = %d %s", deployed.Code, deployed.Body.String())
	}
}

func TestOwnerApproveStops(t *testing.T) {
	srv := accountServer(t)
	owner := seedUser(t, srv, "owner@example.com", "password-long-1", "product_owner", true)
	if err := identity.SaveGitHub(context.Background(), srv.DB, owner.ID, "github.com", "ada", nil, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMember(context.Background(), srv.DB, owner, "tribe_default", owner.ID); err != nil {
		t.Fatal(err)
	}
	card := reviewCard(t, srv, "proj_default")
	token := loginToken(t, srv, "owner@example.com", "password-long-1")
	approved := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/approve", token, map[string]string{"sha": headSHA})
	if approved.Code != 200 || !strings.Contains(approved.Body.String(), `"status":"approved"`) {
		t.Fatalf("approve = %d %s", approved.Code, approved.Body.String())
	}
	if strings.Contains(approved.Body.String(), "deploying_dev") {
		t.Fatal("approval started deploy")
	}
	denied := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/deploy", token, map[string]string{})
	if denied.Code != 403 {
		t.Fatalf("owner deploy = %d", denied.Code)
	}
}

func TestAcceptSeniorRoles(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-0", "product_owner", true)
	if _, err := srv.DB.Exec(`
		INSERT INTO requirements (
			id, project_id, source_kind, source_path, heading, statement,
			acceptance_json, non_goals_json, content_hash
		) VALUES ('REQ-001', 'proj_default', 'prd', 'docs/prd', 'REQ-001', 'text', '[]', '[]', 'hash')`); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		role string
		code int
	}{
		{"lead_developer", 200}, {"senior_developer", 200}, {"qa_lead", 200}, {"product_owner", 200},
		{"developer", 403}, {"qa", 403},
	}
	for _, c := range cases {
		email := c.role + "@example.com"
		user := seedUser(t, srv, email, "password-long-1", c.role, false)
		if err := app.AddMember(context.Background(), srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
		token := loginToken(t, srv, email, "password-long-1")
		rec := postJSON(t, srv, "/api/projects/proj_default/accept", token, map[string]any{"requirementIds": []string{"REQ-001"}})
		if rec.Code != c.code {
			t.Fatalf("%s accept = %d %s", c.role, rec.Code, rec.Body.String())
		}
		intake := getAuth(t, srv, "/api/projects/proj_default/intake", token)
		want := fmt.Sprintf(`"canAccept":%t`, c.code == 200)
		if !strings.Contains(intake.Body.String(), want) {
			t.Fatalf("%s intake lacks %s: %s", c.role, want, intake.Body.String())
		}
	}
}

func TestDeveloperRunOnly(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	dev := seedUser(t, srv, "dev@example.com", "password-long-2", "developer", false)
	if err := app.AddMember(context.Background(), srv.DB, admin, "tribe_default", dev.ID); err != nil {
		t.Fatal(err)
	}
	card := reviewCard(t, srv, "proj_default")
	token := loginToken(t, srv, "dev@example.com", "password-long-2")
	run := postJSON(t, srv, "/api/projects/proj_default/runs", token, map[string]string{})
	if run.Code != 200 || !strings.Contains(run.Body.String(), `"queued":0`) {
		t.Fatalf("run = %d %s", run.Code, run.Body.String())
	}
	denied := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/approve", token, map[string]string{"sha": headSHA})
	if denied.Code != 403 {
		t.Fatalf("approve = %d %s", denied.Code, denied.Body.String())
	}
	deploy := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/deploy", token, map[string]string{})
	if deploy.Code != 403 {
		t.Fatalf("deploy = %d", deploy.Code)
	}
}

func TestActorOnTransition(t *testing.T) {
	srv := accountServer(t)
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", true)
	if err := identity.SaveGitHub(context.Background(), srv.DB, lead.ID, "github.com", "ada", nil, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMember(context.Background(), srv.DB, lead, "tribe_default", lead.ID); err != nil {
		t.Fatal(err)
	}
	card := reviewCard(t, srv, "proj_default")
	token := loginToken(t, srv, "lead@example.com", "password-long-1")
	if rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/approve", token, map[string]string{"sha": headSHA}); rec.Code != 200 {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	var payload string
	if err := srv.DB.QueryRow(`SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'transition' ORDER BY ts DESC LIMIT 1`, card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, lead.ID) {
		t.Fatalf("payload = %s", payload)
	}
	var old string
	if err := srv.DB.QueryRow(`SELECT count(*) FROM events WHERE actor_id IS NULL`).Scan(&old); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyCardRoutesCheckRole(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	dev := seedUser(t, srv, "dev@example.com", "password-long-1", "developer", true)
	outsider := seedUser(t, srv, "out@example.com", "password-long-1", "lead_developer", true)
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", true)
	for _, u := range []identity.User{dev, lead} {
		if err := app.AddMember(ctx, srv.DB, u, "tribe_default", u.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range []identity.User{outsider, lead} {
		if err := identity.SaveGitHub(ctx, srv.DB, u.ID, "github.com", "ada", nil, false, srv.TokenKey); err != nil {
			t.Fatal(err)
		}
	}
	card := reviewCard(t, srv, "proj_default")
	path := "/api/cards/" + card.ID + "/approve"
	body := map[string]string{"sha": headSHA}
	if rec := postJSON(t, srv, path, loginToken(t, srv, "dev@example.com", "password-long-1"), body); rec.Code != 403 {
		t.Fatalf("developer approve = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, path, loginToken(t, srv, "out@example.com", "password-long-1"), body); rec.Code != 404 {
		t.Fatalf("non-member approve = %d %s", rec.Code, rec.Body.String())
	}
	if got := reload(t, srv, card.ID); got.Status != "human_review" {
		t.Fatalf("status after refused approvals = %s", got.Status)
	}
	if rec := postJSON(t, srv, path, loginToken(t, srv, "lead@example.com", "password-long-1"), body); rec.Code != 200 {
		t.Fatalf("lead approve = %d %s", rec.Code, rec.Body.String())
	}
}

func TestCardShowsLatestReview(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", true)
	if err := app.AddMember(ctx, srv.DB, lead, "tribe_default", lead.ID); err != nil {
		t.Fatal(err)
	}
	card := reviewCard(t, srv, "proj_default")
	token := loginToken(t, srv, "lead@example.com", "password-long-1")
	path := "/api/projects/proj_default/cards/" + card.ID
	if body := getAuth(t, srv, path, token).Body.String(); !strings.Contains(body, `"reviewedSha":null`) {
		t.Fatalf("unreviewed card = %s", body)
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET pr_url = 'https://github.com/o/r/pull/1' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	attempt := board.Attempt{CardID: card.ID, N: 1, TaskClass: "implement", ModelID: "m", MaxTurns: 1, BaseSHA: headSHA}
	if err := board.InsertAttempt(ctx, srv.DB, attempt); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := srv.DB.QueryRow(`SELECT id FROM attempts WHERE card_id = $1`, card.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := board.FinishAttempt(ctx, srv.DB, id, "checks_passed", headSHA); err != nil {
		t.Fatal(err)
	}
	if err := board.RecordReview(ctx, srv.DB, id, "reviewer", "hash", false, "recommend_approve", headSHA); err != nil {
		t.Fatal(err)
	}
	body := getAuth(t, srv, path, token).Body.String()
	if !strings.Contains(body, `"reviewedSha":"`+headSHA+`"`) || !strings.Contains(body, `"reviewIndependent":false`) ||
		!strings.Contains(body, `"reviewVerdict":"recommend_approve"`) {
		t.Fatalf("reviewed card = %s", body)
	}
	if err := board.RecordReview(ctx, srv.DB, id, "", "", false, "failed", headSHA); err != nil {
		t.Fatal(err)
	}
	body = getAuth(t, srv, path, token).Body.String()
	if !strings.Contains(body, `"reviewVerdict":"failed"`) || !strings.Contains(body, `"reviewIndependent":null`) {
		t.Fatalf("failed review card = %s", body)
	}
}

func reload(t *testing.T, srv *httpapi.Server, id string) board.Card {
	t.Helper()
	card, err := board.GetCard(context.Background(), srv.DB, id)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func reviewCard(t *testing.T, srv *httpapi.Server, project string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: "human_review"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET project_id = $2, head_sha = $3 WHERE id = $1`, card.ID, project, headSHA); err != nil {
		t.Fatal(err)
	}
	card.HeadSHA = headSHA
	card.Status = "human_review"
	return card
}
