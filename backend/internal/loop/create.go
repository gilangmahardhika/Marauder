package loop

import (
	"context"
	"database/sql"
	"encoding/json"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/trace"
)

func statements(ctx context.Context, db *sql.DB, projectID string, ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		var statement string
		err := db.QueryRowContext(ctx, `SELECT statement FROM requirements WHERE id = $1 AND project_id = $2`, id, projectID).Scan(&statement)
		if err != nil {
			return nil, err
		}
		out = append(out, statement)
	}
	return out, nil
}

func latestNote(ctx context.Context, db *sql.DB, cardID string) (string, error) {
	var note string
	err := db.QueryRowContext(ctx, `
		SELECT note FROM failure_notes WHERE card_id = $1 ORDER BY created_at DESC LIMIT 1`, cardID).Scan(&note)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return note, err
}

// CardConventions reads a card's rules and skills once for an attempt, with the
// trace items that name each one by id, version, and hash (RFC-055).
func CardConventions(ctx context.Context, db *sql.DB, projectID, cardID string) ([]contextpack.Convention, []map[string]any, error) {
	items, err := conventions.ForCard(ctx, db, projectID, cardID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]contextpack.Convention, 0, len(items))
	refs := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, contextpack.Convention{
			Kind: it.Kind, ID: it.ID, Version: it.Version, Hash: it.Hash, Title: it.Title,
			Label: it.Label, Category: it.Category, Slug: it.Slug, UseWhen: it.UseWhen, Body: it.Body,
		})
		refs = append(refs, map[string]any{"kind": it.Kind, "id": it.ID, "version": it.Version, "hash": it.Hash})
	}
	return out, refs, nil
}

type CreateInput struct {
	DB        *sql.DB
	ProjectID string
	Settings  Settings
	Args      map[string]any
	Secrets   []string
}

func CreateCard(ctx context.Context, in CreateInput) (board.Card, error) {
	if in.ProjectID == "" {
		in.ProjectID = "proj_default"
	}
	ids := stringList(in.Args["requirementIds"])
	owner := ""
	for i, id := range ids {
		var accepted, creator sql.NullString
		err := in.DB.QueryRowContext(ctx, `
			SELECT accepted_at, created_by FROM requirements WHERE id = $1 AND project_id = $2`,
			id, in.ProjectID).Scan(&accepted, &creator)
		if err != nil || !accepted.Valid || accepted.String == "" {
			return board.Card{}, ErrRequirement
		}
		if i == 0 {
			owner = creator.String
		} else if creator.String != owner {
			return board.Card{}, ErrOwners
		}
	}
	skillIDs := stringList(in.Args["skillIds"])
	if len(skillIDs) > 0 {
		enabled, err := conventions.EnabledSkills(ctx, in.DB, in.ProjectID)
		if err != nil {
			return board.Card{}, err
		}
		ok := map[string]bool{}
		for _, k := range enabled {
			ok[k.ID] = true
		}
		for _, id := range skillIDs {
			if !ok[id] {
				return board.Card{}, ErrSkill
			}
		}
	}
	if _, ok := in.Args["command"]; ok {
		if _, err := trace.Append(ctx, in.DB, trace.Event{
			ProjectID: in.ProjectID,
			Kind:      "tool_call",
			Payload: map[string]any{
				"name":     "create_card",
				"ok":       false,
				"rejected": true,
				"summary":  "command ignored",
			},
		}, in.Secrets); err != nil {
			return board.Card{}, err
		}
	}
	title, _ := in.Args["title"].(string)
	risk, _ := in.Args["risk"].(string)
	tx, err := in.DB.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	card, err := board.InsertCard(ctx, tx, board.Card{
		ProjectID:      in.ProjectID,
		OwnerID:        owner,
		Title:          title,
		Status:         "backlog",
		Risk:           risk,
		RequirementIDs: ids,
		LocalCheck:     in.Settings.LocalCheck,
		DevCheck:       in.Settings.DevCheck,
		MaxAttempts:    in.Settings.MaxAttempts,
	})
	if err != nil {
		return board.Card{}, err
	}
	if err := conventions.SetCardSkills(ctx, tx, in.ProjectID, card.ID, skillIDs); err != nil {
		return board.Card{}, err
	}
	return card, tx.Commit()
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		var out []string
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var out []string
		if json.Unmarshal(raw, &out) != nil {
			return nil
		}
		return out
	}
}
