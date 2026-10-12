package review

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/machine"
)

// VerdictFailed is the review_verdict of an attempt whose review gave no
// verdict (RFC-069). The pull request checks still decide the card.
const VerdictFailed = "failed"

// NoChecksWait is how long after a card enters pr_open an empty check list
// still counts as checks that have not registered yet (RFC-069).
const NoChecksWait = 2 * time.Minute

type Check struct {
	Name  string
	State string
}

func ApplyCI(ctx context.Context, db *sql.DB, card board.Card, attemptID string, checks []Check, secrets []string) (board.Card, error) {
	var failed []string
	pending := false
	for _, check := range checks {
		switch check.State {
		case "failure", "fail":
			failed = append(failed, check.Name)
		case "pending":
			pending = true
		}
	}
	if len(failed) > 0 {
		if err := board.InsertFailureNote(ctx, db, card.ID, attemptID, "ci", "ci "+strings.Join(failed, ",")); err != nil {
			return card, err
		}
		return move(ctx, db, card, "changes_requested", machine.Evidence{Reason: "ci"}, secrets)
	}
	if pending {
		return card, nil
	}
	return move(ctx, db, card, "human_review", machine.Evidence{}, secrets)
}

func move(ctx context.Context, db *sql.DB, card board.Card, to string, ev machine.Evidence, secrets []string) (board.Card, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return card, err
	}
	defer tx.Rollback()
	next, err := machine.Transition(ctx, tx, card, to, ev, secrets)
	if err != nil {
		return card, err
	}
	if err := tx.Commit(); err != nil {
		return card, err
	}
	return next, nil
}
