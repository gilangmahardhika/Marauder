package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
)

type intakeRequirement struct {
	ID          string   `json:"id"`
	SourceKind  string   `json:"sourceKind"`
	Statement   string   `json:"statement"`
	Acceptance  []string `json:"acceptance"`
	Accepted    bool     `json:"accepted"`
	CreatedBy   *string  `json:"createdBy"`
	LinkedCards []string `json:"linkedCards"`
	Mentions    []string `json:"mentions"`
}

type intakeReply struct {
	Requirements []intakeRequirement `json:"requirements"`
}

func TestManualRequirementAdd(t *testing.T) {
	srv, dev, _ := manualServer(t)
	seedRequirement(t, srv, "REQ-001", "prd")
	path := "/api/projects/proj_default/requirements"

	blank := postJSON(t, srv, path, dev, map[string]any{"id": "", "statement": "Export the board.", "acceptance": []string{"One row per card."}})
	if blank.Code != 200 {
		t.Fatalf("blank id add = %d %s", blank.Code, blank.Body.String())
	}
	added := find(t, blank.Body.Bytes(), "REQ-002")
	if added.SourceKind != "manual" || added.Accepted || added.CreatedBy == nil || *added.CreatedBy != "dev@example.com" {
		t.Fatalf("REQ-002 = %+v", added)
	}
	if len(added.Acceptance) != 1 || added.Acceptance[0] != "One row per card." {
		t.Fatalf("acceptance = %v", added.Acceptance)
	}

	typed := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-010", "statement": "Typed id.", "acceptance": []string{}})
	if typed.Code != 200 {
		t.Fatalf("typed id add = %d %s", typed.Code, typed.Body.String())
	}
	find(t, typed.Body.Bytes(), "REQ-010")
	next := postJSON(t, srv, path, dev, map[string]any{"statement": "After the typed one."})
	find(t, next.Body.Bytes(), "REQ-011")

	if rec := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-001", "statement": "Duplicate."}); rec.Code != 409 {
		t.Fatalf("duplicate = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, path, dev, map[string]any{"id": "req-1", "statement": "Bad id."}); rec.Code != 422 {
		t.Fatalf("bad id = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, path, dev, map[string]any{"statement": "  "}); rec.Code != 422 {
		t.Fatalf("empty statement = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, path, dev, map[string]any{"statement": "Blank line.", "acceptance": []string{""}}); rec.Code != 422 {
		t.Fatalf("empty acceptance line = %d %s", rec.Code, rec.Body.String())
	}

	seedUser(t, srv, "out@example.com", "password-long-3", "product_owner", false)
	outsider := loginToken(t, srv, "out@example.com", "password-long-3")
	if rec := postJSON(t, srv, path, outsider, map[string]any{"statement": "Not mine."}); rec.Code != 404 {
		t.Fatalf("outsider add = %d %s", rec.Code, rec.Body.String())
	}
}

func TestManualRequirementEdit(t *testing.T) {
	srv, dev, lead := manualServer(t)
	seedRequirement(t, srv, "REQ-001", "prd")
	path := "/api/projects/proj_default/requirements"
	if rec := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-005", "statement": "First text."}); rec.Code != 200 {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/projects/proj_default/accept", lead, map[string]any{"requirementIds": []string{"REQ-005"}}); rec.Code != 200 {
		t.Fatalf("accept = %d %s", rec.Code, rec.Body.String())
	}

	same := putJSON(t, srv, path+"/REQ-005", dev, map[string]any{"statement": "First text.", "acceptance": []string{}})
	if same.Code != 200 || !find(t, same.Body.Bytes(), "REQ-005").Accepted {
		t.Fatalf("unchanged edit = %d %s", same.Code, same.Body.String())
	}
	edited := putJSON(t, srv, path+"/REQ-005", dev, map[string]any{"statement": "Second text.", "acceptance": []string{"Checked."}})
	if edited.Code != 200 {
		t.Fatalf("edit = %d %s", edited.Code, edited.Body.String())
	}
	if got := find(t, edited.Body.Bytes(), "REQ-005"); got.Accepted || got.Statement != "Second text." {
		t.Fatalf("edited = %+v", got)
	}

	if rec := putJSON(t, srv, path+"/REQ-001", dev, map[string]any{"statement": "Rewrite."}); rec.Code != 409 {
		t.Fatalf("edit repo row = %d %s", rec.Code, rec.Body.String())
	}
	if rec := sendJSON(t, srv, http.MethodDelete, path+"/REQ-001", dev, nil); rec.Code != 409 {
		t.Fatalf("delete repo row = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, path+"/REQ-404", dev, map[string]any{"statement": "Missing."}); rec.Code != 404 {
		t.Fatalf("edit missing = %d %s", rec.Code, rec.Body.String())
	}

	card, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: "backlog", RequirementIDs: []string{"REQ-005"}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := sendJSON(t, srv, http.MethodDelete, path+"/REQ-005", dev, nil); rec.Code != 409 {
		t.Fatalf("delete used row = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := srv.DB.Exec(`DELETE FROM cards WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	gone := sendJSON(t, srv, http.MethodDelete, path+"/REQ-005", dev, nil)
	if gone.Code != 200 {
		t.Fatalf("delete = %d %s", gone.Code, gone.Body.String())
	}
	var reply intakeReply
	if err := json.Unmarshal(gone.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	for _, req := range reply.Requirements {
		if req.ID == "REQ-005" {
			t.Fatal("REQ-005 still listed")
		}
	}
}

func TestIntakeLinkedCards(t *testing.T) {
	srv, dev, _ := manualServer(t)
	seedRequirement(t, srv, "REQ-001", "prd")
	seedRequirement(t, srv, "REQ-002", "prd")
	if _, err := srv.DB.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_other', 'tribe_default', 'Other', '/tmp/other', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	mine, err := board.InsertCard(ctx, srv.DB, board.Card{Status: "backlog", RequirementIDs: []string{"REQ-001"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := board.InsertCard(ctx, srv.DB, board.Card{ProjectID: "proj_other", Status: "backlog", RequirementIDs: []string{"REQ-001", "REQ-002"}}); err != nil {
		t.Fatal(err)
	}

	rec := getAuth(t, srv, "/api/projects/proj_default/intake", dev)
	if rec.Code != 200 {
		t.Fatalf("intake = %d %s", rec.Code, rec.Body.String())
	}
	if got := find(t, rec.Body.Bytes(), "REQ-001").LinkedCards; len(got) != 1 || got[0] != mine.ID {
		t.Fatalf("REQ-001 linkedCards = %v, want [%s]", got, mine.ID)
	}
	if got := find(t, rec.Body.Bytes(), "REQ-002").LinkedCards; got == nil || len(got) != 0 {
		t.Fatalf("REQ-002 linkedCards = %v, want []", got)
	}
}

func TestIntakeUnnumbered(t *testing.T) {
	srv, dev, _ := manualServer(t)
	path := "/api/projects/proj_default/intake"
	if rec := getAuth(t, srv, path, dev); !strings.Contains(rec.Body.String(), `"unnumbered":[]`) {
		t.Fatalf("intake before ingest = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := srv.DB.Exec(`UPDATE intake_state SET draft_pending = true, unnumbered_json = '["docs/product/PRD.md"]' WHERE project_id = 'proj_default'`); err != nil {
		t.Fatal(err)
	}
	rec := getAuth(t, srv, path, dev)
	if !strings.Contains(rec.Body.String(), `"unnumbered":["docs/product/PRD.md"]`) || !strings.Contains(rec.Body.String(), `"draftPending":true`) {
		t.Fatalf("intake = %d %s", rec.Code, rec.Body.String())
	}
}

func TestStatementLimitAndLineEndings(t *testing.T) {
	srv, dev, _ := manualServer(t)
	path := "/api/projects/proj_default/requirements"

	long := strings.Repeat("é", 10000)
	if rec := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-001", "statement": long}); rec.Code != 200 {
		t.Fatalf("10000 characters = %d %s", rec.Code, rec.Body.String())
	}
	over := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-002", "statement": long + "x"})
	if over.Code != 422 || !strings.Contains(over.Body.String(), `"statement":"Keep the statement to 10000 characters or fewer."`) {
		t.Fatalf("10001 characters = %d %s", over.Code, over.Body.String())
	}

	added := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-003", "statement": "# Export\r\n\r\n- one\r- two"})
	if got := find(t, added.Body.Bytes(), "REQ-003").Statement; got != "# Export\n\n- one\n- two" {
		t.Fatalf("stored statement = %q", got)
	}
	edited := putJSON(t, srv, path+"/REQ-003", dev, map[string]any{"statement": "a\r\nb", "acceptance": []string{}})
	if got := find(t, edited.Body.Bytes(), "REQ-003").Statement; got != "a\nb" {
		t.Fatalf("edited statement = %q", got)
	}
}

func TestAcceptanceLineMarkers(t *testing.T) {
	srv, dev, _ := manualServer(t)
	path := "/api/projects/proj_default/requirements"

	lines := []string{"- a", "* b", "+ c", "1. d", "2) e", "- [ ] f", "- [x] `g`", "**h** stays", "-1 stays", "i\r\nj"}
	added := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-001", "statement": "Export", "acceptance": lines})
	want := []string{"a", "b", "c", "d", "e", "f", "`g`", "**h** stays", "-1 stays", "i j"}
	if got := find(t, added.Body.Bytes(), "REQ-001").Acceptance; !reflect.DeepEqual(got, want) {
		t.Fatalf("stored acceptance = %q", got)
	}

	if rec := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-002", "statement": "x", "acceptance": []string{"- "}}); rec.Code != 422 ||
		!strings.Contains(rec.Body.String(), "Acceptance lines cannot be empty.") {
		t.Fatalf("marker only = %d %s", rec.Code, rec.Body.String())
	}
	over := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-003", "statement": "x", "acceptance": []string{strings.Repeat("a", 1001)}})
	if over.Code != 422 || !strings.Contains(over.Body.String(), "Keep each acceptance line to 1000 characters or fewer.") {
		t.Fatalf("1001 characters = %d %s", over.Code, over.Body.String())
	}
}

func TestManualRequirementMentions(t *testing.T) {
	srv, dev, lead := manualServer(t)
	path := "/api/projects/proj_default/requirements"
	added := postJSON(t, srv, path, dev, map[string]any{
		"id": "REQ-001", "statement": "Look up the ticket in @tickets, mail ada@example.com.",
		"acceptance": []string{"Docs come from @docs.", "`@raw` stays code."},
	})
	if added.Code != 200 {
		t.Fatalf("add = %d %s", added.Code, added.Body.String())
	}
	if got := find(t, added.Body.Bytes(), "REQ-001").Mentions; !reflect.DeepEqual(got, []string{"tickets", "docs"}) {
		t.Fatalf("mentions = %q", got)
	}
	if rec := postJSON(t, srv, "/api/projects/proj_default/accept", lead, map[string]any{"requirementIds": []string{"REQ-001"}}); rec.Code != 200 {
		t.Fatalf("accept = %d %s", rec.Code, rec.Body.String())
	}

	edited := putJSON(t, srv, path+"/REQ-001", dev, map[string]any{
		"statement":  "Look up the ticket in @tickets, mail ada@example.com.",
		"acceptance": []string{"Docs come from `@docs`.", "`@raw` stays code."},
	})
	if edited.Code != 200 {
		t.Fatalf("edit = %d %s", edited.Code, edited.Body.String())
	}
	got := find(t, edited.Body.Bytes(), "REQ-001")
	if !reflect.DeepEqual(got.Mentions, []string{"tickets"}) || got.Accepted {
		t.Fatalf("after dropping @docs = %+v", got)
	}

	plain := postJSON(t, srv, path, dev, map[string]any{"id": "REQ-002", "statement": "No mention."})
	if got := find(t, plain.Body.Bytes(), "REQ-002").Mentions; got == nil || len(got) != 0 {
		t.Fatalf("plain mentions = %v, want []", got)
	}
}

func TestIntakeMCPServers(t *testing.T) {
	srv, dev, _ := manualServer(t)
	for _, stmt := range []string{
		`INSERT INTO tribes (id, name, created_at) VALUES ('tribe_gadget', 'Gadget tribe', '2026-10-01T00:00:00Z')`,
		`INSERT INTO tribe_mcp_servers (id, tribe_id, slug, transport, url, enabled, secret_names_json, secrets_ciphertext, created_at, updated_at)
		 VALUES ('mcp_a', 'tribe_default', 'tickets', 'http', 'https://tickets.internal.example/mcp', true, '["Authorization"]', 'cipher-tickets', 'x', 'x'),
		        ('mcp_b', 'tribe_default', 'docs', 'http', 'https://docs.internal.example/mcp', false, '[]', '', 'x', 'x'),
		        ('mcp_c', 'tribe_gadget', 'gadget', 'http', 'https://gadget.example/mcp', true, '[]', '', 'x', 'x')`,
		`INSERT INTO tribe_mcp_tools (server_id, name, description, schema_json, schema_sha256)
		 VALUES ('mcp_a', 'lookup', '', '{}', 'h1'), ('mcp_a', 'search', '', '{}', 'h2')`,
	} {
		if _, err := srv.DB.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	rec := getAuth(t, srv, "/api/projects/proj_default/intake", dev)
	if rec.Code != 200 {
		t.Fatalf("intake = %d %s", rec.Code, rec.Body.String())
	}
	var reply struct {
		MCPServers []map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{
		{"slug": "docs", "enabled": false, "toolCount": float64(0)},
		{"slug": "tickets", "enabled": true, "toolCount": float64(2)},
	}
	if !reflect.DeepEqual(reply.MCPServers, want) {
		t.Fatalf("mcpServers = %v", reply.MCPServers)
	}
	for _, leak := range []string{"internal.example", "cipher-tickets", "Authorization", "mcp_a", "gadget"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("intake reply holds %q: %s", leak, rec.Body.String())
		}
	}
}

// manualServer returns a server with a developer and a lead developer who
// are members of tribe_default, and their bearers.
func manualServer(t *testing.T) (*httpapi.Server, string, string) {
	t.Helper()
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-0", "product_owner", true)
	for _, u := range []struct{ email, role string }{{"dev@example.com", "developer"}, {"lead@example.com", "lead_developer"}} {
		user := seedUser(t, srv, u.email, "password-long-1", u.role, false)
		if err := app.AddMember(context.Background(), srv.DB, admin, "tribe_default", user.ID); err != nil {
			t.Fatal(err)
		}
	}
	return srv, loginToken(t, srv, "dev@example.com", "password-long-1"), loginToken(t, srv, "lead@example.com", "password-long-1")
}

func seedRequirement(t *testing.T, srv *httpapi.Server, id, kind string) {
	t.Helper()
	if _, err := srv.DB.Exec(`
		INSERT INTO requirements (
			id, project_id, source_kind, source_path, heading, statement,
			acceptance_json, non_goals_json, content_hash
		) VALUES ($1, 'proj_default', $2, 'docs/prd/one.md', $1, 'text', '[]', '[]', 'hash')`, id, kind); err != nil {
		t.Fatal(err)
	}
}

func find(t *testing.T, body []byte, id string) intakeRequirement {
	t.Helper()
	var reply intakeReply
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	for _, req := range reply.Requirements {
		if req.ID == id {
			return req
		}
	}
	t.Fatalf("%s not in %s", id, body)
	return intakeRequirement{}
}
