package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/tenancy"
)

const (
	routerConfig = "router:\n  routes:\n    implement:\n      model: fake-model\n    implement_hard:\n      model: fake-model\n  budgets:\n    maxAttempts: 3\n    maxModelTurnsPerAttempt: 6\n"
	commitScript = `{"implement":[{"tool":"edit_file","path":"hello.txt","text":"job\n"},{"tool":"commit","message":"Job commit"}],
		"implement_hard":[{"tool":"edit_file","path":"hello.txt","text":"job again\n"},{"tool":"commit","message":"Job commit"}]}`
)

type jobFixture struct {
	db    *sql.DB
	data  string
	repo  string
	card  board.Card
	calls *jobExec
}

// jobExec runs git for real and fakes the network: fetch, the FETCH_HEAD
// checkout, push, and every gh call.
type jobExec struct {
	mu          sync.Mutex
	faked       [][]string
	failArchive int
	failPush    int
	failComment int
	prView      string
	// bodies holds each pull request body or comment gh sent: create, edit, comment.
	bodies map[string][]string
	titles []string
}

func (e *jobExec) record(argv []string) {
	add := func(kind, text string) {
		if e.bodies == nil {
			e.bodies = map[string][]string{}
		}
		e.bodies[kind] = append(e.bodies[kind], text)
	}
	for i, arg := range argv {
		if i+1 >= len(argv) {
			break
		}
		switch arg {
		case "--title":
			e.titles = append(e.titles, argv[i+1])
		case "--body-file":
			text, _ := os.ReadFile(argv[i+1])
			add(argv[2], string(text))
		case "--input":
			raw, _ := os.ReadFile(argv[i+1])
			var payload map[string]string
			_ = json.Unmarshal(raw, &payload)
			kind := "comment"
			if slices.Contains(argv, "PATCH") {
				kind = "edit"
			}
			if title, ok := payload["title"]; ok {
				e.titles = append(e.titles, title)
			}
			add(kind, payload["body"])
		}
	}
}

func isCommentCall(argv []string) bool {
	return slices.ContainsFunc(argv, func(arg string) bool { return strings.HasSuffix(arg, "/comments") })
}

func (e *jobExec) body(sub string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.bodies[sub]...)
}

// setPR sets the JSON that `gh pr view --json headRefOid,...` returns.
func (e *jobExec) setPR(json string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.prView = json
}

func (e *jobExec) run(ctx context.Context, child execx.Child) (execx.Result, error) {
	argv := child.Argv
	if filepath.Base(argv[0]) == "gh" && slices.Contains(argv, "headRefOid,body,comments,statusCheckRollup") {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.faked = append(e.faked, argv)
		return execx.Result{Out: []byte(e.prView)}, nil
	}
	if slices.Contains(argv, "archive") {
		e.mu.Lock()
		fail := e.failArchive > 0
		if fail {
			e.failArchive--
		}
		e.mu.Unlock()
		if fail {
			return execx.Result{Exit: 128, Out: []byte("fatal: archive failed")}, nil
		}
	}
	if filepath.Base(argv[0]) == "gh" || slices.Contains(argv, "fetch") || slices.Contains(argv, "push") || slices.Contains(argv, "FETCH_HEAD") {
		e.mu.Lock()
		e.faked = append(e.faked, argv)
		if filepath.Base(argv[0]) == "gh" && len(argv) > 2 {
			e.record(argv)
		}
		fail := slices.Contains(argv, "push") && e.failPush > 0
		if fail {
			e.failPush--
		}
		failComment := filepath.Base(argv[0]) == "gh" && isCommentCall(argv) && e.failComment > 0
		if failComment {
			e.failComment--
		}
		e.mu.Unlock()
		if fail {
			return execx.Result{Exit: 128, Out: []byte("remote: Write access to repository not granted.")}, nil
		}
		if failComment {
			return execx.Result{Exit: 1, Out: []byte("HTTP 502: bad gateway")}, nil
		}
		if slices.Contains(argv, "create") {
			return execx.Result{Out: []byte(prURL + "\n")}, nil
		}
		return execx.Result{}, nil
	}
	return execx.Command(ctx, child)
}

