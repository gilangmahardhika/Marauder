package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/machine"
)

var (
	ErrSHA      = errors.New("sha does not match the current head")
	ErrApprover = errors.New("operator cannot approve")
	ErrUnblock  = errors.New("unblock is refused")
	// ErrOwnerApprove refuses an approval by the card owner (RFC-054).
	ErrOwnerApprove = errors.New("the card owner cannot approve their own card")
)

type GateConfig struct {
	Operator  string
	BotLogin  string
	Approvers []string
	ActorID   string
}

func Approve(ctx context.Context, db *sql.DB, cardID, sha string, cfg GateConfig) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	if card.OwnerID != "" && cfg.ActorID == card.OwnerID {
		return card, ErrOwnerApprove
	}
	if sha == "" || sha != card.HeadSHA {
		return card, ErrSHA
	}
	if !canApprove(cfg) {
		return card, ErrApprover
	}
	var existing int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM approvals WHERE card_id = $1 AND sha = $2 AND cleared_at IS NULL`,
		cardID, sha).Scan(&existing); err != nil {
		return card, err
	}
	if existing > 0 {
		return card, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return card, err
	}
	defer tx.Rollback()
	next, err := machine.Transition(ctx, tx, card, "approved", machine.Evidence{SHA: sha, Approver: cfg.Operator, ActorID: cfg.ActorID}, nil)
	if err != nil {
		if errors.Is(err, machine.ErrIllegal) || errors.Is(err, machine.ErrEvidence) {
			_ = tx.Commit()
		}
		return card, err
	}
	id, err := board.NewID("apr_")
	if err != nil {
		return card, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approvals (id, card_id, sha, approver, created_at, project_id) VALUES ($1, $2, $3, $4, $5, $6)`,
		id, cardID, sha, cfg.Operator, now, next.ProjectID); err != nil {
		return card, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runs (id, trigger, created_at, project_id) VALUES ($1, 'approve', $2, $3)`,
		mustID("run_"), now, next.ProjectID); err != nil {
		return card, err
	}
	if err := tx.Commit(); err != nil {
		return card, err
	}
	return next, nil
}

func RequestChanges(ctx context.Context, db *sql.DB, cardID string, cfg GateConfig) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	if !canApprove(cfg) {
		return card, ErrApprover
	}
	next, err := transition(ctx, db, card, "changes_requested", machine.Evidence{Approver: cfg.Operator})
	if err != nil {
		return card, err
	}
	_ = board.InsertFailureNote(ctx, db, cardID, "approver", "approver", "changes requested")
	return next, nil
}

func Reject(ctx context.Context, db *sql.DB, cardID string, cfg GateConfig) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	if !canApprove(cfg) {
		return card, ErrApprover
	}
	return transition(ctx, db, card, "blocked", machine.Evidence{Reason: "rejected", Approver: cfg.Operator})
}

func Deploy(ctx context.Context, db *sql.DB, cardID, actorID string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	return transition(ctx, db, card, "deploying_dev", machine.Evidence{SHA: card.ApprovedSHA, ActorID: actorID})
}

func HeadMoved(ctx context.Context, db *sql.DB, cardID, head string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	card.HeadSHA = head
	if _, err := db.ExecContext(ctx, `UPDATE cards SET head_sha = $2 WHERE id = $1`, cardID, head); err != nil {
		return card, err
	}
	if card.Status == "approved" && card.ApprovedSHA != "" && card.ApprovedSHA != head {
		return transition(ctx, db, card, "human_review", machine.Evidence{SHA: head})
	}
	return board.GetCard(ctx, db, cardID)
}

func Unblock(ctx context.Context, db *sql.DB, cardID string, reset bool) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	dest, ok := UnblockTo(ctx, db, card)
	if !ok {
		return card, ErrUnblock
	}
	if reset {
		if _, err := db.ExecContext(ctx, `UPDATE cards SET attempts_used = 0 WHERE id = $1`, cardID); err != nil {
			return card, err
		}
		card.AttemptsUsed = 0
	} else if card.AttemptsUsed >= card.MaxAttempts {
		if _, err := db.ExecContext(ctx, `UPDATE cards SET max_attempts = max_attempts + 1 WHERE id = $1`, cardID); err != nil {
			return card, err
		}
		card.MaxAttempts++
	}
	return transition(ctx, db, card, dest, machine.Evidence{Reason: card.BlockedReason})
}

func UnblockTo(ctx context.Context, db *sql.DB, card board.Card) (string, bool) {
	if card.BlockedReason == "source_changed" && linkedUnaccepted(ctx, db, card) {
		return "", false
	}
	switch card.BlockedReason {
	case "health", "deploy_failed", "deploy_config":
		return "human_review", true
	default:
		return "ready", true
	}
}

func AtCap(ctx context.Context, db *sql.DB, cardID string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return card, err
	}
	if card.Status != "changes_requested" && card.Status != "in_progress" {
		return card, nil
	}
	if card.AttemptsUsed < card.MaxAttempts {
		return card, nil
	}
	return transition(ctx, db, card, "blocked", machine.Evidence{Reason: "attempt_cap"})
}

func linkedUnaccepted(ctx context.Context, db *sql.DB, card board.Card) bool {
	for _, id := range card.RequirementIDs {
		var accepted sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT accepted_at FROM requirements WHERE id = $1`, id).Scan(&accepted); err != nil || !accepted.Valid {
			return true
		}
	}
	return false
}

func canApprove(cfg GateConfig) bool {
	if cfg.Operator == "" || cfg.Operator == cfg.BotLogin {
		return false
	}
	for _, login := range cfg.Approvers {
		if login == cfg.Operator {
			return true
		}
	}
	return false
}

func transition(ctx context.Context, db *sql.DB, card board.Card, to string, ev machine.Evidence) (board.Card, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return card, err
	}
	defer tx.Rollback()
	next, err := machine.Transition(ctx, tx, card, to, ev, nil)
	if err != nil {
		if errors.Is(err, machine.ErrIllegal) || errors.Is(err, machine.ErrEvidence) {
			_ = tx.Commit()
		}
		return card, err
	}
	if err := tx.Commit(); err != nil {
		return card, err
	}
	return next, nil
}

func mustID(prefix string) string {
	id, err := board.NewID(prefix)
	if err != nil {
		return prefix + "000000000000"
	}
	return id
}
