package app_test

import (
	"context"
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
