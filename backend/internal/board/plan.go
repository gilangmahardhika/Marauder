package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PlanStep is one checklist item of a card plan (RFC-129).
type PlanStep struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Detail string `json:"detail"`
}

// Plan is one card_plans row.
type Plan struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"projectId"`
	CardID     string     `json:"cardId"`
	AttemptID  string     `json:"attemptId"`
	AttemptN   int        `json:"attemptN"`
	Title      string     `json:"title"`
	Steps      []PlanStep `json:"steps"`
	Status     string     `json:"status"`
	AcceptedBy string     `json:"acceptedBy"`
	AcceptedAt string     `json:"acceptedAt"`
	CreatedAt  string     `json:"createdAt"`
}

// HasAcceptedPlan reports whether the card has any accepted plan.
func HasAcceptedPlan(ctx context.Context, q DBTX, cardID string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `
		SELECT count(*) FROM card_plans WHERE card_id = $1 AND status = 'accepted'`, cardID).Scan(&n)
	return n > 0, err
}

// OpenPlan returns the card's open plan, or ErrNotFound.
func OpenPlan(ctx context.Context, q DBTX, projectID, cardID string) (Plan, error) {
	return scanPlan(ctx, q, `
		SELECT p.id, p.project_id, p.card_id, p.attempt_id, COALESCE(a.n, 0), p.title, p.steps_json,
			p.status, COALESCE(p.accepted_by, ''), COALESCE(p.accepted_at, ''), p.created_at
		FROM card_plans p LEFT JOIN attempts a ON a.id = p.attempt_id
		WHERE p.card_id = $1 AND p.project_id = $2 AND p.status = 'open'`, cardID, projectID)
}

// CardPlanForDisplay returns the open plan, else the latest accepted plan, or
// ErrNotFound when the card has none.
func CardPlanForDisplay(ctx context.Context, q DBTX, projectID, cardID string) (Plan, error) {
	plan, err := OpenPlan(ctx, q, projectID, cardID)
	if err == nil {
		return plan, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Plan{}, err
	}
	return scanPlan(ctx, q, `
		SELECT p.id, p.project_id, p.card_id, p.attempt_id, COALESCE(a.n, 0), p.title, p.steps_json,
			p.status, COALESCE(p.accepted_by, ''), COALESCE(p.accepted_at, ''), p.created_at
		FROM card_plans p LEFT JOIN attempts a ON a.id = p.attempt_id
		WHERE p.card_id = $1 AND p.project_id = $2 AND p.status = 'accepted'
		ORDER BY p.accepted_at DESC NULLS LAST, p.created_at DESC
		LIMIT 1`, cardID, projectID)
}

// AcceptedPlan returns the latest accepted plan for the pack, or ErrNotFound.
func AcceptedPlan(ctx context.Context, q DBTX, cardID string) (Plan, error) {
	return scanPlan(ctx, q, `
		SELECT p.id, p.project_id, p.card_id, p.attempt_id, COALESCE(a.n, 0), p.title, p.steps_json,
			p.status, COALESCE(p.accepted_by, ''), COALESCE(p.accepted_at, ''), p.created_at
		FROM card_plans p LEFT JOIN attempts a ON a.id = p.attempt_id
		WHERE p.card_id = $1 AND p.status = 'accepted'
		ORDER BY p.accepted_at DESC NULLS LAST, p.created_at DESC
		LIMIT 1`, cardID)
}

func scanPlan(ctx context.Context, q DBTX, query string, args ...any) (Plan, error) {
	var plan Plan
	var stepsJSON string
	err := q.QueryRowContext(ctx, query, args...).Scan(
		&plan.ID, &plan.ProjectID, &plan.CardID, &plan.AttemptID, &plan.AttemptN,
		&plan.Title, &stepsJSON, &plan.Status, &plan.AcceptedBy, &plan.AcceptedAt, &plan.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	if err := json.Unmarshal([]byte(stepsJSON), &plan.Steps); err != nil {
		return Plan{}, fmt.Errorf("plan %s steps: %w", plan.ID, err)
	}
	if plan.Steps == nil {
		plan.Steps = []PlanStep{}
	}
	return plan, nil
}

// InsertOpenPlan replaces any open plan of the card with a new open row.
func InsertOpenPlan(ctx context.Context, q DBTX, cardID, attemptID, title string, steps []PlanStep) (Plan, error) {
	if _, err := q.ExecContext(ctx, `
		DELETE FROM card_plans WHERE card_id = $1 AND status = 'open'`, cardID); err != nil {
		return Plan{}, err
	}
	id, err := NewID("pln_")
	if err != nil {
		return Plan{}, err
	}
	steps = ensureStepIDs(steps)
	raw, err := json.Marshal(steps)
	if err != nil {
		return Plan{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var projectID string
	if err := q.QueryRowContext(ctx, `SELECT project_id FROM cards WHERE id = $1`, cardID).Scan(&projectID); err != nil {
		return Plan{}, err
	}
	if _, err := q.ExecContext(ctx, `
		INSERT INTO card_plans (
			id, project_id, card_id, attempt_id, title, steps_json, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'open', $7)`,
		id, projectID, cardID, attemptID, title, string(raw), now,
	); err != nil {
		return Plan{}, err
	}
	return Plan{
		ID: id, ProjectID: projectID, CardID: cardID, AttemptID: attemptID,
		Title: title, Steps: steps, Status: "open", CreatedAt: now,
	}, nil
}

func ensureStepIDs(steps []PlanStep) []PlanStep {
	out := make([]PlanStep, len(steps))
	for i, step := range steps {
		out[i] = step
		if out[i].ID == "" {
			id, err := NewID("stp_")
			if err != nil {
				out[i].ID = fmt.Sprintf("stp_%02d", i+1)
			} else {
				out[i].ID = id
			}
		}
	}
	return out
}

// UpdateOpenPlan stores new title and steps on the open plan.
func UpdateOpenPlan(ctx context.Context, q DBTX, projectID, cardID, title string, steps []PlanStep) (Plan, error) {
	steps = ensureStepIDs(steps)
	raw, err := json.Marshal(steps)
	if err != nil {
		return Plan{}, err
	}
	res, err := q.ExecContext(ctx, `
		UPDATE card_plans SET title = $1, steps_json = $2
		WHERE card_id = $3 AND project_id = $4 AND status = 'open'`,
		title, string(raw), cardID, projectID)
	if err != nil {
		return Plan{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Plan{}, err
	}
	if n != 1 {
		return Plan{}, ErrConflict
	}
	return OpenPlan(ctx, q, projectID, cardID)
}

// AcceptOpenPlan marks the open plan accepted with the given steps and actor.
func AcceptOpenPlan(ctx context.Context, q DBTX, projectID, cardID, title string, steps []PlanStep, userID string) (Plan, error) {
	steps = ensureStepIDs(steps)
	raw, err := json.Marshal(steps)
	if err != nil {
		return Plan{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := q.ExecContext(ctx, `
		UPDATE card_plans SET title = $1, steps_json = $2, status = 'accepted',
			accepted_by = $3, accepted_at = $4
		WHERE card_id = $5 AND project_id = $6 AND status = 'open'`,
		title, string(raw), userID, now, cardID, projectID)
	if err != nil {
		return Plan{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Plan{}, err
	}
	if n != 1 {
		return Plan{}, ErrConflict
	}
	return AcceptedPlan(ctx, q, cardID)
}

// DiscardOpenPlan deletes the card's open plan. Zero rows is ErrConflict.
func DiscardOpenPlan(ctx context.Context, q DBTX, projectID, cardID string) error {
	res, err := q.ExecContext(ctx, `
		DELETE FROM card_plans WHERE card_id = $1 AND project_id = $2 AND status = 'open'`,
		cardID, projectID)
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

// SetPlanMode stores the card's plan mode and returns the old one.
func SetPlanMode(ctx context.Context, q DBTX, projectID, cardID, mode string) (string, error) {
	var old string
	err := q.QueryRowContext(ctx, `
		SELECT plan_mode FROM cards WHERE id = $1 AND project_id = $2 FOR UPDATE`, cardID, projectID).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil || old == mode {
		return old, err
	}
	_, err = q.ExecContext(ctx, `
		UPDATE cards SET plan_mode = $1, updated_at = $2 WHERE id = $3 AND project_id = $4`,
		mode, time.Now().UTC().Format(time.RFC3339), cardID, projectID)
	return old, err
}
