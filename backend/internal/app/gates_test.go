package app_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
)

const (
	shaA = "0123456789abcdef0123456789abcdef01234567"
	shaB = "abcdef0123456789abcdef0123456789abcdef01"
)

func TestApproveSha(t *testing.T) {
	db := boardtest.Open(t)
	card := gateCard(t, db, "human_review", shaA)
	_, err := app.Approve(context.Background(), db, card.ID, shaB, gateCfg())
	if !errors.Is(err, app.ErrSHA) {
		t.Fatalf("err = %v", err)
	}
	got := mustGet(t, db, card.ID)
	if got.ApprovedSHA != "" {
		t.Fatalf("approved_sha = %s", got.ApprovedSHA)
	}
	if approvals(t, db, card.ID) != 0 {
		t.Fatal("approval row written")
	}
}

func TestApproveIdempotent(t *testing.T) {
	db := boardtest.Open(t)
	card := gateCard(t, db, "human_review", shaA)
	if _, err := app.Approve(context.Background(), db, card.ID, shaA, gateCfg()); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Approve(context.Background(), db, card.ID, shaA, gateCfg()); err != nil {
		t.Fatal(err)
	}
	if approvals(t, db, card.ID) != 1 {
		t.Fatalf("approvals = %d", approvals(t, db, card.ID))
	}
}

func TestApprovedHeadMoved(t *testing.T) {
	db := boardtest.Open(t)
	card := gateCard(t, db, "human_review", shaA)
	if _, err := app.Approve(context.Background(), db, card.ID, shaA, gateCfg()); err != nil {
		t.Fatal(err)
	}
	next, err := app.HeadMoved(context.Background(), db, card.ID, shaB)
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "human_review" || next.ApprovedSHA != "" {
		t.Fatalf("card = %+v", next)
	}
	if liveApprovals(t, db, card.ID) != 0 {
		t.Fatal("approval still live")
	}
}

func TestStaleSHA(t *testing.T) {
	TestApprovedHeadMoved(t)
}

func TestRequestChangesFromApproved(t *testing.T) {
	db := boardtest.Open(t)
	card := gateCard(t, db, "human_review", shaA)
	if _, err := app.Approve(context.Background(), db, card.ID, shaA, gateCfg()); err != nil {
		t.Fatal(err)
	}
	next, err := app.RequestChanges(context.Background(), db, card.ID, "", gateCfg())
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "changes_requested" || next.ApprovedSHA != "" {
		t.Fatalf("card = %+v", next)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM failure_notes WHERE card_id = $1 AND source = 'approver'`, card.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("notes = %d", n)
	}
}

func TestRejectFromApproved(t *testing.T) {
	db := boardtest.Open(t)
	card := gateCard(t, db, "human_review", shaA)
	if _, err := app.Approve(context.Background(), db, card.ID, shaA, gateCfg()); err != nil {
		t.Fatal(err)
	}
	next, err := app.Reject(context.Background(), db, card.ID, gateCfg())
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "blocked" || next.BlockedReason != "rejected" {
		t.Fatalf("card = %+v", next)
	}
}

func TestUnblockDestinations(t *testing.T) {
	db := boardtest.Open(t)
	for _, reason := range []string{"health", "deploy_failed", "deploy_config"} {
		card := blocked(t, db, reason, 0, 3)
		dest, ok := app.UnblockTo(context.Background(), db, card)
		if !ok || dest != "human_review" {
			t.Fatalf("%s -> %s %v", reason, dest, ok)
		}
	}
	card := blocked(t, db, "turn_cap", 1, 3)
	dest, ok := app.UnblockTo(context.Background(), db, card)
	if !ok || dest != "ready" {
		t.Fatalf("turn_cap -> %s", dest)
	}
}

func TestUnblockAtCap(t *testing.T) {
	db := boardtest.Open(t)
	card := blocked(t, db, "attempt_cap", 3, 3)
	next, err := app.Unblock(context.Background(), db, card.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.MaxAttempts != 4 || next.Status != "ready" {
		t.Fatalf("card = %+v", next)
	}
	if _, err := db.Exec(`UPDATE cards SET status = 'in_progress', attempts_used = 4 WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	again, err := app.AtCap(context.Background(), db, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "blocked" || again.BlockedReason != "attempt_cap" {
		t.Fatalf("again = %+v", again)
	}
}

func TestUnblockReset(t *testing.T) {
	db := boardtest.Open(t)
	card := blocked(t, db, "turn_cap", 2, 3)
	if err := board.InsertAttempt(context.Background(), db, board.Attempt{
		CardID: card.ID, N: 2, TaskClass: "implement", BaseSHA: shaA,
	}); err != nil {
		t.Fatal(err)
	}
	next, err := app.Unblock(context.Background(), db, card.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if next.AttemptsUsed != 0 {
		t.Fatalf("attempts_used = %d", next.AttemptsUsed)
	}
	if err := board.InsertAttempt(context.Background(), db, board.Attempt{
		CardID: card.ID, N: 3, TaskClass: "implement", BaseSHA: shaA,
	}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT MAX(n) FROM attempts WHERE card_id = $1`, card.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("n = %d", n)
	}
}

func TestUnblockSourceChanged(t *testing.T) {
	db := boardtest.Open(t)
	if _, err := db.Exec(`
		INSERT INTO requirements (
			id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json, content_hash
		) VALUES ('REQ-001', 'prd', 'docs/prd/a.md', 'REQ-001', 's', '[]', '[]', 'h')`); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(context.Background(), db, board.Card{
		Status: "blocked", RequirementIDs: []string{"REQ-001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET blocked_reason = 'source_changed' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	card = mustGet(t, db, card.ID)
	if _, ok := app.UnblockTo(context.Background(), db, card); ok {
		t.Fatal("unblock allowed")
	}
	if _, err := app.Unblock(context.Background(), db, card.ID, false); !errors.Is(err, app.ErrUnblock) {
		t.Fatalf("err = %v", err)
	}
}

func TestCapFromChangesRequested(t *testing.T) {
	db := boardtest.Open(t)
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: "changes_requested", AttemptsUsed: 3, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	next, err := app.AtCap(context.Background(), db, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "blocked" || next.BlockedReason != "attempt_cap" {
		t.Fatalf("card = %+v", next)
	}
}

func gateCfg() app.GateConfig {
	return app.GateConfig{Operator: "ada", BotLogin: "bot", Approvers: []string{"ada"}}
}

func gateCard(t *testing.T, db *sql.DB, status, head string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: status, Title: "card"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET head_sha = $2 WHERE id = $1`, card.ID, head); err != nil {
		t.Fatal(err)
	}
	return mustGet(t, db, card.ID)
}

func blocked(t *testing.T, db *sql.DB, reason string, used, max int) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), db, board.Card{
		Status: "blocked", AttemptsUsed: used, MaxAttempts: max, Title: reason,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET blocked_reason = $2 WHERE id = $1`, card.ID, reason); err != nil {
		t.Fatal(err)
	}
	return mustGet(t, db, card.ID)
}

func mustGet(t *testing.T, db *sql.DB, id string) board.Card {
	t.Helper()
	card, err := board.GetCard(context.Background(), db, id)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func approvals(t *testing.T, db *sql.DB, id string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM approvals WHERE card_id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func liveApprovals(t *testing.T, db *sql.DB, id string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM approvals WHERE card_id = $1 AND cleared_at IS NULL`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
