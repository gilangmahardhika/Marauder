package runner_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/runner"
	"orchestrator/backend/internal/runnerapi"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/tenancy"
)

const (
	companyToken = "company-token-value"
	ownerToken   = "owner-token-value"
	modelKey     = "sk-model-key-value"
	databaseURL  = "postgres://board:board-password@db.internal/board"
	prURL        = "https://github.com/acme/widget/pull/7"

	routerConfig  = "jobs:\n  executor: runners\nrouter:\n  routes:\n    implement:\n      model: fake-model\n    implement_hard:\n      model: fake-model\n  budgets:\n    maxAttempts: 3\n    maxModelTurnsPerAttempt: 6\n"
	projectConfig = "checks:\n  allowExecutables: [\"true\", \"false\"]\n"
	commitScript  = `{"implement":[{"tool":"edit_file","path":"hello.txt","text":"job\n","say":"editing hello"},{"tool":"commit","message":"Job commit"}],
		"implement_hard":[{"tool":"edit_file","path":"hello.txt","text":"job again\n"},{"tool":"commit","message":"Job commit"}]}`
)

var tokenKey = bytes.Repeat([]byte("k"), 32)

// netFake runs git for real and fakes the network: fetch, the FETCH_HEAD
// checkout, push, and every gh call.
type netFake struct {
	mu    sync.Mutex
	faked [][]string
}

func (f *netFake) run(ctx context.Context, child execx.Child) (execx.Result, error) {
	argv := child.Argv
	if filepath.Base(argv[0]) == "gh" || slices.Contains(argv, "fetch") || slices.Contains(argv, "push") || slices.Contains(argv, "FETCH_HEAD") {
		f.mu.Lock()
		f.faked = append(f.faked, argv)
		f.mu.Unlock()
		if filepath.Base(argv[0]) == "gh" && slices.Contains(argv, "create") {
			return execx.Result{Out: []byte(prURL + "\n")}, nil
		}
		return execx.Result{}, nil
	}
	return execx.Command(ctx, child)
}

func (f *netFake) has(word string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, argv := range f.faked {
		if slices.Contains(argv, word) {
			return true
		}
	}
	return false
}

// recording is the control plane's model provider. It records the model and
// task class of each request and streams like the fake it wraps.
type recording struct {
	*fake.Fake
	mu     sync.Mutex
	models []string
}

func (r *recording) note(req providers.Request) {
	r.mu.Lock()
	r.models = append(r.models, req.Model+"/"+req.TaskClass)
	r.mu.Unlock()
}

func (r *recording) Complete(ctx context.Context, req providers.Request) (providers.Response, error) {
	r.note(req)
	return r.Fake.Complete(ctx, req)
}

func (r *recording) Stream(ctx context.Context, req providers.Request, onText func(string)) (providers.Response, error) {
	r.note(req)
	return r.Fake.Stream(ctx, req, onText)
}

func (r *recording) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.models...)
}

// tap keeps every body the control plane sends on /api/runner/*.
type tap struct {
	mu     sync.Mutex
	bodies map[string][]string
}

type tapWriter struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (w *tapWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	return w.ResponseWriter.Write(p)
}

func (t *tap) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/runner/") {
			next.ServeHTTP(w, r)
			return
		}
		tw := &tapWriter{ResponseWriter: w}
		next.ServeHTTP(tw, r)
		route := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		t.mu.Lock()
		if t.bodies == nil {
			t.bodies = map[string][]string{}
		}
		t.bodies[route] = append(t.bodies[route], tw.buf.String())
		t.mu.Unlock()
	})
}

func (t *tap) all() map[string][]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := map[string][]string{}
	for k, v := range t.bodies {
		out[k] = append([]string(nil), v...)
	}
	return out
}

type harness struct {
	db       *sql.DB
	data     string
	repo     string
	card     board.Card
	admin    identity.User
	srv      *httpapi.Server
	url      string
	cp       *netFake
	provider *recording
	tap      *tap
}

