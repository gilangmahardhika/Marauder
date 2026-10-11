package httpapi_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
)

func TestCardDetail(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	adminToken := loginToken(t, srv, "admin@example.com", "password-long-0")
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", lead.ID); err != nil {
		t.Fatal(err)
	}
	gadget := newProject(t, srv, adminToken, "tribe_default", "Gadget", "gadget")
	for project, statement := range map[string]string{"proj_default": "Export orders as CSV", gadget: "Gadget statement"} {
		if _, err := srv.DB.Exec(`
			INSERT INTO requirements (
				id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
				content_hash, accepted_at, accepted_by, project_id, created_by
			) VALUES ('REQ-001', 'prd', 'docs/prd/a.md', 'CSV export', $2, '[]', '[]', 'h',
				'2026-01-01T00:00:00Z', 'ada', $1, $3)`, project, statement, lead.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`INSERT INTO project_stacks (project_id, stack_id, source, created_at) VALUES ('proj_default', 'stk_go', 'manual', 'now')`,
		`INSERT INTO convention_rules (id, scope, stack_id, category, title, body, content_hash, created_at, updated_at)
			VALUES ('rule_go_errors', 'stack', 'stk_go', 'style', 'Wrap errors', 'b', 'h', 'now', 'now')`,
		`INSERT INTO project_skills (project_id, skill_id, created_at) VALUES ('proj_default', 'skl_oauth2', 'now')`,
	} {
		if _, err := srv.DB.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	id := reviewerCard(t, srv, "proj_default", lead.ID)
	if _, err := srv.DB.Exec(`INSERT INTO card_skills (project_id, card_id, skill_id) VALUES ('proj_default', $1, 'skl_oauth2')`, id); err != nil {
		t.Fatal(err)
	}
	token := loginToken(t, srv, "lead@example.com", "password-long-1")

	type detail struct {
		Requirements []struct {
			ID        string `json:"id"`
			Heading   string `json:"heading"`
			Statement string `json:"statement"`
			Accepted  bool   `json:"accepted"`
		} `json:"requirements"`
		Rules []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"rules"`
		Skills []struct {
			Slug    string `json:"slug"`
			UseWhen string `json:"useWhen"`
		} `json:"skills"`
	}
	check := func(name, body string) {
		t.Helper()
		var got detail
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatalf("%s = %s", name, body)
		}
		if len(got.Requirements) != 1 || got.Requirements[0].Statement != "Export orders as CSV" ||
			got.Requirements[0].Heading != "CSV export" || !got.Requirements[0].Accepted {
			t.Fatalf("%s requirements = %+v", name, got.Requirements)
		}
		labels := map[string]string{}
		for _, rule := range got.Rules {
			labels[rule.ID] = rule.Label
		}
		if labels["rule_naming"] != "global" || labels["rule_go_errors"] != "Go" {
			t.Fatalf("%s rules = %+v", name, got.Rules)
		}
		if len(got.Skills) != 1 || got.Skills[0].Slug != "oauth2-integration" || got.Skills[0].UseWhen == "" {
			t.Fatalf("%s skills = %+v", name, got.Skills)
		}
	}

	one := getAuth(t, srv, "/api/projects/proj_default/cards/"+id, token)
	if one.Code != 200 {
		t.Fatalf("card = %d %s", one.Code, one.Body.String())
	}
	check("card", one.Body.String())

	list := getAuth(t, srv, "/api/projects/proj_default/cards", token)
	if list.Code != 200 || strings.Contains(list.Body.String(), `"requirements"`) || strings.Contains(list.Body.String(), `"rules"`) {
		t.Fatalf("board list = %d %s", list.Code, list.Body.String())
	}

	if _, err := srv.DB.Exec(`UPDATE cards SET status = 'blocked', blocked_reason = 'attempt_cap' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	unblocked := postJSON(t, srv, "/api/projects/proj_default/cards/"+id+"/unblock", token, map[string]any{})
	if unblocked.Code != 200 {
		t.Fatalf("unblock = %d %s", unblocked.Code, unblocked.Body.String())
	}
	check("unblock", unblocked.Body.String())
}
