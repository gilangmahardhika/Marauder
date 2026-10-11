package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/settings"
)

type Result struct {
	Created  int
	Promoted int
	Started  int
}

func (r Result) Changed() bool {
	return r.Created > 0 || r.Promoted > 0
}

// Pass runs one model-free worker pass for a project (RFC-062): promote
// backlog cards, make one card per accepted requirement without a card, and
// close the project's run requests once nothing changed. Card-scoped requests
// (RFC-061) stay open for the implement job.
func Pass(ctx context.Context, db *sql.DB, dataRoot, projectID string) (Result, error) {
	var result Result
	promoted, err := promote(ctx, db, projectID)
	if err != nil {
		return result, err
	}
	result.Promoted = promoted
	created, err := plan(ctx, db, dataRoot, projectID)
	if err != nil {
		return result, err
	}
	result.Created = created
	if !result.Changed() {
		if _, err := db.ExecContext(ctx, `
			UPDATE runs SET processed_at = $1
			WHERE project_id = $2 AND card_id IS NULL AND processed_at IS NULL`,
			time.Now().UTC().Format(time.RFC3339), projectID); err != nil {
			return result, err
		}
	}
	return result, nil
}

// Pending reports whether the project has an open run request that is not
// scoped to one card.
func Pending(ctx context.Context, db *sql.DB, projectID string) (bool, error) {
	var open bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM runs WHERE project_id = $1 AND card_id IS NULL AND processed_at IS NULL)`,
		projectID).Scan(&open)
	return open, err
}

// Wake reports whether the project loop has work: any open run request, for
// the whole project or for one card.
func Wake(ctx context.Context, db *sql.DB, projectID string) (bool, error) {
	var open bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM runs WHERE project_id = $1 AND processed_at IS NULL)`,
		projectID).Scan(&open)
	return open, err
}

func promote(ctx context.Context, db *sql.DB, projectID string) (int, error) {
	backlog, err := ids(ctx, db, `SELECT id FROM cards WHERE project_id = $1 AND status = 'backlog' ORDER BY created_at`, projectID)
	if err != nil {
		return 0, err
	}
	if len(backlog) == 0 {
		return 0, nil
	}
	doneIDs, err := ids(ctx, db, `SELECT id FROM cards WHERE project_id = $1 AND status = 'done'`, projectID)
	if err != nil {
		return 0, err
	}
	done := make(map[string]bool, len(doneIDs))
	for _, id := range doneIDs {
		done[id] = true
	}
	n := 0
	for _, id := range backlog {
		card, err := board.GetCard(ctx, db, id)
		if err != nil {
			return n, err
		}
		if !CanStart(done, card.DependsOn) {
			continue
		}
		ok, err := promotable(ctx, db, card)
		if err != nil {
			return n, err
		}
		if !ok {
			continue
		}
		moved, err := toReady(ctx, db, card)
		if err != nil {
			return n, err
		}
		if moved {
			n++
		}
	}
	return n, nil
}

// promotable is true when every requirement of the card is accepted in its
// project, and the card is manual or one requirement has acceptance lines.
func promotable(ctx context.Context, db *sql.DB, card board.Card) (bool, error) {
	if len(card.RequirementIDs) == 0 {
		return card.Manual, nil
	}
	lines := card.Manual
	for _, id := range card.RequirementIDs {
		var accepted sql.NullString
		var acceptance string
		err := db.QueryRowContext(ctx, `
			SELECT accepted_at, acceptance_json FROM requirements WHERE project_id = $1 AND id = $2`,
			card.ProjectID, id).Scan(&accepted, &acceptance)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !accepted.Valid || accepted.String == "" {
			return false, nil
		}
		var items []any
		if json.Unmarshal([]byte(acceptance), &items) == nil && len(items) > 0 {
			lines = true
		}
	}
	return lines, nil
}

func toReady(ctx context.Context, db *sql.DB, card board.Card) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := machine.Transition(ctx, tx, card, "ready", machine.Evidence{}, nil); err != nil {
		if errors.Is(err, board.ErrConflict) {
			return false, nil
		}
		return false, err
	}
	return true, tx.Commit()
}

func plan(ctx context.Context, db *sql.DB, dataRoot, projectID string) (int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT r.id, r.heading FROM requirements r
		WHERE r.project_id = $1 AND r.accepted_at IS NOT NULL AND r.accepted_at <> ''
		  AND NOT EXISTS (
		    SELECT 1 FROM cards c
		    WHERE c.project_id = r.project_id AND c.requirement_ids_json::jsonb ? r.id)
		ORDER BY r.id`, projectID)
	if err != nil {
		return 0, err
	}
	type pending struct{ id, heading string }
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.heading); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(todo) == 0 {
		return 0, nil
	}
	cardSettings, err := snapshot(dataRoot, projectID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range todo {
		title := p.heading
		if title == "" {
			title = p.id
		}
		_, err := loop.CreateCard(ctx, loop.CreateInput{
			DB: db, ProjectID: projectID, Settings: cardSettings,
			Args: map[string]any{"title": title, "risk": "low", "requirementIds": []string{p.id}},
		})
		if errors.Is(err, loop.ErrRequirement) {
			continue
		}
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func snapshot(dataRoot, projectID string) (loop.Settings, error) {
	if dataRoot == "" {
		return loop.Settings{}, nil
	}
	project, _, err := settings.LoadProject(dataRoot, projectID)
	if err != nil {
		return loop.Settings{}, err
	}
	global, _, err := settings.Load(dataRoot)
	if err != nil {
		return loop.Settings{}, err
	}
	return loop.Settings{
		LocalCheck:  project.Checks.Local,
		DevCheck:    project.Checks.Dev,
		MaxAttempts: global.Router.Budgets.MaxAttempts,
	}, nil
}

func ids(ctx context.Context, db *sql.DB, query, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
