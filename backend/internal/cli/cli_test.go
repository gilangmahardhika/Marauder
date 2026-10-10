package cli_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/cli"
	"orchestrator/backend/internal/worker"
)

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
