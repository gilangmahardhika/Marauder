package app_test

import (
	"context"
	"database/sql"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/runtime"
)

// runLoop runs one attempt of the tool loop on the fixture repository and
// returns the result with the fake, whose prompts hold every request.
func runLoop(t *testing.T, db *sql.DB, repo, data, cardID, script string, maxTurns int) (loop.Result, *fake.Fake) {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RunImplementer(context.Background(), loop.Input{
		DB: db, Git: runtime.Git{Bin: lookGit(t), Repo: repo, Data: data, Author: runtime.LoginAuthor("bot", 0)},
		CardID: cardID, Model: "fake-model", TaskClass: "implement", MaxTurns: maxTurns,
		Provider: provider, Allow: []string{"true", "false"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result, provider
}

func loopCard(t *testing.T, db *sql.DB, local string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), db, board.Card{Title: "Say hello again", Status: "ready", LocalCheck: local})
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func headMessage(t *testing.T, data, cardID string) string {
	t.Helper()
	out, err := exec.Command(lookGit(t), "-C", filepath.Join(data, "worktrees", cardID), "log", "-1", "--format=%B").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRereadUnchanged(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := loopCard(t, db, "true")
	defer cleanupWorktree(t, repo, data, card.ID)
	_, provider := runLoop(t, db, repo, data, card.ID, `{"implement":[
		{"tool":"read_file","path":"hello.txt"},
		{"tool":"read_file","path":"hello.txt"},
		{"tool":"edit_file","path":"hello.txt","text":"new text\n"},
		{"tool":"read_file","path":"hello.txt"},
		{"tool":"commit","message":"Change hello"}]}`, 8)
	turns := provider.Prompts()[5].Turns
	if !strings.HasPrefix(turns[1].Results[0], "unchanged since turn 1") {
		t.Fatalf("second read = %q", turns[1].Results[0])
	}
	if !strings.HasPrefix(turns[0].Results[0], "1|") || !strings.Contains(turns[3].Results[0], "1|new text") {
		t.Fatalf("reads = %q, %q", turns[0].Results[0], turns[3].Results[0])
	}
}

func TestTextReplyNudged(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := loopCard(t, db, "true")
	defer cleanupWorktree(t, repo, data, card.ID)
	result, provider := runLoop(t, db, repo, data, card.ID, `{"implement":[
		{"text":"The change looks done to me."},
		{"tool":"edit_file","path":"hello.txt","text":"nudged\n"},
		{"tool":"commit","message":"Say nudged"}]}`, 8)
	if result.Outcome != "" {
		t.Fatalf("result = %+v", result)
	}
	prompts := provider.Prompts()
	first := prompts[1].Turns[0]
	if first.Text != "The change looks done to me." || !strings.Contains(first.Note, "without a tool call") || len(first.Calls) != 0 {
		t.Fatalf("nudged turn = %+v", first)
	}

	other := loopCard(t, db, "true")
	defer cleanupWorktree(t, repo, data, other.ID)
	result, provider = runLoop(t, db, repo, data, other.ID, `{"implement":[{"text":"done"}]}`, 8)
	if result.Outcome != "no_commit" || len(provider.Prompts()) != 3 {
		t.Fatalf("result = %+v after %d calls, want no_commit after 1 reply and 2 nudges", result, len(provider.Prompts()))
	}
}

func TestTruncatedReplyNudged(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := loopCard(t, db, "true")
	defer cleanupWorktree(t, repo, data, card.ID)
	result, provider := runLoop(t, db, repo, data, card.ID, `{"implement":[
		{"stop":"max_tokens","text":"Here is the whole fi"},
		{"tool":"edit_file","path":"hello.txt","text":"short\n"},
		{"tool":"commit","message":"Say short"}]}`, 8)
	if result.Outcome != "" {
		t.Fatalf("result = %+v", result)
	}
	note := provider.Prompts()[1].Turns[0].Note
	if !strings.Contains(note, "cut at the token limit") {
		t.Fatalf("note = %q", note)
	}
}

func TestAutoCommitAfterPassingCheck(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := loopCard(t, db, "true")
	defer cleanupWorktree(t, repo, data, card.ID)
	result, _ := runLoop(t, db, repo, data, card.ID, `{"implement":[
		{"tool":"edit_file","path":"hello.txt","text":"auto\n"},
		{"tool":"run_local_check"}]}`, 2)
	if result.Outcome != "" {
		t.Fatalf("result = %+v, want a commit", result)
	}
	message := headMessage(t, data, card.ID)
	if !strings.HasPrefix(message, "Say hello again\n") || !strings.Contains(message, "Committed-By: harness after passing local check") ||
		!strings.Contains(message, "Orchestrator-Card: "+card.ID) {
		t.Fatalf("message = %q", message)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'tool_call' AND payload_json LIKE '%"auto":true%'`, card.ID).Scan(&payload); err != nil {
		t.Fatalf("auto commit event: %v", err)
	}
}

func TestNoAutoCommitOnDirtyTree(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := loopCard(t, db, "true")
	defer cleanupWorktree(t, repo, data, card.ID)
	result, _ := runLoop(t, db, repo, data, card.ID, `{"implement":[
		{"tool":"edit_file","path":"hello.txt","text":"checked\n"},
		{"tool":"run_local_check"},
		{"tool":"edit_file","path":"hello.txt","text":"changed after the check\n"}]}`, 8)
	if result.Outcome != "no_commit" {
		t.Fatalf("result = %+v, want no_commit", result)
	}
	failed := loopCard(t, db, "false")
	defer cleanupWorktree(t, repo, data, failed.ID)
	result, _ = runLoop(t, db, repo, data, failed.ID, `{"implement":[
		{"tool":"edit_file","path":"hello.txt","text":"red\n"},
		{"tool":"run_local_check"}]}`, 8)
	if result.Outcome != "no_commit" {
		t.Fatalf("result after a failing check = %+v, want no_commit", result)
	}
}

func TestBadToolJSONReturnedToModel(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := loopCard(t, db, "true")
	defer cleanupWorktree(t, repo, data, card.ID)
	result, provider := runLoop(t, db, repo, data, card.ID, `{"implement":[
		{"tool":"edit_file","badArgs":"unexpected end of JSON input"},
		{"tool":"edit_file","path":"hello.txt","text":"fixed\n"},
		{"tool":"commit","message":"Say fixed"}]}`, 8)
	if result.Outcome != "" {
		t.Fatalf("result = %+v", result)
	}
	got := provider.Prompts()[1].Turns[0].Results[0]
	if !strings.HasPrefix(got, "error: the arguments were not valid JSON (unexpected end of JSON input)") {
		t.Fatalf("result = %q", got)
	}
}

func blockedWith(t *testing.T, card board.Card, reason string, used int) {
	t.Helper()
	if card.Status != "blocked" || card.BlockedReason != reason || card.AttemptsUsed != used {
		t.Fatalf("card = %s %s used %d, want blocked %s used %d", card.Status, card.BlockedReason, card.AttemptsUsed, reason, used)
	}
}

func TestTurnCapStops(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	script := `{"implement":[` + strings.Repeat(`{"tool":"list_files","path":"."},`, 6) + `{"tool":"commit","message":"late"}]}`
	card := f.run(t, script)
	blockedWith(t, card, "turn_cap", 1)
	stops := payloads(t, f, "budget_stop")
	if len(stops) != 1 || stops[0]["cap"] != "turn_cap" || stops[0]["turns"] != float64(6) {
		t.Fatalf("budget_stop = %v", stops)
	}
	var outcome string
	if err := f.db.QueryRow(`SELECT outcome FROM attempts WHERE card_id = $1`, card.ID).Scan(&outcome); err != nil || outcome != "budget" {
		t.Fatalf("outcome = %s, %v", outcome, err)
	}
}

func TestTokenCapStops(t *testing.T) {
	router := strings.Replace(routerConfig, "maxModelTurnsPerAttempt: 6\n", "maxModelTurnsPerAttempt: 6\n    maxTokensPerCard: 100\n", 1)
	f := newJobFixture(t, "true", 3, router, fixtureProject)
	provider, err := fake.Load([]byte(`{"implement":[` + strings.Repeat(`{"tool":"list_files","path":"."},`, 5) + `{"tool":"list_files"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	provider.Usage = providers.Response{InputTokens: 80, OutputTokens: 30}
	card, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	blockedWith(t, card, "token_cap", 1)
	if n := len(provider.Prompts()); n != 1 {
		t.Fatalf("model calls = %d, want 1 before the cap", n)
	}
	if stops := payloads(t, f, "budget_stop"); len(stops) != 1 || stops[0]["cap"] != "token_cap" {
		t.Fatalf("budget_stop = %v", stops)
	}
}

func TestInfraErrorKeepsAttempt(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	f.calls.gateExit = 125
	card := f.run(t, commitScript)
	blockedWith(t, card, "sandbox_config", 0)
	errs := payloads(t, f, "job_error")
	if len(errs) == 0 || !strings.Contains(errs[len(errs)-1]["message"].(string), "exited 125") {
		t.Fatalf("job_error = %v", errs)
	}
	if _, err := app.Unblock(context.Background(), f.db, card.ID, false); err != nil {
		t.Fatal(err)
	}
	provider, err := fake.Load([]byte(commitScript))
	if err != nil {
		t.Fatal(err)
	}
	card, err = app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" || card.AttemptsUsed != 1 || len(provider.Prompts()) != 0 {
		t.Fatalf("rerun = %s used %d after %d model calls", card.Status, card.AttemptsUsed, len(provider.Prompts()))
	}
}

func TestProviderErrorKeepsAttempt(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	card, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: failingProvider{},
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	blockedWith(t, card, "provider_error", 0)
}

func TestNoChangeGoesToGate(t *testing.T) {
	f := newJobFixture(t, "false", 3, routerConfig, fixtureProject)
	card := f.run(t, commitScript)
	if card.Status != "in_progress" || card.AttemptsUsed != 1 {
		t.Fatalf("first run = %s used %d", card.Status, card.AttemptsUsed)
	}
	var firstSHA string
	if err := f.db.QueryRow(`SELECT commit_sha FROM attempts WHERE card_id = $1`, card.ID).Scan(&firstSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE cards SET local_check = 'true' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	provider, err := fake.Load([]byte(`{"implement":[{"text":"The branch already says job."}]}`))
	if err != nil {
		t.Fatal(err)
	}
	card, err = app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" || card.AttemptsUsed != 2 {
		t.Fatalf("second run = %s used %d", card.Status, card.AttemptsUsed)
	}
	var sha, outcome string
	if err := f.db.QueryRow(`SELECT commit_sha, outcome FROM attempts WHERE card_id = $1 AND n = 2`, card.ID).Scan(&sha, &outcome); err != nil {
		t.Fatal(err)
	}
	if sha != firstSHA || outcome != "checks_passed" {
		t.Fatalf("second attempt = %s %s, want the first commit %s gated", sha, outcome, firstSHA)
	}
	if notes := countNotes(t, f.db, card.ID, "no_commit"); notes != 0 {
		t.Fatalf("no_commit notes = %d", notes)
	}
}

func TestCacheUsageRecorded(t *testing.T) {
	db := boardtest.Open(t)
	repo := openFixture(t)
	data := dataDir(t)
	card := loopCard(t, db, "true")
	defer cleanupWorktree(t, repo, data, card.ID)
	provider, err := fake.Load([]byte(`{"implement":[
		{"tool":"edit_file","path":"hello.txt","text":"cached\n"},
		{"tool":"commit","message":"Say cached"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	provider.Usage = providers.Response{InputTokens: 40, OutputTokens: 5, CachedInputTokens: 900}
	result, err := app.RunImplementer(context.Background(), loop.Input{
		DB: db, Git: runtime.Git{Bin: lookGit(t), Repo: repo, Data: data, Author: runtime.LoginAuthor("bot", 0)},
		CardID: card.ID, Model: "fake-model", TaskClass: "implement", MaxTurns: 4, Provider: provider, Allow: []string{"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var in, out, cached int
	if err := db.QueryRow(`SELECT input_tokens, output_tokens, tokens_cached FROM attempts WHERE id = $1`, result.AttemptID).Scan(&in, &out, &cached); err != nil {
		t.Fatal(err)
	}
	calls := len(provider.Prompts())
	if in != 40*calls || out != 5*calls || cached != 900*calls {
		t.Fatalf("attempt tokens = in %d out %d cached %d after %d calls", in, out, cached, calls)
	}
	used, err := board.CardTokens(context.Background(), db, card.ID)
	if err != nil || used != 45*calls+90*calls {
		t.Fatalf("card tokens = %d, %v; want cached reads at a tenth", used, err)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'model_call' LIMIT 1`, card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"cachedInputTokens":900`) {
		t.Fatalf("model_call = %s", payload)
	}
}

func TestTokenCapCountsCachedAtTenth(t *testing.T) {
	router := strings.Replace(routerConfig, "maxModelTurnsPerAttempt: 6\n", "maxModelTurnsPerAttempt: 6\n    maxTokensPerCard: 100\n", 1)
	f := newJobFixture(t, "true", 3, router, fixtureProject)
	provider, err := fake.Load([]byte(`{"implement":[` + strings.Repeat(`{"tool":"list_files","path":"."},`, 5) + `{"tool":"list_files"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	provider.Usage = providers.Response{InputTokens: 10, OutputTokens: 10, CachedInputTokens: 500}
	card, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	blockedWith(t, card, "token_cap", 1)
	if n := len(provider.Prompts()); n != 2 {
		t.Fatalf("model calls = %d, want 2: each counts 20 plus 500 cached at a tenth", n)
	}
}
