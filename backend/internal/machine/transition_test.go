package machine_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/trace"
)

func TestEdgesMatchPRD(t *testing.T) {
	want := map[[2]string]struct{}{
		{"backlog", "ready"}:                  {},
		{"ready", "in_progress"}:              {},
		{"in_progress", "self_check"}:         {},
		{"in_progress", "blocked"}:            {},
		{"self_check", "in_progress"}:         {},
		{"self_check", "pr_open"}:             {},
		{"self_check", "blocked"}:             {},
		{"pr_open", "agent_review"}:           {},
		{"agent_review", "changes_requested"}: {},
		{"agent_review", "human_review"}:      {},
		{"agent_review", "blocked"}:           {},
		{"agent_review", "pr_open"}:           {},
		{"changes_requested", "in_progress"}:  {},
		{"changes_requested", "blocked"}:      {},
		{"human_review", "changes_requested"}: {},
		{"human_review", "approved"}:          {},
		{"human_review", "blocked"}:           {},
		{"approved", "deploying_dev"}:         {},
		{"approved", "human_review"}:          {},
		{"approved", "changes_requested"}:     {},
		{"approved", "blocked"}:               {},
		{"deploying_dev", "verifying"}:        {},
		{"deploying_dev", "blocked"}:          {},
		{"verifying", "done"}:                 {},
		{"verifying", "changes_requested"}:    {},
		{"verifying", "blocked"}:              {},
		{"blocked", "ready"}:                  {},
		{"blocked", "human_review"}:           {},
	}
	got := machine.Edges()
	if len(got) != len(want) {
		t.Fatalf("edges = %d, want %d", len(got), len(want))
	}
	for pair := range want {
		if _, ok := got[pair]; !ok {
			t.Errorf("missing %s -> %s", pair[0], pair[1])
		}
	}
	for pair := range got {
		if _, ok := want[pair]; !ok {
			t.Errorf("unexpected %s -> %s", pair[0], pair[1])
		}
		if pair[0] == "done" {
			t.Errorf("done has outbound edge to %s", pair[1])
		}
	}
}

func TestIllegalDeploySkip(t *testing.T) {
	db := boardtest.Open(t)
	card := insertCard(t, db, "in_progress")
	_, err := apply(t, db, card, "deploying_dev", machine.Evidence{})
	if !errors.Is(err, machine.ErrIllegal) {
		t.Fatalf("err = %v, want ErrIllegal", err)
	}
	got := reload(t, db, card.ID)
	if got.Status != "in_progress" {
		t.Fatalf("status = %s, want in_progress", got.Status)
	}
	if n := countKind(t, db, card.ID, "transition_rejected"); n != 1 {
		t.Fatalf("transition_rejected = %d, want 1", n)
	}
	if n := countKind(t, db, card.ID, "transition"); n != 0 {
		t.Fatalf("transition = %d, want 0", n)
	}
	if n := countGates(t, db, card.ID); n != 0 {
		t.Fatalf("gates = %d, want 0", n)
	}
}

func TestEvidenceRequired(t *testing.T) {
	db := boardtest.Open(t)
	card := insertCard(t, db, "human_review")
	_, err := apply(t, db, card, "approved", machine.Evidence{Approver: "ada"})
	if !errors.Is(err, machine.ErrEvidence) {
		t.Fatalf("err = %v, want ErrEvidence", err)
	}
	got := reload(t, db, card.ID)
	if got.Status != "human_review" {
		t.Fatalf("status = %s, want human_review", got.Status)
	}
	if got.ApprovedSHA != "" {
		t.Fatalf("approved_sha = %s, want empty", got.ApprovedSHA)
	}
	if n := countKind(t, db, card.ID, "transition_rejected"); n != 1 {
		t.Fatalf("transition_rejected = %d, want 1", n)
	}
	if n := countGates(t, db, card.ID); n != 0 {
		t.Fatalf("gates = %d, want 0", n)
	}
}

