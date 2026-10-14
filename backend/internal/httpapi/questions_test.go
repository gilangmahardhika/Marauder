package httpapi_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"orchestrator/backend/internal/board"
)

type questionCard struct {
	Status       string   `json:"status"`
	Actions      []string `json:"actions"`
	Queued       bool     `json:"queued"`
	QuestionMode string   `json:"questionMode"`
	Questions    []struct {
		ID           string        `json:"id"`
		Kind         string        `json:"kind"`
		Answer       *board.Answer `json:"answer"`
		AnswerSource *string       `json:"answerSource"`
		AnsweredBy   *struct {
			Email string `json:"email"`
		} `json:"answeredBy"`
	} `json:"questions"`
}

type fieldErrors struct {
	Error struct {
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

// parkCard puts the fixture card in waiting_answer with a single, a multiple
// with Other, and a text question, and returns their ids in that order.
func parkCard(t *testing.T, f contextFixture) []string {
	t.Helper()
	ctx := context.Background()
	if _, err := f.srv.DB.Exec(`UPDATE cards SET status = 'waiting_answer' WHERE id = $1`, f.card); err != nil {
		t.Fatal(err)
	}
	attempt := board.Attempt{ID: "att_" + f.card[5:], CardID: f.card, N: 1, TaskClass: "implement", ModelID: "fake-model", MaxTurns: 6}
	if err := board.InsertAttempt(ctx, f.srv.DB, attempt); err != nil {
		t.Fatal(err)
	}
	if err := board.FinishAttempt(ctx, f.srv.DB, attempt.ID, "question", ""); err != nil {
		t.Fatal(err)
	}
	if err := board.InsertQuestions(ctx, f.srv.DB, f.card, attempt.ID, 0, []board.Question{
		{Question: "Which format?", Kind: "single", Options: []string{"CSV", "Excel"}, Recommended: board.Answer{Selected: []string{"CSV"}}},
		{Question: "Which columns?", Kind: "multiple", Options: []string{"Id", "Total", "Date"}, AllowOther: true, Recommended: board.Answer{Selected: []string{"Id", "Total"}}},
		{Question: "File name?", Kind: "text", Options: []string{}, Recommended: board.Answer{Text: "orders.csv"}},
	}, false); err != nil {
		t.Fatal(err)
	}
	open, err := board.OpenQuestions(ctx, f.srv.DB, "proj_default", f.card)
	if err != nil || len(open) != 3 {
		t.Fatalf("open = %d, %v", len(open), err)
	}
	return []string{open[0].ID, open[1].ID, open[2].ID}
}

func (f contextFixture) answersPath() string {
	return "/api/projects/proj_default/cards/" + f.card + "/answers"
}

func decodeCard(t *testing.T, raw []byte) questionCard {
	t.Helper()
	var body questionCard
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func cardStatus(t *testing.T, f contextFixture) string {
	t.Helper()
	var status string
	if err := f.srv.DB.QueryRow(`SELECT status FROM cards WHERE id = $1`, f.card).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestAnswerQueuesRun(t *testing.T) {
	f := newContextFixture(t)
	ids := parkCard(t, f)
	before := decodeCard(t, getAuth(t, f.srv, "/api/projects/proj_default/cards/"+f.card, f.devToken).Body.Bytes())
	if before.Status != "waiting_answer" || !slices.Contains(before.Actions, "answer") || len(before.Questions) != 3 || before.Questions[0].Answer != nil {
		t.Fatalf("before = %+v", before)
	}
	rec := postJSON(t, f.srv, f.answersPath(), f.devToken, map[string]any{"answers": []map[string]any{
		{"questionId": ids[2], "text": " orders\r\nexport "},
		{"questionId": ids[0], "selected": []string{"Excel"}},
		{"questionId": ids[1], "selected": []string{"Date", "Id"}, "text": "Customer"},
	}})
	if rec.Code != 200 {
		t.Fatalf("answers = %d %s", rec.Code, rec.Body.String())
	}
	body := decodeCard(t, rec.Body.Bytes())
	if body.Status != "ready" || !body.Queued || slices.Contains(body.Actions, "answer") {
		t.Fatalf("card = %+v", body)
	}
	for _, q := range body.Questions {
		if q.Answer == nil || q.AnswerSource == nil || *q.AnswerSource != "person" || q.AnsweredBy == nil || q.AnsweredBy.Email != "dev@example.com" {
			t.Fatalf("question = %+v", q)
		}
	}
	answers := map[string]board.Answer{}
	for _, q := range body.Questions {
		answers[q.ID] = *q.Answer
	}
	if answers[ids[2]].Text != "orders\nexport" || answers[ids[1]].Selected[0] != "Id" || answers[ids[1]].Text != "Customer" {
		t.Fatalf("answers = %+v", answers)
	}
	var runs, events int
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM runs WHERE card_id = $1 AND processed_at IS NULL`, f.card).Scan(&runs); err != nil || runs != 1 {
		t.Fatalf("open runs = %d, %v", runs, err)
	}
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'questions_answered'`, f.card).Scan(&events); err != nil || events != 1 {
		t.Fatalf("questions_answered = %d, %v", events, err)
	}
	if again := postJSON(t, f.srv, f.answersPath(), f.devToken, map[string]any{"useSuggestions": true}); again.Code != 409 {
		t.Fatalf("second answer = %d", again.Code)
	}
}

func TestAnswerValidation(t *testing.T) {
	f := newContextFixture(t)
	ids := parkCard(t, f)
	good := func() []map[string]any {
		return []map[string]any{
			{"questionId": ids[0], "selected": []string{"CSV"}},
			{"questionId": ids[1], "selected": []string{"Id"}},
			{"questionId": ids[2], "text": "orders.csv"},
		}
	}
	cases := map[string]struct {
		mutate func([]map[string]any) []map[string]any
		field  string
	}{
		"missing":     {func(a []map[string]any) []map[string]any { return a[:2] }, "answers"},
		"unknown":     {func(a []map[string]any) []map[string]any { a[0]["questionId"] = "qst_000000000000"; return a }, "answers"},
		"repeated":    {func(a []map[string]any) []map[string]any { return append(a[:2], a[0]) }, "answers"},
		"two singles": {func(a []map[string]any) []map[string]any { a[0]["selected"] = []string{"CSV", "Excel"}; return a }, "answers.0"},
		"single text": {func(a []map[string]any) []map[string]any {
			a[0]["selected"] = []string{}
			a[0]["text"] = "PDF"
			return a
		}, "answers.0"},
		"not listed":     {func(a []map[string]any) []map[string]any { a[1]["selected"] = []string{"Weight"}; return a }, "answers.1"},
		"empty text":     {func(a []map[string]any) []map[string]any { a[2]["text"] = "   "; return a }, "answers.2"},
		"text an option": {func(a []map[string]any) []map[string]any { a[2]["selected"] = []string{"CSV"}; return a }, "answers.2"},
	}
	for name, tc := range cases {
		rec := postJSON(t, f.srv, f.answersPath(), f.devToken, map[string]any{"answers": tc.mutate(good())})
		var body fieldErrors
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != 422 || body.Error.Fields[tc.field] == "" {
			t.Errorf("%s = %d %s, want 422 on %s", name, rec.Code, rec.Body.String(), tc.field)
		}
	}
	if status := cardStatus(t, f); status != "waiting_answer" {
		t.Fatalf("status = %s after bad answers", status)
	}
	if rec := postJSON(t, f.srv, f.answersPath(), f.poToken, map[string]any{"answers": good()}); rec.Code != 403 {
		t.Fatalf("product owner = %d", rec.Code)
	}
	if actions := cardActions(t, f, f.poToken); slices.Contains(actions, "answer") || slices.Contains(actions, "question_mode") {
		t.Fatalf("product owner actions = %v", actions)
	}
	var answered int
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM card_questions WHERE card_id = $1 AND answer_json IS NOT NULL`, f.card).Scan(&answered); err != nil || answered != 0 {
		t.Fatalf("answered = %d, %v", answered, err)
	}
	if _, err := f.srv.DB.Exec(`UPDATE cards SET status = 'ready' WHERE id = $1`, f.card); err != nil {
		t.Fatal(err)
	}
	if rec := postJSON(t, f.srv, f.answersPath(), f.devToken, map[string]any{"answers": good()}); rec.Code != 409 {
		t.Fatalf("ready card = %d", rec.Code)
	}
}

func TestAnswerSuggestions(t *testing.T) {
	f := newContextFixture(t)
	parkCard(t, f)
	rec := postJSON(t, f.srv, f.answersPath(), f.chiefToken, map[string]any{"useSuggestions": true})
	if rec.Code != 200 {
		t.Fatalf("suggestions = %d %s", rec.Code, rec.Body.String())
	}
	body := decodeCard(t, rec.Body.Bytes())
	if body.Status != "ready" || !body.Queued {
		t.Fatalf("card = %+v", body)
	}
	want := map[string]string{"single": "CSV", "multiple": "Id,Total", "text": "orders.csv"}
	for _, q := range body.Questions {
		got := q.Answer.Text
		if q.Kind != "text" {
			got = ""
			for i, s := range q.Answer.Selected {
				if i > 0 {
					got += ","
				}
				got += s
			}
		}
		if got != want[q.Kind] || q.AnswerSource == nil || *q.AnswerSource != "suggestion" || q.AnsweredBy.Email != "chief@example.com" {
			t.Fatalf("question %s = %+v", q.Kind, q)
		}
	}
	var source string
	if err := f.srv.DB.QueryRow(`SELECT payload_json::json->>'source' FROM events WHERE card_id = $1 AND kind = 'questions_answered'`, f.card).Scan(&source); err != nil || source != "suggestion" {
		t.Fatalf("event source = %s, %v", source, err)
	}
}

func TestQuestionModeRights(t *testing.T) {
	f := newContextFixture(t)
	path := "/api/projects/proj_default/cards/" + f.card + "/question-mode"
	before := decodeCard(t, getAuth(t, f.srv, "/api/projects/proj_default/cards/"+f.card, f.devToken).Body.Bytes())
	if before.QuestionMode != "manual" {
		t.Fatalf("default mode = %q", before.QuestionMode)
	}
	rec := putJSON(t, f.srv, path, f.devToken, map[string]string{"questionMode": "auto"})
	if rec.Code != 200 || decodeCard(t, rec.Body.Bytes()).QuestionMode != "auto" {
		t.Fatalf("developer = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, f.srv, path, f.devToken, map[string]string{"questionMode": "auto"}); rec.Code != 200 {
		t.Fatalf("unchanged = %d", rec.Code)
	}
	if rec := putJSON(t, f.srv, path, f.chiefToken, map[string]string{"questionMode": "manual"}); rec.Code != 200 {
		t.Fatalf("tribe admin = %d", rec.Code)
	}
	if rec := putJSON(t, f.srv, path, f.poToken, map[string]string{"questionMode": "auto"}); rec.Code != 403 {
		t.Fatalf("product owner = %d", rec.Code)
	}
	rec = putJSON(t, f.srv, path, f.devToken, map[string]string{"questionMode": "sometimes"})
	var errs fieldErrors
	_ = json.Unmarshal(rec.Body.Bytes(), &errs)
	if rec.Code != 422 || errs.Error.Fields["questionMode"] == "" {
		t.Fatalf("bad mode = %d %s", rec.Code, rec.Body.String())
	}
	var events int
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'question_mode_changed'`, f.card).Scan(&events); err != nil || events != 2 {
		t.Fatalf("question_mode_changed = %d, %v; want 2", events, err)
	}
	for token, want := range map[string]bool{f.devToken: true, f.leadToken: true, f.chiefToken: true, f.poToken: false} {
		if got := slices.Contains(cardActions(t, f, token), "question_mode"); got != want {
			t.Fatalf("question_mode listed = %v, want %v", got, want)
		}
	}
	if _, err := f.srv.DB.Exec(`UPDATE cards SET status = 'done' WHERE id = $1`, f.card); err != nil {
		t.Fatal(err)
	}
	if rec := putJSON(t, f.srv, path, f.devToken, map[string]string{"questionMode": "auto"}); rec.Code != 409 {
		t.Fatalf("done card = %d", rec.Code)
	}
	if slices.Contains(cardActions(t, f, f.leadToken), "question_mode") {
		t.Fatal("done card lists question_mode")
	}
}
