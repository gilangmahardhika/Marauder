package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/sandbox"
)

const testImage = "ghcr.io/acme/check@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// commandSession is a session whose worktree is a plain directory and whose
// sandbox children go to exec instead of an engine.
func commandSession(t *testing.T, commands []string, exec func(context.Context, execx.Child) (execx.Result, error)) *session {
	t.Helper()
	root := t.TempDir()
	wt := filepath.Join(root, "worktree")
	writeTree(t, wt, map[string]string{"main.go": "package main\n", "pkg/a.go": "package pkg\n"})
	return &session{
		in: Input{
			Git:      runtime.Git{Repo: filepath.Join(root, "repo"), Data: filepath.Join(root, "data")},
			Engine:   "/usr/bin/podman",
			Image:    testImage,
			Commands: commands,
			Exec:     exec,
		},
		wt:        runtime.Worktree{Path: wt},
		attemptID: "att_cmd",
	}
}

func commandCall(argv ...any) providers.ToolCall {
	return providers.ToolCall{Name: "run_command", Args: map[string]any{"argv": argv}}
}

func call(t *testing.T, s *session, c providers.ToolCall) string {
	t.Helper()
	text, rejected, err := s.dispatch(context.Background(), c)
	if err != nil || rejected {
		t.Fatalf("dispatch = %q, rejected %v, %v", text, rejected, err)
	}
	return text
}

// mountSrc returns the host directory bound at /work in a container argv.
func mountSrc(t *testing.T, argv []string) string {
	t.Helper()
	for i, arg := range argv {
		if arg == "--mount" && i+1 < len(argv) {
			for _, part := range strings.Split(argv[i+1], ",") {
				if src, ok := strings.CutPrefix(part, "src="); ok {
					return src
				}
			}
		}
	}
	t.Fatalf("no --mount in %v", argv)
	return ""
}

func refuseExec(t *testing.T) func(context.Context, execx.Child) (execx.Result, error) {
	return func(context.Context, execx.Child) (execx.Result, error) {
		t.Fatal("the command must not run")
		return execx.Result{}, nil
	}
}

func TestRunCommandParse(t *testing.T) {
	allow := []string{"go", "npx"}
	long := strings.Repeat("a", sandbox.MaxArgvBytes)
	cases := []struct {
		argv []string
		want error
	}{
		{[]string{"go", "test", "./..."}, nil},
		{[]string{"npx", "tsc", "--noEmit"}, nil},
		{nil, sandbox.ErrArgv},
		{make([]string, sandbox.MaxArgs+1), sandbox.ErrArgv},
		{[]string{"go", long}, sandbox.ErrArgv},
		{[]string{"go", "a\x00b"}, sandbox.ErrArgv},
		{[]string{""}, sandbox.ErrBare},
		{[]string{"/usr/local/go/bin/go", "test"}, sandbox.ErrBare},
		{[]string{"../go"}, sandbox.ErrBare},
		{[]string{"go test ./..."}, sandbox.ErrBare},
		{[]string{"make"}, sandbox.ErrCommand},
	}
	for _, c := range cases {
		if err := sandbox.ParseArgv(c.argv, allow); !errors.Is(err, c.want) {
			t.Errorf("ParseArgv(%q) = %v, want %v", c.argv, err, c.want)
		}
	}
	full := make([]string, sandbox.MaxArgs)
	full[0] = "go"
	if err := sandbox.ParseArgv(full, allow); err != nil {
		t.Fatalf("64 strings: %v", err)
	}

	s := commandSession(t, allow, refuseExec(t))
	for _, c := range []providers.ToolCall{
		commandCall("go", 1),
		{Name: "run_command", Args: map[string]any{"argv": "go test ./..."}},
		{Name: "run_command", Args: map[string]any{}},
	} {
		if got := call(t, s, c); got != providers.ErrorPrefix+"argv must be an array of strings" {
			t.Fatalf("%v = %q", c.Args, got)
		}
	}
	if got := call(t, s, commandCall("make", "all")); got != providers.ErrorPrefix+sandbox.ErrCommand.Error() {
		t.Fatalf("off the list = %q", got)
	}
}

