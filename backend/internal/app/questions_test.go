package app_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/providers/fake"
)

const askScript = `{"implement":[
	{"tool":"ask_user","questions":[
		{"question":"Which export format should the button offer?","kind":"single","options":["CSV","Excel"],
		 "recommended":{"selected":["CSV"]},"why":"The requirement says CSV; owner-token-value is not needed."},
		{"question":"Anything to add to the file name?","kind":"text","recommended":{"text":"The date"},"why":"Files sort by name."}]},
	{"tool":"edit_file","path":"hello.txt","text":"too late\n"}]}`

func (f jobFixture) runWith(t *testing.T, script string) (board.Card, *fake.Fake) {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	card, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return card, provider
}

func questionAnswerer(f jobFixture) app.User {
	return app.User{ID: f.card.OwnerID, Role: "developer"}
}

func openQuestions(t *testing.T, f jobFixture) []board.Question {
	t.Helper()
	open, err := board.OpenQuestions(context.Background(), f.db, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return open
}

func TestAskUserParksCard(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	card, provider := f.runWith(t, askScript)
	if card.Status != "waiting_answer" || card.AttemptsUsed != 0 {
		t.Fatalf("card = %s used %d, want waiting_answer used 0", card.Status, card.AttemptsUsed)
	}
	if prompts := provider.Prompts(); len(prompts) != 2 || prompts[1].ToolChoice != "none" {
		t.Fatalf("model calls = %d, want the ask and the handoff summary", len(prompts))
	}
	var outcome string
	if err := f.db.QueryRow(`SELECT outcome FROM attempts WHERE card_id = $1`, card.ID).Scan(&outcome); err != nil || outcome != "question" {
		t.Fatalf("outcome = %s, %v", outcome, err)
	}
	open := openQuestions(t, f)
	if len(open) != 2 || open[0].Kind != "single" || open[0].Recommended.Selected[0] != "CSV" || open[1].Position != 1 {
		t.Fatalf("open = %+v", open)
	}
	if strings.Contains(open[0].Why, ownerToken) {
		t.Fatalf("why kept the owner token: %q", open[0].Why)
	}
	if f.calls.has("archive") {
		t.Fatal("a parked attempt ran the gating check")
	}
	calls := payloads(t, f, "tool_call")
	last := calls[len(calls)-1]
	if last["name"] != "ask_user" || last["mode"] != "manual" || last["ok"] != true {
		t.Fatalf("tool_call = %v", last)
	}
}

func TestAskUserAutoAnswers(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	if _, err := f.db.Exec(`UPDATE cards SET question_mode = 'auto' WHERE id = $1`, f.card.ID); err != nil {
		t.Fatal(err)
	}
	ask := `{"tool":"ask_user","questions":[{"question":"Which format?","kind":"single","options":["CSV","Excel"],"recommended":{"selected":["CSV"]},"why":"CSV is named."}]}`
	card, provider := f.runWith(t, `{"implement":[
		{"calls":[`+ask+`,`+ask+`,`+ask+`,`+ask+`]},
		{"tool":"edit_file","path":"hello.txt","text":"auto\n"},
		{"tool":"commit","message":"Say auto"}]}`)
	if card.Status != "pr_open" || card.AttemptsUsed != 1 {
		t.Fatalf("card = %s used %d, want pr_open", card.Status, card.AttemptsUsed)
	}
	results := provider.Prompts()[1].Turns[0].Results
	if len(results) != 4 || !strings.HasPrefix(results[0], "No person answers on this card.") ||
		!strings.Contains(results[0], "1. Which format? -> CSV") || !strings.Contains(results[3], "error: ask_user: this attempt has asked three times") {
		t.Fatalf("results = %q", results)
	}
	all, err := board.CardQuestions(context.Background(), f.db, "proj_default", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("questions = %d, want 3", len(all))
	}
	for _, q := range all {
		if q.AnswerSource != "auto" || q.Answer == nil || q.Answer.Selected[0] != "CSV" || q.AnsweredBy != "" {
			t.Fatalf("question = %+v", q)
		}
	}
}

func TestQuestionAttemptNotCounted(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	if card, _ := f.runWith(t, askScript); card.Status != "waiting_answer" {
		t.Fatalf("status = %s", card.Status)
	}
	if _, err := app.AnswerQuestions(context.Background(), f.db, questionAnswerer(f), "proj_default", f.card.ID, nil, true); err != nil {
		t.Fatal(err)
	}
	card := f.run(t, commitScript)
	if card.Status != "pr_open" || card.AttemptsUsed != 1 {
		t.Fatalf("card = %s used %d, want pr_open used 1", card.Status, card.AttemptsUsed)
	}
	var n int
	if err := f.db.QueryRow(`SELECT MAX(n) FROM attempts WHERE card_id = $1`, card.ID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("attempt n = %d, %v; want 2", n, err)
	}
}

func TestPackIncludesAnswers(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	f.runWith(t, askScript)
	open := openQuestions(t, f)
	answers := []app.GivenAnswer{
		{QuestionID: open[0].ID, Selected: []string{"Excel"}},
		{QuestionID: open[1].ID, Text: "Add the order id"},
	}
	if _, err := app.AnswerQuestions(context.Background(), f.db, questionAnswerer(f), "proj_default", f.card.ID, answers, false); err != nil {
		t.Fatal(err)
	}
	if err := board.InsertQuestions(context.Background(), f.db, f.card.ID, open[0].AttemptID, 5, []board.Question{{
		Question: "Still open question?", Kind: "text", Recommended: board.Answer{Text: "no"},
	}}, false); err != nil {
		t.Fatal(err)
	}
	_, provider := f.runWith(t, commitScript)
	user := provider.Prompts()[0].User
	start, end := strings.Index(user, contextpack.MarkerStart), strings.LastIndex(user, contextpack.MarkerEnd)
	block := strings.Index(user, contextpack.AnswersHeading)
	if block < start || block > end || !strings.Contains(user, contextpack.AnswersEnd) {
		t.Fatalf("answers block outside the markers:\n%s", user)
	}
	for _, want := range []string{
		"1. question: Which export format should the button offer?\nanswer: Excel\nsource: person",
		"2. question: Anything to add to the file name?\nanswer: Add the order id\nsource: person",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("pack lacks %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "Still open question?") {
		t.Fatalf("pack holds an open question:\n%s", user)
	}
}
