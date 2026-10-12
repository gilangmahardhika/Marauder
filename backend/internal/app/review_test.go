package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/worker"
)

const (
	reviewRouter = "router:\n  routes:\n    implement:\n      model: fake-model\n    implement_hard:\n      model: fake-model\n" +
		"    review:\n      model: fake-model\n      fallback: fake-fallback\n  budgets:\n    maxAttempts: 3\n    maxModelTurnsPerAttempt: 6\n"
	approveScript = `{"review":[{"tool":"read_diff"},{"tool":"verdict","verdict":"recommend_approve","body":"Meets REQ-001."}]}`
	noChecks      = `[]`
	pendingChecks = `[{"name":"ci","status":"IN_PROGRESS","conclusion":""}]`
	greenChecks   = `[{"name":"ci","status":"COMPLETED","conclusion":"SUCCESS"}]`
)

// reviewFixture is a card in pr_open with the commit its attempt checked.
type reviewFixture struct {
	jobFixture
	card board.Card
	sha  string
}

func newReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	f := newJobFixture(t, "true", 3, reviewRouter, fixtureProject)
	card := f.run(t, commitScript)
	if card.Status != "pr_open" {
		t.Fatalf("card = %s", card.Status)
	}
	var sha string
	if err := f.db.QueryRow(`SELECT commit_sha FROM attempts WHERE card_id = $1`, card.ID).Scan(&sha); err != nil {
		t.Fatal(err)
	}
	f.calls.setPR(prJSON(sha, noChecks))
	return reviewFixture{jobFixture: f, card: card, sha: sha}
}

func prJSON(head, checks string) string {
	return fmt.Sprintf(`{"headRefOid":%q,"body":"Say job","comments":[],"statusCheckRollup":%s}`, head, checks)
}

func (f reviewFixture) deps(t *testing.T, script string) (app.JobDeps, *fake.Fake) {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	return app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, provider
}

