package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/cli"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
	"orchestrator/backend/internal/webpush"
	"orchestrator/backend/internal/worker"
)

func TestCloudBind(t *testing.T) {
	t.Setenv("ORCHESTRATOR_CLOUD", "1")
	repo := t.TempDir()
	data := t.TempDir()
	if err := os.Chmod(data, 0o700); err != nil {
		t.Fatal(err)
	}
	opened := false
	var stderr bytes.Buffer
	code := cli.Execute(cli.Options{
		Args:   []string{"serve", "--addr", "0.0.0.0:8787", "--repo", repo, "--data", data},
		Stderr: &stderr,
		Getenv: func(key string) string {
			if key == "ORCHESTRATOR_DATABASE_URL" {
				return "postgres://local/unused"
			}
			return os.Getenv(key)
		},
		Open: func(string) error {
			opened = true
			return nil
		},
	})
	if code != 0 || !opened {
		t.Fatalf("code = %d opened = %v stderr = %s", code, opened, stderr.String())
	}
	srv := httpapi.New()
	srv.Cloud = true
	srv.AllowHosts = map[string]bool{"127.0.0.1": true}
	srv.TestBearer = "bearer-token-for-tests"
	req := httptest.NewRequest(http.MethodGet, "/api/cards", nil)
	req.Host = "evil.example:8787"
	req.Header.Set("Authorization", "Bearer bearer-token-for-tests")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("host = %d", rec.Code)
	}
}

