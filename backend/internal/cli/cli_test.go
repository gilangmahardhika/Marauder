package cli_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/cli"
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
