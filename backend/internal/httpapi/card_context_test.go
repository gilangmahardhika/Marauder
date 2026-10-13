package httpapi_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/httpapi"
)

type contextFixture struct {
	srv        *httpapi.Server
	card       string
	gadget     string
	leadToken  string
	devToken   string
	poToken    string
	chiefToken string
}

// newContextFixture makes a backlog card in proj_default, an enabled and a
// disabled MCP server in tribe_default, a server in another tribe, and a
// Lead Developer, Developer, Product Owner, and tribe admin QA Lead member.
func newContextFixture(t *testing.T) contextFixture {
	t.Helper()
	ctx := context.Background()
	srv := accountServer(t)
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	adminToken := loginToken(t, srv, "admin@example.com", "password-long-0")
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", false)
	dev := seedUser(t, srv, "dev@example.com", "password-long-2", "developer", false)
	po := seedUser(t, srv, "po@example.com", "password-long-3", "product_owner", false)
	chief := seedUser(t, srv, "chief@example.com", "password-long-4", "qa_lead", false)
	for _, user := range []app.User{lead, dev, po, chief} {
		if err := app.AddMember(ctx, srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.SetTribeAdmin(ctx, srv.DB, admin, "tribe_default", chief.ID, true); err != nil {
		t.Fatal(err)
	}
	other := newTribe(t, srv, adminToken, "Payments")
	gadget := newProject(t, srv, adminToken, "tribe_default", "Gadget", "gadget")
	for _, stmt := range []string{
		`INSERT INTO tribe_mcp_servers (id, tribe_id, slug, transport, url, created_at, updated_at)
			VALUES ('mcp_tickets', 'tribe_default', 'tickets', 'http', 'https://mcp.example.com/t', 'now', 'now')`,
		`INSERT INTO tribe_mcp_servers (id, tribe_id, slug, transport, url, enabled, created_at, updated_at)
			VALUES ('mcp_off', 'tribe_default', 'wiki', 'http', 'https://mcp.example.com/w', false, 'now', 'now')`,
		`INSERT INTO tribe_mcp_servers (id, tribe_id, slug, transport, url, created_at, updated_at)
			VALUES ('mcp_other', '` + other + `', 'billing', 'http', 'https://mcp.example.com/b', 'now', 'now')`,
		`INSERT INTO requirements (
			id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
			content_hash, accepted_at, accepted_by, project_id
		) VALUES ('REQ-001', 'prd', 'docs/prd/a.md', 'CSV', 'Export orders', '[]', '[]', 'h', '2026-01-01T00:00:00Z', 'ada', 'proj_default')`,
	} {
		if _, err := srv.DB.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return contextFixture{
		srv: srv, card: reviewerCard(t, srv, "proj_default", lead.ID), gadget: gadget,
		leadToken:  loginToken(t, srv, "lead@example.com", "password-long-1"),
		devToken:   loginToken(t, srv, "dev@example.com", "password-long-2"),
		poToken:    loginToken(t, srv, "po@example.com", "password-long-3"),
		chiefToken: loginToken(t, srv, "chief@example.com", "password-long-4"),
	}
}

func (f contextFixture) path() string {
	return "/api/projects/proj_default/cards/" + f.card + "/context"
}

func (f contextFixture) put(t *testing.T, token, kind, item, mode string) (int, cardContextBody) {
	t.Helper()
	rec := putJSON(t, f.srv, f.path(), token, map[string]string{"kind": kind, "itemId": item, "mode": mode})
	var body cardContextBody
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, body
}

type contextItem struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	State  string `json:"state"`
	Origin string `json:"origin"`
}

type cardContextBody struct {
	Actions []string `json:"actions"`
	Context struct {
		Rules    []contextItem `json:"rules"`
		Skills   []contextItem `json:"skills"`
		MCP      []contextItem `json:"mcp"`
		MCPScope string        `json:"mcpScope"`
	} `json:"context"`
}

func findItem(items []contextItem, id string) contextItem {
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	return contextItem{}
}

func cardActions(t *testing.T, f contextFixture, token string) []string {
	t.Helper()
	rec := getAuth(t, f.srv, "/api/projects/proj_default/cards/"+f.card, token)
	var body cardContextBody
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatalf("card = %d %s", rec.Code, rec.Body.String())
	}
	return body.Actions
}

func TestCardContextRoles(t *testing.T) {
	f := newContextFixture(t)
	code, body := f.put(t, f.devToken, "skill", "skl_oauth2", "attach")
	if code != 200 || findItem(body.Context.Skills, "skl_oauth2") != (contextItem{ID: "skl_oauth2", Slug: "oauth2-integration", State: "on", Origin: "card"}) {
		t.Fatalf("developer attach skill = %d %+v", code, body.Context.Skills)
	}
	code, body = f.put(t, f.devToken, "rule", "rule_naming", "detach")
	if code != 200 || findItem(body.Context.Rules, "rule_naming").State != "off" || findItem(body.Context.Rules, "rule_database").Origin != "project" {
		t.Fatalf("developer detach rule = %d %+v", code, body.Context.Rules)
	}
	code, body = f.put(t, f.devToken, "mcp", "mcp_tickets", "detach")
	if code != 200 || findItem(body.Context.MCP, "mcp_tickets") != (contextItem{ID: "mcp_tickets", Slug: "tickets", State: "off", Origin: "card"}) {
		t.Fatalf("developer detach server = %d %+v", code, body.Context.MCP)
	}
	if code, _ := f.put(t, f.devToken, "mcp", "mcp_tickets", "attach"); code != 403 {
		t.Fatalf("developer attach server = %d", code)
	}
	if code, body := f.put(t, f.leadToken, "mcp", "mcp_tickets", "attach"); code != 200 || findItem(body.Context.MCP, "mcp_tickets").State != "on" {
		t.Fatalf("lead attach server = %d %+v", code, body.Context.MCP)
	}
	if code, _ := f.put(t, f.chiefToken, "mcp", "mcp_tickets", "inherit"); code != 200 {
		t.Fatalf("tribe admin inherit server = %d", code)
	}
	if code, _ := f.put(t, f.chiefToken, "mcp", "mcp_tickets", "attach"); code != 200 {
		t.Fatalf("tribe admin attach server = %d", code)
	}
	if code, _ := f.put(t, f.poToken, "skill", "skl_oauth2", "detach"); code != 403 {
		t.Fatalf("product owner change = %d", code)
	}

	for token, want := range map[string][2]bool{
		f.devToken: {true, false}, f.leadToken: {true, true}, f.chiefToken: {true, true}, f.poToken: {false, false},
	} {
		actions := cardActions(t, f, token)
		if slices.Contains(actions, "context") != want[0] || slices.Contains(actions, "attach-mcp") != want[1] {
			t.Fatalf("actions = %v, want context %v attach-mcp %v", actions, want[0], want[1])
		}
	}
	var n int
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'context_changed'`, f.card).Scan(&n); err != nil || n != 6 {
		t.Fatalf("context_changed events = %d %v", n, err)
	}
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'transition'`, f.card).Scan(&n); err != nil || n != 0 {
		t.Fatalf("transitions = %d %v", n, err)
	}
}

