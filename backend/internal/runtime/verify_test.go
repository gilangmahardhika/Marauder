package runtime_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyAtDeployedSHA(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "uncommitted.txt"), []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := dataDir(t)
	g := newGit(t, repo, data)
	sha, err := g.Head(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(data, "exports", "check")
	if err := g.Export(t.Context(), sha, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "uncommitted.txt")); !os.IsNotExist(err) {
		t.Fatal("uncommitted file was in the export")
	}
}
