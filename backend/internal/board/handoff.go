package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Todo is one item of an attempt's todo list.
type Todo struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

// Handoff is what an attempt that ended without a commit leaves the card's
// next attempt (RFC-118): a summary, its todo list, and a patch of its
// uncommitted changes against PatchBaseSHA. The API never returns one.
type Handoff struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	CardID        string `json:"cardId"`
	AttemptID     string `json:"attemptId"`
	Summary       string `json:"summary"`
	SummarySource string `json:"summarySource"`
	Todos         []Todo `json:"todos"`
	Patch         string `json:"patch"`
	PatchBaseSHA  string `json:"patchBaseSha"`
	PatchFiles    int    `json:"patchFiles"`
	CreatedAt     string `json:"createdAt"`
}

// SaveHandoff stores an attempt's handoff and returns its id. An attempt has
// at most one; saving again keeps the first.
func SaveHandoff(ctx context.Context, q DBTX, h Handoff) (string, error) {
	id, err := NewID("hof_")
	if err != nil {
		return "", err
	}
	todos := h.Todos
	if todos == nil {
		todos = []Todo{}
	}
	todosJSON, err := json.Marshal(todos)
	if err != nil {
		return "", err
	}
	res, err := q.ExecContext(ctx, `
		INSERT INTO attempt_handoffs (
			id, project_id, card_id, attempt_id, summary, summary_source, todos_json, patch, patch_base_sha, patch_files, created_at
		) SELECT $1, c.project_id, c.id, a.id, $4, $5, $6, $7, $8, $9, $10
		FROM attempts a JOIN cards c ON c.id = a.card_id
		WHERE a.id = $3 AND c.id = $2
		ON CONFLICT (attempt_id) DO NOTHING`,
		id, h.CardID, h.AttemptID, h.Summary, h.SummarySource, string(todosJSON), h.Patch, h.PatchBaseSHA, h.PatchFiles,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	if n, err := res.RowsAffected(); err != nil {
		return "", err
	} else if n != 1 {
		return "", ErrConflict
	}
	return id, nil
}

// PendingHandoff returns the handoff of the card's latest attempt when it is
// not yet applied. ok is false when there is none.
func PendingHandoff(ctx context.Context, q DBTX, cardID string) (Handoff, bool, error) {
	var h Handoff
	var todosJSON string
	err := q.QueryRowContext(ctx, `
		SELECT h.id, h.project_id, h.card_id, h.attempt_id, h.summary, h.summary_source, h.todos_json,
			h.patch, h.patch_base_sha, h.patch_files, h.created_at
		FROM attempt_handoffs h
		WHERE h.card_id = $1 AND h.applied_at IS NULL
			AND h.attempt_id = (SELECT id FROM attempts WHERE card_id = $1 ORDER BY n DESC LIMIT 1)`, cardID).Scan(
		&h.ID, &h.ProjectID, &h.CardID, &h.AttemptID, &h.Summary, &h.SummarySource, &todosJSON,
		&h.Patch, &h.PatchBaseSHA, &h.PatchFiles, &h.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Handoff{}, false, nil
	}
	if err != nil {
		return Handoff{}, false, err
	}
	if err := json.Unmarshal([]byte(todosJSON), &h.Todos); err != nil {
		return Handoff{}, false, fmt.Errorf("handoff %s todos: %w", h.ID, err)
	}
	return h, true, nil
}

// MarkHandoffApplied records that the card's next attempt used the handoff,
// whether its patch applied or not. Zero rows means it was already used or is
// not the card's, which is a conflict.
func MarkHandoffApplied(ctx context.Context, q DBTX, cardID, id string, ok bool, reason string) error {
	res, err := q.ExecContext(ctx, `
		UPDATE attempt_handoffs SET applied_at = $1, applied_ok = $2, applied_reason = $3
		WHERE id = $4 AND card_id = $5 AND applied_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), ok, reason, id, cardID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}
