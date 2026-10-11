package runtime_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"orchestrator/backend/internal/runtime"
)

func TestPathEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "leave")); err != nil {
		t.Fatal(err)
	}
	for _, requested := range []string{"../x", "/etc/passwd", ".git/config", "leave"} {
		if _, err := runtime.SafePath(root, requested); err == nil {
			t.Fatalf("%s was accepted", requested)
		}
	}
}

func TestWorktreeOnce(t *testing.T) {
	repo := openFixture(t)
	data := dataDir(t)
	g := newGit(t, repo, data)
	card := "card_0123456789ab"
	ctx := context.Background()
	first, err := g.EnsureWorktree(ctx, card, "main")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command(g.Bin, "-C", repo, "worktree", "remove", "--force", first.Path).Run()
		_ = exec.Command(g.Bin, "-C", repo, "branch", "-D", first.Branch).Run()
	})
	second, err := g.EnsureWorktree(ctx, card, "main")
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != second.Path || first.Branch != second.Branch || first.GitDir != second.GitDir {
		t.Fatalf("first %+v second %+v", first, second)
	}
	if first.Branch != "orch/"+card {
		t.Fatalf("branch = %s", first.Branch)
	}
	out, err := exec.Command(g.Bin, "-C", repo, "worktree", "list").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), first.Path) != 1 {
		t.Fatalf("worktree list =\n%s", out)
	}
}

func TestHookNotRun(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	data := dataDir(t)
	g := newGit(t, repo, data)
	ctx := context.Background()
	wt, err := g.EnsureWorktree(ctx, "card_abcdef012345", "main")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := "#!/bin/sh\necho ran > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-commit"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "next.txt"), []byte("next\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.Commit(ctx, wt, "next"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("hook marker stat = %v", err)
	}
	out, err := exec.Command(g.Bin, "--git-dir="+wt.GitDir, "log", "-1", "--format=%an %ae").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "bot bot@users.noreply.github.com" {
		t.Fatalf("identity = %q", out)
	}
}

func TestDiscardChangesKeepsCommits(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	data := dataDir(t)
	g := newGit(t, repo, data)
	ctx := context.Background()
	wt, err := g.EnsureWorktree(ctx, "card_abcdef012346", "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "kept.txt"), []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.Commit(ctx, wt, "kept"); err != nil {
		t.Fatal(err)
	}
	head, err := g.WorktreeHead(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "kept.txt"), []byte("half done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wt.Path, "stray"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "stray", "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := g.DiscardChanges(ctx, wt); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(wt.Path, "kept.txt"))
	if err != nil || string(body) != "kept\n" {
		t.Fatalf("kept.txt = %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(wt.Path, "stray")); !os.IsNotExist(err) {
		t.Fatalf("untracked dir stat = %v", err)
	}
	if after, _ := g.WorktreeHead(ctx, wt); after != head {
		t.Fatalf("head moved from %s to %s", head, after)
	}
}

func TestFsmonitorOff(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	data := dataDir(t)
	g := newGit(t, repo, data)
	marker := filepath.Join(t.TempDir(), "fsmonitor-ran")
	script := filepath.Join(t.TempDir(), "fsmonitor")
	body := "#!/bin/sh\necho ran > " + marker + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(g.Bin, "-C", repo, "config", "core.fsmonitor", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("config: %v %s", err, out)
	}
	if err := g.Status(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("fsmonitor marker stat = %v", err)
	}
}

func TestCheckOnExport(t *testing.T) {
	repo := openFixture(t)
	data := dataDir(t)
	g := newGit(t, repo, data)
	ctx := context.Background()
	card := "card_001122334455"
	wt, err := g.EnsureWorktree(ctx, card, "main")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command(g.Bin, "-C", repo, "worktree", "remove", "--force", wt.Path).Run()
		_ = exec.Command(g.Bin, "-C", repo, "branch", "-D", wt.Branch).Run()
	})
	sha, err := g.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(wt.Path, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(data, "exports", card+"-"+sha)
	if err := g.Export(ctx, sha, dest); err != nil {
		t.Fatal(err)
	}
	if _, passed, err := runtime.RunCheck(ctx, repo, "touch .git", []string{"touch"}, dest, ""); err != nil || !passed {
		t.Fatalf("touch err = %v passed %v", err, passed)
	}
	after, err := os.ReadFile(filepath.Join(wt.Path, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("worktree .git changed\nbefore %q\nafter %q", before, after)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		t.Fatal(err)
	}
}

func TestCheckExit(t *testing.T) {
	repo := openFixture(t)
	dir := t.TempDir()
	ctx := context.Background()
	allow := []string{"true", "false"}
	_, passed, err := runtime.RunCheck(ctx, repo, "true", allow, dir, "")
	if err != nil || !passed {
		t.Fatalf("true err = %v passed %v", err, passed)
	}
	_, passed, err = runtime.RunCheck(ctx, repo, "false", allow, dir, "the tests passed")
	if err != nil {
		t.Fatal(err)
	}
	if passed {
		t.Fatal("exit 1 was treated as a pass")
	}
}

func openFixture(t *testing.T) string {
	t.Helper()
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

func initRepo(t *testing.T, repo string) {
	t.Helper()
	git := lookGit(t)
	if err := os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, git, "init", "-b", "main")
	runGit(t, repo, git, "add", "hello.txt")
	runGit(t, repo, git, "-c", "user.name=fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "init")
}

func newGit(t *testing.T, repo, data string) runtime.Git {
	t.Helper()
	return runtime.Git{Bin: lookGit(t), Repo: repo, Data: data, Author: runtime.LoginAuthor("bot", 0)}
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