func TestRunCommandRefusesShell(t *testing.T) {
	names := []string{"sh", "bash", "zsh", "dash", "ksh", "fish", "csh", "tcsh", "pwsh", "env", "xargs", "busybox",
		"timeout", "nice", "sudo", "doas", "su", "nohup", "exec", "eval", "command", "BASH"}
	s := commandSession(t, names, refuseExec(t))
	for _, name := range names {
		if got := call(t, s, commandCall(name, "-c", "go test ./...")); got != providers.ErrorPrefix+sandbox.ErrWrapper.Error() {
			t.Fatalf("%s = %q", name, got)
		}
	}
	if s.commands != 0 {
		t.Fatalf("refused calls counted: %d", s.commands)
	}
}

func TestRunCommandFlags(t *testing.T) {
	var child execx.Child
	s := commandSession(t, []string{"go"}, func(_ context.Context, c execx.Child) (execx.Result, error) {
		child = c
		return execx.Result{Exit: 1, Out: []byte("FAIL\n")}, nil
	})
	got := call(t, s, commandCall("go", "test", "./pkg"))
	if got != "exit 1\nFAIL\n" {
		t.Fatalf("result = %q", got)
	}
	argv := strings.Join(child.Argv, " ")
	for _, want := range []string{"/usr/bin/podman run --rm --pull=never --network=none", "--cap-drop=ALL", "--read-only",
		"--security-opt=no-new-privileges", "--user 65532:65532", testImage + " go test ./pkg"} {
		if !strings.Contains(argv, want) {
			t.Fatalf("container argv lacks %q: %s", want, argv)
		}
	}
	if !strings.HasSuffix(argv, testImage+" go test ./pkg") {
		t.Fatalf("argv does not end with the command: %s", argv)
	}
	if child.Timeout != 5*time.Minute || child.MaxOut != 1<<20 {
		t.Fatalf("timeout %v, max out %d", child.Timeout, child.MaxOut)
	}
	if src := mountSrc(t, child.Argv); filepath.Dir(src) != filepath.Join(s.in.Git.Data, "exports") || src == s.wt.Path {
		t.Fatalf("mount %s is not a fresh directory under exports", src)
	}
}

