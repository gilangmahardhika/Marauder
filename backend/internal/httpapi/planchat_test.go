package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/fake"
)

const planChatReply = `{"plan_chat":[{"text":"Keep the first step, then write the CSV.\n\n{\"title\":\"Ship CSV\",\"steps\":[{\"text\":\"Find the handler\"},{\"text\":\"Write the export\",\"detail\":\"Reuse auth\"}]}"}]}`

// parkPlan puts the fixture card in waiting_answer with one open plan.
func parkPlan(t *testing.T, f contextFixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.srv.DB.Exec(`UPDATE cards SET status = 'waiting_answer', plan_mode = 'required' WHERE id = $1`, f.card); err != nil {
		t.Fatal(err)
	}
	attempt := board.Attempt{ID: "att_" + f.card[5:], CardID: f.card, N: 1, TaskClass: "implement", ModelID: "fake-model", MaxTurns: 6}
	if err := board.InsertAttempt(ctx, f.srv.DB, attempt); err != nil {
		t.Fatal(err)
	}
	if err := board.FinishAttempt(ctx, f.srv.DB, attempt.ID, "plan", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := board.InsertOpenPlan(ctx, f.srv.DB, f.card, attempt.ID, "Ship CSV", []board.PlanStep{
		{Text: "Find the handler"},
		{Text: "Write the export"},
	}); err != nil {
		t.Fatal(err)
	}
}

func planChatServer(t *testing.T, script string) (contextFixture, *fake.Fake) {
	t.Helper()
	f := newContextFixture(t)
	f.srv.Data = t.TempDir()
	if err := os.WriteFile(filepath.Join(f.srv.Data, "config.yaml"), []byte(refineRouter), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.srv.Data, "secrets.json"), []byte(`{"llmKeys":{"anthropic":"`+refineKey+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	provider.Usage = providers.Response{InputTokens: 100, OutputTokens: 40}
	f.srv.RefineProvider = provider
	return f, provider
}

func planChatStream(t *testing.T, f contextFixture, token string, body map[string]any) (*httptest.ResponseRecorder, []refineEvent) {
	t.Helper()
	path := "/api/projects/proj_default/cards/" + f.card + "/plan/chat"
	rec := postJSON(t, f.srv, path, token, body)
	if rec.Code != 200 {
		t.Fatalf("plan chat = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("headers = %v", rec.Header())
	}
	return rec, readEvents(t, rec.Body.Bytes())
}

func TestPlanChatStreams(t *testing.T) {
	f, provider := planChatServer(t, planChatReply)
	parkPlan(t, f)
	_, events := planChatStream(t, f, f.devToken, map[string]any{
		"message": "Tighten the steps.",
		"title":   "Ship CSV",
		"steps": []map[string]string{
			{"text": "Find the handler"},
			{"text": "Write the export"},
		},
	})
	if len(events) < 3 || events[0].Kind != "start" {
		t.Fatalf("events = %+v", events)
	}
	if text := deltaText(events); !strings.Contains(text, "Keep the first step") {
		t.Fatalf("deltas = %q", text)
	}
	done := events[len(events)-1]
	if done.Kind != "done" {
		t.Fatalf("done = %+v", done)
	}
	rev, _ := done.Data["revision"].(map[string]any)
	if rev == nil || rev["title"] != "Ship CSV" {
		t.Fatalf("revision = %+v", done.Data)
	}
	steps, _ := rev["steps"].([]any)
	if len(steps) != 2 {
		t.Fatalf("revision steps = %+v", steps)
	}
	prompts := provider.Prompts()
	if len(prompts) != 1 || prompts[0].TaskClass != "plan_chat" {
		t.Fatalf("prompts = %+v", prompts)
	}
}

func TestPlanChatFieldErrors(t *testing.T) {
	f, _ := planChatServer(t, planChatReply)
	parkPlan(t, f)
	path := "/api/projects/proj_default/cards/" + f.card + "/plan/chat"

	rec := postJSON(t, f.srv, path, f.devToken, map[string]any{
		"message": "ok",
		"title":   "",
		"steps":   []map[string]string{{"text": "One step"}},
	})
	if rec.Code != 422 {
		t.Fatalf("empty title = %d %s", rec.Code, rec.Body.String())
	}
	var errs fieldErrors
	if err := json.Unmarshal(rec.Body.Bytes(), &errs); err != nil || errs.Error.Fields["title"] == "" {
		t.Fatalf("fields = %+v %v", errs, err)
	}

	ready := newContextFixture(t)
	ready.srv.Data = f.srv.Data
	ready.srv.RefineProvider = f.srv.RefineProvider
	rec = postJSON(t, ready.srv, "/api/projects/proj_default/cards/"+ready.card+"/plan/chat", ready.devToken, map[string]any{
		"message": "ok",
		"title":   "Ship CSV",
		"steps":   []map[string]string{{"text": "One step"}},
	})
	if rec.Code != 409 {
		t.Fatalf("no open plan = %d %s", rec.Code, rec.Body.String())
	}
}
