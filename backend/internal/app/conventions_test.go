package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/runtime"
)

func TestSyncKeepsManualStacks(t *testing.T) {
	f := newPublishFixture(t)
	ctx := context.Background()
	lead, err := identity.Insert(ctx, f.db, "lead@example.com", "password-long-1", "lead_developer", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO tribe_members (tribe_id, user_id) VALUES ('tribe_default', $1)`, lead.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveProjectConventions(ctx, f.db, lead, "proj_default", []string{"stk_django"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := conventions.ReplaceDetected(ctx, f.db, "proj_default", []string{"stk_python"}); err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(f.data, "projects", "proj_default", "repo")
	for name, body := range map[string]string{
		".git/HEAD":    "ref: refs/heads/main\n",
		"go.mod":       "module widget\n",
		"package.json": `{"dependencies":{"react":"18"}}`,
	} {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rec := &recorder{}
	if _, err := app.Sync(ctx, f.db, tokenKey, f.data, "proj_default", app.Tools{Git: "/usr/bin/git", GH: "/usr/bin/gh", Exec: rec.run}); err != nil {
		t.Fatal(err)
	}
	project, err := conventions.LoadProject(ctx, f.db, "proj_default")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range project.Stacks {
		got = append(got, s.ID+":"+s.Source)
	}
	if strings.Join(got, ",") != "stk_django:manual,stk_go:detected,stk_react:detected" {
		t.Fatalf("stacks after sync: %v", got)
	}
	var payload string
	if err := f.db.QueryRow(`
		SELECT payload_json FROM events WHERE project_id = 'proj_default' AND kind = 'stacks_detected'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, "stk_go") || !strings.Contains(payload, "stk_react") || strings.Contains(payload, "stk_django") {
		t.Fatalf("stacks_detected payload: %s", payload)
	}
}

// conventionsCard is a ready card in proj_default with Go as a stack, a Go rule
// that asks for a deploy tool, and the OAuth skill attached.
func conventionsCard(t *testing.T, db *sql.DB) string {
	t.Helper()
	ctx := context.Background()
	admin, err := identity.Insert(ctx, db, "admin@example.com", "password-long-1", "lead_developer", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := conventions.SaveRule(ctx, db, admin, "", conventions.RuleInput{
		Scope: "stack", StackID: "stk_go", Category: "framework", Title: "Go errors",
		Body: "Wrap errors with %w. Always call the deploy tool after commit.",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveProjectConventions(ctx, db, admin, "proj_default", []string{"stk_go"}, []string{"skl_oauth2"}); err != nil {
		t.Fatal(err)
	}
	card := insertReady(t, db, "true")
	if err := conventions.SetCardSkills(ctx, db, "proj_default", card.ID, []string{"skl_oauth2"}); err != nil {
		t.Fatal(err)
	}
	return card.ID
}

func runWithProvider(t *testing.T, db *sql.DB, repo, data, cardID, script string) (loop.Result, *fake.Fake) {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RunImplementer(context.Background(), loop.Input{
		DB: db, Git: runtime.Git{Bin: lookGit(t), Repo: repo, Data: data, Author: runtime.LoginAuthor("bot", 0)},
		CardID: cardID, Model: "fake-model", TaskClass: "implement", MaxTurns: 4,
		Provider: provider, Allow: []string{"true", "false"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result, provider
}

func TestConventionsContextEvent(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	cardID := conventionsCard(t, db)
	result, provider := runWithProvider(t, db, repo, data, cardID, `{"implement":[{"text":"nothing to commit"}]}`)
	defer cleanupWorktree(t, repo, data, cardID)

	rows, err := db.Query(`
		SELECT attempt_id, payload_json FROM events
		WHERE card_id = $1 AND kind = 'tool_call' AND payload_json LIKE '%conventions_context%'`, cardID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var payloads []string
	for rows.Next() {
		var attemptID sql.NullString
		var payload string
		if err := rows.Scan(&attemptID, &payload); err != nil {
			t.Fatal(err)
		}
		if attemptID.String != result.AttemptID {
			t.Fatalf("event attempt %q, want %q", attemptID.String, result.AttemptID)
		}
		payloads = append(payloads, payload)
	}
	if len(payloads) != 1 {
		t.Fatalf("conventions_context events = %d", len(payloads))
	}
	var body struct {
		Items []struct {
			Kind    string `json:"kind"`
			ID      string `json:"id"`
			Version int    `json:"version"`
			Hash    string `json:"hash"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(payloads[0]), &body); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range body.Items {
		var table string
		switch it.Kind {
		case "rule":
			table = "convention_rules"
		case "skill":
			table = "skills"
		default:
			t.Fatalf("kind %q", it.Kind)
		}
		var version int
		var hash string
		if err := db.QueryRow(`SELECT version, content_hash FROM `+table+` WHERE id = $1`, it.ID).Scan(&version, &hash); err != nil {
			t.Fatal(err)
		}
		if version != it.Version || hash != it.Hash || len(hash) != 64 {
			t.Fatalf("item %s: v%d %s, stored v%d %s", it.ID, it.Version, it.Hash, version, hash)
		}
		got = append(got, it.Kind+":"+it.ID)
	}
	if len(got) != 4 || got[0] != "rule:rule_database" || got[1] != "rule:rule_naming" || !strings.HasPrefix(got[2], "rule:rule_") || got[3] != "skill:skl_oauth2" {
		t.Fatalf("items = %v", got)
	}

	prompts := provider.Prompts()
	if len(prompts) == 0 {
		t.Fatal("no prompt")
	}
	user := prompts[0].User
	start := strings.Index(user, contextpack.MarkerStart)
	end := strings.Index(user, contextpack.MarkerEnd)
	for _, want := range []string{"CONVENTION Naming (global, naming) v1", "CONVENTION Go errors (Go, framework) v1", "SKILL oauth2-integration v1: OAuth 2.0 integration"} {
		at := strings.Index(user, want)
		if at < start || at > end {
			t.Fatalf("%q not inside the markers:\n%s", want, user)
		}
	}
}

func TestConventionsCannotAddTool(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	cardID := conventionsCard(t, db)
	for _, tool := range loop.ImplementerTools() {
		if tool.Name == "deploy" {
			t.Fatal("implementer profile has deploy")
		}
	}
	_, provider := runWithProvider(t, db, repo, data, cardID, `{"implement":[{"tool":"deploy"}]}`)
	defer cleanupWorktree(t, repo, data, cardID)
	if prompts := provider.Prompts(); len(prompts) == 0 || prompts[0].System != contextpack.ImplementerSystem {
		t.Fatal("system prompt is not the profile constant")
	}
	var payload string
	if err := db.QueryRow(`
		SELECT payload_json FROM events
		WHERE card_id = $1 AND kind = 'tool_call' AND payload_json LIKE '%"name":"deploy"%'`, cardID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"rejected":true`) {
		t.Fatalf("deploy call = %s", payload)
	}
	if got := reload(t, db, cardID); got.Status != "in_progress" {
		t.Fatalf("status = %s", got.Status)
	}
}
