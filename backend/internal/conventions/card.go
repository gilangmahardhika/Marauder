package conventions

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"orchestrator/backend/internal/identity"
)

type CardRule struct {
	EffectiveRule
	State string `json:"state"`
}

type CardSkill struct {
	EffectiveSkill
	State string `json:"state"`
}

// CardSet is a card's rules and skills with their state for the next attempt,
// and the MCP server ids its context attached or detached (RFC-105).
type CardSet struct {
	Rules     []CardRule
	Skills    []CardSkill
	MCPAttach []string
	MCPDetach []string
}

var (
	CardKinds = []string{"skill", "rule", "mcp"}
	CardModes = []string{"attach", "detach", "inherit"}
)

// CardContext resolves the card's rules and skills over the project's
// effective set. Every row a card set shows with origin card, so a person can
// restore it.
func CardContext(ctx context.Context, q Querier, projectID, cardID string) (CardSet, error) {
	out := CardSet{Rules: []CardRule{}, Skills: []CardSkill{}, MCPAttach: []string{}, MCPDetach: []string{}}
	project, cat, err := loadProject(ctx, q, projectID)
	if err != nil {
		return CardSet{}, err
	}
	modes := map[string]map[string]string{"rule": {}, "skill": {}, "mcp": {}}
	rows, err := q.QueryContext(ctx, `
		SELECT kind, item_id, mode FROM card_context WHERE project_id = $1 AND card_id = $2 ORDER BY kind, item_id`,
		projectID, cardID)
	if err != nil {
		return CardSet{}, err
	}
	for rows.Next() {
		var kind, id, mode string
		if err := rows.Scan(&kind, &id, &mode); err != nil {
			rows.Close()
			return CardSet{}, err
		}
		if m, ok := modes[kind]; ok {
			m[id] = mode
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return CardSet{}, err
	}
	planner, err := strings1(ctx, q, `SELECT skill_id FROM card_skills WHERE project_id = $1 AND card_id = $2`, projectID, cardID)
	if err != nil {
		return CardSet{}, err
	}

	inherited := map[string]EffectiveRule{}
	for _, r := range project.Rules {
		inherited[r.ID] = r
	}
	names, enabledStack := map[string]string{}, map[string]bool{}
	for _, s := range cat.Stacks {
		names[s.ID] = s.Name
		enabledStack[s.ID] = !s.Disabled
	}
	for _, r := range cat.Rules {
		if r.Disabled || (r.Scope == "stack" && !enabledStack[r.StackID]) {
			continue
		}
		item, ok := inherited[r.ID]
		if !ok {
			item = EffectiveRule{
				ID: r.ID, Title: r.Title, Scope: r.Scope, StackName: names[r.StackID], Category: r.Category,
				Version: r.Version, Hash: r.ContentHash, Body: r.Body,
			}
		}
		switch modes["rule"][r.ID] {
		case "attach":
			item.Origin = OriginCard
			out.Rules = append(out.Rules, CardRule{item, "on"})
		case "detach":
			item.Origin = OriginCard
			out.Rules = append(out.Rules, CardRule{item, "off"})
		default:
			if ok {
				item.Origin = OriginProject
				out.Rules = append(out.Rules, CardRule{item, "on"})
			}
		}
	}

	fromPlanner := set(planner)
	for _, k := range cat.Skills {
		if k.Disabled {
			continue
		}
		item := EffectiveSkill{
			ID: k.ID, Slug: k.Slug, Title: k.Title, UseWhen: k.UseWhen, Version: k.Version,
			Hash: k.ContentHash, Body: k.Body,
		}
		switch modes["skill"][k.ID] {
		case "attach":
			item.Origin = OriginCard
			out.Skills = append(out.Skills, CardSkill{item, "on"})
		case "detach":
			item.Origin = OriginCard
			out.Skills = append(out.Skills, CardSkill{item, "off"})
		default:
			if fromPlanner[k.ID] {
				item.Origin = OriginPlanner
				out.Skills = append(out.Skills, CardSkill{item, "on"})
			}
		}
	}

	for id, mode := range modes["mcp"] {
		if mode == "attach" {
			out.MCPAttach = append(out.MCPAttach, id)
		} else {
			out.MCPDetach = append(out.MCPDetach, id)
		}
	}
	sort.Strings(out.MCPAttach)
	sort.Strings(out.MCPDetach)
	return out, nil
}

// ItemEnabled reports whether id names an enabled rule or skill of the
// catalog.
func ItemEnabled(ctx context.Context, q Querier, kind, id string) (bool, error) {
	table := map[string]string{"rule": "convention_rules", "skill": "skills"}[kind]
	if table == "" {
		return false, nil
	}
	found, err := strings1(ctx, q, `SELECT id FROM `+table+` WHERE id = $1 AND NOT disabled`, id)
	return len(found) > 0, err
}

// SetCardContext writes one row of a card's context in the caller's
// transaction. Mode inherit deletes the row. It reports whether anything
// changed.
func SetCardContext(ctx context.Context, tx *sql.Tx, actor identity.User, projectID, cardID, kind, itemID, mode string) (bool, error) {
	var current string
	err := tx.QueryRowContext(ctx, `
		SELECT mode FROM card_context WHERE project_id = $1 AND card_id = $2 AND kind = $3 AND item_id = $4`,
		projectID, cardID, kind, itemID).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if mode == "inherit" {
		if current == "" {
			return false, nil
		}
		_, err := tx.ExecContext(ctx, `
			DELETE FROM card_context WHERE project_id = $1 AND card_id = $2 AND kind = $3 AND item_id = $4`,
			projectID, cardID, kind, itemID)
		return err == nil, err
	}
	if current == mode {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO card_context (project_id, card_id, kind, item_id, mode, added_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project_id, card_id, kind, item_id) DO UPDATE
		SET mode = EXCLUDED.mode, added_by = EXCLUDED.added_by, created_at = EXCLUDED.created_at`,
		projectID, cardID, kind, itemID, mode, actor.ID, now())
	return err == nil, err
}