func (e *jobExec) has(word string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, argv := range e.faked {
		if slices.Contains(argv, word) {
			return true
		}
	}
	return false
}

func newJobFixture(t *testing.T, local string, maxAttempts int, router, project string) jobFixture {
	t.Helper()
	db := boardtest.Open(t)
	ctx := context.Background()
	data := dataDir(t)
	repo := openFixture(t)
	admin := githubUser(t, db, "admin@example.com", "", 0, "")
	if _, err := db.Exec(`UPDATE users SET admin = true WHERE id = $1`, admin.ID); err != nil {
		t.Fatal(err)
	}
	admin.Admin = true
	token := companyToken
	if _, err := app.SaveCompany(ctx, db, admin, "github.com", "acme", &token, false, tokenKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE workspace_github SET bot_login = 'acme-bot' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := tenancy.PrepareProject(data, "proj_default"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE projects SET github_repo = 'widget', github_base = 'main', repo_path = $1 WHERE id = 'proj_default'`, repo); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(data, "config.yaml"), router)
	writeFile(t, filepath.Join(settings.ProjectDir(data, "proj_default"), "config.yaml"), project)
	owner := githubUser(t, db, "grace@example.com", "grace", 11, ownerToken)
	acceptedReq(t, db, "REQ-001", owner.ID)
	card, err := board.InsertCard(ctx, db, board.Card{
		ProjectID: "proj_default", OwnerID: owner.ID, Title: "Say job", Status: "ready",
		RequirementIDs: []string{"REQ-001"}, LocalCheck: local, MaxAttempts: maxAttempts,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupWorktree(t, repo, settings.ProjectDir(data, "proj_default"), card.ID) })
	return jobFixture{db: db, data: data, repo: repo, card: card, calls: &jobExec{}}
}

func (f jobFixture) run(t *testing.T, script string) board.Card {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	card, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

const fixtureProject = "checks:\n  allowExecutables: [\"true\", \"false\"]\n"

func TestImplementReachesPROpen(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	if _, err := f.db.Exec(`
		UPDATE requirements SET statement = $1, acceptance_json = '["Hello says job"]'
		WHERE id = 'REQ-001' AND project_id = 'proj_default'`, "Say job. token "+ownerToken); err != nil {
		t.Fatal(err)
	}
	card := f.run(t, commitScript)
	if card.Status != "pr_open" || card.PRURL != prURL {
		t.Fatalf("card = %s %s", card.Status, card.PRURL)
	}
	if !f.calls.has("push") || !f.calls.has("create") {
		t.Fatalf("faked calls = %v", f.calls.faked)
	}
	if !slices.Contains(f.calls.titles, "REQ-001: Say job") {
		t.Fatalf("pr titles = %v", f.calls.titles)
	}
	bodies := f.calls.body("create")
	if len(bodies) != 1 {
		t.Fatalf("create bodies = %d", len(bodies))
	}
	body := bodies[0]
	for _, want := range []string{"## Summary\n\nJob commit", "**REQ-001**: Say job. token [redacted]", "- [ ] Hello says job",
		"- `hello.txt` (+", "Local check `true` exited 0", "<details><summary>Orchestrator metadata</summary>", "card: " + card.ID} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, ownerToken) || strings.Contains(body, "Orchestrator-Card:") {
		t.Fatalf("body leaks a token or trailer:\n%s", body)
	}
	var outcome, sha string
	var used int
	if err := f.db.QueryRow(`
		SELECT a.outcome, a.commit_sha, c.attempts_used FROM attempts a JOIN cards c ON c.id = a.card_id
		WHERE a.card_id = $1`, card.ID).Scan(&outcome, &sha, &used); err != nil {
		t.Fatal(err)
	}
	if outcome != "checks_passed" || len(sha) != 40 || used != 1 {
		t.Fatalf("attempt = %s %s used %d", outcome, sha, used)
	}
	var gates int
	if err := f.db.QueryRow(`SELECT count(*) FROM gates WHERE card_id = $1 AND name = 'local_checks'`, card.ID).Scan(&gates); err != nil {
		t.Fatal(err)
	}
	if gates != 1 {
		t.Fatalf("local_checks gates = %d", gates)
	}
}

func TestImplementGatesExistingCommit(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	f.calls.failArchive = 1
	card := f.run(t, commitScript)
	if card.Status != "blocked" || card.BlockedReason != "check_error" {
		t.Fatalf("first run = %s %s", card.Status, card.BlockedReason)
	}
	ctx := context.Background()
	if _, err := app.Unblock(ctx, f.db, card.ID, true); err != nil {
		t.Fatal(err)
	}
	provider, err := fake.Load([]byte(commitScript))
	if err != nil {
		t.Fatal(err)
	}
	card, err = app.ImplementCard(ctx, f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" || card.PRURL != prURL {
		t.Fatalf("rerun = %s %s", card.Status, card.PRURL)
	}
	if n := len(provider.Prompts()); n != 0 {
		t.Fatalf("rerun made %d model calls, want 0", n)
	}
	var attempts int
	var outcome string
	if err := f.db.QueryRow(`
		SELECT count(*), max(outcome) FROM attempts WHERE card_id = $1`, card.ID).Scan(&attempts, &outcome); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || outcome != "checks_passed" {
		t.Fatalf("attempts = %d outcome %s", attempts, outcome)
	}
}

func TestImplementRepublishesPassedCommit(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	f.calls.failPush = 1
	card := f.run(t, commitScript)
	if card.Status != "blocked" || card.BlockedReason != "publish_failed" || card.AttemptsUsed != 1 {
		t.Fatalf("first run = %s %s used %d", card.Status, card.BlockedReason, card.AttemptsUsed)
	}
	ctx := context.Background()
	if _, err := app.Unblock(ctx, f.db, card.ID, false); err != nil {
		t.Fatal(err)
	}
	provider, err := fake.Load([]byte(commitScript))
	if err != nil {
		t.Fatal(err)
	}
	card, err = app.ImplementCard(ctx, f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" || card.PRURL != prURL || card.AttemptsUsed != 1 {
		t.Fatalf("rerun = %s %s used %d", card.Status, card.PRURL, card.AttemptsUsed)
	}
	if n := len(provider.Prompts()); n != 0 {
		t.Fatalf("rerun made %d model calls, want 0", n)
	}
	var attempts int
	if err := f.db.QueryRow(`SELECT count(*) FROM attempts WHERE card_id = $1`, card.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestImplementCheckFailsReturnsInProgress(t *testing.T) {
	f := newJobFixture(t, "false", 3, routerConfig, fixtureProject)
	card := f.run(t, commitScript)
	if card.Status != "in_progress" || card.AttemptsUsed != 1 {
		t.Fatalf("card = %s used %d", card.Status, card.AttemptsUsed)
	}
	if n := countNotes(t, f.db, card.ID, "local_check"); n != 1 {
		t.Fatalf("local_check notes = %d", n)
	}
	if f.calls.has("push") {
		t.Fatal("a failed check pushed")
	}
	ok, err := app.CanStartImplement(context.Background(), f.db, card)
	if err != nil || !ok {
		t.Fatalf("idle in_progress card cannot start again: %t %v", ok, err)
	}
}

func TestImplementAttemptCap(t *testing.T) {
	f := newJobFixture(t, "false", 1, routerConfig, fixtureProject)
	card := f.run(t, commitScript)
	if card.Status != "blocked" || card.BlockedReason != "attempt_cap" {
		t.Fatalf("card = %s %s", card.Status, card.BlockedReason)
	}
}

func TestImplementBlocksSandboxConfig(t *testing.T) {
	f := newJobFixture(t, "rm -r src", 3, routerConfig, fixtureProject)
	card := f.run(t, commitScript)
	if card.Status != "blocked" || card.BlockedReason != "sandbox_config" {
		t.Fatalf("not allowlisted = %s %s", card.Status, card.BlockedReason)
	}
}

func TestImplementBlocksUnpinnedImage(t *testing.T) {
	real := newJobFixture(t, "", 3, routerConfig, "checks:\n  local: npm test\n  allowExecutables: [npm]\nsandbox:\n  image: node:22\n")
	if _, err := real.db.Exec(`UPDATE projects SET repo_path = $1 WHERE id = 'proj_default'`, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	card := real.run(t, commitScript)
	if card.Status != "blocked" || card.BlockedReason != "sandbox_config" {
		t.Fatalf("unpinned image = %s %s", card.Status, card.BlockedReason)
	}
	if card.LocalCheck != "npm test" {
		t.Fatalf("local check not copied from the project: %q", card.LocalCheck)
	}
	if len(real.calls.faked) != 0 {
		t.Fatalf("network calls before preflight passed: %v", real.calls.faked)
	}
}

func TestImplementBlocksWhenEngineDown(t *testing.T) {
	image := "localhost/check@sha256:" + strings.Repeat("a", 64)
	real := newJobFixture(t, "", 3, routerConfig, "checks:\n  local: scbd-check\n  allowExecutables: [scbd-check]\nsandbox:\n  engine: podman\n  image: "+image+"\n")
	if _, err := real.db.Exec(`UPDATE projects SET repo_path = $1 WHERE id = 'proj_default'`, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	var probed []string
	down := func(ctx context.Context, child execx.Child) (execx.Result, error) {
		if filepath.Base(child.Argv[0]) == "podman" {
			probed = child.Argv
			return execx.Result{Exit: 125, Out: []byte("Cannot connect to Podman. Please verify your connection\n")}, nil
		}
		return real.calls.run(ctx, child)
	}
	provider, err := fake.Load([]byte(commitScript))
	if err != nil {
		t.Fatal(err)
	}
	card, err := app.ImplementCard(context.Background(), real.db, app.JobDeps{
		Data: real.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Podman: "/opt/podman/bin/podman", Exec: down},
	}, "proj_default", real.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "blocked" || card.BlockedReason != "sandbox_config" {
		t.Fatalf("card = %s %s", card.Status, card.BlockedReason)
	}
	if len(probed) != 2 || probed[1] != "version" {
		t.Fatalf("probe argv = %v", probed)
	}
	var payload string
	if err := real.db.QueryRow(`SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'job_error'`, card.ID).Scan(&payload); err != nil {
		t.Fatalf("job_error event: %v", err)
	}
	if !strings.Contains(payload, "not reachable") || !strings.Contains(payload, "Cannot connect to Podman") {
		t.Fatalf("payload = %s", payload)
	}
	if n := len(provider.Prompts()); n != 0 {
		t.Fatalf("model calls = %d", n)
	}
}

func TestImplementBlocksRouterConfig(t *testing.T) {
	f := newJobFixture(t, "true", 3, "router:\n  routes: {}\n", fixtureProject)
	card := f.run(t, commitScript)
	if card.Status != "blocked" || card.BlockedReason != "router_config" {
		t.Fatalf("card = %s %s", card.Status, card.BlockedReason)
	}
	var attempts int
	if err := f.db.QueryRow(`SELECT count(*) FROM attempts WHERE card_id = $1`, card.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("attempts = %d", attempts)
	}
}

type failingProvider struct{}

func (failingProvider) Complete(context.Context, providers.Request) (providers.Response, error) {
	return providers.Response{}, fmt.Errorf("%w: status 413: Request too large", app.ErrProvider)
}

func TestImplementProviderErrorRecordsMessage(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	card, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: failingProvider{},
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "blocked" || card.BlockedReason != "provider_error" {
		t.Fatalf("card = %s %s", card.Status, card.BlockedReason)
	}
	var payload string
	if err := f.db.QueryRow(`SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'job_error'`, card.ID).Scan(&payload); err != nil {
		t.Fatalf("job_error event: %v", err)
	}
	if !strings.Contains(payload, "status 413") {
		t.Fatalf("payload = %s", payload)
	}
}

func TestImplementRefusesOpenAttempt(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	if err := board.InsertAttempt(context.Background(), f.db, board.Attempt{CardID: f.card.ID, N: 1, TaskClass: "implement"}); err != nil {
		t.Fatal(err)
	}
	provider, err := fake.Load([]byte(commitScript))
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.ImplementCard(context.Background(), f.db, app.JobDeps{Data: f.data, Key: tokenKey, Provider: provider}, "proj_default", f.card.ID)
	if err == nil || !strings.Contains(err.Error(), "cannot start") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(settings.ProjectDir(f.data, "proj_default"), "worktrees", f.card.ID)); !os.IsNotExist(err) {
		t.Fatalf("worktree created: %v", err)
	}
}
