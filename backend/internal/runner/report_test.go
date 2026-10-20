package runner_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/runner"
	"orchestrator/backend/internal/runnerapi"
	"orchestrator/backend/internal/trace"
)

func count(t *testing.T, h *harness, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunnerCannotSetStatus(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	ctx := context.Background()
	c := h.register(t, "agent-1")
	b, leaseID := startAttempt(t, h, c, "att_status000001")
	before := h.status(t)
	transitions := count(t, h, `SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'transition'`, h.card.ID)
	for _, op := range []runnerapi.BoardOp{
		{Op: "move"},
		{Op: "transition"},
		{Op: "set_status"},
		{Op: "event", Kind: "transition", Payload: map[string]any{"from": "in_progress", "to": "approved"}},
		{Op: "event", Kind: "gate", Payload: map[string]any{"name": "local_checks", "passed": true}},
		{Op: "finish_attempt", Outcome: "checks_passed"},
	} {
		if _, err := c.Board(ctx, leaseID, op); runner.StatusOf(err) != http.StatusUnprocessableEntity {
			t.Fatalf("op %s %s = %v, want 422", op.Op, op.Kind, err)
		}
	}
	for _, fact := range []string{"approved", "pr_open", "transition"} {
		if _, err := c.Report(ctx, leaseID, runnerapi.Report{Fact: fact}); runner.StatusOf(err) != http.StatusUnprocessableEntity {
			t.Fatalf("report %s = %v, want 422", fact, err)
		}
	}
	if _, err := c.Report(ctx, leaseID, runnerapi.Report{Fact: runnerapi.FactCheck, Exit: 0}); runner.StatusOf(err) != http.StatusUnprocessableEntity {
		t.Fatalf("check report outside self_check = %v, want 422", err)
	}
	after := h.status(t)
	if after.Status != before.Status || after.Status != "in_progress" {
		t.Fatalf("card status %s -> %s", before.Status, after.Status)
	}
	if n := count(t, h, `SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'transition'`, h.card.ID); n != transitions {
		t.Fatalf("transition events %d -> %d", transitions, n)
	}
	if n := count(t, h, `SELECT count(*) FROM gates WHERE card_id = $1`, h.card.ID); n != 0 {
		t.Fatalf("gates = %d", n)
	}
	if err := b.AppendEvent(ctx, trace.Event{Kind: "tool_call", Payload: map[string]any{"name": "read_file", "ok": true}}, nil); err != nil {
		t.Fatalf("an allowed event kind failed: %v", err)
	}
}

func TestCheckExitDecides(t *testing.T) {
	t.Run("exit 1 retries", func(t *testing.T) {
		h := newHarness(t, commitScript, "false")
		c := h.register(t, "agent-1")
		r, _ := h.worker(t, c)
		if err := r.RunJob(context.Background(), claim(t, c).LeaseID); err != nil {
			t.Fatal(err)
		}
		card := h.status(t)
		if card.Status != "in_progress" || card.AttemptsUsed != 1 {
			t.Fatalf("card = %s used %d, want in_progress used 1", card.Status, card.AttemptsUsed)
		}
		var outcome string
		if err := h.db.QueryRow(`SELECT outcome FROM attempts WHERE card_id = $1`, card.ID).Scan(&outcome); err != nil {
			t.Fatal(err)
		}
		if outcome != "checks_failed" {
			t.Fatalf("attempt outcome = %s", outcome)
		}
		if n := count(t, h, `SELECT count(*) FROM failure_notes WHERE card_id = $1 AND source = 'local_check'`, card.ID); n != 1 {
			t.Fatalf("local_check notes = %d", n)
		}
		if n := count(t, h, `SELECT count(*) FROM runs WHERE card_id = $1 AND trigger = 'retry' AND processed_at IS NULL`, card.ID); n != 1 {
			t.Fatalf("open retry rows = %d", n)
		}
		if n := count(t, h, `SELECT count(*) FROM runner_leases WHERE card_id = $1 AND ended_at IS NULL`, card.ID); n != 0 {
			t.Fatalf("open leases = %d", n)
		}
	})
	t.Run("exit 0 publishes", func(t *testing.T) {
		h := newHarness(t, commitScript, "true")
		c := h.register(t, "agent-1")
		r, _ := h.worker(t, c)
		if err := r.RunJob(context.Background(), claim(t, c).LeaseID); err != nil {
			t.Fatal(err)
		}
		card := h.status(t)
		if card.Status != "pr_open" || card.PRURL != prURL || card.AttemptsUsed != 1 {
			t.Fatalf("card = %s %q used %d", card.Status, card.PRURL, card.AttemptsUsed)
		}
		if n := count(t, h, `SELECT count(*) FROM gates WHERE card_id = $1 AND name = 'local_checks' AND passed`, card.ID); n != 1 {
			t.Fatalf("local_checks gates = %d", n)
		}
		var sha string
		if err := h.db.QueryRow(`SELECT commit_sha FROM attempts WHERE card_id = $1 AND outcome = 'checks_passed'`, card.ID).Scan(&sha); err != nil {
			t.Fatal(err)
		}
		if head := gitOut(t, h.repo, "rev-parse", "refs/heads/orch/"+card.ID); head != sha {
			t.Fatalf("control plane orch head = %s, want the attempt commit %s", head, sha)
		}
	})
}

// cardCommit makes a clone of the fixture with a commit on orch/<card>, as a
// runner's attempt leaves it, and returns the clone, the base, and the commit.
func cardCommit(t *testing.T, h *harness) (string, string, string) {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "repo")
	if out, err := exec.Command(lookGit(t), "clone", "--quiet", "--branch", "main", h.repo, clone).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	base := gitOut(t, clone, "rev-parse", "HEAD")
	gitOut(t, clone, "checkout", "--quiet", "-b", "orch/"+h.card.ID)
	writeFile(t, filepath.Join(clone, "hello.txt"), "bundled\n")
	gitOut(t, clone, "-c", "user.name=runner", "-c", "user.email=runner@example.com", "commit", "--quiet", "-am", "Bundled commit")
	return clone, base, gitOut(t, clone, "rev-parse", "HEAD")
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command(lookGit(t), append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func bundle(t *testing.T, clone string, refs ...string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "card.bundle")
	gitOut(t, clone, append([]string{"bundle", "create", "--quiet", path}, refs...)...)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// committedAttempt records an attempt with base and commit through the lease
// and reports it finished, which moves the card to self_check.
func committedAttempt(t *testing.T, _ *harness, c *runner.Client, base, sha string) string {
	t.Helper()
	ctx := context.Background()
	claimed := claim(t, c)
	if _, err := c.Start(ctx, claimed.LeaseID); err != nil {
		t.Fatal(err)
	}
	b := &runner.Board{Client: c, LeaseID: claimed.LeaseID}
	if err := b.InsertAttempt(ctx, board.Attempt{ID: "att_bundle000001", N: 1, BaseSHA: base, MaxTurns: 3}); err != nil {
		t.Fatal(err)
	}
	if err := b.FinishAttempt(ctx, "", "", sha); err != nil {
		t.Fatal(err)
	}
	reply, err := c.Report(ctx, claimed.LeaseID, runnerapi.Report{Fact: runnerapi.FactFinished})
	if err != nil || reply.Next != "check" || reply.SHA != sha {
		t.Fatalf("finished = %+v %v", reply, err)
	}
	return claimed.LeaseID
}

func TestBundleOnlyCardBranch(t *testing.T) {
	t.Run("refs and head", func(t *testing.T) {
		h := newHarness(t, commitScript, "true")
		ctx := context.Background()
		c := h.register(t, "agent-1")
		clone, base, sha := cardCommit(t, h)
		leaseID := committedAttempt(t, h, c, base, sha)
		orch := "refs/heads/orch/" + h.card.ID
		if _, err := c.Report(ctx, leaseID, runnerapi.Report{Fact: runnerapi.FactCheck, Exit: 0}); runner.StatusOf(err) != http.StatusUnprocessableEntity {
			t.Fatalf("passing check before the bundle = %v, want 422", err)
		}
		gitOut(t, clone, "branch", "other", sha)
		refused := func(name string, f *os.File, at string) {
			t.Helper()
			if err := c.Bundle(ctx, leaseID, at, f); runner.StatusOf(err) != http.StatusUnprocessableEntity {
				t.Fatalf("%s = %v, want 422", name, err)
			}
			if out, err := exec.Command(lookGit(t), "-C", h.repo, "rev-parse", "--verify", "--quiet", orch).Output(); err == nil {
				t.Fatalf("%s moved %s to %s", name, orch, out)
			}
		}
		refused("another branch", bundle(t, clone, "refs/heads/other", "^"+base), sha)
		refused("two heads", bundle(t, clone, orch, "refs/heads/other", "^"+base), sha)
		refused("sha that is not the attempt commit", bundle(t, clone, orch, "^"+base), base)
		gitOut(t, clone, "-c", "user.name=runner", "-c", "user.email=runner@example.com", "commit", "--quiet", "--allow-empty", "-m", "Later commit")
		refused("head past the attempt commit", bundle(t, clone, orch, "^"+base), sha)
		if exec.Command(lookGit(t), "-C", h.repo, "rev-parse", "--verify", "--quiet", "refs/heads/other").Run() == nil {
			t.Fatal("a refused bundle created refs/heads/other")
		}
		gitOut(t, clone, "reset", "--quiet", "--hard", sha)
		if err := c.Bundle(ctx, leaseID, sha, bundle(t, clone, orch, "^"+base)); err != nil {
			t.Fatalf("card bundle = %v", err)
		}
		if head := gitOut(t, h.repo, "rev-parse", orch); head != sha {
			t.Fatalf("orch = %s, want %s", head, sha)
		}
		reply, err := c.Report(ctx, leaseID, runnerapi.Report{Fact: runnerapi.FactCheck, Exit: 0})
		if err != nil || reply.Status != "pr_open" {
			t.Fatalf("passing check = %+v %v", reply, err)
		}
	})
	t.Run("commit not from the base", func(t *testing.T) {
		h := newHarness(t, commitScript, "true")
		c := h.register(t, "agent-1")
		clone, base, sha := cardCommit(t, h)
		foreign := strings.Repeat("1", 40)
		leaseID := committedAttempt(t, h, c, foreign, sha)
		err := c.Bundle(context.Background(), leaseID, sha, bundle(t, clone, "refs/heads/orch/"+h.card.ID, "^"+base))
		if runner.StatusOf(err) != http.StatusUnprocessableEntity {
			t.Fatalf("bundle off the attempt base = %v, want 422", err)
		}
		if exec.Command(lookGit(t), "-C", h.repo, "rev-parse", "--verify", "--quiet", "refs/heads/orch/"+h.card.ID).Run() == nil {
			t.Fatal("a refused bundle moved the card branch")
		}
	})
}

func TestRunnerQuestionOutcome(t *testing.T) {
	t.Run("job parks the card", func(t *testing.T) {
		h := newHarness(t, `{"implement":[{"tool":"ask_user","questions":[{"question":"Which format?","kind":"single",
			"options":["CSV","Excel"],"recommended":{"selected":["CSV"]},"why":"CSV is named."}]}]}`, "true")
		c := h.register(t, "agent-1")
		r, _ := h.worker(t, c)
		if err := r.RunJob(context.Background(), claim(t, c).LeaseID); err != nil {
			t.Fatal(err)
		}
		card := h.status(t)
		if card.Status != "waiting_answer" || card.AttemptsUsed != 0 {
			t.Fatalf("card = %s used %d, want waiting_answer used 0", card.Status, card.AttemptsUsed)
		}
		if n := count(t, h, `SELECT count(*) FROM attempts WHERE card_id = $1 AND outcome = 'question'`, card.ID); n != 1 {
			t.Fatalf("question attempts = %d", n)
		}
		if n := count(t, h, `SELECT count(*) FROM card_questions WHERE card_id = $1 AND answer_json IS NULL`, card.ID); n != 1 {
			t.Fatalf("open questions = %d", n)
		}
		if n := count(t, h, `SELECT count(*) FROM runner_leases WHERE card_id = $1 AND ended_at IS NULL`, card.ID); n != 0 {
			t.Fatalf("open leases = %d", n)
		}
	})
	t.Run("questions pinned to the lease", func(t *testing.T) {
		h := newHarness(t, commitScript, "true")
		ctx := context.Background()
		other, err := board.InsertCard(ctx, h.db, board.Card{ProjectID: "proj_default", Title: "Other card", Status: "ready", MaxAttempts: 3})
		if err != nil {
			t.Fatal(err)
		}
		c := h.register(t, "agent-1")
		b, _ := startAttempt(t, h, c, "att_question0001")
		q := board.Question{Question: "Which format? " + ownerToken, Kind: "single", Options: []string{"CSV", "Excel"},
			Recommended: board.Answer{Selected: []string{"CSV"}}, Why: "named"}
		if err := b.AskQuestions(ctx, "att_forged000001", other.ID, 0, []board.Question{q}, false); err != nil {
			t.Fatal(err)
		}
		var cardID, attemptID, question string
		if err := h.db.QueryRow(`SELECT card_id, attempt_id, question FROM card_questions`).Scan(&cardID, &attemptID, &question); err != nil {
			t.Fatal(err)
		}
		if cardID != h.card.ID || attemptID != "att_question0001" || strings.Contains(question, ownerToken) {
			t.Fatalf("question row = %s %s %q, want the lease card and attempt, redacted", cardID, attemptID, question)
		}
		bad := q
		bad.Options = []string{"CSV"}
		if err := b.AskQuestions(ctx, "att_question0001", h.card.ID, 1, []board.Question{bad}, false); runner.StatusOf(err) != http.StatusUnprocessableEntity {
			t.Fatalf("one-option question = %v, want 422", err)
		}
	})
}

func TestRunnerPlanOutcome(t *testing.T) {
	h := newHarness(t, `{"plan":[{"tool":"submit_plan","title":"Ship CSV","steps":[{"text":"Find the handler"},{"text":"Write the export"}]}]}`, "true")
	if _, err := h.db.Exec(`UPDATE cards SET plan_mode = 'required' WHERE id = $1`, h.card.ID); err != nil {
		t.Fatal(err)
	}
	c := h.register(t, "agent-1")
	r, _ := h.worker(t, c)
	if err := r.RunJob(context.Background(), claim(t, c).LeaseID); err != nil {
		t.Fatal(err)
	}
	card := h.status(t)
	if card.Status != "waiting_answer" || card.AttemptsUsed != 0 {
		t.Fatalf("card = %s used %d, want waiting_answer used 0", card.Status, card.AttemptsUsed)
	}
	if n := count(t, h, `SELECT count(*) FROM attempts WHERE card_id = $1 AND outcome = 'plan'`, card.ID); n != 1 {
		t.Fatalf("plan attempts = %d", n)
	}
	if n := count(t, h, `SELECT count(*) FROM card_plans WHERE card_id = $1 AND status = 'open'`, card.ID); n != 1 {
		t.Fatalf("open plans = %d", n)
	}
}

func TestRunnerLeaseScope(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	ctx := context.Background()
	other, err := board.InsertCard(ctx, h.db, board.Card{ProjectID: "proj_default", Title: "Other card", Status: "ready", MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	mine := h.register(t, "agent-1")
	theirs := h.register(t, "agent-2")
	claimed := claim(t, mine)
	if _, err := mine.Start(ctx, claimed.LeaseID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := theirs.Claim(ctx, 0); err != nil || ok {
		t.Fatalf("second claim of one run = %v %v", ok, err)
	}
	calls := map[string]func() error{
		"start": func() error { _, err := theirs.Start(ctx, claimed.LeaseID); return err },
		"board": func() error { _, err := theirs.Board(ctx, claimed.LeaseID, runnerapi.BoardOp{Op: "card"}); return err },
		"model": func() error {
			_, err := theirs.Model(ctx, claimed.LeaseID, runnerapi.ModelCall{AttemptID: "att_x"})
			return err
		},
		"mcp": func() error { _, err := theirs.MCPTools(ctx, claimed.LeaseID); return err },
		"report": func() error {
			_, err := theirs.Report(ctx, claimed.LeaseID, runnerapi.Report{Fact: "finished"})
			return err
		},
		"end": func() error { return theirs.End(ctx, claimed.LeaseID) },
		"release": func() error {
			return theirs.Release(ctx, claimed.LeaseID)
		},
	}
	for name, call := range calls {
		if err := call(); runner.StatusOf(err) != http.StatusNotFound {
			t.Fatalf("%s on another runner's lease = %v, want 404", name, err)
		}
	}
	reply, err := mine.Board(ctx, claimed.LeaseID, runnerapi.BoardOp{Op: "card", IDs: []string{other.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Card == nil || reply.Card.ID != h.card.ID {
		t.Fatalf("card op naming %s returned %+v, want the lease card", other.ID, reply.Card)
	}
	if _, err := mine.Board(ctx, claimed.LeaseID, runnerapi.BoardOp{Op: "lock", Holder: "lok_forged", IDs: []string{other.ID}}); err != nil {
		t.Fatal(err)
	}
	var holder string
	if err := h.db.QueryRow(`SELECT holder FROM locks WHERE card_id = $1`, h.card.ID).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if holder != claimed.LeaseID {
		t.Fatalf("lock holder = %s, want the lease", holder)
	}
	if n := count(t, h, `SELECT count(*) FROM locks WHERE card_id = $1`, other.ID); n != 0 {
		t.Fatalf("other card locks = %d", n)
	}
	if err := mine.End(ctx, claimed.LeaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := mine.Board(ctx, claimed.LeaseID, runnerapi.BoardOp{Op: "card"}); runner.StatusOf(err) != http.StatusNotFound {
		t.Fatalf("op on an ended lease = %v, want 404", err)
	}
}
