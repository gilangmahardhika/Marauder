package board

import (
	"context"
	"database/sql"
	"errors"
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
			prompt_hash, system_hash, tool_profile, max_turns, base_sha, started_at, project_id
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, 'implementer', $9, $10, $11, (SELECT project_id FROM cards WHERE id = $2)
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
		INSERT INTO failure_notes (id, card_id, attempt_id, source, note, created_at, project_id)
		VALUES ($1, $2, $3, $4, $5, $6, (SELECT project_id FROM cards WHERE id = $2))`,
		id, cardID, attemptID, source, note, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// CurrentInstructions returns the card's newest instructions note, or "" when
// it has none or the newest one cleared them (RFC-073).
func CurrentInstructions(ctx context.Context, q DBTX, cardID string) (string, error) {
	var note string
	err := q.QueryRowContext(ctx, `
		SELECT note FROM failure_notes WHERE card_id = $1 AND source = 'instructions'
		ORDER BY created_at DESC LIMIT 1`, cardID).Scan(&note)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return note, err
}

// SaveInstructions stores text as the card's newest instructions note. A save
// in the same second as the current note replaces its text, since created_at
// has one-second resolution and could not order the two.
func SaveInstructions(ctx context.Context, q DBTX, cardID, text string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := q.ExecContext(ctx, `
		UPDATE failure_notes SET note = $2
		WHERE card_id = $1 AND source = 'instructions' AND created_at = $3`, cardID, text, now)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return err
	}
	return InsertFailureNote(ctx, q, cardID, "instructions", "instructions", text)
}

func AcquireLock(ctx context.Context, q DBTX, cardID, holder string) error {
	res, err := q.ExecContext(ctx, `
		INSERT INTO locks (card_id, holder, acquired_at, project_id)
		VALUES ($1, $2, $3, (SELECT project_id FROM cards WHERE id = $1))
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

// SetAttemptOutcome records the outcome of an attempt that already ended.
func SetAttemptOutcome(ctx context.Context, q DBTX, id, outcome string) error {
	_, err := q.ExecContext(ctx, `UPDATE attempts SET outcome = $2 WHERE id = $1`, id, outcome)
	return err
}

// AddAttemptTokens adds one model call's token counts to the attempt.
func AddAttemptTokens(ctx context.Context, q DBTX, id string, input, output int) error {
	_, err := q.ExecContext(ctx, `
		UPDATE attempts SET input_tokens = input_tokens + $2, output_tokens = output_tokens + $3 WHERE id = $1`,
		id, input, output)
	return err
}

// UnpublishedCommit returns the card's latest attempt when it ended with a
// commit that has no pull request yet: the gating check never ran on it (no
// outcome), or it passed and the publish failed (checks_passed while the card
// has no pull request). It returns a zero Attempt when there is none.
func UnpublishedCommit(ctx context.Context, q DBTX, cardID string) (Attempt, error) {
	var a Attempt
	var commit, outcome, ended sql.NullString
	var prURL string
	err := q.QueryRowContext(ctx, `
		SELECT a.id, a.n, COALESCE(a.model_id, ''), a.commit_sha, a.outcome, a.ended_at, COALESCE(c.pr_url, '')
		FROM attempts a JOIN cards c ON c.id = a.card_id
		WHERE a.card_id = $1 ORDER BY a.n DESC LIMIT 1`, cardID).Scan(&a.ID, &a.N, &a.ModelID, &commit, &outcome, &ended, &prURL)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, nil
	}
	if err != nil {
		return Attempt{}, err
	}
	if !commit.Valid || commit.String == "" || !ended.Valid {
		return Attempt{}, nil
	}
	if outcome.Valid && (outcome.String != "checks_passed" || prURL != "") {
		return Attempt{}, nil
	}
	a.CardID = cardID
	a.CommitSHA = commit.String
	a.Outcome = outcome.String
	return a, nil
}

// Review is the review state of the card's latest attempt that passed its
// gating check, the attempt the pull request carries.
type Review struct {
	AttemptID   string
	ModelID     string
	CommitSHA   string
	Verdict     string
	ReviewedSHA string
	Independent sql.NullBool
}

// LatestReview returns the review state of the card's latest checks_passed
// attempt, or a zero Review when there is none.
func LatestReview(ctx context.Context, q DBTX, cardID string) (Review, error) {
	var r Review
	err := q.QueryRowContext(ctx, `
		SELECT id, COALESCE(model_id, ''), COALESCE(commit_sha, ''), COALESCE(review_verdict, ''),
			COALESCE(reviewed_sha, ''), review_independent
		FROM attempts WHERE card_id = $1 AND outcome = 'checks_passed'
		ORDER BY n DESC LIMIT 1`, cardID).Scan(&r.AttemptID, &r.ModelID, &r.CommitSHA, &r.Verdict, &r.ReviewedSHA, &r.Independent)
	if errors.Is(err, sql.ErrNoRows) {
		return Review{}, nil
	}
	return r, err
}