func TestParallelProjects(t *testing.T) {
	first, err := worker.Lock(filepath.Join(t.TempDir(), "worker.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := worker.Lock(filepath.Join(t.TempDir(), "worker.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
}

func TestDataDirInsideRepo(t *testing.T) {
	t.Run("data inside repo", func(t *testing.T) {
		repo := t.TempDir()
		data := filepath.Join(repo, ".orchestrator")
		opened := false
		code := cli.Execute(cli.Options{
			Args:   []string{"serve", "--repo", repo, "--data", data},
			Stderr: io.Discard,
			Open: func(string) error {
				opened = true
				return nil
			},
		})
		if code == 0 {
			t.Fatal("exit = 0, want non-zero")
		}
		if opened {
			t.Fatal("database opened")
		}
		if _, err := os.Stat(data); !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", data, err)
		}
	})

	t.Run("repo inside data", func(t *testing.T) {
		data := t.TempDir()
		repo := filepath.Join(data, "repo")
		if err := os.Mkdir(repo, 0o755); err != nil {
			t.Fatal(err)
		}
		opened := false
		code := cli.Execute(cli.Options{
			Args:   []string{"serve", "--repo", repo, "--data", data},
			Stderr: io.Discard,
			Open: func(string) error {
				opened = true
				return nil
			},
		})
		if code == 0 {
			t.Fatal("exit = 0, want non-zero")
		}
		if opened {
			t.Fatal("database opened")
		}
	})

	t.Run("equal paths", func(t *testing.T) {
		dir := t.TempDir()
		opened := false
		code := cli.Execute(cli.Options{
			Args:   []string{"serve", "--repo", dir, "--data", dir},
			Stderr: io.Discard,
			Open: func(string) error {
				opened = true
				return nil
			},
		})
		if code == 0 {
			t.Fatal("exit = 0, want non-zero")
		}
		if opened {
			t.Fatal("database opened")
		}
	})
}

func TestLoopbackOnly(t *testing.T) {
	repo := t.TempDir()
	data := t.TempDir()
	var stderr bytes.Buffer
	opened := false
	code := cli.Execute(cli.Options{
		Args:   []string{"serve", "--addr", "0.0.0.0:8787", "--repo", repo, "--data", data},
		Stderr: &stderr,
		Open: func(string) error {
			opened = true
			return nil
		},
	})
	if code == 0 {
		t.Fatal("exit = 0, want non-zero")
	}
	if opened {
		t.Fatal("database opened")
	}
	if !strings.Contains(stderr.String(), "loopback") {
		t.Fatalf("stderr = %q, want loopback refusal", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(data, "secrets.json")); !os.IsNotExist(err) {
		t.Fatalf("secrets.json exists: %v", err)
	}
}

func TestStatusEmpty(t *testing.T) {
	repo := t.TempDir()
	data := t.TempDir()
	if err := os.Chmod(data, 0o700); err != nil {
		t.Fatal(err)
	}
	databaseURL := boardtest.IsolatedURL(t)
	var stdout, stderr bytes.Buffer
	code := cli.Execute(cli.Options{
		Args:   []string{"status", "--repo", repo, "--data", data},
		Stdout: &stdout,
		Stderr: &stderr,
		Getenv: func(key string) string {
			if key == "ORCHESTRATOR_DATABASE_URL" {
				return databaseURL
			}
			return os.Getenv(key)
		},
	})
	if code != 0 {
		t.Fatalf("exit = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "board is empty") || !strings.Contains(stdout.String(), "orchestrator ingest") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestApproveMissingSHA(t *testing.T) {
	opened := false
	var stderr bytes.Buffer
	code := cli.Execute(cli.Options{
		Args:   []string{"approve", "card_x"},
		Stderr: &stderr,
		Open: func(string) error {
			opened = true
			return nil
		},
	})
	if code == 0 {
		t.Fatal("exit = 0, want non-zero")
	}
	if opened {
		t.Fatal("database opened")
	}
}

func TestApproveWrongSHA(t *testing.T) {
	repo, data, databaseURL := cliDirs(t)
	db, err := board.Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: "human_review", Title: "card"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET head_sha = $2 WHERE id = $1`, card.ID, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := cli.Execute(cli.Options{
		Args:   []string{"approve", card.ID, "--sha", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "--repo", repo, "--data", data},
		Stderr: &stderr,
		Getenv: func(key string) string {
			if key == "ORCHESTRATOR_DATABASE_URL" {
				return databaseURL
			}
			return ""
		},
	})
	if code == 0 {
		t.Fatal("exit = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "sha") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM approvals WHERE card_id = $1`, card.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("approvals = %d", n)
	}
}

func TestRunNextIdle(t *testing.T) {
	repo, data, databaseURL := cliDirs(t)
	var stdout, stderr bytes.Buffer
	code := cli.Execute(cli.Options{
		Args:   []string{"run-next", "--repo", repo, "--data", data},
		Stdout: &stdout,
		Stderr: &stderr,
		Getenv: func(key string) string {
			if key == "ORCHESTRATOR_DATABASE_URL" {
				return databaseURL
			}
			return ""
		},
	})
	if code != 0 {
		t.Fatalf("exit = %d stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "no card is ready") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunNextPlans(t *testing.T) {
	repo, data, databaseURL := cliDirs(t)
	db, err := board.Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO requirements (
			id, project_id, source_kind, source_path, heading, statement,
			acceptance_json, non_goals_json, content_hash, accepted_at
		) VALUES ('REQ-001', 'proj_default', 'prd', 'docs/prd', 'Export', 'text', '["csv"]', '[]', 'hash', '2026-10-11T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := cli.Execute(cli.Options{
		Args:   []string{"run-next", "--repo", repo, "--data", data},
		Stdout: &stdout,
		Stderr: &stderr,
		Getenv: func(key string) string {
			if key == "ORCHESTRATOR_DATABASE_URL" {
				return databaseURL
			}
			return ""
		},
	})
	if code != 0 {
		t.Fatalf("exit = %d stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "created 1\npromoted 1\n") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	var open int
	if err := db.QueryRow(`SELECT count(*) FROM runs WHERE trigger = 'cli' AND processed_at IS NULL`).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open cli runs = %d", open)
	}
}

func TestUnblockReset(t *testing.T) {
	repo, data, databaseURL := cliDirs(t)
	db, err := board.Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(context.Background(), db, board.Card{
		Status: "blocked", AttemptsUsed: 3, MaxAttempts: 3, Title: "capped",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET blocked_reason = 'turn_cap' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := cli.Execute(cli.Options{
		Args:   []string{"unblock", card.ID, "--reset-attempts", "--repo", repo, "--data", data},
		Stderr: &stderr,
		Getenv: func(key string) string {
			if key == "ORCHESTRATOR_DATABASE_URL" {
				return databaseURL
			}
			return ""
		},
	})
	if code != 0 {
		t.Fatalf("exit = %d stderr %q", code, stderr.String())
	}
	got, err := board.GetCard(context.Background(), db, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ready" || got.AttemptsUsed != 0 {
		t.Fatalf("card = %+v", got)
	}
}

func TestSecondServe(t *testing.T) {
	repo, data, _ := cliDirs(t)
	lock, err := worker.Lock(filepath.Join(data, "worker.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	opened := false
	code := cli.Execute(cli.Options{
		Args:   []string{"serve", "--repo", repo, "--data", data},
		Stderr: io.Discard,
		Open: func(string) error {
			opened = true
			return nil
		},
	})
	if code == 0 || opened {
		t.Fatalf("exit = %d opened = %v", code, opened)
	}
}

func cliDirs(t *testing.T) (string, string, string) {
	t.Helper()
	repo := t.TempDir()
	data := t.TempDir()
	if err := os.Chmod(data, 0o700); err != nil {
		t.Fatal(err)
	}
	return repo, data, boardtest.IsolatedURL(t)
}

func TestVAPIDKeySecret(t *testing.T) {
	repo, data, databaseURL := cliDirs(t)
	serve := func() {
		t.Helper()
		code := cli.Execute(cli.Options{
			Args:   []string{"serve", "--repo", repo, "--data", data},
			Stderr: io.Discard,
			Getenv: func(key string) string {
				if key == "ORCHESTRATOR_DATABASE_URL" {
					return databaseURL
				}
				return ""
			},
			Open: func(string) error { return nil },
		})
		if code != 0 {
			t.Fatalf("serve exit = %d", code)
		}
	}
	vapid := func() string {
		t.Helper()
		_, secrets, err := settings.Load(data)
		if err != nil {
			t.Fatal(err)
		}
		return secrets.VAPIDPrivateKey
	}
	serve()
	first := vapid()
	if _, err := webpush.ParseKey(first); err != nil {
		t.Fatalf("vapidPrivateKey %q: %v", first, err)
	}
	serve()
	if vapid() != first {
		t.Fatal("serve replaced the VAPID key")
	}
	families := []settings.Family{{ID: "f", Protocol: "openai", BaseURL: "http://127.0.0.1:1", Models: []settings.Model{{ID: "m"}}}}
	if _, err := settings.SaveLLM(data, families, nil); err != nil {
		t.Fatal(err)
	}
	if vapid() != first {
		t.Fatal("saving LLM settings dropped the VAPID key")
	}

	db, err := board.Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Insert(context.Background(), db, "dev@example.com", "password-long-1", "developer", false); err != nil {
		t.Fatal(err)
	}
	key, _ := webpush.ParseKey(first)
	srv := httpapi.New()
	srv.DB = db
	srv.Data = data
	srv.Push = &webpush.Pusher{DB: db, Sender: &webpush.Sender{Key: key}}
	srv.PushKey, _ = webpush.PublicKey(key)
	login := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(`{"email":"dev@example.com","password":"password-long-1"}`))
	loginReq.Host = "127.0.0.1:8787"
	srv.Handler().ServeHTTP(login, loginReq)
	var session struct {
		Bearer string `json:"bearer"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.Bearer == "" {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/push/key", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Authorization", "Bearer "+session.Bearer)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 {
		t.Fatalf("push key = %d %s", rec.Code, rec.Body.String())
	}
	if body["publicKey"] != srv.PushKey || len(body) != 2 || strings.Contains(rec.Body.String(), first) {
		t.Fatalf("push key body = %s", rec.Body.String())
	}
}

func TestUsageCLI(t *testing.T) {
	repo, data, databaseURL := cliDirs(t)
	db, err := board.Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	card, err := board.InsertCard(ctx, db, board.Card{Status: "human_review", Title: "card"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []map[string]any{
		{"role": "implementer", "taskClass": "implement", "modelId": "model-a", "inputTokens": 100, "outputTokens": 20},
		{"role": "reviewer", "taskClass": "review", "modelId": "model-b", "inputTokens": 30, "outputTokens": 5},
	} {
		if _, err := trace.Append(ctx, db, trace.Event{TS: "2026-10-02T10:00:00Z", CardID: card.ID, ProjectID: "proj_default", Kind: "model_call", Payload: p}, nil); err != nil {
			t.Fatal(err)
		}
	}
	usage := func(by string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := cli.Execute(cli.Options{
			Args:   []string{"usage", "--from", "2026-10-01", "--to", "2026-10-03", "--by", by, "--repo", repo, "--data", data},
			Stdout: &stdout,
			Stderr: &stderr,
			Getenv: func(key string) string {
				if key == "ORCHESTRATOR_DATABASE_URL" {
					return databaseURL
				}
				return ""
			},
		})
		return code, stdout.String(), stderr.String()
	}
	code, stdout, stderr := usage("task")
	if code != 0 {
		t.Fatalf("exit = %d stderr %q", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 || !strings.Contains(lines[0], "2026-10-01 to 2026-10-03 by task") ||
		strings.Join(strings.Fields(lines[2]), " ") != "implement 1 100 20 0 120" ||
		strings.Join(strings.Fields(lines[3]), " ") != "review 1 30 5 0 35" {
		t.Fatalf("stdout = %q", stdout)
	}
	if code, _, stderr := usage("cost"); code == 0 || !strings.Contains(stderr, "--by") {
		t.Fatalf("--by cost exit = %d stderr %q", code, stderr)
	}
}

func TestUsageCLIScope(t *testing.T) {
	repo, data, databaseURL := cliDirs(t)
	db, err := board.Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, stmt := range []string{
		`INSERT INTO users (id, name, email, password_hash, role, created_at) VALUES ('usr_ana', 'Ana', 'ana@example.com', 'x', 'developer', '2026-10-01T00:00:00Z')`,
		`INSERT INTO tribes (id, name, created_at) VALUES ('tribe_gadget', 'Gadget tribe', '2026-10-01T00:00:00Z')`,
		`INSERT INTO projects (id, tribe_id, name, repo_path, created_at) VALUES ('proj_gadget', 'tribe_gadget', 'Gadget', '/tmp/gadget', '2026-10-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		project, owner string
		input          int
	}{{"proj_gadget", "usr_ana", 100}, {"proj_default", "", 30}} {
		card, err := board.InsertCard(ctx, db, board.Card{Status: "human_review", Title: "card", ProjectID: c.project, OwnerID: c.owner})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := trace.Append(ctx, db, trace.Event{TS: "2026-10-02T10:00:00Z", CardID: card.ID, ProjectID: c.project, Kind: "model_call",
			Payload: map[string]any{"role": "implementer", "taskClass": "implement", "modelId": "model-a", "inputTokens": c.input}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	usage := func(flags ...string) string {
		var stdout, stderr bytes.Buffer
		args := append([]string{"usage", "--from", "2026-10-01", "--to", "2026-10-03", "--by", "project", "--repo", repo, "--data", data}, flags...)
		code := cli.Execute(cli.Options{
			Args: args, Stdout: &stdout, Stderr: &stderr,
			Getenv: func(key string) string {
				if key == "ORCHESTRATOR_DATABASE_URL" {
					return databaseURL
				}
				return ""
			},
		})
		if code != 0 {
			t.Fatalf("%v exit = %d stderr %q", flags, code, stderr.String())
		}
		return stdout.String()
	}
	rows := func(out string) []string {
		var got []string
		for _, line := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
			got = append(got, strings.Join(strings.Fields(line), " "))
		}
		return got
	}
	if got := rows(usage()); len(got) != 3 || got[1] != "Gadget 1 100 0 0 100" || got[2] != "Default 1 30 0 0 30" {
		t.Fatalf("company = %q", got)
	}
	if got := rows(usage("--tribe", "tribe_gadget")); len(got) != 2 || got[1] != "Gadget 1 100 0 0 100" {
		t.Fatalf("--tribe = %q", got)
	}
	if got := rows(usage("--user", "usr_ana")); len(got) != 2 || got[1] != "Gadget 1 100 0 0 100" {
		t.Fatalf("--user = %q", got)
	}
	if out := usage("--tribe", "tribe_default", "--user", "usr_ana"); !strings.Contains(out, "no model calls in this range") {
		t.Fatalf("--tribe and --user = %q", out)
	}
}

func TestUsageCLICard(t *testing.T) {
	repo, data, databaseURL := cliDirs(t)
	db, err := board.Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := board.Migrate(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	card, err := board.InsertCard(ctx, db, board.Card{Status: "human_review", Title: "Export CSV"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []trace.Event{
		{TS: "2026-10-02T10:00:00Z", CardID: card.ID, ProjectID: "proj_default", Kind: "model_call",
			Payload: map[string]any{"role": "implementer", "taskClass": "implement", "modelId": "model-a", "inputTokens": 100}},
		{TS: "2026-10-02T11:00:00Z", ProjectID: "proj_default", Kind: "requirement_refined",
			Payload: map[string]any{"requirementId": "new", "modelId": "model-a", "inputTokens": 40}},
	} {
		if _, err := trace.Append(ctx, db, ev, nil); err != nil {
			t.Fatal(err)
		}
	}
	usage := func(by string) []string {
		var stdout, stderr bytes.Buffer
		code := cli.Execute(cli.Options{
			Args:   []string{"usage", "--from", "2026-10-01", "--to", "2026-10-03", "--by", by, "--repo", repo, "--data", data},
			Stdout: &stdout,
			Stderr: &stderr,
			Getenv: func(key string) string {
				if key == "ORCHESTRATOR_DATABASE_URL" {
					return databaseURL
				}
				return ""
			},
		})
		if code != 0 {
			t.Fatalf("--by %s exit = %d stderr %q", by, code, stderr.String())
		}
		var got []string
		for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n")[1:] {
			got = append(got, strings.Join(strings.Fields(line), " "))
		}
		return got
	}
	if got := usage("source"); len(got) != 3 || got[1] != "card 1 100 0 0 100" || got[2] != "intake 1 40 0 0 40" {
		t.Fatalf("--by source = %q", got)
	}
	if got := usage("card"); len(got) != 3 || got[1] != "Export CSV ("+card.ID+") 1 100 0 0 100" || got[2] != "Intake 1 40 0 0 40" {
		t.Fatalf("--by card = %q", got)
	}
}
