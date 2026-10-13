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
		`INSERT INTO project_conventions (project_id, kind, item_id, mode, created_at) VALUES ('proj_default', 'stack', 'stk_go', 'on', 'now')`,
		`INSERT INTO convention_rules (id, scope, stack_id, category, title, body, content_hash, created_at, updated_at)
			VALUES ('rule_go_errors', 'stack', 'stk_go', 'style', 'Wrap errors', 'b', 'h', 'now', 'now')`,
		`INSERT INTO project_conventions (project_id, kind, item_id, mode, created_at) VALUES ('proj_default', 'skill', 'skl_oauth2', 'on', 'now')`,
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

func TestCardDetailNotes(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	adminToken := loginToken(t, srv, "admin@example.com", "password-long-0")
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", lead.ID); err != nil {
		t.Fatal(err)
	}
	gadget := newProject(t, srv, adminToken, "tribe_default", "Gadget", "gadget")
	id := reviewerCard(t, srv, "proj_default", lead.ID)
	for _, stmt := range []string{
		`INSERT INTO attempts (id, card_id, n, task_class, started_at, project_id)
			VALUES ('att_one', $1, 1, 'implement', '2026-10-12T03:00:00Z', 'proj_default'),
				('att_two', $1, 2, 'implement', '2026-10-12T04:30:00Z', 'proj_default')`,
		`INSERT INTO failure_notes (id, card_id, attempt_id, source, note, created_at, project_id) VALUES
			('note_check', $1, 'att_one', 'local_check', 'exit 1', '2026-10-12T04:00:00Z', 'proj_default'),
			('note_review', $1, 'att_two', 'review', 'Run the **lint** scripts.', '2026-10-12T05:00:00Z', 'proj_default'),
			('note_approver', $1, 'approver', 'approver', 'Keep the cache.', '2026-10-12T06:00:00Z', 'proj_default')`,
		`INSERT INTO failure_notes (id, card_id, attempt_id, source, note, created_at, project_id)
			VALUES ('note_other', $1, 'att_two', 'review', 'other project', '2026-10-12T07:00:00Z', '` + gadget + `')`,
	} {
		if _, err := srv.DB.Exec(stmt, id); err != nil {
			t.Fatal(err)
		}
	}
	token := loginToken(t, srv, "lead@example.com", "password-long-1")

	one := getAuth(t, srv, "/api/projects/proj_default/cards/"+id, token)
	if one.Code != 200 {
		t.Fatalf("card = %d %s", one.Code, one.Body.String())
	}
	var got struct {
		Notes []struct {
			Source    string `json:"source"`
			Note      string `json:"note"`
			CreatedAt string `json:"createdAt"`
			AttemptN  int    `json:"attemptN"`
		} `json:"notes"`
	}
	if err := json.Unmarshal(one.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	type note struct {
		source, note string
		n            int
	}
	want := []note{{"approver", "Keep the cache.", 0}, {"review", "Run the **lint** scripts.", 2}, {"local_check", "exit 1", 1}}
	if len(got.Notes) != len(want) {
		t.Fatalf("notes = %+v", got.Notes)
	}
	for i, w := range want {
		if n := got.Notes[i]; n.Source != w.source || n.Note != w.note || n.AttemptN != w.n || n.CreatedAt == "" {
			t.Fatalf("notes[%d] = %+v, want %+v", i, n, w)
		}
	}

	list := getAuth(t, srv, "/api/projects/proj_default/cards", token)
	if list.Code != 200 || strings.Contains(list.Body.String(), `"notes"`) {
		t.Fatalf("board list = %d %s", list.Code, list.Body.String())
	}
}
