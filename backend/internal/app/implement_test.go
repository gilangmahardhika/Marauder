package app_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/runtime"
)

func TestNoCommitAttempt(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := insertReady(t, db, "")
	ctx := context.Background()
	first := runScript(t, ctx, db, repo, data, card.ID, `{"implement":[{"text":"nothing to commit"}]}`)
	if first.Outcome != "no_commit" || first.Status != "in_progress" {
		t.Fatalf("first = %+v", first)
	}
	if got := reload(t, db, card.ID); got.AttemptsUsed != 1 || got.Status != "in_progress" {
		t.Fatalf("card after first = %+v", got)
	}
	if notes := countNotes(t, db, card.ID, "no_commit"); notes != 1 {
		t.Fatalf("notes = %d", notes)
	}
	second := runScript(t, ctx, db, repo, data, card.ID, `{"implement":[{"text":"still nothing"}]}`)
	if second.Outcome != "no_commit" || second.AttemptID == first.AttemptID {
		t.Fatalf("second = %+v first %s", second, first.AttemptID)
	}
	var n int
	if err := db.QueryRow(`SELECT n FROM attempts WHERE id = $1`, second.AttemptID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("attempt n = %d", n)
	}
	cleanupWorktree(t, repo, data, card.ID)
}

func TestToolCheckDoesNotMove(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: "in_progress", LocalCheck: "true"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before := reload(t, db, card.ID)
	result := runScript(t, ctx, db, repo, data, card.ID, `{"implement":[{"tool":"run_local_check"}]}`)
	after := reload(t, db, card.ID)
	if after.Status != before.Status || after.Status != "in_progress" {
		t.Fatalf("status %s -> %s result %+v", before.Status, after.Status, result)
	}
	cleanupWorktree(t, repo, data, card.ID)
}

func TestCommitIdentity(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := insertReady(t, db, "true")
	script := `{"implement":[{"tool":"edit_file","path":"hello.txt","text":"changed\n"},{"tool":"commit","message":"Make the check pass"}]}`
	result := runScript(t, context.Background(), db, repo, data, card.ID, script)
	gitDir := filepath.Join(data, "worktrees", card.ID, ".git")
	body, err := os.ReadFile(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(body)), "gitdir:"))
	out, err := exec.Command(lookGit(t), "--git-dir="+gitdir, "log", "-1", "--format=%an%n%ae%n%B").Output()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"bot",
		"bot@users.noreply.github.com",
		"Orchestrator-Card: " + card.ID,
		"Orchestrator-Attempt: " + result.AttemptID,
		"Model: fake-model",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("log missing %s\n%s", want, text)
		}
	}
	cleanupWorktree(t, repo, data, card.ID)
}

