package app_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/providers/fake"
)

const (
	noCommitScript       = `{"implement":[{"text":"nothing to commit"}]}`
	requestChangesScript = `{"review":[{"tool":"verdict","verdict":"request_changes","body":"Say done instead of job."}]}`
)

func openRetries(t *testing.T, db *sql.DB, cardID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`
		SELECT count(*) FROM runs WHERE card_id = $1 AND trigger = 'retry' AND processed_at IS NULL`, cardID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRetryAfterNoCommit(t *testing.T) {
	t.Run("attempts left", func(t *testing.T) {
		f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
		card := f.run(t, noCommitScript)
		if card.Status != "in_progress" || card.AttemptsUsed != 1 {
			t.Fatalf("card = %s used %d", card.Status, card.AttemptsUsed)
		}
		if n := openRetries(t, f.db, card.ID); n != 1 {
			t.Fatalf("open retry rows = %d, want 1", n)
		}
	})
	t.Run("at the cap", func(t *testing.T) {
		f := newJobFixture(t, "true", 1, routerConfig, fixtureProject)
		card := f.run(t, noCommitScript)
		if card.Status != "blocked" || card.BlockedReason != "attempt_cap" {
			t.Fatalf("card = %s %s, want blocked attempt_cap", card.Status, card.BlockedReason)
		}
		if n := openRetries(t, f.db, card.ID); n != 0 {
			t.Fatalf("open retry rows = %d", n)
		}
	})
}

func TestRetryAfterChecksFailed(t *testing.T) {
	f := newJobFixture(t, "false", 3, routerConfig, fixtureProject)
	card := f.run(t, commitScript)
	if card.Status != "in_progress" || card.AttemptsUsed != 1 {
		t.Fatalf("card = %s used %d", card.Status, card.AttemptsUsed)
	}
	if n := openRetries(t, f.db, card.ID); n != 1 {
		t.Fatalf("open retry rows = %d, want 1", n)
	}
}

func TestRunRefusedAtCap(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	card, err := board.InsertCard(ctx, db, board.Card{ProjectID: "proj_default", Status: "in_progress", MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET attempts_used = 2 WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	card = reload(t, db, card.ID)
	if ok, err := app.CanStartImplement(ctx, db, card); err != nil || ok {
		t.Fatalf("card at the cap is startable: %t %v", ok, err)
	}
	if _, err := app.QueueCardRun(ctx, db, "proj_default", card.ID, nil, ""); !errors.Is(err, app.ErrNotRunnable) {
		t.Fatalf("queue at the cap = %v, want ErrNotRunnable", err)
	}
	if n := openRetries(t, db, card.ID); n != 0 {
		t.Fatalf("open rows = %d", n)
	}
}

func TestReviewContinueQueuesRetry(t *testing.T) {
	t.Run("attempts left", func(t *testing.T) {
		f := newReviewFixture(t)
		card := f.review(t, requestChangesScript)
		if card.Status != "changes_requested" {
			t.Fatalf("card = %s, want changes_requested", card.Status)
		}
		if n := openRetries(t, f.db, card.ID); n != 1 {
			t.Fatalf("open retry rows = %d, want 1", n)
		}
		comments := f.calls.body("comment")
		if len(comments) != 1 || !strings.Contains(comments[0], "went back to the implementer") {
			t.Fatalf("comments = %q", comments)
		}
		// A newer check note must not hide the review body on the queued retry (RFC-131).
		if err := board.InsertFailureNote(context.Background(), f.db, card.ID, "att_check", "local_check", "exit 1: stale"); err != nil {
			t.Fatal(err)
		}
		provider, err := fake.Load([]byte(commitScript))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
			Data: f.data, Key: tokenKey, Provider: provider,
			Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
		}, "proj_default", card.ID); err != nil {
			t.Fatal(err)
		}
		prompts := provider.Prompts()
		if len(prompts) == 0 {
			t.Fatal("no model call on the retry")
		}
		user := prompts[0].User
		want := contextpack.ReviewHeading + "\nSay done instead of job.\n" + contextpack.ReviewEnd
		if !strings.Contains(user, want) {
			t.Fatalf("retry prompt lacks the review block:\n%s", user)
		}
		if !strings.Contains(user, "exit 1: stale") {
			t.Fatalf("retry prompt lacks the newer check note:\n%s", user)
		}
	})
	t.Run("at the cap", func(t *testing.T) {
		f := newReviewFixture(t)
		if _, err := f.db.Exec(`UPDATE cards SET attempts_used = max_attempts WHERE id = $1`, f.card.ID); err != nil {
			t.Fatal(err)
		}
		card := f.review(t, requestChangesScript)
		if card.Status != "blocked" || card.BlockedReason != "attempt_cap" {
			t.Fatalf("card = %s %s, want blocked attempt_cap", card.Status, card.BlockedReason)
		}
		if n := openRetries(t, f.db, card.ID); n != 0 {
			t.Fatalf("open retry rows = %d", n)
		}
	})
}

func manualReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	f := newReviewFixture(t)
	if _, err := f.db.Exec(`UPDATE requirements SET ai_review_mode = 'manual' WHERE id = 'REQ-001'`); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestReviewManualReachesHumanReview(t *testing.T) {
	t.Run("green checks", func(t *testing.T) {
		f := manualReviewFixture(t)
		f.calls.setPR(prJSON(f.sha, greenChecks))
		card := f.review(t, requestChangesScript)
		if card.Status != "human_review" || card.HeadSHA != f.sha {
			t.Fatalf("card = %s head %s, want human_review", card.Status, card.HeadSHA)
		}
		if r := latestReviewRow(t, f.db, card.ID); r.verdict != "request_changes" {
			t.Fatalf("verdict = %q", r.verdict)
		}
		if n := countNotes(t, f.db, card.ID, "review"); n != 1 {
			t.Fatalf("review notes = %d", n)
		}
		if n := openRetries(t, f.db, card.ID); n != 0 {
			t.Fatalf("open retry rows = %d", n)
		}
		comments := f.calls.body("comment")
		if len(comments) != 1 || !strings.Contains(comments[0], "A person decides in the orchestrator") {
			t.Fatalf("comments = %q", comments)
		}
	})
	t.Run("pending checks", func(t *testing.T) {
		f := manualReviewFixture(t)
		f.calls.setPR(prJSON(f.sha, pendingChecks))
		if card := f.review(t, requestChangesScript); card.Status != "agent_review" {
			t.Fatalf("card = %s, want agent_review", card.Status)
		}
		if card := f.refresh(t); card.Status != "agent_review" {
			t.Fatalf("refresh while pending = %s", card.Status)
		}
		f.calls.setPR(prJSON(f.sha, greenChecks))
		if card := f.refresh(t); card.Status != "human_review" {
			t.Fatalf("refresh with green checks = %s, want human_review", card.Status)
		}
	})
	t.Run("red checks", func(t *testing.T) {
		f := manualReviewFixture(t)
		f.calls.setPR(prJSON(f.sha, `[{"name":"ci","status":"COMPLETED","conclusion":"FAILURE"}]`))
		if card := f.review(t, requestChangesScript); card.Status != "changes_requested" {
			t.Fatalf("card = %s, want changes_requested", card.Status)
		}
		if n := openRetries(t, f.db, f.card.ID); n != 0 {
			t.Fatalf("red checks queued %d retry rows", n)
		}
	})
	t.Run("red checks out of attempts", func(t *testing.T) {
		f := manualReviewFixture(t)
		if _, err := f.db.Exec(`UPDATE cards SET attempts_used = max_attempts WHERE id = $1`, f.card.ID); err != nil {
			t.Fatal(err)
		}
		f.calls.setPR(prJSON(f.sha, `[{"name":"ci","status":"COMPLETED","conclusion":"FAILURE"}]`))
		if card := f.review(t, requestChangesScript); card.Status != "blocked" || card.BlockedReason != "attempt_cap" {
			t.Fatalf("card = %s %q, want blocked attempt_cap", card.Status, card.BlockedReason)
		}
	})
}

func TestRefreshSendsBackWhenNoLongerManual(t *testing.T) {
	f := manualReviewFixture(t)
	f.calls.setPR(prJSON(f.sha, pendingChecks))
	if card := f.review(t, requestChangesScript); card.Status != "agent_review" {
		t.Fatalf("card = %s", card.Status)
	}
	if _, err := f.db.Exec(`UPDATE requirements SET ai_review_mode = 'continue' WHERE id = 'REQ-001'`); err != nil {
		t.Fatal(err)
	}
	card := f.refresh(t)
	if card.Status != "changes_requested" {
		t.Fatalf("refresh after the switch = %s, want changes_requested", card.Status)
	}
	if n := openRetries(t, f.db, card.ID); n != 1 {
		t.Fatalf("open retry rows = %d, want 1", n)
	}
}

func TestCardAIReviewMode(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	ada := githubUser(t, db, "ada@example.com", "", 0, "")
	for _, id := range []string{"REQ-001", "REQ-002"} {
		acceptedReq(t, db, id, ada.ID)
	}
	if _, err := db.Exec(`UPDATE requirements SET ai_review_mode = 'manual' WHERE id = 'REQ-001'`); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		ids  []string
		want string
	}{
		"one manual":      {[]string{"REQ-001", "REQ-002"}, app.ReviewManual},
		"only manual":     {[]string{"REQ-001"}, app.ReviewManual},
		"none manual":     {[]string{"REQ-002"}, app.ReviewContinue},
		"no requirements": {nil, app.ReviewContinue},
		"unknown id":      {[]string{"REQ-404"}, app.ReviewContinue},
	} {
		card, err := board.InsertCard(ctx, db, board.Card{Status: "agent_review", RequirementIDs: tc.ids})
		if err != nil {
			t.Fatal(err)
		}
		got, err := app.CardAIReviewMode(ctx, db, card)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: mode = %s, want %s", name, got, tc.want)
		}
	}
}
