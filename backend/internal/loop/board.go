package loop

import (
	"context"
	"database/sql"
	"errors"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/trace"
)

// Board is every board read and write an implement attempt makes. DBBoard
// runs them on the database; a remote runner sends them to the control plane,
// which pins each to the lease card (RFC-108). There is no status write here:
// only the control plane moves a card.
type Board interface {
	GetCard(ctx context.Context, cardID string) (board.Card, error)
	StoreWorktree(ctx context.Context, cardID, branch, path, gitDir string) error
	AcquireLock(ctx context.Context, cardID, holder string) error
	ReleaseLock(ctx context.Context, cardID, holder string) error
	Requirements(ctx context.Context, projectID string, ids []string) ([]string, error)
	Decisions(ctx context.Context, projectID string, ids []string) ([]string, error)
	LatestNote(ctx context.Context, cardID string) (string, error)
	LatestReviewNote(ctx context.Context, cardID string) (string, error)
	Instructions(ctx context.Context, cardID string) (string, error)
	Conventions(ctx context.Context, projectID, cardID string) ([]contextpack.Convention, []map[string]any, error)
	NextAttempt(ctx context.Context, cardID string) (int, string, error)
	InsertAttempt(ctx context.Context, attempt board.Attempt) error
	FinishAttempt(ctx context.Context, id, outcome, commitSHA string) error
	AddAttemptUsed(ctx context.Context, cardID string) error
	InsertFailureNote(ctx context.Context, cardID, attemptID, source, note string) error
	AddAttemptTokens(ctx context.Context, id string, input, output, cached int) error
	CardTokens(ctx context.Context, cardID string) (int, error)
	AppendEvent(ctx context.Context, ev trace.Event, secrets []string) error
	// AskQuestions stores an attempt's questions numbered from first, answered
	// with their recommendations when auto is set (RFC-114, RFC-115).
	AskQuestions(ctx context.Context, attemptID, cardID string, first int, questions []board.Question, auto bool) error
	// Answers lists the card's answered questions, oldest first.
	Answers(ctx context.Context, cardID string) ([]board.Question, error)
	// SubmitPlan stores an open plan for the card, replacing any earlier open
	// plan (RFC-128).
	SubmitPlan(ctx context.Context, attemptID, cardID, title string, steps []board.PlanStep) error
	// ApprovedPlan returns the card's latest accepted plan, if any.
	ApprovedPlan(ctx context.Context, cardID string) (board.Plan, bool, error)
	// SaveHandoff stores what an attempt without a commit leaves the next
	// one; PendingHandoff reads the card's latest attempt's handoff while it
	// is unused, and MarkHandoffApplied uses it up (RFC-118).
	SaveHandoff(ctx context.Context, h board.Handoff) error
	PendingHandoff(ctx context.Context, cardID string) (board.Handoff, bool, error)
	MarkHandoffApplied(ctx context.Context, cardID, id string, ok bool, reason string) error
}

// DBBoard is the Board of a job that runs beside the database.
type DBBoard struct {
	DB *sql.DB
}

func (b DBBoard) GetCard(ctx context.Context, cardID string) (board.Card, error) {
	return board.GetCard(ctx, b.DB, cardID)
}

func (b DBBoard) StoreWorktree(ctx context.Context, cardID, branch, path, gitDir string) error {
	return board.StoreWorktree(ctx, b.DB, cardID, branch, path, gitDir)
}

func (b DBBoard) AcquireLock(ctx context.Context, cardID, holder string) error {
	return board.AcquireLock(ctx, b.DB, cardID, holder)
}

func (b DBBoard) ReleaseLock(ctx context.Context, cardID, holder string) error {
	return board.ReleaseLock(ctx, b.DB, cardID, holder)
}

func (b DBBoard) Requirements(ctx context.Context, projectID string, ids []string) ([]string, error) {
	return requirementBlocks(ctx, b.DB, projectID, ids)
}

func (b DBBoard) Decisions(ctx context.Context, projectID string, ids []string) ([]string, error) {
	return decisionLines(ctx, b.DB, projectID, ids)
}

func (b DBBoard) LatestNote(ctx context.Context, cardID string) (string, error) {
	return latestNote(ctx, b.DB, cardID)
}

func (b DBBoard) LatestReviewNote(ctx context.Context, cardID string) (string, error) {
	return latestReviewNote(ctx, b.DB, cardID)
}

func (b DBBoard) Instructions(ctx context.Context, cardID string) (string, error) {
	return board.CurrentInstructions(ctx, b.DB, cardID)
}

func (b DBBoard) Conventions(ctx context.Context, projectID, cardID string) ([]contextpack.Convention, []map[string]any, error) {
	return CardConventions(ctx, b.DB, projectID, cardID)
}

func (b DBBoard) NextAttempt(ctx context.Context, cardID string) (int, string, error) {
	return board.NextAttempt(ctx, b.DB, cardID)
}

func (b DBBoard) InsertAttempt(ctx context.Context, attempt board.Attempt) error {
	return board.InsertAttempt(ctx, b.DB, attempt)
}

func (b DBBoard) FinishAttempt(ctx context.Context, id, outcome, commitSHA string) error {
	return board.FinishAttempt(ctx, b.DB, id, outcome, commitSHA)
}

func (b DBBoard) AddAttemptUsed(ctx context.Context, cardID string) error {
	return board.AddAttemptUsed(ctx, b.DB, cardID)
}

func (b DBBoard) InsertFailureNote(ctx context.Context, cardID, attemptID, source, note string) error {
	return board.InsertFailureNote(ctx, b.DB, cardID, attemptID, source, note)
}

func (b DBBoard) AddAttemptTokens(ctx context.Context, id string, input, output, cached int) error {
	return board.AddAttemptTokens(ctx, b.DB, id, input, output, cached)
}

func (b DBBoard) CardTokens(ctx context.Context, cardID string) (int, error) {
	return board.CardTokens(ctx, b.DB, cardID)
}

func (b DBBoard) AskQuestions(ctx context.Context, attemptID, cardID string, first int, questions []board.Question, auto bool) error {
	return board.InsertQuestions(ctx, b.DB, cardID, attemptID, first, questions, auto)
}

func (b DBBoard) Answers(ctx context.Context, cardID string) ([]board.Question, error) {
	return board.AnsweredQuestions(ctx, b.DB, cardID)
}

func (b DBBoard) SubmitPlan(ctx context.Context, attemptID, cardID, title string, steps []board.PlanStep) error {
	_, err := board.InsertOpenPlan(ctx, b.DB, cardID, attemptID, title, steps)
	return err
}

func (b DBBoard) ApprovedPlan(ctx context.Context, cardID string) (board.Plan, bool, error) {
	plan, err := board.AcceptedPlan(ctx, b.DB, cardID)
	if errors.Is(err, board.ErrNotFound) {
		return board.Plan{}, false, nil
	}
	return plan, err == nil, err
}

func (b DBBoard) SaveHandoff(ctx context.Context, h board.Handoff) error {
	_, err := board.SaveHandoff(ctx, b.DB, h)
	return err
}

func (b DBBoard) PendingHandoff(ctx context.Context, cardID string) (board.Handoff, bool, error) {
	return board.PendingHandoff(ctx, b.DB, cardID)
}

func (b DBBoard) MarkHandoffApplied(ctx context.Context, cardID, id string, ok bool, reason string) error {
	return board.MarkHandoffApplied(ctx, b.DB, cardID, id, ok, reason)
}

func (b DBBoard) AppendEvent(ctx context.Context, ev trace.Event, secrets []string) error {
	_, err := trace.Append(ctx, b.DB, ev, secrets)
	return err
}
