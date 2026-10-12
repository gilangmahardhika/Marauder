package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/deploy"
	"orchestrator/backend/internal/envfile"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/review"
	"orchestrator/backend/internal/router"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/trace"
	"orchestrator/backend/internal/vcs"
	"orchestrator/backend/internal/verify"
	"orchestrator/backend/internal/worker"
)

const (
	sha1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := envfile.Load(); err != nil {
		return err
	}
	raw := os.Getenv("ORCHESTRATOR_TEST_DATABASE_URL")
	if raw == "" {
		return errors.New("ORCHESTRATOR_TEST_DATABASE_URL is required")
	}
	repo, err := fixtureRepo()
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(repo+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	data, err := os.MkdirTemp("", "orchestrator-fixture-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(data)
	if err := os.Chmod(data, 0o700); err != nil {
		return err
	}
	stub, err := copyStub()
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(stub))

	ctx := context.Background()
	db, drop, err := openSchema(raw)
	if err != nil {
		return err
	}
	defer drop()
	defer db.Close()

	provider, err := fake.Load([]byte(`{"implement":[{"text":"ok"}],"review":[{"text":"recommend_approve"}]}`))
	if err != nil {
		return err
	}
	if _, err := provider.Complete(ctx, providers.Request{TaskClass: "implement", System: "implementer", User: "card"}); err != nil {
		return err
	}
	model, independent := router.ReviewModel("anthropic", []router.Family{
		{ID: "anthropic", Models: []string{"author"}},
		{ID: "openai", Models: []string{"reviewer"}},
	})
	if !independent || model == "" {
		return errors.New("step 4: review was not independent")
	}

	if err := step1(ctx, db, data); err != nil {
		return err
	}
	first, second, err := step2(ctx, db)
	if err != nil {
		return err
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	if err := ensureMain(repo, gitBin); err != nil {
		return err
	}
	g := runtime.Git{Bin: gitBin, Repo: repo, Data: data, Author: runtime.LoginAuthor("bot", 0)}
	first, err = step3(ctx, db, g, first)
	if err != nil {
		return err
	}
	defer removeWorktree(g, first.ID)
	first, err = step4(ctx, db, first)
	if err != nil {
		return err
	}
	first, err = step5(ctx, db, first)
	if err != nil {
		return err
	}
	first, err = step6(ctx, db, first, stub)
	if err != nil {
		return err
	}
	first, err = step7(ctx, db, first, stub)
	if err != nil {
		return err
	}
	if _, err = step8(ctx, db, g, first, second, stub); err != nil {
		return err
	}
	if err := step9(ctx, db, repo, data, stub); err != nil {
		return err
	}
	if err := budgetCaps(ctx, db, g); err != nil {
		return err
	}
	out, err := exec.Command(gitBin, "-C", repo, "status", "--porcelain").Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("fixture repo is dirty: %s", out)
	}
	return nil
}

func step1(ctx context.Context, db *sql.DB, data string) error {
	prd := filepath.Join(data, "prd")
	rfc := filepath.Join(data, "rfc")
	if err := os.MkdirAll(prd, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(rfc, 0o700); err != nil {
		return err
	}
	body := "# Fixture\n\n## REQ-001 First requirement\n\nThe first requirement.\n\n#### Acceptance\n\n- recorded\n\n## REQ-002 Second requirement\n\nThe second requirement.\n\n#### Acceptance\n\n- recorded\n"
	if err := os.WriteFile(filepath.Join(prd, "fixture.md"), []byte(body), 0o600); err != nil {
		return err
	}
	parsed, err := intake.Parse(data, prd, rfc)
	if err != nil {
		return err
	}
	if len(parsed.Requirements) != 2 {
		return fmt.Errorf("step 1: requirements = %d", len(parsed.Requirements))
	}
	if _, err := intake.Apply(ctx, db, "proj_default", "", parsed); err != nil {
		return err
	}
	n, err := board.CountCards(ctx, db)
	if err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("step 1: run-next before accept created %d cards", n)
	}
	if _, err := os.Stat(filepath.Join(data, "worktrees")); !os.IsNotExist(err) {
		return errors.New("step 1: worktree existed before accept")
	}
	return nil
}

func step2(ctx context.Context, db *sql.DB) (board.Card, board.Card, error) {
	if err := intake.Accept(ctx, db, "proj_default", "ada", []string{"REQ-001", "REQ-002"}); err != nil {
		return board.Card{}, board.Card{}, err
	}
	planned, err := worker.Plan([]worker.Proposal{
		{Title: "card-1", Reqs: []string{"REQ-001"}, Risk: "low"},
		{Title: "card-2", Reqs: []string{"REQ-002"}, Depends: []string{"card-1"}, Risk: "low"},
	}, map[string]bool{"REQ-001": true, "REQ-002": true}, "check.sh", "false")
	if err != nil {
		return board.Card{}, board.Card{}, err
	}
	first, err := board.InsertCard(ctx, db, planned[0])
	if err != nil {
		return board.Card{}, board.Card{}, err
	}
	planned[1].DependsOn = []string{first.ID}
	second, err := board.InsertCard(ctx, db, planned[1])
	if err != nil {
		return board.Card{}, board.Card{}, err
	}
	if len(second.DependsOn) != 1 || second.DependsOn[0] != first.ID {
		return board.Card{}, board.Card{}, errors.New("step 2: second card does not depend on the first")
	}
	return first, second, nil
}

func step3(ctx context.Context, db *sql.DB, g runtime.Git, card board.Card) (board.Card, error) {
	var err error
	card, err = walk(ctx, db, card, "ready", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 3: %w", err)
	}
	card, err = walk(ctx, db, card, "in_progress", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 3: %w", err)
	}
	wt, err := g.EnsureWorktree(ctx, card.ID, "main")
	if err != nil {
		return card, fmt.Errorf("step 3: %w", err)
	}
	if err := board.StoreWorktree(ctx, db, card.ID, wt.Branch, wt.Path, wt.GitDir); err != nil {
		return card, err
	}
	cmd := exec.Command("/bin/sh", filepath.Join(wt.Path, "check.sh"))
	cmd.Dir = wt.Path
	if err := cmd.Run(); err != nil {
		return card, fmt.Errorf("step 3: gating check: %w", err)
	}
	creates := 0
	pr := vcs.EnsurePR("", "", "summary", "requirements: REQ-001", &creates)
	if creates != 1 {
		return card, errors.New("step 3: pull request was not created once")
	}
	exit := 0
	card, err = walk(ctx, db, card, "self_check", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 3: %w", err)
	}
	card, err = walk(ctx, db, card, "pr_open", machine.Evidence{ExitCode: &exit, PRURL: pr.URL})
	if err != nil {
		return card, fmt.Errorf("step 3: %w", err)
	}
	if card.PRURL == "" {
		return card, errors.New("step 3: missing pull request URL")
	}
	return card, nil
}

func step4(ctx context.Context, db *sql.DB, card board.Card) (board.Card, error) {
	var err error
	card, err = walk(ctx, db, card, "agent_review", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 4: %w", err)
	}
	_, user, _, err := contextpack.BuildReviewer([]string{"REQ-001"}, "diff --git a/hello.txt", "", "", "")
	if err != nil {
		return card, err
	}
	if strings.Contains(user, "author-transcript") {
		return card, errors.New("step 4: reviewer prompt contains author-transcript")
	}
	id, err := board.NewID("att_")
	if err != nil {
		return card, err
	}
	n, parent, err := board.NextAttempt(ctx, db, card.ID)
	if err != nil {
		return card, err
	}
	if err := board.InsertAttempt(ctx, db, board.Attempt{
		ID: id, CardID: card.ID, N: n, ParentID: parent, TaskClass: "implement", ModelID: "author",
	}); err != nil {
		return card, err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE attempts SET review_independent = true, review_model_id = 'reviewer', review_prompt_hash = $2
		WHERE id = $1`, id, user); err != nil {
		return card, err
	}
	var independent bool
	var prompt string
	if err := db.QueryRowContext(ctx, `
		SELECT review_independent, review_prompt_hash FROM attempts WHERE id = $1`, id).Scan(&independent, &prompt); err != nil {
		return card, err
	}
	if !independent || strings.Contains(prompt, "author-transcript") {
		return card, errors.New("step 4: stored review is not independent")
	}
	return review.ApplyCI(ctx, db, card, id, []review.Check{{Name: "test", State: "success"}}, nil)
}

func step5(ctx context.Context, db *sql.DB, card board.Card) (board.Card, error) {
	if _, err := db.ExecContext(ctx, `UPDATE cards SET head_sha = $2 WHERE id = $1`, card.ID, sha1); err != nil {
		return card, err
	}
	card, err := app.Approve(ctx, db, card.ID, sha1, gate())
	if err != nil {
		return card, fmt.Errorf("step 5: %w", err)
	}
	card, err = app.HeadMoved(ctx, db, card.ID, sha2)
	if err != nil {
		return card, fmt.Errorf("step 5: %w", err)
	}
	if card.Status != "human_review" || card.ApprovedSHA != "" {
		return card, fmt.Errorf("step 5: status %s approved %s", card.Status, card.ApprovedSHA)
	}
	card, err = app.Approve(ctx, db, card.ID, sha2, gate())
	if err != nil {
		return card, fmt.Errorf("step 5: %w", err)
	}
	return card, nil
}

func step6(ctx context.Context, db *sql.DB, card board.Card, stub string) (board.Card, error) {
	var err error
	card, err = walk(ctx, db, card, "deploying_dev", machine.Evidence{SHA: card.ApprovedSHA})
	if err != nil {
		return card, fmt.Errorf("step 6: %w", err)
	}
	rollbacks := 0
	card, err = deploy.Run(ctx, deploy.Job{
		DB: db, Card: card, Command: stub, Rollback: stub, Fixture: true,
		Exec:      func([]string) int { return 0 },
		RollbackN: &rollbacks,
		ProbeFn:   func() error { return errors.New("health failed") },
	})
	if err != nil {
		return card, fmt.Errorf("step 6: %w", err)
	}
	if card.Status != "blocked" || card.BlockedReason != "health" || rollbacks != 1 {
		return card, fmt.Errorf("step 6: status %s reason %s rollbacks %d", card.Status, card.BlockedReason, rollbacks)
	}
	var verifies int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'verify'`, card.ID).Scan(&verifies); err != nil {
		return card, err
	}
	if verifies != 0 {
		return card, errors.New("step 6: verify event was written")
	}
	dest, ok := app.UnblockTo(ctx, db, card)
	if !ok || dest != "human_review" {
		return card, fmt.Errorf("step 6: unblockTo %s", dest)
	}
	return card, nil
}

func step7(ctx context.Context, db *sql.DB, card board.Card, stub string) (board.Card, error) {
	before, err := attemptCount(ctx, db, card.ID)
	if err != nil {
		return card, err
	}
	pr := card.PRURL
	card, err = app.Unblock(ctx, db, card.ID, false)
	if err != nil {
		return card, fmt.Errorf("step 7: %w", err)
	}
	after, err := attemptCount(ctx, db, card.ID)
	if err != nil {
		return card, err
	}
	if card.Status != "human_review" || after != before {
		return card, fmt.Errorf("step 7: status %s attempts %d to %d", card.Status, before, after)
	}
	card, err = app.Approve(ctx, db, card.ID, sha2, gate())
	if err != nil {
		return card, fmt.Errorf("step 7: %w", err)
	}
	card, err = walk(ctx, db, card, "deploying_dev", machine.Evidence{SHA: card.ApprovedSHA})
	if err != nil {
		return card, fmt.Errorf("step 7: %w", err)
	}
	card, err = deploy.Run(ctx, deploy.Job{
		DB: db, Card: card, Command: stub, Rollback: stub, HealthURL: "http://127.0.0.1/health",
		Hosts: []string{"127.0.0.1"}, Fixture: true,
		Exec:    func([]string) int { return 0 },
		ProbeFn: func() error { return nil },
	})
	if err != nil {
		return card, fmt.Errorf("step 7: %w", err)
	}
	card, err = verify.Finish(ctx, db, card, 1, false)
	if err != nil {
		return card, fmt.Errorf("step 7: %w", err)
	}
	if card.Status != "changes_requested" || card.PRURL != pr {
		return card, fmt.Errorf("step 7: status %s pr %s", card.Status, card.PRURL)
	}
	n, parent, err := board.NextAttempt(ctx, db, card.ID)
	if err != nil {
		return card, err
	}
	if err := board.InsertAttempt(ctx, db, board.Attempt{
		ID: "", CardID: card.ID, N: n, ParentID: parent, TaskClass: "implement", ModelID: "author",
	}); err != nil {
		return card, err
	}
	var notes int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM failure_notes WHERE card_id = $1 AND source = 'dev_check'`, card.ID).Scan(&notes); err != nil {
		return card, err
	}
	if notes == 0 {
		return card, errors.New("step 7: failure note was not stored")
	}
	return card, nil
}

func step8(ctx context.Context, db *sql.DB, g runtime.Git, card, second board.Card, stub string) (board.Card, error) {
	var err error
	card, err = walk(ctx, db, card, "in_progress", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	exit := 0
	card, err = walk(ctx, db, card, "self_check", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	card, err = walk(ctx, db, card, "pr_open", machine.Evidence{ExitCode: &exit, PRURL: card.PRURL})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	card, err = walk(ctx, db, card, "agent_review", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	card, err = walk(ctx, db, card, "human_review", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE cards SET head_sha = $2 WHERE id = $1`, card.ID, sha2); err != nil {
		return card, err
	}
	card, err = app.Approve(ctx, db, card.ID, sha2, gate())
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	card, err = walk(ctx, db, card, "deploying_dev", machine.Evidence{SHA: card.ApprovedSHA})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	card, err = deploy.Run(ctx, deploy.Job{
		DB: db, Card: card, Command: stub, Rollback: stub, Fixture: true,
		Exec:    func([]string) int { return 0 },
		ProbeFn: func() error { return nil },
	})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	second, err = board.GetCard(ctx, db, second.ID)
	if err != nil {
		return card, err
	}
	if second.Status == "in_progress" {
		return card, errors.New("step 8: card 2 started before card 1 was done")
	}
	card, err = verify.Finish(ctx, db, card, 0, false)
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	if card.Status != "done" {
		return card, fmt.Errorf("step 8: status %s", card.Status)
	}
	if !worker.CanStart(map[string]bool{card.ID: true}, second.DependsOn) {
		return card, errors.New("step 8: dependency did not release card 2")
	}
	second, err = walk(ctx, db, second, "ready", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	second, err = walk(ctx, db, second, "in_progress", machine.Evidence{})
	if err != nil {
		return card, fmt.Errorf("step 8: %w", err)
	}
	if second.Status != "in_progress" {
		return card, errors.New("step 8: card 2 did not start")
	}
	_ = g
	return card, nil
}

func step9(ctx context.Context, db *sql.DB, repo, data, stub string) error {
	card, err := board.InsertCard(ctx, db, board.Card{Title: "reconcile", Status: "backlog"})
	if err != nil {
		return err
	}
	for _, hop := range []struct {
		to string
		ev machine.Evidence
	}{
		{"ready", machine.Evidence{}},
		{"in_progress", machine.Evidence{}},
		{"self_check", machine.Evidence{}},
		{"pr_open", machine.Evidence{ExitCode: intPtr(0), PRURL: "http://127.0.0.1/pull/9"}},
		{"agent_review", machine.Evidence{}},
		{"human_review", machine.Evidence{}},
	} {
		card, err = walk(ctx, db, card, hop.to, hop.ev)
		if err != nil {
			return fmt.Errorf("step 9: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE cards SET head_sha = $2 WHERE id = $1`, card.ID, sha1); err != nil {
		return err
	}
	card, err = app.Approve(ctx, db, card.ID, sha1, gate())
	if err != nil {
		return fmt.Errorf("step 9: %w", err)
	}
	card, err = walk(ctx, db, card, "deploying_dev", machine.Evidence{SHA: sha1})
	if err != nil {
		return fmt.Errorf("step 9: %w", err)
	}
	if _, err := trace.Append(ctx, db, trace.Event{CardID: card.ID, Kind: "deploy", Payload: map[string]any{"exitCode": 0}}, nil); err != nil {
		return err
	}
	execs := 1
	deployedAgain := false
	if err := deploy.Reconcile(ctx, db, card, &execs, func([]string) int { return 0 }); err != nil {
		return err
	}
	if execs != 0 || deployedAgain {
		return fmt.Errorf("step 9: deploy ran again, execs %d", execs)
	}
	if err := deploy.ValidCommand(stub, repo, data, os.Getuid()); err != nil {
		return fmt.Errorf("step 9: stub: %w", err)
	}
	return nil
}

// budgetCaps runs the tool loop until each cap stops it (RFC-081): every turn
// spent without a commit ends in turn_cap, and a card over its token cap ends
// in token_cap. Each blocks the card with a budget_stop event.
func budgetCaps(ctx context.Context, db *sql.DB, g runtime.Git) error {
	listing := strings.Repeat(`{"tool":"list_files","path":"."},`, 3)
	for _, c := range []struct {
		cap, script string
		maxTokens   int
	}{
		{"turn_cap", `{"implement":[` + listing + `{"tool":"commit","message":"late"}]}`, 0},
		{"token_cap", `{"implement":[` + listing + `{"tool":"list_files"}]}`, 100},
	} {
		card, err := board.InsertCard(ctx, db, board.Card{Title: c.cap, Status: "backlog", LocalCheck: "check.sh"})
		if err != nil {
			return err
		}
		for _, to := range []string{"ready", "in_progress"} {
			if card, err = walk(ctx, db, card, to, machine.Evidence{}); err != nil {
				return fmt.Errorf("budget caps: %w", err)
			}
		}
		provider, err := fake.Load([]byte(c.script))
		if err != nil {
			return err
		}
		provider.Usage = providers.Response{InputTokens: 80, OutputTokens: 30}
		result, err := app.RunImplementer(ctx, loop.Input{
			DB: db, Git: g, CardID: card.ID, Model: "author", TaskClass: "implement",
			MaxTurns: 3, MaxTokensPerCard: c.maxTokens, Provider: provider, Allow: []string{"true", "false"},
		})
		removeWorktree(g, card.ID)
		if err != nil {
			return fmt.Errorf("budget caps: %w", err)
		}
		if result.Outcome != "budget" || result.Reason != c.cap {
			return fmt.Errorf("budget caps: %s ended %q %q", c.cap, result.Outcome, result.Reason)
		}
		if card, err = board.GetCard(ctx, db, card.ID); err != nil {
			return err
		}
		if card, err = app.BlockCard(ctx, db, card, result.Reason, nil); err != nil {
			return fmt.Errorf("budget caps: %w", err)
		}
		if card.Status != "blocked" || card.BlockedReason != c.cap {
			return fmt.Errorf("budget caps: %s card is %s %s", c.cap, card.Status, card.BlockedReason)
		}
		var stops int
		if err := db.QueryRowContext(ctx, `
			SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'budget_stop' AND payload_json LIKE '%"cap":"' || $2 || '"%'`,
			card.ID, c.cap).Scan(&stops); err != nil {
			return err
		}
		if stops != 1 {
			return fmt.Errorf("budget caps: %s budget_stop events = %d", c.cap, stops)
		}
	}
	return nil
}

func walk(ctx context.Context, db *sql.DB, card board.Card, to string, ev machine.Evidence) (board.Card, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return card, err
	}
	defer tx.Rollback()
	next, err := machine.Transition(ctx, tx, card, to, ev, nil)
	if err != nil {
		if errors.Is(err, machine.ErrIllegal) || errors.Is(err, machine.ErrEvidence) {
			_ = tx.Commit()
		}
		return card, err
	}
	if err := tx.Commit(); err != nil {
		return card, err
	}
	return next, nil
}

func attemptCount(ctx context.Context, db *sql.DB, cardID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM attempts WHERE card_id = $1 AND task_class = 'implement'`, cardID).Scan(&n)
	return n, err
}

func gate() app.GateConfig {
	return app.GateConfig{Operator: "ada", BotLogin: "bot", Approvers: []string{"ada"}}
}

func intPtr(n int) *int { return &n }

func fixtureRepo() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(cwd, "testdata", "fixture-repo"),
		filepath.Join(cwd, "backend", "testdata", "fixture-repo"),
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}
	return "", errors.New("testdata/fixture-repo is missing")
}

func copyStub() (string, error) {
	repo, err := fixtureRepo()
	if err != nil {
		return "", err
	}
	src := filepath.Join(filepath.Dir(repo), "bin", "deploy-stub")
	dir, err := os.MkdirTemp("", "orchestrator-deploy-")
	if err != nil {
		return "", err
	}
	dest := filepath.Join(dir, "deploy-stub")
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0o700)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(dest, 0o700); err != nil {
		return "", err
	}
	return dest, nil
}

func openSchema(raw string) (*sql.DB, func(), error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, nil, err
	}
	schema := "s" + hex.EncodeToString(buf[:])
	admin, err := board.Open(raw)
	if err != nil {
		return nil, nil, err
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		return nil, nil, err
	}
	db, err := board.OpenInSchema(raw, schema)
	if err != nil {
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
		return nil, nil, err
	}
	if err := board.Migrate(db); err != nil {
		db.Close()
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
		return nil, nil, err
	}
	return db, func() {
		db.Close()
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	}, nil
}

func ensureMain(repo, gitBin string) error {
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		if out, err := exec.Command(gitBin, "-C", repo, "init", "-b", "main").CombinedOutput(); err != nil {
			return fmt.Errorf("git init: %s", out)
		}
		if out, err := exec.Command(gitBin, "-C", repo, "add", "hello.txt", "check.sh").CombinedOutput(); err != nil {
			return fmt.Errorf("git add: %s", out)
		}
		cmd := exec.Command(gitBin, "-C", repo, "-c", "user.name=bot", "-c", "user.email=bot@users.noreply.github.com", "commit", "-m", "fixture")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git commit: %s", out)
		}
	}
	return nil
}

func removeWorktree(g runtime.Git, cardID string) {
	path := filepath.Join(g.Data, "worktrees", cardID)
	_, _ = exec.Command(g.Bin, "-C", g.Repo, "worktree", "remove", "--force", path).CombinedOutput()
	_, _ = exec.Command(g.Bin, "-C", g.Repo, "branch", "-D", "orch/"+cardID).CombinedOutput()
}
