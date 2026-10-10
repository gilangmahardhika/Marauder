package sandbox_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/sandbox"
)

func TestCheckParse(t *testing.T) {
	allow := []string{"go"}
	got, err := sandbox.ParseCheck("go test ./...", allow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "go test ./..." {
		t.Fatalf("argv = %q", got)
	}
	if _, err := sandbox.ParseCheck("go test; id", allow); !errors.Is(err, sandbox.ErrParse) {
		t.Fatalf("semicolon err = %v", err)
	}
	if _, err := sandbox.ParseCheck("$(id)", allow); !errors.Is(err, sandbox.ErrParse) {
		t.Fatalf("substitution err = %v", err)
	}
	if _, err := sandbox.ParseCheck("bash -c x", allow); !errors.Is(err, sandbox.ErrAllow) {
		t.Fatalf("bash err = %v", err)
	}
}

func TestCommandAllowlist(t *testing.T) {
	called := false
	_, executed, err := sandbox.ExecuteCheck(context.Background(), sandbox.ExecuteOpts{
		Command: "id",
		Allow:   []string{"go"},
		Fixture: true,
		Dir:     t.TempDir(),
		Timeout: time.Second,
		MaxOut:  1024,
	}, func(context.Context, execx.Child) (execx.Result, error) {
		called = true
		return execx.Result{}, nil
	})
	if called || executed {
		t.Fatal("command outside the allowlist was executed")
	}
	if err == nil {
		t.Fatal("expected allowlist error")
	}
}

func TestSandboxFlags(t *testing.T) {
	const (
		image    = "example@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		export   = "/data/exports/card"
		worktree = "/data/worktrees/card"
	)
	argv, err := sandbox.LocalArgv("/usr/bin/podman", image, export, []string{"go", "test", "./..."})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(argv, "\n")
	for _, want := range []string{"--network=none", "--cap-drop=ALL", image, "src=" + export, "dst=/work"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv missing %s\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "docker.sock") || strings.Contains(joined, worktree) {
		t.Fatalf("argv mounts a socket or the worktree\n%s", joined)
	}
	dev, err := sandbox.DevArgv("/usr/bin/podman", image, export, "https://dev.example", "abc", []string{"go", "test"})
	if err != nil {
		t.Fatal(err)
	}
	devJoined := strings.Join(dev, "\n")
	if strings.Contains(devJoined, "--network=none") || !strings.Contains(devJoined, "DEV_URL=https://dev.example") {
		t.Fatalf("dev argv = %s", devJoined)
	}
}

func TestEnvScrub(t *testing.T) {
	for _, env := range [][]string{
		sandbox.ContainerEnv(),
		sandbox.EngineEnv("/usr/bin/podman"),
	} {
		for _, item := range env {
			if strings.Contains(item, "GH_TOKEN") || strings.Contains(item, "DEPLOY_TOKEN") || strings.Contains(item, "API_KEY") {
				t.Fatalf("secret in env %s", item)
			}
		}
	}
}

func TestTarEscape(t *testing.T) {
	var body bytes.Buffer
	tw := tar.NewWriter(&body)
	hdr := &tar.Header{Name: "../x", Mode: 0o644, Size: 1}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("z")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	err := sandbox.ExtractArchive(bytes.NewReader(body.Bytes()), dest)
	if err == nil {
		t.Fatal("expected ../x to be refused")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dest), "x")); !os.IsNotExist(statErr) {
		t.Fatalf("escaped file stat = %v", statErr)
	}
}
