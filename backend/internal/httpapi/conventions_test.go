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

	choices := func(stacksOn, skillsOn, rulesOff []string) map[string]any {
		return map[string]any{
			"stacks": map[string]any{"on": stacksOn, "off": []string{}},
			"skills": map[string]any{"on": skillsOn, "off": []string{}},
			"rules":  map[string]any{"on": []string{}, "off": rulesOff},
		}
	}

	saved := putJSON(t, srv, path, leadToken, choices([]string{"stk_django"}, []string{"skl_oauth2"}, []string{"rule_naming"}))
	if saved.Code != 200 {
		t.Fatalf("lead save = %d %s", saved.Code, saved.Body.String())
	}
	var view struct {
		Choices struct {
			Rules struct {
				Off []string `json:"off"`
			} `json:"rules"`
		} `json:"choices"`
		Stacks []struct{ ID, Origin string } `json:"stacks"`
		Skills []struct{ ID, Origin string } `json:"skills"`
		Rules  []struct{ ID string }         `json:"rules"`
	}
	if err := json.Unmarshal(saved.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Stacks) != 1 || view.Stacks[0].ID != "stk_django" || view.Stacks[0].Origin != "project" ||
		len(view.Skills) != 1 || view.Skills[0].Origin != "project" || strings.Join(view.Choices.Rules.Off, ",") != "rule_naming" {
		t.Fatalf("lead save body = %s", saved.Body.String())
	}
	for _, r := range view.Rules {
		if r.ID == "rule_naming" {
			t.Fatalf("rule switched off still applies: %s", saved.Body.String())
		}
	}
	if rec := putJSON(t, srv, path, devToken, choices(nil, nil, nil)); rec.Code != 403 {
		t.Fatalf("developer save = %d", rec.Code)
	}
	if rec := getAuth(t, srv, path, devToken); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"rule_database"`) {
		t.Fatalf("developer read = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, path, leadToken, choices([]string{"stk_nope"}, nil, nil)); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"stacks.on"`) {
		t.Fatalf("unknown stack = %d %s", rec.Code, rec.Body.String())
	}
	both := map[string]any{"rules": map[string]any{"on": []string{"rule_naming"}, "off": []string{"rule_naming"}}}
	if rec := putJSON(t, srv, path, leadToken, both); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"rules.off"`) {
		t.Fatalf("both lists = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := srv.DB.Exec(`UPDATE skills SET disabled = true WHERE id = 'skl_oauth2'`); err != nil {
		t.Fatal(err)
	}
	if rec := putJSON(t, srv, path, leadToken, choices(nil, []string{"skl_oauth2"}, nil)); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"skills.on"`) {
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
	if rec := putJSON(t, srv, "/api/projects/proj_default/settings/conventions", leadToken, map[string]any{"skills": map[string]any{"on": []string{"skl_oauth2"}}}); rec.Code != 200 {
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

func TestTribeConventionsSave(t *testing.T) {
	f := newTribeFixture(t)
	path := "/api/tribes/" + f.tribe + "/conventions"
	memberToken := loginToken(t, f.srv, "member@example.com", "password-long-3")
	outsiderToken := loginToken(t, f.srv, "out@example.com", "password-long-4")
	body := map[string]any{"stackIds": []string{"stk_go"}, "skillIds": []string{"skl_oauth2"}, "rulesOff": []string{"rule_naming"}}

	read := getAuth(t, f.srv, path, memberToken)
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"canEdit":false`) || !strings.Contains(read.Body.String(), `"stackIds":[]`) {
		t.Fatalf("member read = %d %s", read.Code, read.Body.String())
	}
	if rec := putJSON(t, f.srv, path, memberToken, body); rec.Code != 403 {
		t.Fatalf("member save = %d", rec.Code)
	}
	if rec := getAuth(t, f.srv, path, outsiderToken); rec.Code != 403 {
		t.Fatalf("outsider read = %d", rec.Code)
	}
	if rec := getAuth(t, f.srv, "/api/tribes/"+f.other+"/conventions", f.chiefToken); rec.Code != 403 {
		t.Fatalf("other tribe read = %d", rec.Code)
	}
	if rec := putJSON(t, f.srv, "/api/tribes/"+f.other+"/conventions", f.chiefToken, body); rec.Code != 403 {
		t.Fatalf("other tribe save = %d", rec.Code)
	}
	if rec := getAuth(t, f.srv, "/api/tribes/tribe_nope/conventions", f.adminToken); rec.Code != 404 {
		t.Fatalf("unknown tribe = %d", rec.Code)
	}

	saved := putJSON(t, f.srv, path, f.chiefToken, body)
	want := `{"stackIds":["stk_go"],"skillIds":["skl_oauth2"],"rulesOff":["rule_naming"],"canEdit":true}`
	if saved.Code != 200 || strings.TrimSpace(saved.Body.String()) != want {
		t.Fatalf("tribe admin save = %d %s", saved.Code, saved.Body.String())
	}
	if rec := getAuth(t, f.srv, path, memberToken); !strings.Contains(rec.Body.String(), `"rulesOff":["rule_naming"]`) {
		t.Fatalf("member read after save = %s", rec.Body.String())
	}
	if _, err := f.srv.DB.Exec(`UPDATE skills SET disabled = true WHERE id = 'skl_oauth2'`); err != nil {
		t.Fatal(err)
	}
	if rec := putJSON(t, f.srv, path, f.adminToken, body); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"skillIds"`) {
		t.Fatalf("disabled skill = %d %s", rec.Code, rec.Body.String())
	}
	var payload string
	if err := f.srv.DB.QueryRow(`
		SELECT payload_json FROM events WHERE kind = 'config_changed' AND payload_json LIKE '%tribe_conventions%'`).Scan(&payload); err != nil {
		t.Fatalf("config_changed: %v", err)
	}
}
