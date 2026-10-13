package app

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/trace"
)

// CardContextRights is what the actor may change in a card's context
// (RFC-105). The caller already sees the card's project.
type CardContextRights struct {
	Change    bool
	AttachMCP bool
}

var runRoles = []string{"lead_developer", "senior_developer", "developer", "qa"}

// CardContextRightsFor lets the roles that may run a card, a tribe admin of
// the project's tribe, and a company admin change skills and rules and detach
// a server. Attaching a server needs a Lead Developer, a tribe admin, or a
// company admin.
func CardContextRightsFor(ctx context.Context, db *sql.DB, actor User, projectID string) (CardContextRights, error) {
	rights := CardContextRights{
		Change:    actor.Admin || slices.Contains(runRoles, actor.Role),
		AttachMCP: actor.Admin || actor.Role == "lead_developer",
	}
	if rights.Change && rights.AttachMCP {
		return rights, nil
	}
	project, err := tenancy.GetProject(ctx, db, projectID)
	if err != nil {
		return CardContextRights{}, err
	}
	tribeAdmin, err := tenancy.IsTribeAdmin(ctx, db, actor.ID, project.TribeID)
	if err != nil {
		return CardContextRights{}, err
	}
	return CardContextRights{Change: rights.Change || tribeAdmin, AttachMCP: rights.AttachMCP || tribeAdmin}, nil
}

// SetCardContext attaches, detaches, or inherits one skill, rule, or MCP
// server on a card for its next attempt (RFC-105). It writes no status.
func SetCardContext(ctx context.Context, db *sql.DB, actor User, projectID, cardID, kind, itemID, mode string) (board.Card, error) {
	itemID = strings.TrimSpace(itemID)
	fields := map[string]string{}
	if !slices.Contains(conventions.CardKinds, kind) {
		fields["kind"] = "Choose skill, rule, or mcp."
	}
	if !slices.Contains(conventions.CardModes, mode) {
		fields["mode"] = "Choose attach, detach, or inherit."
	}
	if itemID == "" {
		fields["itemId"] = "Choose an item."
	}
	if len(fields) > 0 {
		return board.Card{}, &settings.Invalid{Fields: fields}
	}
	rights, err := CardContextRightsFor(ctx, db, actor, projectID)
	if err != nil {
		return board.Card{}, err
	}
	if !rights.Change || (kind == "mcp" && mode == "attach" && !rights.AttachMCP) {
		return board.Card{}, identity.ErrForbidden
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	card, err := lockCard(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	if card.Status == "done" {
		return card, ErrCardDone
	}
	if mode != "inherit" {
		ok, err := cardItemEnabled(ctx, tx, projectID, kind, itemID)
		if err != nil {
			return card, err
		}
		if !ok {
			msg := "Choose an enabled " + kind + "."
			if kind == "mcp" {
				msg = "Choose an enabled MCP server of this tribe."
			}
			return card, &settings.Invalid{Fields: map[string]string{"itemId": msg}}
		}
	}
	changed, err := conventions.SetCardContext(ctx, tx, actor, projectID, card.ID, kind, itemID, mode)
	if err != nil {
		return card, err
	}
	if changed {
		if _, err := trace.Append(ctx, tx, trace.Event{
			CardID: card.ID, ProjectID: card.ProjectID, ActorID: actor.ID, Kind: "context_changed",
			Payload: map[string]any{"kind": kind, "itemId": itemID, "mode": mode},
		}, nil); err != nil {
			return card, err
		}
	}
	return card, tx.Commit()
}

func cardItemEnabled(ctx context.Context, tx *sql.Tx, projectID, kind, itemID string) (bool, error) {
	if kind != "mcp" {
		return conventions.ItemEnabled(ctx, tx, kind, itemID)
	}
	var n int
	err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM tribe_mcp_servers
		WHERE id = $1 AND enabled AND tribe_id = (SELECT tribe_id FROM projects WHERE id = $2)`, itemID, projectID).Scan(&n)
	return n > 0, err
}

// CardMCPServer is one server of the card's tribe as the card page shows it:
// no transport, host, or secret (RFC-101).
type CardMCPServer struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Enabled   bool   `json:"enabled"`
	ToolCount int    `json:"toolCount"`
	State     string `json:"state"`
	Origin    string `json:"origin"`
}

// CardContextView is the card's context for the card page.
type CardContextView struct {
	Rules    []conventions.CardRule  `json:"rules"`
	Skills   []conventions.CardSkill `json:"skills"`
	MCP      []CardMCPServer         `json:"mcp"`
	MCPScope string                  `json:"mcpScope"`
}

// CardContextFor resolves what the card's next attempt would carry.
func CardContextFor(ctx context.Context, db *sql.DB, card board.Card) (CardContextView, error) {
	set, err := conventions.CardContext(ctx, db, card.ProjectID, card.ID)
	if err != nil {
		return CardContextView{}, err
	}
	view := CardContextView{Rules: set.Rules, Skills: set.Skills, MCP: []CardMCPServer{}}
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.slug, s.enabled, (SELECT count(*) FROM tribe_mcp_tools t WHERE t.server_id = s.id)
		FROM tribe_mcp_servers s
		WHERE s.tribe_id = (SELECT tribe_id FROM projects WHERE id = $1) ORDER BY s.slug`, card.ProjectID)
	if err != nil {
		return CardContextView{}, err
	}
	var enabled []MCPConn
	for rows.Next() {
		var s CardMCPServer
		if err := rows.Scan(&s.ID, &s.Slug, &s.Enabled, &s.ToolCount); err != nil {
			rows.Close()
			return CardContextView{}, err
		}
		view.MCP = append(view.MCP, s)
		if s.Enabled {
			enabled = append(enabled, MCPConn{MCPServer: tenancy.MCPServer{ID: s.ID, Slug: s.Slug, Enabled: true}})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return CardContextView{}, err
	}
	kept, scope, err := MCPScopeFor(ctx, db, card.ProjectID, card.ID, card.RequirementIDs, enabled)
	if err != nil {
		return CardContextView{}, err
	}
	view.MCPScope = scope.Scope
	for i, s := range view.MCP {
		on := slices.ContainsFunc(kept, func(c MCPConn) bool { return c.ID == s.ID })
		view.MCP[i].State = map[bool]string{true: "on", false: "off"}[on]
		view.MCP[i].Origin = "scope"
		if slices.Contains(set.MCPAttach, s.ID) || slices.Contains(set.MCPDetach, s.ID) {
			view.MCP[i].Origin = conventions.OriginCard
		}
	}
	return view, nil
}