// RecordReview stores the review of an attempt (RFC-065).
func RecordReview(ctx context.Context, q DBTX, attemptID, model, promptHash string, independent bool, verdict, reviewedSHA string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE attempts SET review_model_id = $2, review_prompt_hash = $3, review_independent = $4,
			review_verdict = $5, reviewed_sha = $6
		WHERE id = $1`, attemptID, model, promptHash, independent, verdict, reviewedSHA)
	return err
}

// ClearReview removes the review of an attempt, so the next review job
// records a fresh one (RFC-076).
func ClearReview(ctx context.Context, q DBTX, attemptID string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE attempts SET review_model_id = NULL, review_prompt_hash = NULL, review_independent = NULL,
			review_verdict = NULL, reviewed_sha = NULL
		WHERE id = $1`, attemptID)
	return err
}

// AttemptSummary is what the trace shows for one attempt. Empty strings are
// values the attempt does not have yet.
type AttemptSummary struct {
	ID                string
	N                 int
	TaskClass         string
	ModelID           string
	FallbackModelID   string
	InputTokens       int
	OutputTokens      int
	MaxTurns          int
	CommitSHA         string
	ReviewModelID     string
	ReviewVerdict     string
	ReviewIndependent sql.NullBool
	StartedAt         string
	EndedAt           string
	Outcome           string
}

// CardAttempts lists the card's attempts in the project, first attempt first.
func CardAttempts(ctx context.Context, db *sql.DB, projectID, cardID string) ([]AttemptSummary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, n, task_class, COALESCE(model_id, ''), COALESCE(fallback_model_id, ''),
			input_tokens, output_tokens, COALESCE(max_turns, 0), COALESCE(commit_sha, ''), COALESCE(review_model_id, ''),
			COALESCE(review_verdict, ''), review_independent, started_at, COALESCE(ended_at, ''),
			COALESCE(outcome, '')
		FROM attempts WHERE card_id = $1 AND project_id = $2 ORDER BY n`, cardID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := []AttemptSummary{}
	for rows.Next() {
		var a AttemptSummary
		if err := rows.Scan(&a.ID, &a.N, &a.TaskClass, &a.ModelID, &a.FallbackModelID,
			&a.InputTokens, &a.OutputTokens, &a.MaxTurns, &a.CommitSHA, &a.ReviewModelID,
			&a.ReviewVerdict, &a.ReviewIndependent, &a.StartedAt, &a.EndedAt, &a.Outcome); err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

// Note is one failure note on a card. AttemptN is 0 when the note belongs to
// no attempt, such as an approver's request for changes.
type Note struct {
	Source    string
	Note      string
	CreatedAt string
	AttemptN  int
}

// CardNotes lists the card's failure notes in the project, newest first.
func CardNotes(ctx context.Context, db *sql.DB, projectID, cardID string, limit int) ([]Note, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT f.source, f.note, f.created_at, COALESCE(a.n, 0)
		FROM failure_notes f
		LEFT JOIN attempts a ON a.id = f.attempt_id AND a.project_id = f.project_id
		WHERE f.card_id = $1 AND f.project_id = $2
		ORDER BY f.created_at DESC, a.n DESC NULLS LAST, f.id DESC
		LIMIT $3`, cardID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.Source, &n.Note, &n.CreatedAt, &n.AttemptN); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// SetCIRequired records whether the pull request head had checks when the
// review finished.
func SetCIRequired(ctx context.Context, q DBTX, attemptID string, required bool) error {
	_, err := q.ExecContext(ctx, `UPDATE attempts SET ci_required = $2 WHERE id = $1`, attemptID, required)
	return err
}

// PublishedAt returns when the card last entered pr_open, the time of its
// latest local_checks gate, or the zero time when it has none.
func PublishedAt(ctx context.Context, q DBTX, cardID string) (time.Time, error) {
	var at string
	err := q.QueryRowContext(ctx, `
		SELECT created_at FROM gates WHERE card_id = $1 AND name = 'local_checks'
		ORDER BY created_at DESC LIMIT 1`, cardID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, at)
}

// SetHeadSHA stores the pull request head read from GitHub.
func SetHeadSHA(ctx context.Context, q DBTX, cardID, sha string) error {
	_, err := q.ExecContext(ctx, `UPDATE cards SET head_sha = $2 WHERE id = $1`, cardID, sha)
	return err
}

// OpenAttempt reports whether the card has an attempt that has not ended.
func OpenAttempt(ctx context.Context, q DBTX, cardID string) (bool, error) {
	var open bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM attempts WHERE card_id = $1 AND ended_at IS NULL)`, cardID).Scan(&open)
	return open, err
}

// SetLocalCheck stores the card's local check command.
func SetLocalCheck(ctx context.Context, q DBTX, cardID, command string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE cards SET local_check = $2, updated_at = $3 WHERE id = $1`,
		cardID, command, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ReconcileProject ends the project's open attempts as cancelled and deletes
// its card locks. Call it only when no job of the project is alive.
func ReconcileProject(ctx context.Context, q DBTX, projectID string) (int, error) {
	res, err := q.ExecContext(ctx, `
		UPDATE attempts SET ended_at = $2, outcome = 'cancelled'
		WHERE ended_at IS NULL AND card_id IN (SELECT id FROM cards WHERE project_id = $1)`,
		projectID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	_, err = q.ExecContext(ctx, `
		DELETE FROM locks WHERE card_id IN (SELECT id FROM cards WHERE project_id = $1)`, projectID)
	return int(n), err
}
