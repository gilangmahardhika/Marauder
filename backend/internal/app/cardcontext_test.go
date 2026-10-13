package app_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/tenancy"
)

func TestCardContextPack(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	ctx := context.Background()
	cardID := conventionsCard(t, db)
	dev, err := identity.Insert(ctx, db, "dev@example.com", "password-long-2", "developer", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO skills (id, slug, title, use_when, body, content_hash, created_at, updated_at)
		VALUES ('skl_pay', 'payments', 'Payments', 'Charging cards', 'Use the payments API.', 'h', 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][3]string{{"skill", "skl_pay", "attach"}, {"skill", "skl_oauth2", "detach"}, {"rule", "rule_naming", "detach"}} {
		if _, err := app.SetCardContext(ctx, db, dev, "proj_default", cardID, c[0], c[1], c[2]); err != nil {
			t.Fatalf("%v: %v", c, err)
		}
	}
	runWithProvider(t, db, repo, data, cardID, `{"implement":[{"text":"nothing to commit"}]}`)
	defer cleanupWorktree(t, repo, data, cardID)
	got := packItems(t, db, cardID)
	if !strings.HasPrefix(got, "rule:rule_database,rule:rule_") || !strings.HasSuffix(got, ",skill:skl_pay") ||
		strings.Contains(got, "rule_naming") || strings.Contains(got, "skl_oauth2") {
		t.Fatalf("items = %s", got)
	}

	if _, err := app.SetCardContext(ctx, db, dev, "proj_default", cardID, "rule", "rule_naming", "inherit"); err != nil {
		t.Fatal(err)
	}
	items, err := conventions.ForCard(ctx, db, "proj_default", cardID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	if !strings.Contains(strings.Join(ids, ","), "rule_naming") {
		t.Fatalf("after inherit = %v", ids)
	}
}

func mcpConn(id, slug string) app.MCPConn {
	return app.MCPConn{MCPServer: tenancy.MCPServer{ID: id, Slug: slug, Enabled: true}}
}

func slugs(conns []app.MCPConn) string {
	var out []string
	for _, c := range conns {
		out = append(out, c.Slug)
	}
	return strings.Join(out, ",")
}

func TestMCPScopeCardAttach(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	lead, err := identity.Insert(ctx, db, "lead@example.com", "password-long-1", "lead_developer", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO tribe_mcp_servers (id, tribe_id, slug, transport, url, created_at, updated_at)
			VALUES ('mcp_t', 'tribe_default', 'tickets', 'http', 'https://mcp.example.com/t', 'now', 'now'),
			       ('mcp_w', 'tribe_default', 'wiki', 'http', 'https://mcp.example.com/w', 'now', 'now')`,
		`INSERT INTO requirements (id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
			content_hash, accepted_at, accepted_by, mentions_json)
			VALUES ('REQ-001', 'prd', 'a.md', 'R1', 'Use @tickets', '[]', '[]', 'h', '2026-01-01T00:00:00Z', 'ada', '["tickets"]'),
			       ('REQ-002', 'prd', 'a.md', 'R2', 'No mention', '[]', '[]', 'h', '2026-01-01T00:00:00Z', 'ada', '[]')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	conns := []app.MCPConn{mcpConn("mcp_t", "tickets"), mcpConn("mcp_w", "wiki")}
	newCard := func(req string) board.Card {
		card, err := board.InsertCard(ctx, db, board.Card{Status: "ready", RequirementIDs: []string{req}})
		if err != nil {
			t.Fatal(err)
		}
		return card
	}
	set := func(card board.Card, id, mode string) {
		if _, err := app.SetCardContext(ctx, db, lead, "proj_default", card.ID, "mcp", id, mode); err != nil {
			t.Fatal(err)
		}
	}

	mentioned := newCard("REQ-001")
	set(mentioned, "mcp_w", "attach")
	kept, scope, err := app.MCPScopeFor(ctx, db, "proj_default", mentioned.ID, mentioned.RequirementIDs, conns)
	if err != nil || slugs(kept) != "tickets,wiki" || scope.Scope != "card" || strings.Join(scope.Attached, ",") != "wiki" {
		t.Fatalf("attach = %s %+v %v", slugs(kept), scope, err)
	}
	set(mentioned, "mcp_t", "detach")
	kept, scope, _ = app.MCPScopeFor(ctx, db, "proj_default", mentioned.ID, mentioned.RequirementIDs, conns)
	if slugs(kept) != "wiki" || strings.Join(scope.Detached, ",") != "tickets" || strings.Join(scope.Offered, ",") != "wiki" {
		t.Fatalf("detach = %s %+v", slugs(kept), scope)
	}

	all := newCard("REQ-002")
	set(all, "mcp_t", "detach")
	kept, scope, _ = app.MCPScopeFor(ctx, db, "proj_default", all.ID, all.RequirementIDs, conns)
	if slugs(kept) != "wiki" || scope.Scope != "card" {
		t.Fatalf("detach from all = %s %+v", slugs(kept), scope)
	}

	if _, err := db.Exec(`UPDATE requirements SET accepted_at = NULL WHERE id = 'REQ-001'`); err != nil {
		t.Fatal(err)
	}
	kept, scope, _ = app.MCPScopeFor(ctx, db, "proj_default", mentioned.ID, mentioned.RequirementIDs, conns)
	if len(kept) != 0 || scope.Scope != "none" || len(scope.Attached) != 0 {
		t.Fatalf("unaccepted = %s %+v", slugs(kept), scope)
	}
}

