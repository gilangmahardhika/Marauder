package runner_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/runner"
	"orchestrator/backend/internal/runnerapi"
)

func TestRunnerChaos(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	ctx := context.Background()
	a := h.register(t, "agent-a")
	b := h.register(t, "agent-b")
	for _, c := range []*runner.Client{a, b} {
		if _, err := c.Heartbeat(ctx, runnerapi.Heartbeat{Version: "test", Protocol: runnerapi.Protocol}); err != nil {
			t.Fatal(err)
		}
	}
	_, lost := startAttempt(t, h, a, "att_chaos0000001")
	if _, ok, err := b.Claim(ctx, 0); err != nil || ok {
		t.Fatalf("second runner claim while the first holds the card = %v %v", ok, err)
	}

	past := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	if _, err := h.db.Exec(`UPDATE runners SET heartbeat_at = $1 WHERE name = 'agent-a'`, past); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(`UPDATE runner_leases SET leased_until = $1 WHERE id = $2`, past, lost); err != nil {
		t.Fatal(err)
	}
	if n, err := app.ReapRemote(ctx, h.db, h.srv.Remote); err != nil || n != 1 {
		t.Fatalf("reaped = %d %v, want 1", n, err)
	}
	var outcome string
	if err := h.db.QueryRow(`SELECT outcome FROM runner_leases WHERE id = $1`, lost).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome != "lost" {
		t.Fatalf("lease outcome = %s", outcome)
	}
	if err := h.db.QueryRow(`SELECT outcome FROM attempts WHERE id = 'att_chaos0000001'`).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome != "cancelled" {
		t.Fatalf("lost attempt outcome = %s", outcome)
	}
	if card := h.status(t); card.Status != "in_progress" {
		t.Fatalf("card after the reaper = %s", card.Status)
	}
	if _, err := a.Report(ctx, lost, runnerapi.Report{Fact: runnerapi.FactFinished}); runner.StatusOf(err) != http.StatusNotFound {
		t.Fatalf("report on the lost lease = %v, want 404", err)
	}

	r, net := h.worker(t, b)
	claimed := claim(t, b)
	if claimed.CardID != h.card.ID || claimed.LeaseID == lost {
		t.Fatalf("second runner claimed %+v", claimed)
	}
	if err := r.RunJob(ctx, claimed.LeaseID); err != nil {
		t.Fatal(err)
	}
	card := h.status(t)
	if card.Status != "pr_open" || card.PRURL != prURL {
		t.Fatalf("card = %s %q, want pr_open", card.Status, card.PRURL)
	}
	if n := count(t, h, `SELECT count(*) FROM attempts WHERE card_id = $1 AND outcome = 'checks_passed'`, card.ID); n != 1 {
		t.Fatalf("passed attempts = %d", n)
	}
	if n := count(t, h, `SELECT count(*) FROM runner_leases WHERE card_id = $1 AND ended_at IS NULL`, card.ID); n != 0 {
		t.Fatalf("open leases = %d", n)
	}
	var holder string
	if err := h.db.QueryRow(`SELECT r.name FROM runner_leases l JOIN runners r ON r.id = l.runner_id WHERE l.id = $1`, claimed.LeaseID).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if holder != "agent-b" {
		t.Fatalf("finishing lease held by %s", holder)
	}
	if net.has("push") {
		t.Fatal("the runner pushed; only the control plane publishes")
	}
	if !h.cp.has("push") {
		t.Fatal("the control plane did not push the card branch")
	}
}