func TestTransitionCAS(t *testing.T) {
	db := boardtest.Open(t)
	card := insertCard(t, db, "human_review")
	ev := machine.Evidence{
		Approver: "ada",
		SHA:      "0123456789abcdef0123456789abcdef01234567",
	}
	first, err := apply(t, db, card, "approved", ev)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "approved" {
		t.Fatalf("status = %s, want approved", first.Status)
	}
	if first.ApprovedSHA != ev.SHA {
		t.Fatalf("approved_sha = %s, want %s", first.ApprovedSHA, ev.SHA)
	}

	_, err = apply(t, db, card, "approved", ev)
	if !errors.Is(err, board.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	if n := countKind(t, db, card.ID, "transition"); n != 1 {
		t.Fatalf("transition = %d, want 1", n)
	}
	if n := countKind(t, db, card.ID, "transition_rejected"); n != 0 {
		t.Fatalf("transition_rejected = %d, want 0", n)
	}
	if n := countGates(t, db, card.ID); n != 1 {
		t.Fatalf("gates = %d, want 1", n)
	}
	got := reload(t, db, card.ID)
	if got.Status != "approved" {
		t.Fatalf("status = %s, want approved", got.Status)
	}
}

func TestLeavingBlockedClearsReason(t *testing.T) {
	db := boardtest.Open(t)
	card := insertCard(t, db, "in_progress")
	blocked, err := apply(t, db, card, "blocked", machine.Evidence{Reason: "provider_error"})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.BlockedReason != "provider_error" {
		t.Fatalf("blocked_reason = %q, want provider_error", blocked.BlockedReason)
	}
	ready, err := apply(t, db, blocked, "ready", machine.Evidence{Reason: "provider_error"})
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != "ready" || ready.BlockedReason != "" {
		t.Fatalf("after unblock = %s %q, want ready with no reason", ready.Status, ready.BlockedReason)
	}
}

func TestAgentReviewToPROpen(t *testing.T) {
	db := boardtest.Open(t)
	card := insertCard(t, db, "agent_review")
	for _, ev := range []machine.Evidence{{ActorID: "usr_ada"}, {AttemptID: "att_1"}} {
		if _, err := apply(t, db, card, "pr_open", ev); !errors.Is(err, machine.ErrEvidence) {
			t.Fatalf("evidence %+v: err = %v, want ErrEvidence", ev, err)
		}
	}
	next, err := apply(t, db, card, "pr_open", machine.Evidence{AttemptID: "att_1", ActorID: "usr_ada", Reason: "retry AI review"})
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "pr_open" {
		t.Fatalf("status = %s, want pr_open", next.Status)
	}
	if n := countGates(t, db, card.ID); n != 0 {
		t.Fatalf("gates = %d, want 0: a retried review is not a new local check", n)
	}
}

func insertCard(t *testing.T, db *sql.DB, status string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: status})
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func apply(t *testing.T, db *sql.DB, card board.Card, to string, ev machine.Evidence) (board.Card, error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	next, err := machine.Transition(context.Background(), tx, card, to, ev, nil)
	if err == nil || errors.Is(err, machine.ErrIllegal) || errors.Is(err, machine.ErrEvidence) {
		if cerr := tx.Commit(); cerr != nil {
			t.Fatal(cerr)
		}
		return next, err
	}
	_ = tx.Rollback()
	return next, err
}

func reload(t *testing.T, db *sql.DB, id string) board.Card {
	t.Helper()
	card, err := board.GetCard(context.Background(), db, id)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func countKind(t *testing.T, db *sql.DB, cardID, kind string) int {
	t.Helper()
	n, err := trace.Count(context.Background(), db, cardID, kind)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func countGates(t *testing.T, db *sql.DB, cardID string) int {
	t.Helper()
	n, err := board.CountGates(context.Background(), db, cardID)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
