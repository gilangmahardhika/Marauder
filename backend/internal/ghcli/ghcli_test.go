package ghcli_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/ghcli"
)

func TestRunnerPathHasOnlyGitDir(t *testing.T) {
	var env []string
	g := ghcli.Runner{Bin: "/usr/bin/gh", Git: "/opt/tools/bin/git", ConfigDir: t.TempDir(),
		Exec: func(_ context.Context, child execx.Child) (execx.Result, error) {
			env = child.Env
			return execx.Result{}, nil
		}}
	if _, err := g.Run(context.Background(), "github.com", "secret-token", "pr", "create"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(env, "PATH=/opt/tools/bin") {
		t.Fatalf("env = %v, want PATH=/opt/tools/bin", env)
	}
	for _, arg := range env {
		if strings.HasPrefix(arg, "PATH=") && arg != "PATH=/opt/tools/bin" {
			t.Fatalf("extra PATH entry %s", arg)
		}
	}
	g.Git = ""
	if _, err := g.Run(context.Background(), "github.com", "secret-token", "api", "user"); err != nil {
		t.Fatal(err)
	}
	for _, arg := range env {
		if strings.HasPrefix(arg, "PATH=") {
			t.Fatalf("runner without Git set %s", arg)
		}
	}
}
