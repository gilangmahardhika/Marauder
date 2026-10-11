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
