package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/worker"
)

var ErrNotRunnable = errors.New("only a ready card whose dependencies are done can run")

type querier interface {
	board.DBTX
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// CardRunState reports whether a card has an open run request and which of
// its dependencies are not done yet.
func CardRunState(ctx context.Context, db querier, card board.Card) (bool, []string, error) {
	waitingOn := []string{}
	if len(card.DependsOn) > 0 {
		done, err := doneCards(ctx, db, card.ProjectID)
		if err != nil {
			return false, nil, err
		}
		for _, dep := range card.DependsOn {
			if !done[dep] {
				waitingOn = append(waitingOn, dep)
			}
		}
	}
	var queued bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM runs WHERE card_id = $1 AND processed_at IS NULL)`, card.ID).Scan(&queued); err != nil {
		return false, nil, err
	}
	return queued, waitingOn, nil
}

// QueueCardRun records one run request for a ready card. A card that is
// already queued keeps its single open request.
func QueueCardRun(ctx context.Context, db *sql.DB, projectID, cardID string) (board.Card, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	var found string
	err = tx.QueryRowContext(ctx, `SELECT id FROM cards WHERE id = $1 AND project_id = $2 FOR UPDATE`, cardID, projectID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return board.Card{}, board.ErrNotFound
	}
	if err != nil {
		return board.Card{}, err
	}
	card, err := board.GetCard(ctx, tx, cardID)
	if err != nil {
		return board.Card{}, err
	}
	done, err := doneCards(ctx, tx, projectID)
	if err != nil {
		return card, err
	}
	if !runnable(card, done) {
		return card, ErrNotRunnable
	}
	if _, err := insertCardRun(ctx, tx, projectID, cardID); err != nil {
		return card, err
	}
	return card, tx.Commit()
}

// QueueReadyRuns records a run request for every ready card of the project
// whose dependencies are done, and returns how many it added.
func QueueReadyRuns(ctx context.Context, db *sql.DB, projectID string) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	ids, err := cardIDs(ctx, tx, `
		SELECT id FROM cards WHERE project_id = $1 AND status = 'ready' ORDER BY updated_at FOR UPDATE`, projectID)
	if err != nil {
		return 0, err
	}
	done, err := doneCards(ctx, tx, projectID)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, id := range ids {
		card, err := board.GetCard(ctx, tx, id)
		if err != nil {
			return 0, err
		}
		if !runnable(card, done) {
			continue
		}
		added, err := insertCardRun(ctx, tx, projectID, id)
		if err != nil {
			return 0, err
		}
		if added {
			queued++
		}
	}
	return queued, tx.Commit()
}

// ReadyCount counts the project's ready cards that can run and are not queued.
func ReadyCount(ctx context.Context, db querier, projectID string) (int, error) {
	ids, err := cardIDs(ctx, db, `
		SELECT c.id FROM cards c
		WHERE c.project_id = $1 AND c.status = 'ready'
		  AND NOT EXISTS (SELECT 1 FROM runs r WHERE r.card_id = c.id AND r.processed_at IS NULL)`, projectID)
	if err != nil {
		return 0, err
	}
	done, err := doneCards(ctx, db, projectID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		card, err := board.GetCard(ctx, db, id)
		if err != nil {
			return 0, err
		}
		if runnable(card, done) {
			n++
		}
	}
	return n, nil
}

func runnable(card board.Card, done map[string]bool) bool {
	return card.Status == "ready" && worker.CanStart(done, card.DependsOn)
}

func insertCardRun(ctx context.Context, q querier, projectID, cardID string) (bool, error) {
	res, err := q.ExecContext(ctx, `
		INSERT INTO runs (id, trigger, created_at, project_id, card_id) VALUES ($1, 'api', $2, $3, $4)
		ON CONFLICT (card_id) WHERE card_id IS NOT NULL AND processed_at IS NULL DO NOTHING`,
		mustID("run_"), time.Now().UTC().Format(time.RFC3339), projectID, cardID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func doneCards(ctx context.Context, q querier, projectID string) (map[string]bool, error) {
	ids, err := cardIDs(ctx, q, `SELECT id FROM cards WHERE project_id = $1 AND status = 'done'`, projectID)
	if err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(ids))
	for _, id := range ids {
		done[id] = true
	}
	return done, nil
}

func cardIDs(ctx context.Context, q querier, query, projectID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
