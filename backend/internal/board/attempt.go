package board

import (
	"context"
	"time"
)

type Attempt struct {
	ID         string
	CardID     string
	N          int
	ParentID   string
	TaskClass  string
	ModelID    string
	PromptHash string
	SystemHash string
	BaseSHA    string
	CommitSHA  string
	Outcome    string
	MaxTurns   int
}

func NextAttempt(ctx context.Context, q DBTX, cardID string) (int, string, error) {
	var n int
	var parent string
	err := q.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(n), 0), COALESCE((
			SELECT id FROM attempts WHERE card_id = $1 ORDER BY n DESC LIMIT 1
		), '')
		FROM attempts WHERE card_id = $1`, cardID).Scan(&n, &parent)
	if err != nil {
		return 0, "", err
	}
	return n + 1, parent, nil
}

func InsertAttempt(ctx context.Context, q DBTX, attempt Attempt) error {
	if attempt.ID == "" {
		id, err := NewID("att_")
		if err != nil {
			return err
		}
		attempt.ID = id
	}
	var parent any
	if attempt.ParentID != "" {
		parent = attempt.ParentID
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO attempts (
			id, card_id, n, parent_attempt_id, task_class, model_id,
			prompt_hash, system_hash, tool_profile, max_turns, base_sha, started_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, 'implementer', $9, $10, $11
		)`,
		attempt.ID, attempt.CardID, attempt.N, parent, attempt.TaskClass, attempt.ModelID,
		attempt.PromptHash, attempt.SystemHash, attempt.MaxTurns, attempt.BaseSHA,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func FinishAttempt(ctx context.Context, q DBTX, id, outcome, commitSHA string) error {
	var outcomeVal, commit any
	if outcome != "" {
		outcomeVal = outcome
	}
	if commitSHA != "" {
		commit = commitSHA
	}
	_, err := q.ExecContext(ctx, `
		UPDATE attempts SET ended_at = $1, outcome = $2, commit_sha = $3 WHERE id = $4`,
		time.Now().UTC().Format(time.RFC3339), outcomeVal, commit, id,
	)
	return err
}

func AddAttemptUsed(ctx context.Context, q DBTX, cardID string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE cards SET attempts_used = attempts_used + 1, updated_at = $2 WHERE id = $1`,
		cardID, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func StoreWorktree(ctx context.Context, q DBTX, cardID, branch, path, gitDir string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE cards
		SET branch = $2, worktree_path = $3, worktree_gitdir = $4, updated_at = $5
		WHERE id = $1`,
		cardID, branch, path, gitDir, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func InsertFailureNote(ctx context.Context, q DBTX, cardID, attemptID, source, note string) error {
	id, err := NewID("note_")
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO failure_notes (id, card_id, attempt_id, source, note, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		id, cardID, attemptID, source, note, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func AcquireLock(ctx context.Context, q DBTX, cardID, holder string) error {
	res, err := q.ExecContext(ctx, `
		INSERT INTO locks (card_id, holder, acquired_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (card_id) DO NOTHING`,
		cardID, holder, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLocked
	}
	return nil
}

func ReleaseLock(ctx context.Context, q DBTX, cardID, holder string) error {
	_, err := q.ExecContext(ctx, `
		DELETE FROM locks WHERE card_id = $1 AND holder = $2`,
		cardID, holder,
	)
	return err
}
