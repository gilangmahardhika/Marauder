package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/trace"
)

// databaseURL stands in for the service secret every job redacts. Its & is
// escaped by json.Marshal, so only the redaction before the cut catches it.
const databaseURL = "postgres://orch:p&ss@db.internal:5432/board"

func (f jobFixture) runSecret(t *testing.T, script string) (board.Card, *fake.Fake) {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	card, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider, Secrets: []string{databaseURL},
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return card, provider
}

func payloads(t *testing.T, f jobFixture, kind string) []map[string]any {
	t.Helper()
	events, err := trace.List(context.Background(), f.db, f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, ev := range events {
		if ev.Kind != kind {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
			t.Fatal(err)
		}
		out = append(out, payload)
	}
	return out
}

func sayScript(say string) string {
	text, _ := json.Marshal(say)
	return `{"implement":[{"tool":"edit_file","path":"hello.txt","text":"job\n","say":` + string(text) + `},{"tool":"commit","message":"Job commit"}]}`
}

func TestModelCallText(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	say := "I will edit hello.txt using " + databaseURL + " then commit. " + strings.Repeat("é", 4<<10) + " The end of my plan."
	if card, _ := f.runSecret(t, sayScript(say)); card.Status != "pr_open" {
		t.Fatalf("card = %s", card.Status)
	}
	calls := payloads(t, f, "model_call")
	if len(calls) == 0 {
		t.Fatal("no model_call events")
	}
	text, _ := calls[0]["text"].(string)
	if !strings.HasPrefix(text, "I will edit hello.txt using [redacted] then commit.") || !strings.HasSuffix(text, "The end of my plan.") {
		t.Fatalf("text head/tail = %q", text)
	}
	if strings.Contains(text, "p&ss") || len(text) > trace.TextCap+64 || !utf8.ValidString(text) || !strings.Contains(text, "bytes cut) ...") {
		t.Fatalf("text is %d bytes, valid %v", len(text), utf8.ValidString(text))
	}
	for _, call := range calls[1:] {
		if _, ok := call["text"]; ok {
			t.Fatalf("a reply with no text stored text: %v", call)
		}
	}
}

func TestToolCallArgs(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	script := `{"implement":[
		{"calls":[
			{"tool":"list_files","path":".","glob":"*.txt","depth":1},
			{"tool":"search_files","text":"hello","glob":"*.txt","context":1,"ignoreCase":true},
			{"tool":"read_file","path":"hello.txt","offset":1,"limit":5}]},
		{"calls":[
			{"tool":"edit_file","path":"hello.txt","text":"FILE-BODY one\nFILE-BODY two\n"},
			{"tool":"replace_in_file","path":"hello.txt","old":"FILE-BODY two","new":"FILE-BODY 2"},
			{"tool":"edit_file","path":"extra.txt","text":"x\n"},
			{"tool":"move_file","from":"extra.txt","to":"moved.txt"},
			{"tool":"delete_file","path":"moved.txt"}]},
		{"tool":"run_local_check"},
		{"tool":"commit","message":"Say job\n\nUses ` + databaseURL + `"}]}`
	if card, _ := f.runSecret(t, script); card.Status != "pr_open" {
		t.Fatalf("card = %s", card.Status)
	}
	args := map[string]map[string]any{}
	for _, call := range payloads(t, f, "tool_call") {
		name, _ := call["name"].(string)
		if raw, err := json.Marshal(call); err != nil || strings.Contains(string(raw), "FILE-BODY") {
			t.Fatalf("tool_call stored file text: %s", raw)
		}
		if got, ok := call["args"].(map[string]any); ok && args[name] == nil {
			args[name] = got
		}
	}
	want := map[string]map[string]any{
		"list_files":      {"path": ".", "glob": "*.txt", "depth": float64(1)},
		"search_files":    {"text": "hello", "glob": "*.txt", "context": float64(1), "ignoreCase": true},
		"read_file":       {"path": "hello.txt", "offset": float64(1), "limit": float64(5)},
		"edit_file":       {"path": "hello.txt", "bytes": float64(28), "lines": float64(2)},
		"replace_in_file": {"path": "hello.txt", "oldBytes": float64(13), "newBytes": float64(11)},
		"move_file":       {"from": "extra.txt", "to": "moved.txt"},
		"delete_file":     {"path": "moved.txt"},
		"run_local_check": {"command": "true"},
		"commit":          {"message": "Say job\n\nUses [redacted]"},
	}
	for name, fields := range want {
		for key, value := range fields {
			if args[name][key] != value {
				t.Fatalf("%s args = %v, want %s = %v", name, args[name], key, value)
			}
		}
	}
}

func TestReviewerPackHasNoAuthorText(t *testing.T) {
	f := newJobFixture(t, "true", 3, reviewRouter, fixtureProject)
	const note = "AUTHOR-REPLY: I skipped the edge case on purpose."
	card, _ := f.runSecret(t, sayScript(note))
	if card.Status != "pr_open" {
		t.Fatalf("card = %s", card.Status)
	}
	if text, _ := payloads(t, f, "model_call")[0]["text"].(string); text != note {
		t.Fatalf("stored text = %q", text)
	}
	var sha string
	if err := f.db.QueryRow(`SELECT commit_sha FROM attempts WHERE card_id = $1`, card.ID).Scan(&sha); err != nil {
		t.Fatal(err)
	}
	f.calls.setPR(prJSON(sha, noChecks))
	rf := reviewFixture{jobFixture: f, card: card, sha: sha, now: newReviewFixtureClock()}
	deps, provider := rf.deps(t, approveScript)
	if _, err := app.ReviewCard(context.Background(), f.db, deps, "proj_default", card.ID); err != nil {
		t.Fatal(err)
	}
	reviewed := 0
	for _, prompt := range provider.Prompts() {
		if prompt.TaskClass != "review" {
			continue
		}
		reviewed++
		if strings.Contains(prompt.System+prompt.User, "AUTHOR-REPLY") {
			t.Fatal("the reviewer prompt carries the implementer's reply text")
		}
	}
	if reviewed == 0 {
		t.Fatal("the reviewer was not called")
	}
}