func TestCardContextDone409(t *testing.T) {
	f := newContextFixture(t)
	if _, err := f.srv.DB.Exec(`UPDATE cards SET status = 'done' WHERE id = $1`, f.card); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.put(t, f.devToken, "skill", "skl_oauth2", "attach"); code != 409 {
		t.Fatalf("done card = %d", code)
	}
	var status string
	if err := f.srv.DB.QueryRow(`SELECT status FROM cards WHERE id = $1`, f.card).Scan(&status); err != nil || status != "done" {
		t.Fatalf("status = %s %v", status, err)
	}
	if actions := cardActions(t, f, f.leadToken); slices.Contains(actions, "context") || slices.Contains(actions, "attach-mcp") {
		t.Fatalf("done card actions = %v", actions)
	}
}

func TestCardContextScoped(t *testing.T) {
	f := newContextFixture(t)
	if _, err := f.srv.DB.Exec(`UPDATE cards SET project_id = $2 WHERE id = $1`, f.card, f.gadget); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.put(t, f.leadToken, "skill", "skl_oauth2", "attach"); code != 404 {
		t.Fatalf("card of another project = %d", code)
	}
	rec := putJSON(t, f.srv, "/api/projects/"+f.gadget+"/cards/"+f.card+"/context", f.leadToken,
		map[string]string{"kind": "skill", "itemId": "skl_oauth2", "mode": "attach"})
	if rec.Code != 200 {
		t.Fatalf("own project = %d %s", rec.Code, rec.Body.String())
	}
	var project string
	if err := f.srv.DB.QueryRow(`SELECT project_id FROM card_context WHERE card_id = $1`, f.card).Scan(&project); err != nil || project != f.gadget {
		t.Fatalf("card_context project = %s %v", project, err)
	}
}

func TestCardContextDisabledServer(t *testing.T) {
	f := newContextFixture(t)
	for _, c := range []struct{ kind, item string }{
		{"mcp", "mcp_off"}, {"mcp", "mcp_other"}, {"mcp", "mcp_nope"}, {"rule", "rule_nope"},
	} {
		rec := putJSON(t, f.srv, f.path(), f.leadToken, map[string]string{"kind": c.kind, "itemId": c.item, "mode": "attach"})
		if rec.Code != 422 || !containsField(rec.Body.Bytes(), "itemId") {
			t.Fatalf("%s %s = %d %s", c.kind, c.item, rec.Code, rec.Body.String())
		}
	}
	if _, err := f.srv.DB.Exec(`UPDATE skills SET disabled = true WHERE id = 'skl_oauth2'`); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.put(t, f.devToken, "skill", "skl_oauth2", "attach"); code != 422 {
		t.Fatalf("disabled skill = %d", code)
	}
	if code, _ := f.put(t, f.devToken, "page", "skl_oauth2", "attach"); code != 422 {
		t.Fatalf("unknown kind = %d", code)
	}
	var n int
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM card_context`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("card_context rows = %d %v", n, err)
	}
}

func containsField(raw []byte, field string) bool {
	var body struct {
		Error struct {
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return false
	}
	_, ok := body.Error.Fields[field]
	return ok
}
