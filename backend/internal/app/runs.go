package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
	"orchestrator/backend/internal/worker"
)

var ErrNotRunnable = errors.New("only a ready or in-progress card that is not running and whose dependencies are done can run")

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

// QueueCardRun records one run request for a card the implement job can start:
// ready, changes_requested, or in_progress with no open attempt (RFC-063). A
// card that is already queued keeps its single open request. Non-nil
// instructions are saved in the same transaction (RFC-073), so invalid ones
// queue nothing.
func QueueCardRun(ctx context.Context, db *sql.DB, projectID, cardID string, instructions *string, actorID string) (board.Card, error) {
	var text string
	if instructions != nil {
		var err error
		if text, err = cleanInstructions(*instructions); err != nil {
			return board.Card{}, err
		}
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
	done, err := doneCards(ctx, tx, projectID)
	if err != nil {
		return card, err
	}
	ok, err := CanStartImplement(ctx, tx, card)
	if err != nil {
		return card, err
	}
	if !ok || !worker.CanStart(done, card.DependsOn) {
		return card, ErrNotRunnable
	}
	if instructions != nil {
		if err := saveInstructions(ctx, tx, card, actorID, text); err != nil {
			return card, err
		}
	}
	if _, err := insertCardRun(ctx, tx, projectID, cardID, "api"); err != nil {
		return card, err
	}
	return card, tx.Commit()
}

const maxInstructions = 4000

// ErrCardDone refuses instructions for a card no attempt will run again.
var ErrCardDone = errors.New("a done card takes no instructions")

// SetInstructions stores the instructions every later attempt of the card
// reads next to its newest failure note, until they are replaced (RFC-073).
// Blank text clears them. Text equal to the current instructions writes nothing.
func SetInstructions(ctx context.Context, db *sql.DB, projectID, cardID, actorID, text string) (board.Card, error) {
	clean, err := cleanInstructions(text)
	if err != nil {
		return board.Card{}, err
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
	if err := saveInstructions(ctx, tx, card, actorID, clean); err != nil {
		return card, err
	}
	return card, tx.Commit()
}

func cleanInstructions(text string) (string, error) {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
	if utf8.RuneCountInString(text) > maxInstructions {
		msg := fmt.Sprintf("Keep the instructions to %d characters or fewer.", maxInstructions)
		return "", &settings.Invalid{Fields: map[string]string{"instructions": msg}}
	}
	return text, nil
}

func saveInstructions(ctx context.Context, tx *sql.Tx, card board.Card, actorID, text string) error {
	current, err := board.CurrentInstructions(ctx, tx, card.ID)
	if err != nil || current == text {
		return err
	}
	if err := board.SaveInstructions(ctx, tx, card.ID, text); err != nil {
		return err
	}
	_, err = trace.Append(ctx, tx, trace.Event{
		CardID: card.ID, ProjectID: card.ProjectID, ActorID: actorID, Kind: "instructions_saved",
		Payload: map[string]any{"length": utf8.RuneCountInString(text)},
	}, nil)
	return err
}

func lockCard(ctx context.Context, tx *sql.Tx, projectID, cardID string) (board.Card, error) {
	var found string
	err := tx.QueryRowContext(ctx, `SELECT id FROM cards WHERE id = $1 AND project_id = $2 FOR UPDATE`, cardID, projectID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return board.Card{}, board.ErrNotFound
	}
	if err != nil {
		return board.Card{}, err
	}
	return board.GetCard(ctx, tx, cardID)
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
		added, err := insertCardRun(ctx, tx, projectID, id, "api")
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

// RunNext records a CLI run request for the project. When no server holds the
// project's worker lock, it runs worker passes until idle, then runs the
// implement jobs of queued cards and waits for them; queued reports that a
// running server will process the request instead.
func RunNext(ctx context.Context, db *sql.DB, dataRoot, projectID string, jobs worker.Jobs, logf func(string, ...any)) (result worker.Result, queued bool, err error) {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runs (id, trigger, created_at, project_id) VALUES ($1, 'cli', $2, $3)`,
		mustID("run_"), time.Now().UTC().Format(time.RFC3339), projectID); err != nil {
		return result, false, err
	}
	lock, err := worker.LockProject(dataRoot, projectID)
	if errors.Is(err, worker.ErrBusy) {
		return result, true, nil
	}
	if err != nil {
		return result, false, err
	}
	defer lock.Close()
	if _, err := board.ReconcileProject(ctx, db, projectID); err != nil {
		return result, false, err
	}
	result, err = worker.Drain(ctx, db, dataRoot, projectID)
	if err != nil {
		return result, false, err
	}
	slots := worker.NewSlots()
	result.Started, err = worker.StartQueued(ctx, db, dataRoot, projectID, jobs, slots, logf)
	slots.Wait()
	return result, false, err
}

// ImplementJobs binds the implement, review, and refresh jobs to the database
// and job dependencies for the worker.
func ImplementJobs(db *sql.DB, deps JobDeps) worker.Jobs {
	bind := func(job func(context.Context, *sql.DB, JobDeps, string, string) (board.Card, error)) func(context.Context, string, string) (string, error) {
		return func(ctx context.Context, projectID, cardID string) (string, error) {
			card, err := job(ctx, db, deps, projectID, cardID)
			return card.Status, err
		}
	}
	return worker.Jobs{Implement: bind(ImplementCard), Review: bind(ReviewCard), Refresh: bind(RefreshCard)}
}

func runnable(card board.Card, done map[string]bool) bool {
	return card.Status == "ready" && worker.CanStart(done, card.DependsOn)
}

// retryOrCap ends an attempt that left the card in_progress or
// changes_requested: it blocks the card with attempt_cap when no attempts
// remain, and otherwise queues the next attempt with a retry request (RFC-071).
func retryOrCap(ctx context.Context, db *sql.DB, cardID string) (board.Card, error) {
	card, err := AtCap(ctx, db, cardID)
	if err != nil || (card.Status != "in_progress" && card.Status != "changes_requested") {
		return card, err
	}
	_, err = insertCardRun(ctx, db, card.ProjectID, card.ID, "retry")
	return card, err
}

func insertCardRun(ctx context.Context, q querier, projectID, cardID, trigger string) (bool, error) {
	res, err := q.ExecContext(ctx, `
		INSERT INTO runs (id, trigger, created_at, project_id, card_id) VALUES ($1, $5, $2, $3, $4)
		ON CONFLICT (card_id) WHERE card_id IS NOT NULL AND processed_at IS NULL DO NOTHING`,
		mustID("run_"), time.Now().UTC().Format(time.RFC3339), projectID, cardID, trigger)
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
