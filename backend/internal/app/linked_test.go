package app_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/runtime"
)

func TestCheckoutUsesBaseBranch(t *testing.T) {
	db := boardtest.Open(t)
	git := lookGit(t)
	repo := tempRepo(t, git, map[string]string{"hello.txt": "main\n"})
	runGit(t, repo, git, "checkout", "-b", "develop")
	writeFile(t, filepath.Join(repo, "hello.txt"), "develop\n")
	commitAll(t, repo, git, "develop")
	develop := revParse(t, repo, git, "develop")
	runGit(t, repo, git, "checkout", "main")
	data := dataDir(t)
	card := insertReady(t, db, "")
	provider, err := fake.Load([]byte(`{"implement":[{"text":"done"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RunImplementer(context.Background(), loop.Input{
		DB: db, Git: runtime.Git{Bin: git, Repo: repo, Data: data, Bot: "bot"}, BaseBranch: "develop",
		CardID: card.ID, Model: "fake-model", MaxTurns: 2, Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	var base string
	if err := db.QueryRow(`SELECT base_sha FROM attempts WHERE id = $1`, result.AttemptID).Scan(&base); err != nil {
		t.Fatal(err)
	}
	if base != develop {
		t.Fatalf("base = %s want develop %s", base, develop)
	}
	body, err := os.ReadFile(filepath.Join(data, "worktrees", card.ID, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "develop\n" {
		t.Fatalf("worktree hello.txt = %q", body)
	}
}

func TestIntakePerProject(t *testing.T) {
	db := boardtest.Open(t)
	insertProject(t, db, "proj_b", "Widget API")
	repoA := t.TempDir()
	repoB := t.TempDir()
	data := t.TempDir()
	writeFile(t, filepath.Join(repoA, "docs", "prd", "a.md"), "### REQ-001 Board\n\nProject A statement.\n")
	writeFile(t, filepath.Join(repoB, "docs", "prd", "b.md"), "### REQ-001 Api\n\nProject B statement.\n")
	writeFile(t, filepath.Join(data, "config.yaml"), "operatorLogin: ada\n")
	ctx := context.Background()
	if _, err := app.Ingest(ctx, db, "proj_default", repoA, data); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Ingest(ctx, db, "proj_b", repoB, data); err != nil {
		t.Fatal(err)
	}
	if err := app.Accept(ctx, db, "proj_b", data, []string{"REQ-001"}); err != nil {
		t.Fatal(err)
	}
	rows := map[string]struct {
		statement string
		accepted  bool
	}{}
	query, err := db.Query(`SELECT project_id, statement, accepted_at IS NOT NULL FROM requirements WHERE id = 'REQ-001'`)
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()
	for query.Next() {
		var project, statement string
		var accepted bool
		if err := query.Scan(&project, &statement, &accepted); err != nil {
			t.Fatal(err)
		}
		rows[project] = struct {
			statement string
			accepted  bool
		}{statement, accepted}
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if !strings.Contains(rows["proj_default"].statement, "Project A") || rows["proj_default"].accepted {
		t.Fatalf("project A = %+v", rows["proj_default"])
	}
	if !strings.Contains(rows["proj_b"].statement, "Project B") || !rows["proj_b"].accepted {
		t.Fatalf("project B = %+v", rows["proj_b"])
	}
	var runs int
	if err := db.QueryRow(`SELECT count(*) FROM runs WHERE project_id = 'proj_b'`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("proj_b runs = %d", runs)
	}
}

func TestPackIncludesLinkedRequirements(t *testing.T) {
	f := newLinkedFixture(t)
	result, provider := f.run(t, `{"implement":[{"text":"done"}]}`)
	prompts := provider.Prompts()
	if len(prompts) == 0 {
		t.Fatal("no prompt")
	}
	user := prompts[0].User
	for _, want := range []string{
		"LINKED Widget API (proj_b) at " + f.sha,
		"Widget API:REQ-001 Linked statement",
		"Widget API:RFC-001 Linked decision",
		"docs/api.yaml",
		"openapi: 3.1.0",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("pack missing %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "Unaccepted statement") {
		t.Fatal("pack includes an unaccepted linked requirement")
	}
	if strings.Contains(user, "private.txt") {
		t.Fatal("pack includes a file outside the shared paths")
	}
	var payload string
	if err := f.db.QueryRow(`
		SELECT payload_json FROM events
		WHERE card_id = $1 AND kind = 'tool_call' AND payload_json LIKE '%linked_context%'`, f.card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"projectId":"proj_b"`) || !strings.Contains(payload, f.sha) {
		t.Fatalf("linked_context = %s", payload)
	}
	export := filepath.Join(f.data, "exports", "linked-proj_b-"+f.sha)
	if _, err := os.Stat(filepath.Join(export, "docs", "api.yaml")); err != nil {
		t.Fatalf("export missing: %v", err)
	}
	_ = result
}

func TestReadLinkedFileReadOnly(t *testing.T) {
	f := newLinkedFixture(t)
	export := filepath.Join(f.data, "exports", "linked-proj_b-"+f.sha)
	script := `{"implement":[
		{"tool":"read_linked_file","project":"proj_b","path":"private.txt"},
		{"tool":"read_linked_file","project":"proj_b","path":"../../secrets.json"},
		{"tool":"read_linked_file","project":"proj_other","path":"docs/api.yaml"},
		{"tool":"edit_file","path":"` + filepath.Join(export, "docs", "api.yaml") + `","text":"overwritten"},
		{"tool":"edit_file","path":"../exports/linked-proj_b-` + f.sha + `/docs/api.yaml","text":"overwritten"}
	]}`
	writeFile(t, filepath.Join(f.data, "secrets.json"), "planted-outside-export")
	f.run(t, script)
	summaries := toolSummaries(t, f.db, f.card.ID)
	reads := strings.Join(summaries["read_linked_file"], "\n---\n")
	if len(summaries["read_linked_file"]) != 3 || !strings.Contains(reads, "private") || !strings.Contains(reads, "not a linked project") {
		t.Fatalf("linked reads = %q", reads)
	}
	if strings.Contains(reads, "planted-outside-export") {
		t.Fatalf("read escaped the export: %q", reads)
	}
	for _, summary := range summaries["edit_file"] {
		if strings.HasPrefix(summary, "wrote") {
			t.Fatalf("edit reached the export: %q", summary)
		}
	}
	body, err := os.ReadFile(filepath.Join(export, "docs", "api.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "overwritten") {
		t.Fatal("linked export was written")
	}
	out, err := exec.Command(lookGit(t), "-C", f.linkedRepo, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("linked clone changed: %s", out)
	}
	for _, tool := range loop.ImplementerTools() {
		if strings.Contains(tool.Name, "linked") && tool.Name != "read_linked_file" {
			t.Fatalf("profile has %s", tool.Name)
		}
	}
}

type linkedFixture struct {
	db         *sql.DB
	repo       string
	data       string
	linkedRepo string
	sha        string
	card       board.Card
}

func newLinkedFixture(t *testing.T) linkedFixture {
	t.Helper()
	db := boardtest.Open(t)
	git := lookGit(t)
	insertProject(t, db, "proj_b", "Widget API")
	if _, err := db.Exec(`
		INSERT INTO requirements (id, project_id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json, content_hash, accepted_at, accepted_by)
		VALUES ('REQ-001', 'proj_b', 'prd', 'docs/prd/a.md', 'REQ-001', 'Linked statement', '[]', '[]', 'h1', '2026-01-01T00:00:00Z', 'ada'),
		       ('REQ-002', 'proj_b', 'prd', 'docs/prd/a.md', 'REQ-002', 'Unaccepted statement', '[]', '[]', 'h2', NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO decisions (id, project_id, source_path, statement, content_hash)
		VALUES ('RFC-001', 'proj_b', 'docs/rfc/a.md', 'Linked decision', 'h3')`); err != nil {
		t.Fatal(err)
	}
	linkedRepo := tempRepo(t, git, map[string]string{
		"docs/api.yaml": "openapi: 3.1.0\n",
		"private.txt":   "private\n",
	})
	repo := tempRepo(t, git, map[string]string{"hello.txt": "hello\n"})
	return linkedFixture{
		db: db, repo: repo, data: dataDir(t), linkedRepo: linkedRepo,
		sha:  revParse(t, linkedRepo, git, "main"),
		card: insertReady(t, db, ""),
	}
}

func (f linkedFixture) run(t *testing.T, script string) (loop.Result, *fake.Fake) {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	git := lookGit(t)
	result, err := app.RunImplementer(context.Background(), loop.Input{
		DB: f.db, Git: runtime.Git{Bin: git, Repo: f.repo, Data: f.data, Bot: "bot"},
		Linked: []loop.LinkedSource{{
			ProjectID: "proj_b", Name: "Widget API", BaseBranch: "main",
			Git:         runtime.Git{Bin: git, Repo: f.linkedRepo, Data: dataDir(t)},
			SharedPaths: []string{"docs"},
		}},
		CardID: f.card.ID, Model: "fake-model", MaxTurns: 8, Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result, provider
}

func toolSummaries(t *testing.T, db *sql.DB, cardID string) map[string][]string {
	t.Helper()
	rows, err := db.Query(`
		SELECT payload_json::jsonb->>'name', payload_json::jsonb->>'summary' FROM events
		WHERE card_id = $1 AND kind = 'tool_call'`, cardID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var name, summary string
		if err := rows.Scan(&name, &summary); err != nil {
			t.Fatal(err)
		}
		out[name] = append(out[name], summary)
	}
	return out
}

func insertProject(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at, github_repo, github_base)
		VALUES ($1, 'tribe_default', $2, '', '2026-01-01T00:00:00Z', $1, 'main')`, id, name); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO intake_state (id, draft_pending, project_id) VALUES (1, false, $1)`, id); err != nil {
		t.Fatal(err)
	}
}

func tempRepo(t *testing.T, git string, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, git, "init", "-b", "main")
	for name, body := range files {
		writeFile(t, filepath.Join(repo, name), body)
	}
	commitAll(t, repo, git, "init")
	return repo
}

func commitAll(t *testing.T, repo, git, message string) {
	t.Helper()
	runGit(t, repo, git, "add", "-A")
	runGit(t, repo, git, "-c", "user.name=fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgSign=false", "commit", "-m", message)
}

func revParse(t *testing.T, repo, git, ref string) string {
	t.Helper()
	out, err := exec.Command(git, "-C", repo, "rev-parse", ref).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
