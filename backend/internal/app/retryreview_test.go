package app_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/review"
)

// failedReviewFixture is a card in agent_review whose AI review gave no
// verdict while its pull request checks are pending.
func failedReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	f := newReviewFixture(t)
	f.calls.setPR(prJSON(f.sha, pendingChecks))
	if card := f.review(t, noVerdictScript); card.Status != "agent_review" {
		t.Fatalf("card = %s, want agent_review", card.Status)
	}
	if r := latestReviewRow(t, f.db, f.card.ID); r.verdict != review.VerdictFailed {
		t.Fatalf("review = %+v, want failed", r)
	}
	return f
}

func localCheckGates(t *testing.T, f reviewFixture) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM gates WHERE card_id = $1 AND name = 'local_checks'`, f.card.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRetryReviewQueuesReview(t *testing.T) {
	f := failedReviewFixture(t)
	card, err := app.RetryReview(context.Background(), f.db, "proj_default", f.card.ID, "usr_ada")
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" {
		t.Fatalf("card = %s, want pr_open", card.Status)
	}
	if r := latestReviewRow(t, f.db, f.card.ID); r.verdict != "" || r.model != "" || r.reviewed != "" {
		t.Fatalf("review = %+v, want cleared", r)
	}
	if n := localCheckGates(t, f); n != 1 {
		t.Fatalf("local_checks gates = %d, a retry is not a new local check", n)
	}
	var actor string
	if err := f.db.QueryRow(`
		SELECT COALESCE(actor_id, '') FROM events WHERE card_id = $1 AND kind = 'transition'
		AND payload_json::jsonb->>'to' = 'pr_open' AND payload_json::jsonb->>'from' = 'agent_review'`, f.card.ID).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if actor != "usr_ada" {
		t.Fatalf("transition actor = %q", actor)
	}

	card = f.review(t, approveScript)
	if card.Status != "agent_review" {
		t.Fatalf("after the retried review = %s, want agent_review waiting on checks", card.Status)
	}
	if r := latestReviewRow(t, f.db, f.card.ID); r.verdict != "recommend_approve" || r.reviewed != f.sha {
		t.Fatalf("review = %+v", r)
	}
	if card.AttemptsUsed != 1 {
		t.Fatalf("attempts_used = %d, a retried review must not count", card.AttemptsUsed)
	}
}

func TestRetryReviewNeedsFailedVerdict(t *testing.T) {
	ctx := context.Background()
	f := newReviewFixture(t)
	if _, err := app.RetryReview(ctx, f.db, "proj_default", f.card.ID, "usr_ada"); !errors.Is(err, app.ErrReviewNotFailed) {
		t.Fatalf("pr_open: err = %v, want ErrReviewNotFailed", err)
	}
	f.calls.setPR(prJSON(f.sha, pendingChecks))
	if card := f.review(t, approveScript); card.Status != "agent_review" {
		t.Fatalf("card = %s", card.Status)
	}
	if _, err := app.RetryReview(ctx, f.db, "proj_default", f.card.ID, "usr_ada"); !errors.Is(err, app.ErrReviewNotFailed) {
		t.Fatalf("recommend_approve: err = %v, want ErrReviewNotFailed", err)
	}
	if _, err := app.RetryReview(ctx, f.db, "proj_other", f.card.ID, "usr_ada"); !errors.Is(err, board.ErrNotFound) {
		t.Fatalf("other project: err = %v, want ErrNotFound", err)
	}
	if card, _ := board.GetCard(ctx, f.db, f.card.ID); card.Status != "agent_review" {
		t.Fatalf("card = %s, want agent_review", card.Status)
	}
}

func TestRetryReviewLocked(t *testing.T) {
	ctx := context.Background()
	f := failedReviewFixture(t)
	if err := board.AcquireLock(ctx, f.db, f.card.ID, "lok_review"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.RetryReview(ctx, f.db, "proj_default", f.card.ID, "usr_ada"); !errors.Is(err, board.ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
	if r := latestReviewRow(t, f.db, f.card.ID); r.verdict != review.VerdictFailed {
		t.Fatalf("review = %+v, want failed kept", r)
	}
}
