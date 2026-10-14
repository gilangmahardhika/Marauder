package runtime_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := exec.Command(lookGit(t), "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

func TestSeedCardBranch(t *testing.T) {
	ctx := context.Background()
	git := lookGit(t)
	published := t.TempDir()
	initRepo(t, published)
	runGit(t, published, git, "checkout", "--quiet", "-b", "orch/card_5eed00000001")
	if err := os.WriteFile(filepath.Join(published, "hello.txt"), []byte("published\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, published, git, "-c", "user.name=bot", "-c", "user.email=bot@example.com", "commit", "--quiet", "-am", "published")
	want := revParse(t, published, "HEAD")
	runGit(t, published, git, "checkout", "--quiet", "main")

	clone := filepath.Join(t.TempDir(), "repo")
	if out, err := exec.Command(git, "clone", "--quiet", "--branch", "main", published, clone).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	g := newGit(t, clone, dataDir(t))
	runGit(t, clone, git, "fetch", "--quiet", published, "refs/heads/orch/card_5eed00000001")
	if err := g.SeedCardBranch(ctx, "card_5eed00000001"); err != nil {
		t.Fatal(err)
	}
	wt, err := g.EnsureWorktree(ctx, "card_5eed00000001", "main")
	if err != nil {
		t.Fatal(err)
	}
	head, err := g.WorktreeHead(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if head != want {
		t.Fatalf("seeded worktree head = %s, want the published %s", head, want)
	}

	if err := g.Commit(ctx, wt, "local work"); err == nil {
		t.Fatal("commit of a clean worktree succeeded")
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "hello.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.Commit(ctx, wt, "local work"); err != nil {
		t.Fatal(err)
	}
	ahead := revParse(t, clone, "refs/heads/orch/card_5eed00000001")
	runGit(t, clone, git, "fetch", "--quiet", published, "refs/heads/orch/card_5eed00000001")
	if err := g.SeedCardBranch(ctx, "card_5eed00000001"); err != nil {
		t.Fatal(err)
	}
	if got := revParse(t, clone, "refs/heads/orch/card_5eed00000001"); got != ahead {
		t.Fatalf("branch ahead of the published one moved to %s, want %s kept", got, ahead)
	}
	if err := g.SeedCardBranch(ctx, "../escape"); err == nil {
		t.Fatal("an invalid card id was accepted")
	}
}