func (f reviewFixture) review(t *testing.T, script string) board.Card {
	t.Helper()
	deps, _ := f.deps(t, script)
	card, err := app.ReviewCard(context.Background(), f.db, deps, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func (f reviewFixture) refresh(t *testing.T) board.Card {
	t.Helper()
	deps, _ := f.deps(t, `{}`)
	card, err := app.RefreshCard(context.Background(), f.db, deps, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

type reviewRow struct {
	model, verdict, reviewed string
	independent, ciRequired  sql.NullBool
}

func latestReviewRow(t *testing.T, db *sql.DB, cardID string) reviewRow {
	t.Helper()
	var r reviewRow
	if err := db.QueryRow(`
		SELECT COALESCE(review_model_id, ''), COALESCE(review_verdict, ''), COALESCE(reviewed_sha, ''),
			review_independent, ci_required
		FROM attempts WHERE card_id = $1 ORDER BY n DESC LIMIT 1`, cardID).Scan(
		&r.model, &r.verdict, &r.reviewed, &r.independent, &r.ciRequired); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReviewApproveNoCI(t *testing.T) {
	f := newReviewFixture(t)
	card := f.review(t, approveScript)
	if card.Status != "human_review" || card.HeadSHA != f.sha {
		t.Fatalf("card = %s head %s, want human_review at %s", card.Status, card.HeadSHA, f.sha)
	}
	r := latestReviewRow(t, f.db, card.ID)
	if r.model != "fake-model" || r.verdict != "recommend_approve" || r.reviewed != f.sha {
		t.Fatalf("review = %+v", r)
	}
	if !r.independent.Valid || r.independent.Bool || !r.ciRequired.Valid || r.ciRequired.Bool {
		t.Fatalf("independent %v ciRequired %v, want false and false", r.independent, r.ciRequired)
	}
	if card.AttemptsUsed != 1 {
		t.Fatalf("attempts_used = %d, a review must not count", card.AttemptsUsed)
	}
	var gates int
	if err := f.db.QueryRow(`SELECT count(*) FROM gates WHERE card_id = $1 AND name = 'agent_review'`, card.ID).Scan(&gates); err != nil {
		t.Fatal(err)
	}
	if gates != 1 {
		t.Fatalf("agent_review gates = %d", gates)
	}
}

func TestReviewRequestChanges(t *testing.T) {
	f := newReviewFixture(t)
	card := f.review(t, `{"review":[{"tool":"comment_pr","body":"hello.txt says job"},{"tool":"verdict","verdict":"request_changes","body":"Say done instead of job."}]}`)
	if card.Status != "changes_requested" {
		t.Fatalf("card = %s", card.Status)
	}
	var note string
	if err := f.db.QueryRow(`SELECT note FROM failure_notes WHERE card_id = $1 AND source = 'review'`, card.ID).Scan(&note); err != nil {
		t.Fatal(err)
	}
	if note != "Say done instead of job." {
		t.Fatalf("note = %q", note)
	}
	comments := f.calls.body("comment")
	if len(comments) != 2 || comments[0] != "hello.txt says job" || !strings.Contains(comments[1], "**AI review: Request changes**") {
		t.Fatalf("comments = %q, want the comment_pr text then the verdict comment", comments)
	}
	ok, err := app.CanStartImplement(context.Background(), f.db, card)
	if err != nil || !ok {
		t.Fatalf("changes_requested card cannot run again: %t %v", ok, err)
	}
}

func TestReviewPostsComment(t *testing.T) {
	f := newReviewFixture(t)
	card := f.review(t, approveScript)
	comments := f.calls.body("comment")
	if card.Status != "human_review" || len(comments) != 1 {
		t.Fatalf("card = %s, comments = %d", card.Status, len(comments))
	}
	for _, want := range []string{"**AI review: Recommend approve**", "Meets REQ-001.", "`" + f.sha[:7] + "`", "`fake-model`", "not independent"} {
		if !strings.Contains(comments[0], want) {
			t.Fatalf("comment lacks %q:\n%s", want, comments[0])
		}
	}
}

func TestReviewCommentFailureKeepsStatus(t *testing.T) {
	f := newReviewFixture(t)
	f.calls.failComment = 1
	card := f.review(t, approveScript)
	if card.Status != "human_review" {
		t.Fatalf("card = %s, want human_review after a failed comment", card.Status)
	}
	var n int
	if err := f.db.QueryRow(`
		SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'job_error' AND payload_json LIKE '%review comment%'`, card.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("review comment job_error events = %d", n)
	}
}

func TestReviewPendingCIWaits(t *testing.T) {
	f := newReviewFixture(t)
	f.calls.setPR(prJSON(f.sha, pendingChecks))
	card := f.review(t, approveScript)
	if card.Status != "agent_review" {
		t.Fatalf("card = %s, want agent_review while CI is pending", card.Status)
	}
	if r := latestReviewRow(t, f.db, card.ID); !r.ciRequired.Bool {
		t.Fatalf("ci_required = %v, want true", r.ciRequired)
	}
}

func TestPassFinishesCIWait(t *testing.T) {
	f := newReviewFixture(t)
	f.calls.setPR(prJSON(f.sha, pendingChecks))
	if card := f.review(t, approveScript); card.Status != "agent_review" {
		t.Fatalf("card = %s", card.Status)
	}
	if card := f.refresh(t); card.Status != "agent_review" {
		t.Fatalf("still pending = %s", card.Status)
	}
	f.calls.setPR(prJSON(f.sha, greenChecks))
	if card := f.refresh(t); card.Status != "human_review" {
		t.Fatalf("green checks = %s, want human_review", card.Status)
	}
}

func TestPassFailedCIRequestsChanges(t *testing.T) {
	f := newReviewFixture(t)
	f.calls.setPR(prJSON(f.sha, pendingChecks))
	f.review(t, approveScript)
	f.calls.setPR(prJSON(f.sha, `[{"name":"ci","status":"COMPLETED","conclusion":"FAILURE"}]`))
	if card := f.refresh(t); card.Status != "changes_requested" {
		t.Fatalf("red checks = %s, want changes_requested", card.Status)
	}
	if n := countNotes(t, f.db, f.card.ID, "ci"); n != 1 {
		t.Fatalf("ci notes = %d", n)
	}
}

func TestReviewParseFallback(t *testing.T) {
	f := newReviewFixture(t)
	card := f.review(t, `{"review":[
		{"tool":"verdict","verdict":"maybe","body":"x"},
		{"tool":"verdict","verdict":"lgtm","body":"x"},
		{"tool":"verdict","verdict":"recommend_approve","body":"ok"}]}`)
	if card.Status != "human_review" {
		t.Fatalf("card = %s", card.Status)
	}
	if r := latestReviewRow(t, f.db, card.ID); r.model != "fake-fallback" {
		t.Fatalf("review model = %s, want the fallback", r.model)
	}
}

func TestReviewWithoutVerdictBlocks(t *testing.T) {
	f := newReviewFixture(t)
	card := f.review(t, `{"review":[{"text":"looks fine"},{"text":"still fine"}]}`)
	if card.Status != "blocked" || card.BlockedReason != "review_parse" {
		t.Fatalf("no verdict = %s %s", card.Status, card.BlockedReason)
	}
}

func TestRefreshStoresHeadSHA(t *testing.T) {
	f := newReviewFixture(t)
	f.review(t, approveScript)
	moved := strings.Repeat("b", 40)
	f.calls.setPR(prJSON(moved, noChecks))
	card := f.refresh(t)
	if card.Status != "human_review" || card.HeadSHA != moved {
		t.Fatalf("card = %s head %s", card.Status, card.HeadSHA)
	}
}

func TestApprovedHeadMovedReturnsToReview(t *testing.T) {
	f := newReviewFixture(t)
	f.review(t, approveScript)
	card, err := app.Approve(context.Background(), f.db, f.card.ID, f.sha, app.GateConfig{
		Operator: "ada", BotLogin: "acme-bot", Approvers: []string{"ada"}, ActorID: "user_approver0001",
	})
	if err != nil || card.Status != "approved" {
		t.Fatalf("approve = %s %v", card.Status, err)
	}
	f.calls.setPR(prJSON(strings.Repeat("c", 40), noChecks))
	card = f.refresh(t)
	if card.Status != "human_review" || card.ApprovedSHA != "" {
		t.Fatalf("head moved = %s approved %q, want human_review with approval cleared", card.Status, card.ApprovedSHA)
	}
}

func TestCardReachesApproved(t *testing.T) {
	f := newJobFixture(t, "true", 3, reviewRouter, fixtureProject)
	ctx := context.Background()
	provider, err := fake.Load([]byte(`{"implement":[{"tool":"edit_file","path":"hello.txt","text":"job\n"},{"tool":"commit","message":"Job commit"}],` +
		`"review":[{"tool":"verdict","verdict":"recommend_approve","body":"ok"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	deps := app.JobDeps{Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run}}
	jobs := app.ImplementJobs(f.db, deps)
	if status, err := jobs.Implement(ctx, "proj_default", f.card.ID); err != nil || status != "pr_open" {
		t.Fatalf("implement = %s %v", status, err)
	}
	var sha string
	if err := f.db.QueryRow(`SELECT commit_sha FROM attempts WHERE card_id = $1`, f.card.ID).Scan(&sha); err != nil {
		t.Fatal(err)
	}
	f.calls.setPR(prJSON(sha, noChecks))
	adv := worker.NewAdvancer()
	if err := adv.Tick(ctx, f.db, "proj_default", jobs, time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	adv.Wait()
	card, err := board.GetCard(ctx, f.db, f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "human_review" || card.HeadSHA != sha {
		t.Fatalf("after the worker tick = %s head %s", card.Status, card.HeadSHA)
	}
	card, err = app.Approve(ctx, f.db, card.ID, card.HeadSHA, app.GateConfig{
		Operator: "ada", BotLogin: "acme-bot", Approvers: []string{"ada"}, ActorID: "user_approver0001",
	})
	if err != nil || card.Status != "approved" || card.ApprovedSHA != sha {
		t.Fatalf("approve = %s %s %v", card.Status, card.ApprovedSHA, err)
	}
}
