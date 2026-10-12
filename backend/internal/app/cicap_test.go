package app_test

import (
	"context"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
)

const redChecks = `[{"name":"ci","status":"COMPLETED","conclusion":"FAILURE"}]`

func TestCIRedQueuesRetry(t *testing.T) {
	f := newReviewFixture(t)
	f.calls.setPR(prJSON(f.sha, redChecks))
	card := f.review(t, approveScript)
	if card.Status != "changes_requested" {
		t.Fatalf("red checks = %s, want changes_requested", card.Status)
	}
	if n := openRetries(t, f.db, f.card.ID); n != 1 {
		t.Fatalf("open retry runs = %d, want 1", n)
	}
	if n := countNotes(t, f.db, f.card.ID, "ci"); n != 1 {
		t.Fatalf("ci notes = %d", n)
	}
}

func TestCIRedAtCapBlocks(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.db.Exec(`UPDATE cards SET attempts_used = max_attempts WHERE id = $1`, f.card.ID); err != nil {
		t.Fatal(err)
	}
	f.calls.setPR(prJSON(f.sha, pendingChecks))
	if card := f.review(t, approveScript); card.Status != "agent_review" {
		t.Fatalf("pending checks = %s", card.Status)
	}
	f.calls.setPR(prJSON(f.sha, redChecks))
	card := f.refresh(t)
	if card.Status != "blocked" || card.BlockedReason != "attempt_cap" {
		t.Fatalf("red checks at the cap = %s %q, want blocked attempt_cap", card.Status, card.BlockedReason)
	}
	if n := openRetries(t, f.db, f.card.ID); n != 0 {
		t.Fatalf("open retry runs = %d, want 0", n)
	}
}

func TestRefreshCapsChangesRequested(t *testing.T) {
	ctx := context.Background()
	db := boardtest.Open(t)
	stuck, err := board.InsertCard(ctx, db, board.Card{Status: "changes_requested", Title: "stuck", AttemptsUsed: 3, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	open, err := board.InsertCard(ctx, db, board.Card{Status: "changes_requested", Title: "open", AttemptsUsed: 2, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	card, err := app.RefreshCard(ctx, db, app.JobDeps{}, stuck.ProjectID, stuck.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "blocked" || card.BlockedReason != "attempt_cap" {
		t.Fatalf("out of attempts = %s %q, want blocked attempt_cap", card.Status, card.BlockedReason)
	}
	if card, err = app.RefreshCard(ctx, db, app.JobDeps{}, open.ProjectID, open.ID); err != nil || card.Status != "changes_requested" {
		t.Fatalf("attempts left = %s %v, want changes_requested", card.Status, err)
	}
}

func TestRequestChangesAtCapBlocks(t *testing.T) {
	ctx := context.Background()
	db := boardtest.Open(t)
	card := gateCard(t, db, "human_review", shaA)
	if _, err := db.Exec(`UPDATE cards SET attempts_used = max_attempts WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	next, err := app.RequestChanges(ctx, db, card.ID, "Add CSV export.", gateCfg())
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "blocked" || next.BlockedReason != "attempt_cap" {
		t.Fatalf("card = %s %q, want blocked attempt_cap", next.Status, next.BlockedReason)
	}
	if n := countNotes(t, db, card.ID, "approver"); n != 1 {
		t.Fatalf("approver notes = %d, want the reason kept for after Unblock", n)
	}
}
