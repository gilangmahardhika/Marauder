package verify

import (
	"context"
	"database/sql"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/trace"
)

func Finish(ctx context.Context, db *sql.DB, card board.Card, devExit int, devEmpty bool) (board.Card, error) {
	mode := "dev_check"
	if devEmpty {
		mode = "health_only"
	}
	if _, err := trace.Append(ctx, db, trace.Event{CardID: card.ID, Kind: "verify", Payload: map[string]any{
		"sha": card.DeployedSHA, "mode": mode, "exitCode": devExit,
	}}, nil); err != nil {
		return card, err
	}
	if devEmpty || devExit == 0 {
		return move(ctx, db, card, "done", machine.Evidence{SHA: card.DeployedSHA, ExitCode: &devExit})
	}
	_ = board.InsertFailureNote(ctx, db, card.ID, "dev", "dev_check", "dev check failed")
	return move(ctx, db, card, "changes_requested", machine.Evidence{ExitCode: &devExit, LogRef: "dev"})
}

func move(ctx context.Context, db *sql.DB, card board.Card, to string, ev machine.Evidence) (board.Card, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return card, err
	}
	defer tx.Rollback()
	next, err := machine.Transition(ctx, tx, card, to, ev, nil)
	if err != nil {
		return card, err
	}
	if err := tx.Commit(); err != nil {
		return card, err
	}
	return next, nil
}

func HealthOnly(ctx context.Context, db *sql.DB, card board.Card) (board.Card, error) {
	return Finish(ctx, db, card, 0, true)
}
