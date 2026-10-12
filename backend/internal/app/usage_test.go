package app_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/trace"
)

const usageConfig = `llm:
  families:
    - id: anthropic-main
      protocol: anthropic
      baseUrl: https://anthropic.invalid
      models:
        - id: claude-a
    - id: openai-main
      protocol: openai
      baseUrl: https://openai.invalid
      models:
        - id: gpt-b
`

var usageNow = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

// usageBoard stores model calls on three cards and one refine: Ana owns a
// default-project card, Ben owns a Gadget card and ran the refine, and one
// card has no owner. A tool call and calls outside October 1 to 3 are noise.
func usageBoard(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	db := boardtest.Open(t)
	data := dataDir(t)
	if err := os.WriteFile(filepath.Join(data, "config.yaml"), []byte(usageConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, name, email, password_hash, role, created_at) VALUES
			('usr_ana', 'Ana', 'ana@example.com', 'x', 'developer', '2026-10-01T00:00:00Z'),
			('usr_ben', '', 'ben@example.com', 'x', 'developer', '2026-10-01T00:00:00Z')`,
		`INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
			VALUES ('proj_gadget', 'tribe_default', 'Gadget', '/tmp/gadget', '2026-10-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	cards := map[string]board.Card{}
	for name, card := range map[string]board.Card{
		"ana":   {Status: "human_review", Title: "Ana card", OwnerID: "usr_ana"},
		"ben":   {Status: "human_review", Title: "Ben card", OwnerID: "usr_ben", ProjectID: "proj_gadget"},
		"loose": {Status: "human_review", Title: "No owner"},
	} {
		stored, err := board.InsertCard(ctx, db, card)
		if err != nil {
			t.Fatal(err)
		}
		cards[name] = stored
	}
	for _, a := range []board.Attempt{
		{ID: "att_ana", CardID: cards["ana"].ID, N: 1, TaskClass: "implement_hard"},
		{ID: "att_ben", CardID: cards["ben"].ID, N: 1, TaskClass: "implement"},
	} {
		if err := board.InsertAttempt(ctx, db, a); err != nil {
			t.Fatal(err)
		}
	}
	call := func(model string, in, out, cached int, extra map[string]any) map[string]any {
		p := map[string]any{"modelId": model, "inputTokens": in, "outputTokens": out, "cachedInputTokens": cached}
		for k, v := range extra {
			p[k] = v
		}
		return p
	}
	ana, ben, loose := cards["ana"].ID, cards["ben"].ID, cards["loose"].ID
	for _, ev := range []trace.Event{
		{TS: "2026-10-01T10:00:00Z", CardID: ana, AttemptID: "att_ana", Kind: "model_call",
			Payload: call("claude-a", 100, 10, 5, map[string]any{"role": "implementer", "taskClass": "implement_hard"})},
		{TS: "2026-10-01T11:00:00Z", CardID: ana, AttemptID: "att_ana", Kind: "model_call",
			Payload: call("gpt-b", 50, 5, 0, map[string]any{"role": "reviewer"})},
		{TS: "2026-10-01T12:00:00Z", CardID: ana, AttemptID: "att_ana", Kind: "model_call",
			Payload: call("claude-a", 20, 2, 0, map[string]any{"role": "implementer"})},
		{TS: "2026-10-02T10:00:00Z", CardID: ben, AttemptID: "att_ben", ProjectID: "proj_gadget", Kind: "model_call",
			Payload: call("retired-model", 30, 3, 0, map[string]any{"role": "implementer"})},
		{TS: "2026-10-02T11:00:00Z", ActorID: "usr_ben", ProjectID: "proj_gadget", Kind: "requirement_refined",
			Payload: call("claude-a", 40, 4, 1, map[string]any{"requirementId": "REQ-001"})},
		{TS: "2026-10-03T23:59:59Z", CardID: loose, Kind: "model_call",
			Payload: call("gpt-b", 7, 1, 0, map[string]any{"role": "implementer"})},
		{TS: "2026-10-01T10:30:00Z", CardID: ana, AttemptID: "att_ana", Kind: "tool_call",
			Payload: map[string]any{"name": "read_file", "inputTokens": 999}},
		{TS: "2026-09-30T23:59:59Z", CardID: ana, AttemptID: "att_ana", Kind: "model_call",
			Payload: call("claude-a", 1000, 0, 0, map[string]any{"role": "implementer"})},
		{TS: "2026-10-04T00:00:00Z", CardID: ana, AttemptID: "att_ana", Kind: "model_call",
			Payload: call("claude-a", 1000, 0, 0, map[string]any{"role": "implementer"})},
	} {
		if ev.ProjectID == "" {
			ev.ProjectID = "proj_default"
		}
		if _, err := trace.Append(ctx, db, ev, nil); err != nil {
			t.Fatal(err)
		}
	}
	return db, data
}

func readUsage(t *testing.T, db *sql.DB, data string) app.UsageReport {
	t.Helper()
	report, err := app.ReadUsage(context.Background(), db, data, "2026-10-01", "2026-10-03", usageNow)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func groupTotals(t *testing.T, rows []app.UsageRow, by string) map[string]app.UsageGroup {
	t.Helper()
	groups, err := app.GroupUsage(rows, by)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]app.UsageGroup{}
	for _, g := range groups {
		out[g.Key] = g
	}
	return out
}

func TestUsageAggregates(t *testing.T) {
	db, data := usageBoard(t)
	report := readUsage(t, db, data)
	if report.From != "2026-10-01" || report.To != "2026-10-03" {
		t.Fatalf("range = %s to %s", report.From, report.To)
	}
	var sum app.UsageGroup
	days := map[string]int64{}
	for _, r := range report.Rows {
		sum.Calls += r.Calls
		sum.InputTokens += r.InputTokens
		sum.OutputTokens += r.OutputTokens
		sum.CachedInputTokens += r.CachedInputTokens
		days[r.Day] += r.Calls
	}
	want := app.UsageGroup{Calls: 6, InputTokens: 247, OutputTokens: 25, CachedInputTokens: 6}
	if sum != want {
		t.Fatalf("totals = %+v, want %+v", sum, want)
	}
	if days["2026-10-01"] != 3 || days["2026-10-02"] != 2 || days["2026-10-03"] != 1 || len(days) != 3 {
		t.Fatalf("calls by day = %v", days)
	}
	groups, err := app.GroupUsage(report.Rows, "model")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 || groups[0].Key != "claude-a" || groups[0].Calls != 3 || groups[0].Total() != 176 {
		t.Fatalf("by model = %+v", groups)
	}
	if _, err := app.GroupUsage(report.Rows, "cost"); err == nil {
		t.Fatal("grouped by an unknown dimension")
	}
	if _, err := app.ReadUsage(context.Background(), db, data, "2026-10-03", "2026-10-01", usageNow); err == nil {
		t.Fatal("read a range that ends before it starts")
	}
}

func TestUsageTaskClass(t *testing.T) {
	db, data := usageBoard(t)
	got := groupTotals(t, readUsage(t, db, data).Rows, "task")
	want := map[string]int64{"implement_hard": 120, "review": 50, "implement": 37, "refine_requirement": 40}
	if len(got) != len(want) {
		t.Fatalf("by task = %+v", got)
	}
	for task, input := range want {
		if got[task].InputTokens != input {
			t.Fatalf("%s input = %d, want %d (all %+v)", task, got[task].InputTokens, input, got)
		}
	}
}

func TestUsageUser(t *testing.T) {
	db, data := usageBoard(t)
	report := readUsage(t, db, data)
	got := groupTotals(t, report.Rows, "user")
	if got["Ana"].Calls != 3 || got["ben@example.com"].Calls != 2 || got["No owner"].Calls != 1 || len(got) != 3 {
		t.Fatalf("by user = %+v", got)
	}
	for _, r := range report.Rows {
		if r.Task == "refine_requirement" && (r.UserID != "usr_ben" || r.ProjectName != "Gadget") {
			t.Fatalf("refine row = %+v, want Ben in Gadget", r)
		}
	}
	projects := groupTotals(t, report.Rows, "project")
	if projects["Gadget"].Calls != 2 || projects["Default"].Calls != 4 {
		t.Fatalf("by project = %+v", projects)
	}
}

func TestUsageFamily(t *testing.T) {
	db, data := usageBoard(t)
	report := readUsage(t, db, data)
	got := groupTotals(t, report.Rows, "llm")
	if got["anthropic-main"].Calls != 3 || got["openai-main"].Calls != 2 || got["Unknown family"].Calls != 1 || len(got) != 3 {
		t.Fatalf("by llm = %+v", got)
	}
	for _, r := range report.Rows {
		switch r.ModelID {
		case "claude-a":
			if r.FamilyID != "anthropic-main" || r.Protocol != "anthropic" {
				t.Fatalf("claude-a row = %+v", r)
			}
		case "retired-model":
			if r.FamilyID != "" || r.Protocol != "" {
				t.Fatalf("retired-model row = %+v", r)
			}
		}
	}
}

func TestModelCallTaskClass(t *testing.T) {
	f := newJobFixture(t, "true", 3, reviewRouter, fixtureProject)
	card, _ := f.runSecret(t, commitScript)
	if card.Status != "pr_open" {
		t.Fatalf("card = %s", card.Status)
	}
	var sha string
	if err := f.db.QueryRow(`SELECT commit_sha FROM attempts WHERE card_id = $1`, card.ID).Scan(&sha); err != nil {
		t.Fatal(err)
	}
	f.calls.setPR(prJSON(sha, noChecks))
	rf := reviewFixture{jobFixture: f, card: card, sha: sha, now: newReviewFixtureClock()}
	deps, _ := rf.deps(t, approveScript)
	if _, err := app.ReviewCard(context.Background(), f.db, deps, "proj_default", card.ID); err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, call := range payloads(t, f, "model_call") {
		role, _ := call["role"].(string)
		class, _ := call["taskClass"].(string)
		if prev, ok := seen[role]; ok && prev != class {
			t.Fatalf("%s calls carry taskClass %q and %q", role, prev, class)
		}
		seen[role] = class
	}
	if seen["implementer"] != "implement" || seen["reviewer"] != "review" {
		t.Fatalf("taskClass by role = %v", seen)
	}
}
