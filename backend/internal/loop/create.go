package loop

import (
	"context"
	"database/sql"
	"encoding/json"

	"orchestrator/backend/internal/board"
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
	card, err := board.InsertCard(ctx, in.DB, board.Card{
		ProjectID:      in.ProjectID,
		OwnerID:        owner,
		Title:          title,
		Status:         "backlog",
		Risk:           risk,
		RequirementIDs: ids,
		LocalCheck:     in.Settings.LocalCheck,
		DevCheck:       in.Settings.DevCheck,
	})
	if err != nil {
		return board.Card{}, err
	}
	return card, nil
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
