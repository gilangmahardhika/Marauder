package app_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
)

func TestCardDiff(t *testing.T) {
	ctx := context.Background()
	db := boardtest.Open(t)
	git := lookGit(t)
	repo := tempRepo(t, git, map[string]string{
		"hello.txt": "hello job\n",
		"keep.txt":  "same\n",
	})
	runGit(t, repo, git, "checkout", "-b", "orch/card")
	writeFile(t, filepath.Join(repo, "hello.txt"), "hello done\nkey sk-planted-secret-123\n")
	writeFile(t, filepath.Join(repo, "big.txt"), strings.Repeat("line of text\n", app.FileLineCap+10))
	commitAll(t, repo, git, "say done")
	head := revParse(t, repo, git, "HEAD")
	runGit(t, repo, git, "checkout", "main")

	data := t.TempDir()
	if err := os.WriteFile(filepath.Join(data, "secrets.json"), []byte(`{"llmKeys":{"fam":"sk-planted-secret-123"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE projects SET github_repo = 'widget', github_base = 'main', repo_path = $1 WHERE id = 'proj_default'`, repo); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(ctx, db, board.Card{Status: "human_review"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET project_id = 'proj_default', head_sha = $2 WHERE id = $1`, card.ID, head); err != nil {
		t.Fatal(err)
	}
	deps := app.JobDeps{Data: data, Key: tokenKey, Tools: app.Tools{Git: git}}

	diff, err := app.ReadCardDiff(ctx, db, deps, "proj_default", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff.SHA != head || diff.Base != "main" || diff.Missing || diff.Truncated {
		t.Fatalf("diff = %+v", diff)
	}
	files := map[string]app.DiffFile{}
	for _, f := range diff.Files {
		files[f.Path] = f
	}
	if len(files) != 2 {
		t.Fatalf("files = %v", diff.Files)
	}
	hello := files["hello.txt"]
	if hello.Added != 2 || hello.Deleted != 1 || len(hello.Hunks) != 1 {
		t.Fatalf("hello = %+v", hello)
	}
	var kinds []string
	for _, line := range hello.Hunks[0].Lines {
		kinds = append(kinds, line.Kind+":"+line.Text)
	}
	want := "del:hello job add:hello done add:key [redacted]"
	if strings.Join(kinds, " ") != want {
		t.Fatalf("lines = %q, want %q", strings.Join(kinds, " "), want)
	}
	big := files["big.txt"]
	if !big.Truncated || big.Added != app.FileLineCap+10 {
		t.Fatalf("big = added %d truncated %v", big.Added, big.Truncated)
	}

	if _, err := app.ReadCardDiff(ctx, db, deps, "proj_other", card.ID); !errors.Is(err, board.ErrNotFound) {
		t.Fatalf("other project err = %v", err)
	}
}

type mergedDiffFixture struct {
	db      *sql.DB
	deps    app.JobDeps
	git     string
	repo    string
	card    board.Card
	head    string
	oldMain string
}

// newMergedDiffFixture commits one card change on orch/card and leaves main
// checked out where the card started.
func newMergedDiffFixture(t *testing.T) mergedDiffFixture {
	t.Helper()
	ctx := context.Background()
	f := mergedDiffFixture{db: boardtest.Open(t), git: lookGit(t)}
	f.repo = tempRepo(t, f.git, map[string]string{"hello.txt": "hello job\n"})
	f.oldMain = revParse(t, f.repo, f.git, "HEAD")
	runGit(t, f.repo, f.git, "checkout", "-b", "orch/card")
	writeFile(t, filepath.Join(f.repo, "hello.txt"), "hello done\n")
	writeFile(t, filepath.Join(f.repo, "new.txt"), "new\n")
	commitAll(t, f.repo, f.git, "say done")
	f.head = revParse(t, f.repo, f.git, "HEAD")
	runGit(t, f.repo, f.git, "checkout", "main")

	if _, err := f.db.Exec(`UPDATE projects SET github_repo = 'widget', github_base = 'main', repo_path = $1 WHERE id = 'proj_default'`, f.repo); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(ctx, f.db, board.Card{Status: "approved"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE cards SET project_id = 'proj_default', head_sha = $2 WHERE id = $1`, card.ID, f.head); err != nil {
		t.Fatal(err)
	}
	f.card = card
	f.deps = app.JobDeps{Data: t.TempDir(), Key: tokenKey, Tools: app.Tools{Git: f.git}}
	return f
}

func (f mergedDiffFixture) wantCardFiles(t *testing.T) {
	t.Helper()
	diff, err := app.ReadCardDiff(context.Background(), f.db, f.deps, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff.SHA != f.head || diff.Missing {
		t.Fatalf("diff = %+v", diff)
	}
	var paths []string
	for _, file := range diff.Files {
		paths = append(paths, file.Path)
	}
	if strings.Join(paths, " ") != "hello.txt new.txt" {
		t.Fatalf("files = %v, want the card's two files", paths)
	}
}

func TestCardDiffAfterMerge(t *testing.T) {
	f := newMergedDiffFixture(t)
	identity := []string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgSign=false"}
	runGit(t, f.repo, f.git, append(identity, "merge", "--no-ff", "-m", "Merge orch/card", "orch/card")...)
	writeFile(t, filepath.Join(f.repo, "later.txt"), "later\n")
	commitAll(t, f.repo, f.git, "later work")
	f.wantCardFiles(t)
}

func TestCardDiffAfterFastForward(t *testing.T) {
	f := newMergedDiffFixture(t)
	runGit(t, f.repo, f.git, "merge", "--ff-only", "orch/card")
	if err := board.InsertAttempt(context.Background(), f.db, board.Attempt{
		ID: "att_ffbase000001", CardID: f.card.ID, N: 1, TaskClass: "implement", ModelID: "m", MaxTurns: 1, BaseSHA: f.oldMain,
	}); err != nil {
		t.Fatal(err)
	}
	f.wantCardFiles(t)
}

func TestReviewIgnoresApprovalStatus(t *testing.T) {
	f := newReviewFixture(t)
	f.calls.setPR(prJSON(f.sha, `[{"context":"`+app.ApprovalContext+`","state":"PENDING"}]`))
	card := f.review(t, approveScript)
	if card.Status != "human_review" {
		t.Fatalf("card = %s, want human_review without waiting on the approval status", card.Status)
	}
	if r := latestReviewRow(t, f.db, card.ID); !r.ciRequired.Valid || r.ciRequired.Bool {
		t.Fatalf("ci_required = %v, want false", r.ciRequired)
	}
}