func newHarness(t *testing.T, script, check string) *harness {
	t.Helper()
	db := boardtest.Open(t)
	ctx := context.Background()
	data := t.TempDir()
	if err := os.Chmod(data, 0o700); err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t)
	admin := user(t, db, "admin@example.com", "", 0, "")
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
	writeFile(t, filepath.Join(data, "config.yaml"), routerConfig)
	writeFile(t, filepath.Join(data, "secrets.json"), `{"llmKeys":{"fake":"`+modelKey+`"}}`)
	writeFile(t, filepath.Join(settings.ProjectDir(data, "proj_default"), "config.yaml"), projectConfig)
	owner := user(t, db, "grace@example.com", "grace", 11, ownerToken)
	if _, err := db.Exec(`
		INSERT INTO requirements (
			id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
			content_hash, accepted_at, accepted_by, project_id, created_by
		) VALUES ('REQ-001', 'prd', 'docs/prd/a.md', 'Say job', $1, '["Hello says job"]', '[]', 'h', '2026-01-01T00:00:00Z', 'ada', 'proj_default', $2)`,
		"Say job. tokens "+ownerToken+" "+modelKey+" "+databaseURL+" "+companyToken, owner.ID); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(ctx, db, board.Card{
		ProjectID: "proj_default", OwnerID: owner.ID, Title: "Say job", Status: "ready",
		RequirementIDs: []string{"REQ-001"}, LocalCheck: check, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		git := lookGit(t)
		_ = exec.Command(git, "-C", repo, "worktree", "prune").Run()
		_ = exec.Command(git, "-C", repo, "branch", "-D", "orch/"+card.ID).Run()
	})
	fk, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{db: db, data: data, repo: repo, card: card, admin: admin, cp: &netFake{}, provider: &recording{Fake: fk}, tap: &tap{}}
	srv := httpapi.New()
	srv.DB, srv.Data, srv.TokenKey = db, data, tokenKey
	srv.Tools = app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: h.cp.run}
	srv.Secrets = []string{databaseURL, hex.EncodeToString(tokenKey)}
	srv.JobProvider = h.provider
	srv.Remote = app.NewRemoteJobs()
	h.srv = srv
	ts := httptest.NewServer(h.tap.wrap(srv.Handler()))
	t.Cleanup(ts.Close)
	h.url = ts.URL
	h.queue(t)
	return h
}

// queue opens a run request for the card, as the Run button does.
func (h *harness) queue(t *testing.T) {
	t.Helper()
	id, err := board.NewID("run_")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(`
		INSERT INTO runs (id, trigger, created_at, project_id, card_id) VALUES ($1, 'api', $2, 'proj_default', $3)`,
		id, time.Now().UTC().Format(time.RFC3339), h.card.ID); err != nil {
		t.Fatal(err)
	}
}

// register mints a token and registers a runner with it.
func (h *harness) register(t *testing.T, name string) *runner.Client {
	t.Helper()
	token, _, err := app.MintRunnerToken(context.Background(), h.db, h.admin)
	if err != nil {
		t.Fatal(err)
	}
	c := &runner.Client{URL: h.url}
	reg, err := c.Register(context.Background(), runnerapi.Register{
		Token: token, Name: name, Slots: 2, Version: "test", Protocol: runnerapi.Protocol,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &runner.Client{URL: h.url, Secret: reg.Secret}
}

// worker is a runner with its own data directory and a clone of the fixture
// repository, as EnsureCheckout would leave it.
func (h *harness) worker(t *testing.T, c *runner.Client) (*runner.Runner, *netFake) {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "projects", "proj_default", "repo")
	if err := os.MkdirAll(filepath.Dir(repo), 0o700); err != nil {
		t.Fatal(err)
	}
	git := lookGit(t)
	if out, err := exec.Command(git, "clone", "--quiet", "--branch", "main", h.repo, repo).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	net := &netFake{}
	return &runner.Runner{
		Client: c, Name: "test-runner", Data: dir, Slots: 1, Version: "test",
		Tools: runner.Tools{Git: git, Exec: net.run}, Logf: t.Logf,
	}, net
}

func (h *harness) status(t *testing.T) board.Card {
	t.Helper()
	card, err := board.GetCard(context.Background(), h.db, h.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func claim(t *testing.T, c *runner.Client) runnerapi.Claimed {
	t.Helper()
	claimed, ok, err := c.Claim(context.Background(), 0)
	if err != nil || !ok {
		t.Fatalf("claim = %v %v", ok, err)
	}
	return claimed
}

func user(t *testing.T, db *sql.DB, email, login string, id int64, token string) identity.User {
	t.Helper()
	ctx := context.Background()
	u, err := identity.Insert(ctx, db, email, "password-long-1", "developer", false)
	if err != nil {
		t.Fatal(err)
	}
	if login == "" {
		return u
	}
	if err := identity.SaveGitHub(ctx, db, u.ID, "github.com", login, &token, false, tokenKey); err != nil {
		t.Fatal(err)
	}
	if err := identity.VerifyGitHub(ctx, db, u.ID, login, id); err != nil {
		t.Fatal(err)
	}
	return u
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func lookGit(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

var (
	fixtureMu   sync.Mutex
	fixtureHeld bool
)

// openFixture locks the shared fixture repository for the test, as the app
// tests do, so packages running at once do not share its branches.
func openFixture(t *testing.T) string {
	t.Helper()
	fixtureMu.Lock()
	held := fixtureHeld
	fixtureHeld = true
	fixtureMu.Unlock()
	if held {
		t.Fatal("openFixture: the fixture repository is already open in this test")
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
		for _, args := range [][]string{
			{"init", "-b", "main"}, {"add", "hello.txt", "check.sh"},
			{"-c", "user.name=fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "fixture"},
		} {
			if out, err := exec.Command(git, append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}
	return repo
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
