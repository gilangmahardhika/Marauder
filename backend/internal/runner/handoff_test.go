package runner_test

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/runner"
)

func TestRunnerHandoffRoundTrip(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	ctx := context.Background()
	other, err := board.InsertCard(ctx, h.db, board.Card{ProjectID: "proj_default", Title: "Other card", Status: "ready", MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	c := h.register(t, "agent-1")
	b, _ := startAttempt(t, h, c, "att_handoff00001")
	base := strings.Repeat("a", 40)
	h0 := board.Handoff{
		CardID: other.ID, AttemptID: "att_forged000001", Summary: "Done: hello.txt. token " + ownerToken, SummarySource: "model",
		Todos: []board.Todo{{Text: "commit", Status: "pending"}}, Patch: "diff --git a/hello.txt b/hello.txt\n", PatchBaseSHA: base, PatchFiles: 1,
	}
	for name, bad := range map[string]board.Handoff{
		"unknown source": {SummarySource: "robot"},
		"long summary":   {SummarySource: "model", Summary: strings.Repeat("x", 9<<10)},
		"bad base":       {SummarySource: "model", PatchBaseSHA: "main"},
		"bad todo":       {SummarySource: "model", Todos: []board.Todo{{Text: "x", Status: "later"}}},
	} {
		if err := b.SaveHandoff(ctx, bad); runner.StatusOf(err) != http.StatusUnprocessableEntity {
			t.Fatalf("%s = %v, want 422", name, err)
		}
	}
	if err := b.SaveHandoff(ctx, h0); err != nil {
		t.Fatal(err)
	}
	var id, cardID, attemptID, summary string
	if err := h.db.QueryRow(`SELECT id, card_id, attempt_id, summary FROM attempt_handoffs`).Scan(&id, &cardID, &attemptID, &summary); err != nil {
		t.Fatal(err)
	}
	if cardID != h.card.ID || attemptID != "att_handoff00001" || strings.Contains(summary, ownerToken) {
		t.Fatalf("handoff row = %s %s %q, want the lease card and attempt, redacted", cardID, attemptID, summary)
	}

	pending, ok, err := b.PendingHandoff(ctx, other.ID)
	if err != nil || !ok || pending.ID != id || pending.Patch != h0.Patch || pending.PatchBaseSHA != base || len(pending.Todos) != 1 {
		t.Fatalf("pending = %+v %v %v", pending, ok, err)
	}
	if err := b.MarkHandoffApplied(ctx, other.ID, id, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := b.MarkHandoffApplied(ctx, h.card.ID, id, true, ""); !errors.Is(err, board.ErrConflict) {
		t.Fatalf("second mark = %v, want board.ErrConflict", err)
	}
	if _, ok, err := b.PendingHandoff(ctx, h.card.ID); err != nil || ok {
		t.Fatalf("an applied handoff is pending: %v %v", ok, err)
	}

	otherAttempt, _ := board.NewID("att_")
	if err := board.InsertAttempt(ctx, h.db, board.Attempt{ID: otherAttempt, CardID: other.ID, N: 1, TaskClass: "implement"}); err != nil {
		t.Fatal(err)
	}
	foreign, err := board.SaveHandoff(ctx, h.db, board.Handoff{CardID: other.ID, AttemptID: otherAttempt, SummarySource: "harness"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.MarkHandoffApplied(ctx, other.ID, foreign, true, ""); !errors.Is(err, board.ErrConflict) {
		t.Fatalf("marking another card's handoff = %v, want board.ErrConflict", err)
	}
	if _, ok, _ := board.PendingHandoff(ctx, h.db, other.ID); !ok {
		t.Fatal("a lease used another card's handoff")
	}
}

func TestRunnerSeedsFromControlPlaneBundle(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	clone, _, sha := cardCommit(t, h)
	orch := "refs/heads/orch/" + h.card.ID
	gitOut(t, h.repo, "fetch", "--quiet", clone, orch+":"+orch)

	c := h.register(t, "agent-1")
	r, _ := h.worker(t, c)
	if err := r.RunJob(context.Background(), claim(t, c).LeaseID); err != nil {
		t.Fatal(err)
	}
	runnerRepo := r.Data + "/projects/proj_default/repo"
	if err := exec.Command(lookGit(t), "-C", runnerRepo, "merge-base", "--is-ancestor", sha, orch).Run(); err != nil {
		t.Fatalf("the runner's card branch does not hold the control plane's commit %s: %v", sha, err)
	}
	if subject := gitOut(t, runnerRepo, "log", "-1", "--format=%s", orch+"^"); subject != "Bundled commit" {
		t.Fatalf("the attempt did not start from the seeded branch: parent %q", subject)
	}
	if card := h.status(t); card.Status != "pr_open" {
		t.Fatalf("card = %s, want pr_open", card.Status)
	}
	read := false
	for _, body := range h.tap.all()["bundle"] {
		read = read || strings.HasPrefix(body, "# v2 git bundle") || strings.HasPrefix(body, "# v3 git bundle")
	}
	if !read {
		t.Fatal("the runner did not read the card bundle")
	}
}