func TestRunCommandOnCopy(t *testing.T) {
	var srcs []string
	s := commandSession(t, []string{"gofmt"}, func(_ context.Context, c execx.Child) (execx.Result, error) {
		src := mountSrc(t, c.Argv)
		srcs = append(srcs, src)
		if body, err := os.ReadFile(filepath.Join(src, "pkg", "a.go")); err != nil || string(body) != "package pkg\n" {
			t.Fatalf("copy lacks the worktree file: %q, %v", body, err)
		}
		if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("changed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "new.txt"), []byte("new\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return execx.Result{}, nil
	})
	call(t, s, commandCall("gofmt", "-w", "."))
	call(t, s, commandCall("gofmt", "-l", "."))
	if len(srcs) != 2 || srcs[0] == srcs[1] {
		t.Fatalf("each call needs a fresh copy: %v", srcs)
	}
	for _, src := range srcs {
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Fatalf("copy %s was not deleted: %v", src, err)
		}
	}
	if body, _ := os.ReadFile(filepath.Join(s.wt.Path, "main.go")); string(body) != "package main\n" {
		t.Fatalf("worktree changed: %q", body)
	}
	if _, err := os.Stat(filepath.Join(s.wt.Path, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("a file the command wrote reached the worktree")
	}
}

func TestRunCommandDisabled(t *testing.T) {
	s := commandSession(t, nil, refuseExec(t))
	if got := call(t, s, commandCall("go", "test")); got != "error: run_command is not enabled for this project" {
		t.Fatalf("result = %q", got)
	}
}

func TestRunCommandBudget(t *testing.T) {
	ran := 0
	s := commandSession(t, []string{"go"}, func(context.Context, execx.Child) (execx.Result, error) {
		ran++
		return execx.Result{}, nil
	})
	for i := 0; i < maxCommands; i++ {
		if got := call(t, s, commandCall("go", "vet")); got != "exit 0\n" {
			t.Fatalf("call %d = %q", i+1, got)
		}
	}
	got := call(t, s, commandCall("go", "vet"))
	if !strings.HasPrefix(got, providers.ErrorPrefix) || !strings.Contains(got, "30") {
		t.Fatalf("call 31 = %q", got)
	}
	if ran != maxCommands {
		t.Fatalf("ran %d commands, want %d", ran, maxCommands)
	}
}

func TestRunCommandCannotPassGate(t *testing.T) {
	var argvs [][]string
	s := commandSession(t, []string{"go"}, func(_ context.Context, c execx.Child) (execx.Result, error) {
		argvs = append(argvs, c.Argv)
		return execx.Result{Exit: 0, Out: []byte("ok\n")}, nil
	})
	s.card.LocalCheck = "make check"
	call(t, s, commandCall("go", "test", "./..."))
	if s.passTree != "" {
		t.Fatalf("run_command recorded a passing tree %q", s.passTree)
	}
	if len(argvs) != 1 || strings.Contains(strings.Join(argvs[0], " "), "make") {
		t.Fatalf("children = %v; only the command may run, never the card's check", argvs)
	}

	fixture := commandSession(t, []string{"go"}, refuseExec(t))
	fixture.in.Git.Repo = filepath.Join(t.TempDir(), "testdata", "fixture-repo")
	if got := call(t, fixture, commandCall("go", "test")); got != "error: run_command needs the container sandbox" {
		t.Fatalf("fixture mode = %q", got)
	}
}

func TestRunCommandRedactsOutput(t *testing.T) {
	secret := "sk-live-0123456789abcdef"
	var out strings.Builder
	out.WriteString("token " + secret + " in the head\n")
	for i := 0; i < 300; i++ {
		out.WriteString("line\n")
	}
	out.WriteString("--- FAIL: TestX token=" + secret + "\n")
	s := commandSession(t, []string{"go"}, func(context.Context, execx.Child) (execx.Result, error) {
		return execx.Result{Exit: 1, Out: []byte(out.String())}, nil
	})
	s.in.Secrets = []string{secret}
	got := call(t, s, commandCall("go", "test"))
	if strings.Contains(got, secret) {
		t.Fatalf("secret in result: %q", got)
	}
	if !strings.HasPrefix(got, "exit 1\n") || !strings.Contains(got, "--- FAIL: TestX") || strings.Contains(got, "in the head") {
		t.Fatalf("result is not the redacted last 200 lines: %q", got[:min(len(got), 200)])
	}
	if lines := strings.Count(got, "\n"); lines > 201 {
		t.Fatalf("result has %d lines", lines)
	}
}

func TestRunCommandExportAssertion(t *testing.T) {
	root := t.TempDir()
	exports := filepath.Join(root, "exports")
	wt := filepath.Join(root, "worktrees", "card")
	for _, dir := range []string{exports, wt, filepath.Join(exports, "inner")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(exports, "link")
	if err := os.Symlink(wt, link); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		dir, worktree string
		ok            bool
	}{
		{filepath.Join(exports, "att-cmd-1"), wt, true},
		{exports, wt, false},
		{root, wt, false},
		{filepath.Join(exports, "..", "worktrees"), wt, false},
		{wt, wt, false},
		{link, wt, false},
		{filepath.Join(exports, "inner"), filepath.Join(exports, "inner", "wt"), false},
		{filepath.Join(exports, "inner", "wt"), filepath.Join(exports, "inner"), false},
	}
	for _, c := range cases {
		err := sandbox.CheckExport(exports, c.dir, c.worktree)
		if (err == nil) != c.ok {
			t.Errorf("CheckExport(%s, wt %s) = %v, want ok %v", c.dir, c.worktree, err, c.ok)
		}
	}

	s := commandSession(t, []string{"go"}, refuseExec(t))
	s.wt.Path = filepath.Join(s.in.Git.Data, "exports")
	writeTree(t, s.wt.Path, map[string]string{"main.go": "package main\n"})
	if got := call(t, s, commandCall("go", "test")); got != providers.ErrorPrefix+sandbox.ErrExport.Error() {
		t.Fatalf("a copy inside the worktree = %q", got)
	}
}
