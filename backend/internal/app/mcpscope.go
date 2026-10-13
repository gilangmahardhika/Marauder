package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"

	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/loop"
)

// MCPScopeFor keeps the enabled servers a card's requirements allow
// (RFC-102): every server when none mentions one, the mentioned servers
// otherwise, and none when no mention names an enabled server or a
// requirement is not accepted. Then the card's context adds the servers it
// attached and drops the ones it detached (RFC-105). conns are the tribe's
// enabled servers.
func MCPScopeFor(ctx context.Context, db *sql.DB, projectID, cardID string, requirementIDs []string, conns []MCPConn) ([]MCPConn, loop.MCPContext, error) {
	var mentions []string
	unaccepted := ""
	if len(requirementIDs) > 0 {
		rows, err := db.QueryContext(ctx, `
			SELECT id, mentions_json, accepted_at IS NOT NULL FROM requirements
			WHERE project_id = $1 AND id = ANY($2) ORDER BY id`, projectID, requirementIDs)
		if err != nil {
			return nil, loop.MCPContext{}, err
		}
		defer rows.Close()
		found := map[string]bool{}
		for rows.Next() {
			var id, raw string
			var accepted bool
			if err := rows.Scan(&id, &raw, &accepted); err != nil {
				return nil, loop.MCPContext{}, err
			}
			found[id] = true
			if !accepted && unaccepted == "" {
				unaccepted = id
			}
			var list []string
			_ = json.Unmarshal([]byte(raw), &list)
			for _, slug := range list {
				if !slices.Contains(mentions, slug) {
					mentions = append(mentions, slug)
				}
			}
		}
		if err := rows.Err(); err != nil {
			return nil, loop.MCPContext{}, err
		}
		for _, id := range requirementIDs {
			if !found[id] && unaccepted == "" {
				unaccepted = id
			}
		}
	}
	if len(conns) == 0 && len(mentions) == 0 {
		return nil, loop.MCPContext{Scope: "all"}, nil
	}
	if unaccepted != "" {
		return nil, loop.MCPContext{
			Scope: "none", Missing: []string{}, Total: len(conns),
			Reason: unaccepted + " changed since it was accepted",
		}, nil
	}
	kept, scope, err := ScopeMentions(conns, mentions)
	if err != nil || cardID == "" {
		return kept, scope, err
	}
	set, err := conventions.CardContext(ctx, db, projectID, cardID)
	if err != nil {
		return nil, loop.MCPContext{}, err
	}
	kept, scope = applyCardMCP(conns, kept, scope, set.MCPAttach, set.MCPDetach)
	return kept, scope, nil
}

// applyCardMCP adds the attached servers that are among conns and drops the
// detached ones. The scope becomes card when that changed the set.
func applyCardMCP(conns, kept []MCPConn, scope loop.MCPContext, attach, detach []string) ([]MCPConn, loop.MCPContext) {
	in := func(list []MCPConn, id string) bool {
		return slices.ContainsFunc(list, func(c MCPConn) bool { return c.ID == id })
	}
	var next []MCPConn
	for _, conn := range kept {
		if slices.Contains(detach, conn.ID) {
			scope.Detached = append(scope.Detached, conn.Slug)
			continue
		}
		next = append(next, conn)
	}
	for _, conn := range conns {
		if slices.Contains(attach, conn.ID) && !in(kept, conn.ID) {
			scope.Attached = append(scope.Attached, conn.Slug)
			next = append(next, conn)
		}
	}
	if len(scope.Attached) == 0 && len(scope.Detached) == 0 {
		return kept, scope
	}
	slices.SortFunc(next, func(a, b MCPConn) int { return strings.Compare(a.Slug, b.Slug) })
	scope.Offered = nil
	for _, conn := range next {
		scope.Offered = append(scope.Offered, conn.Slug)
	}
	if len(next) == 0 {
		scope.Scope = "none"
		scope.Reason = "the card detached every server"
		return nil, scope
	}
	scope.Scope, scope.Reason = "card", ""
	return next, scope
}

// ScopeMentions keeps the conns that mentions name, or every conn when
// mentions is empty (RFC-102).
func ScopeMentions(conns []MCPConn, mentions []string) ([]MCPConn, loop.MCPContext, error) {
	scope := loop.MCPContext{Scope: "all", Missing: []string{}, Total: len(conns)}
	if len(mentions) == 0 {
		return conns, scope, nil
	}
	var kept []MCPConn
	for _, conn := range conns {
		if slices.Contains(mentions, conn.Slug) {
			kept = append(kept, conn)
			scope.Offered = append(scope.Offered, conn.Slug)
		}
	}
	for _, slug := range mentions {
		if !slices.Contains(scope.Offered, slug) {
			scope.Missing = append(scope.Missing, slug)
		}
	}
	if len(kept) == 0 {
		scope.Scope = "none"
		scope.Reason = "no mentioned server is enabled in the tribe"
		return nil, scope, nil
	}
	scope.Scope = "mentioned"
	return kept, scope, nil
}
