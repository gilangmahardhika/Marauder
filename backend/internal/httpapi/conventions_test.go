package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/loop"
)

func TestConventionCatalogAdminOnly(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	admin := loginToken(t, srv, "admin@example.com", "password-long-0")
	lead := loginToken(t, srv, "lead@example.com", "password-long-1")

	got := getAuth(t, srv, "/api/conventions", lead)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"oauth2-integration"`) || !strings.Contains(got.Body.String(), `"stk_go"`) {
		t.Fatalf("lead read = %d %s", got.Code, got.Body.String())
	}

	rule := map[string]any{"scope": "stack", "stackId": "stk_go", "category": "naming", "title": "Go names", "body": "MixedCaps."}
	if rec := postJSON(t, srv, "/api/conventions/rules", lead, rule); rec.Code != 403 {
		t.Fatalf("lead create = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, "/api/conventions/rules/rule_naming", lead, rule); rec.Code != 403 {
		t.Fatalf("lead update = %d", rec.Code)
	}
	if rec := sendJSON(t, srv, "DELETE", "/api/conventions/skills/skl_oauth2", lead, nil); rec.Code != 403 {
		t.Fatalf("lead delete = %d", rec.Code)
	}

	created := postJSON(t, srv, "/api/conventions/rules", admin, rule)
	if created.Code != 201 {
		t.Fatalf("admin create = %d %s", created.Code, created.Body.String())
	}
	var body struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil || body.Version != 1 {
		t.Fatalf("created body: %s", created.Body.String())
	}
	rule["body"] = strings.Repeat("x", 8001)
	if rec := putJSON(t, srv, "/api/conventions/rules/"+body.ID, admin, rule); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"body"`) {
		t.Fatalf("long body = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, "/api/conventions/rules/rule_missing", admin, map[string]any{
		"scope": "global", "category": "style", "title": "t", "body": "b",
	}); rec.Code != 404 {
		t.Fatalf("missing = %d", rec.Code)
	}
	if rec := sendJSON(t, srv, "DELETE", "/api/conventions/rules/"+body.ID, admin, nil); rec.Code != 204 {
		t.Fatalf("admin delete = %d", rec.Code)
	}

	var payload string
	if err := srv.DB.QueryRow(`
		SELECT payload_json FROM events WHERE kind = 'config_changed' ORDER BY ts, id LIMIT 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"section":"conventions"`) || strings.Contains(payload, "MixedCaps") {
		t.Fatalf("config_changed = %s", payload)
	}
}

func TestProjectConventionsLeadOnly(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	dev := seedUser(t, srv, "dev@example.com", "password-long-2", "developer", false)
	for _, user := range []app.User{lead, dev} {
		if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	leadToken := loginToken(t, srv, "lead@example.com", "password-long-1")
	devToken := loginToken(t, srv, "dev@example.com", "password-long-2")
	path := "/api/projects/proj_default/settings/conventions"

	saved := putJSON(t, srv, path, leadToken, map[string]any{"manualStackIds": []string{"stk_django"}, "skillIds": []string{"skl_oauth2"}})
	if saved.Code != 200 {
		t.Fatalf("lead save = %d %s", saved.Code, saved.Body.String())
	}
	if !strings.Contains(saved.Body.String(), `{"id":"stk_django","name":"Django","source":"manual"}`) ||
		!strings.Contains(saved.Body.String(), `"skillIds":["skl_oauth2"]`) {
		t.Fatalf("lead save body = %s", saved.Body.String())
	}
	if rec := putJSON(t, srv, path, devToken, map[string]any{"manualStackIds": []string{}, "skillIds": []string{}}); rec.Code != 403 {
		t.Fatalf("developer save = %d", rec.Code)
	}
	if rec := getAuth(t, srv, path, devToken); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"rule_naming"`) {
		t.Fatalf("developer read = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, path, leadToken, map[string]any{"manualStackIds": []string{"stk_nope"}}); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"manualStackIds"`) {
		t.Fatalf("unknown stack = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := srv.DB.Exec(`UPDATE skills SET disabled = true WHERE id = 'skl_oauth2'`); err != nil {
		t.Fatal(err)
	}
	if rec := putJSON(t, srv, path, leadToken, map[string]any{"skillIds": []string{"skl_oauth2"}}); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"skillIds"`) {
		t.Fatalf("disabled skill = %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreateCardSkills(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", lead.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`
		INSERT INTO requirements (
			id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
			content_hash, accepted_at, accepted_by, project_id, created_by
		) VALUES ('REQ-001', 'prd', 'docs/prd/a.md', 'REQ-001', 'Sign in with Google', '[]', '[]', 'h',
			'2026-01-01T00:00:00Z', 'ada', 'proj_default', $1)`, lead.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`
		INSERT INTO skills (id, slug, title, use_when, body, content_hash, created_at, updated_at)
		VALUES ('skl_other', 'payments', 'Payments', 'u', 'b', 'h', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	leadToken := loginToken(t, srv, "lead@example.com", "password-long-1")
	if rec := putJSON(t, srv, "/api/projects/proj_default/settings/conventions", leadToken, map[string]any{"skillIds": []string{"skl_oauth2"}}); rec.Code != 200 {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body.String())
	}

	_, err := app.CreateCard(ctx, loop.CreateInput{
		DB: srv.DB, ProjectID: "proj_default",
		Args: map[string]any{"title": "Payments", "requirementIds": []any{"REQ-001"}, "skillIds": []any{"skl_other"}},
	})
	if !errors.Is(err, loop.ErrSkill) {
		t.Fatalf("not enabled: %v", err)
	}
	var n int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM cards`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cards after rejected skill = %d %v", n, err)
	}

	card, err := app.CreateCard(ctx, loop.CreateInput{
		DB: srv.DB, ProjectID: "proj_default",
		Args: map[string]any{"title": "Google sign-in", "requirementIds": []any{"REQ-001"}, "skillIds": []any{"skl_oauth2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.QueryRow(`SELECT count(*) FROM card_skills WHERE card_id = $1 AND skill_id = 'skl_oauth2'`, card.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("card_skills = %d %v", n, err)
	}
	view := getAuth(t, srv, "/api/projects/proj_default/cards/"+card.ID, leadToken)
	if view.Code != 200 || !strings.Contains(view.Body.String(), `"skills":[{"id":"skl_oauth2","slug":"oauth2-integration","title":"OAuth 2.0 integration","useWhen":"A requirement adds sign-in`) {
		t.Fatalf("card json = %d %s", view.Code, view.Body.String())
	}
}