func TestToolNotInProfile(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := insertReady(t, db, "true")
	runScript(t, context.Background(), db, repo, data, card.ID, `{"implement":[{"tool":"deploy"}]}`)
	var payload string
	if err := db.QueryRow(`
		SELECT payload_json FROM events
		WHERE card_id = $1 AND kind = 'tool_call' AND payload_json LIKE '%"name":"deploy"%'
		ORDER BY ts DESC LIMIT 1`, card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"rejected":true`) {
		t.Fatalf("payload = %s", payload)
	}
	cleanupWorktree(t, repo, data, card.ID)
}

func TestToolResultsRedacted(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := insertReady(t, db, "true")
	const secret = "sk-live-0123456789abcdef"
	provider, err := fake.Load([]byte(`{"implement":[
		{"tool":"edit_file","path":"notes.txt","text":"key=` + secret + `\n"},
		{"tool":"read_file","path":"notes.txt"},
		{"tool":"search_files","text":"key="}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.RunImplementer(context.Background(), loop.Input{
		DB: db, Git: runtime.Git{Bin: lookGit(t), Repo: repo, Data: data, Author: runtime.LoginAuthor("bot", 0)},
		CardID: card.ID, Model: "fake-model", TaskClass: "implement", MaxTurns: 4,
		Provider: provider, Allow: []string{"true"}, Secrets: []string{secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	prompts := provider.Prompts()
	if len(prompts) < 4 {
		t.Fatalf("prompts = %d", len(prompts))
	}
	turns := prompts[3].Turns
	read, search := turns[1].Results[0], turns[2].Results[0]
	if strings.Contains(read+search, secret) || !strings.Contains(read, "[redacted]") || !strings.Contains(search, "[redacted]") {
		t.Fatalf("results = %q, %q", read, search)
	}
	cleanupWorktree(t, repo, data, card.ID)
}

func TestCreateCardIgnoresCommand(t *testing.T) {
	db := boardtest.Open(t)
	if _, err := db.Exec(`
		INSERT INTO requirements (
			id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json, content_hash, accepted_at, accepted_by
		) VALUES (
			'REQ-001', 'prd', 'docs/prd/a.md', 'REQ-001', 'statement', '[]', '[]', 'abc', '2026-01-01T00:00:00Z', 'ada'
		)`); err != nil {
		t.Fatal(err)
	}
	card, err := app.CreateCard(context.Background(), loop.CreateInput{
		DB: db,
		Settings: loop.Settings{
			LocalCheck: "true",
			DevCheck:   "false",
		},
		Args: map[string]any{
			"title":          "Hello",
			"requirementIds": []any{"REQ-001"},
			"command":        "rm -rf /",
			"risk":           "low",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var local string
	if err := db.QueryRow(`SELECT local_check FROM cards WHERE id = $1`, card.ID).Scan(&local); err != nil {
		t.Fatal(err)
	}
	if local != "true" {
		t.Fatalf("local_check = %s", local)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM events WHERE kind = 'tool_call'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"rejected":true`) {
		t.Fatalf("payload = %s", payload)
	}
}

func runScript(t *testing.T, ctx context.Context, db *sql.DB, repo, data, cardID, script string) loop.Result {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RunImplementer(ctx, loop.Input{
		DB: db, Git: runtime.Git{Bin: lookGit(t), Repo: repo, Data: data, Author: runtime.LoginAuthor("bot", 0)},
		CardID: cardID, Model: "fake-model", TaskClass: "implement", MaxTurns: 4,
		Provider: provider, Allow: []string{"true", "false", "touch"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func insertReady(t *testing.T, db *sql.DB, local string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: "ready", LocalCheck: local})
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func reload(t *testing.T, db *sql.DB, id string) board.Card {
	t.Helper()
	card, err := board.GetCard(context.Background(), db, id)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func countNotes(t *testing.T, db *sql.DB, cardID, source string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM failure_notes WHERE card_id = $1 AND source = $2`, cardID, source).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var (
	fixtureMu   sync.Mutex
	fixtureHeld bool
)

// openFixture locks the shared fixture repository until the test ends. A
// second open in the same test would wait on its own lock forever.
func openFixture(t *testing.T) string {
	t.Helper()
	fixtureMu.Lock()
	held := fixtureHeld
	fixtureHeld = true
	fixtureMu.Unlock()
	if held {
		t.Fatal("openFixture: the fixture repository is already open in this test; it stays locked until cleanup")
	}
	t.Cleanup(func() {
		fixtureMu.Lock()
		fixtureHeld = false
		fixtureMu.Unlock()
	})
	repo := filepath.Join(moduleRoot(t), "testdata", "fixture-repo")
	lock, err := os.OpenFile(repo+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		lock.Close()
	})
	git := lookGit(t)
	if err := os.Chmod(filepath.Join(repo, "check.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); os.IsNotExist(err) {
		runGit(t, repo, git, "init", "-b", "main")
		runGit(t, repo, git, "add", "hello.txt", "check.sh")
		runGit(t, repo, git, "-c", "user.name=fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "fixture")
	}
	return repo
}

func dataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"empty-hooks", "empty-gh", "worktrees", "exports"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func cleanupWorktree(t *testing.T, repo, data, cardID string) {
	t.Helper()
	path := filepath.Join(data, "worktrees", cardID)
	git := lookGit(t)
	_ = exec.Command(git, "-C", repo, "worktree", "remove", "--force", path).Run()
	_ = exec.Command(git, "-C", repo, "branch", "-D", "orch/"+cardID).Run()
}

func lookGit(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func runGit(t *testing.T, dir, git string, args ...string) {
	t.Helper()
	cmd := exec.Command(git, append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
