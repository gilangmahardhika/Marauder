package runner

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/runnerapi"
	"orchestrator/backend/internal/trace"
)

// Board is the loop.Board of a leased attempt. Each call is a board op of
// the lease; the control plane pins it to the lease card, so the ids the loop
// passes are not sent. The lock is held as the lease, and the worktree path
// stays on this machine.
type Board struct {
	Client  *Client
	LeaseID string
	// Secrets are redacted from every event and note before it is sent.
	Secrets []string

	mu        sync.Mutex
	attemptID string
}

// AttemptID is the attempt the loop inserted, or "".
func (b *Board) AttemptID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.attemptID
}

func (b *Board) op(ctx context.Context, op runnerapi.BoardOp) (runnerapi.BoardReply, error) {
	return b.Client.Board(ctx, b.LeaseID, op)
}

func (b *Board) GetCard(ctx context.Context, cardID string) (board.Card, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "card"})
	if err != nil || reply.Card == nil {
		return board.Card{}, err
	}
	c := reply.Card
	return board.Card{
		ID: c.ID, ProjectID: c.ProjectID, Title: c.Title, Status: c.Status, LocalCheck: c.LocalCheck,
		RequirementIDs: c.RequirementIDs, AttemptsUsed: c.AttemptsUsed, MaxAttempts: c.MaxAttempts,
	}, nil
}

func (b *Board) StoreWorktree(ctx context.Context, cardID, branch, path, gitDir string) error {
	return nil
}

func (b *Board) AcquireLock(ctx context.Context, cardID, holder string) error {
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "lock"})
	return err
}

func (b *Board) ReleaseLock(ctx context.Context, cardID, holder string) error {
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "unlock"})
	return err
}

func (b *Board) Requirements(ctx context.Context, projectID string, ids []string) ([]string, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "requirements"})
	return reply.Lines, err
}

func (b *Board) Decisions(ctx context.Context, projectID string, ids []string) ([]string, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "decisions"})
	return reply.Lines, err
}

func (b *Board) LatestNote(ctx context.Context, cardID string) (string, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "latest_note"})
	return reply.Text, err
}

func (b *Board) Instructions(ctx context.Context, cardID string) (string, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "instructions"})
	return reply.Text, err
}

func (b *Board) Conventions(ctx context.Context, projectID, cardID string) ([]contextpack.Convention, []map[string]any, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "conventions"})
	if err != nil {
		return nil, nil, err
	}
	out := make([]contextpack.Convention, 0, len(reply.Conventions))
	for _, c := range reply.Conventions {
		out = append(out, contextpack.Convention{
			Kind: c.Kind, ID: c.ID, Version: c.Version, Hash: c.Hash, Title: c.Title, Label: c.Label,
			Category: c.Category, Slug: c.Slug, UseWhen: c.UseWhen, Body: c.Body,
		})
	}
	items := reply.Items
	if items == nil {
		items = []map[string]any{}
	}
	return out, items, nil
}

func (b *Board) NextAttempt(ctx context.Context, cardID string) (int, string, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "next_attempt"})
	return reply.N, reply.ParentID, err
}

func (b *Board) InsertAttempt(ctx context.Context, a board.Attempt) error {
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "insert_attempt", Attempt: &runnerapi.Attempt{
		ID: a.ID, N: a.N, ParentID: a.ParentID, TaskClass: a.TaskClass, ModelID: a.ModelID,
		PromptHash: a.PromptHash, SystemHash: a.SystemHash, BaseSHA: a.BaseSHA, MaxTurns: a.MaxTurns,
	}})
	if err == nil {
		b.mu.Lock()
		b.attemptID = a.ID
		b.mu.Unlock()
	}
	return err
}

func (b *Board) FinishAttempt(ctx context.Context, id, outcome, commitSHA string) error {
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "finish_attempt", Outcome: outcome, CommitSHA: commitSHA})
	return err
}

func (b *Board) AddAttemptUsed(ctx context.Context, cardID string) error {
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "add_attempt_used"})
	return err
}

func (b *Board) InsertFailureNote(ctx context.Context, cardID, attemptID, source, note string) error {
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "failure_note", Source: source, Note: trace.Redact(note, b.Secrets)})
	return err
}

func (b *Board) AddAttemptTokens(ctx context.Context, id string, input, output, cached int) error {
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "add_tokens", Input: input, Output: output, Cached: cached})
	return err
}

func (b *Board) CardTokens(ctx context.Context, cardID string) (int, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "card_tokens"})
	return reply.Tokens, err
}

// AskQuestions sends the attempt's questions, redacted with the board's
// secrets; the control plane stores them on the lease's attempt and card.
func (b *Board) AskQuestions(ctx context.Context, attemptID, cardID string, first int, questions []board.Question, auto bool) error {
	sent := make([]board.Question, len(questions))
	for i, q := range questions {
		q.Question = trace.Redact(q.Question, b.Secrets)
		q.Why = trace.Redact(q.Why, b.Secrets)
		q.Recommended.Text = trace.Redact(q.Recommended.Text, b.Secrets)
		sent[i] = q
	}
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "ask_questions", Questions: sent, First: first, Auto: auto})
	return err
}

func (b *Board) Answers(ctx context.Context, cardID string) ([]board.Question, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "answers"})
	return reply.Questions, err
}

// SaveHandoff sends the attempt's handoff with its summary redacted; the
// control plane stores it on the lease's attempt and card.
func (b *Board) SaveHandoff(ctx context.Context, h board.Handoff) error {
	h.Summary = trace.Redact(h.Summary, b.Secrets)
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "save_handoff", Handoff: &h})
	return err
}

func (b *Board) PendingHandoff(ctx context.Context, cardID string) (board.Handoff, bool, error) {
	reply, err := b.op(ctx, runnerapi.BoardOp{Op: "pending_handoff"})
	if err != nil || reply.Handoff == nil {
		return board.Handoff{}, false, err
	}
	return *reply.Handoff, true, nil
}

func (b *Board) MarkHandoffApplied(ctx context.Context, cardID, id string, ok bool, reason string) error {
	_, err := b.op(ctx, runnerapi.BoardOp{Op: "handoff_applied", HandoffID: id, OK: ok, Reason: trace.Redact(reason, b.Secrets)})
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == "conflict" {
		return board.ErrConflict
	}
	return err
}

// AppendEvent sends the event's kind and payload, redacted with the loop's
// secrets and the board's own.
func (b *Board) AppendEvent(ctx context.Context, ev trace.Event, secrets []string) error {
	payload, err := redactPayload(ev.Payload, append(append([]string(nil), secrets...), b.Secrets...))
	if err != nil {
		return err
	}
	_, err = b.op(ctx, runnerapi.BoardOp{Op: "event", Kind: ev.Kind, Payload: payload})
	return err
}

func redactPayload(payload any, secrets []string) (map[string]any, error) {
	if payload == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(trace.Redact(string(raw), secrets)), &out); err != nil {
		return nil, err
	}
	return out, nil
}
