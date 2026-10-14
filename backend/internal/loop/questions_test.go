package loop

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/providers"
)

// askBoard records AskQuestions. Any other board call panics.
type askBoard struct {
	Board
	asked [][]board.Question
	auto  []bool
	first []int
}

func (b *askBoard) AskQuestions(_ context.Context, _, _ string, first int, questions []board.Question, auto bool) error {
	b.asked = append(b.asked, questions)
	b.auto = append(b.auto, auto)
	b.first = append(b.first, first)
	return nil
}

func askSession(t *testing.T, mode string, rounds int) (*session, *askBoard) {
	t.Helper()
	s := commandSession(t, nil, refuseExec(t))
	b := &askBoard{}
	s.in.Board = b
	s.in.QuestionMode = mode
	s.in.QuestionRounds = rounds
	s.in.Secrets = []string{"sk-secret-value"}
	s.card = board.Card{ID: "card_ask"}
	s.tools = ImplementerTools()
	s.allowed = map[string]providers.Tool{}
	for _, tool := range s.tools {
		s.allowed[tool.Name] = tool
	}
	return s, b
}

func formatQuestion() map[string]any {
	return map[string]any{
		"question": "Which export format should the button offer?", "kind": "single",
		"options": []any{"CSV", "Excel"}, "allowOther": false,
		"recommended": map[string]any{"selected": []any{"CSV"}, "text": ""},
		"why":         "The requirement says CSV.",
	}
}

func askCall(questions ...map[string]any) providers.ToolCall {
	list := make([]any, len(questions))
	for i, q := range questions {
		list[i] = q
	}
	return providers.ToolCall{Name: "ask_user", Args: map[string]any{"questions": list}}
}

func TestAskUserValidation(t *testing.T) {
	with := func(key string, value any) map[string]any {
		q := formatQuestion()
		q[key] = value
		return q
	}
	options := func(n int) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = strings.Repeat("o", i+1)
		}
		return out
	}
	six := []map[string]any{formatQuestion(), formatQuestion(), formatQuestion(), formatQuestion(), formatQuestion(), formatQuestion()}
	cases := map[string]struct {
		questions []map[string]any
		want      string
	}{
		"none":           {nil, "ask 1 to 5 questions"},
		"six":            {six, "ask 1 to 5 questions"},
		"blank question": {[]map[string]any{with("question", "  ")}, "question must be 1 to 500 characters"},
		"long question":  {[]map[string]any{with("question", strings.Repeat("q", 501))}, "question must be 1 to 500 characters"},
		"kind":           {[]map[string]any{with("kind", "rating")}, "kind must be single, multiple, or text"},
		"one option":     {[]map[string]any{with("options", []any{"CSV"})}, "needs 2 to 8 options"},
		"nine options":   {[]map[string]any{with("options", options(9))}, "needs 2 to 8 options"},
		"long option":    {[]map[string]any{with("options", []any{"CSV", strings.Repeat("x", 121)})}, "each option must be 1 to 120 characters"},
		"repeated":       {[]map[string]any{with("options", []any{"CSV", "CSV"})}, "is repeated"},
		"text options":   {[]map[string]any{with("kind", "text")}, "a text question has no options"},
		"long why":       {[]map[string]any{with("why", strings.Repeat("w", 501))}, "why must be at most 500 characters"},
		"no suggestion":  {[]map[string]any{with("recommended", map[string]any{})}, "recommended: answer this question"},
		"bad suggestion": {[]map[string]any{with("recommended", map[string]any{"selected": []any{"PDF"}})}, `recommended: "PDF" is not one of the options`},
		"two singles":    {[]map[string]any{with("recommended", map[string]any{"selected": []any{"CSV", "Excel"}})}, "recommended: choose one option"},
		"other off":      {[]map[string]any{with("recommended", map[string]any{"text": "PDF"})}, "recommended: choose one of the options"},
	}
	for name, tc := range cases {
		s, b := askSession(t, "manual", 0)
		out, _, _ := s.dispatch(context.Background(), askCall(tc.questions...))
		if !strings.HasPrefix(out, providers.ErrorPrefix) || !strings.Contains(out, tc.want) {
			t.Errorf("%s: result = %q, want %q", name, out, tc.want)
		}
		if len(b.asked) != 0 || s.stop != "" {
			t.Errorf("%s: stored %d asks, stop %q", name, len(b.asked), s.stop)
		}
	}

	answers := []struct {
		q    board.Question
		a    board.Answer
		want string
	}{
		{board.Question{Kind: "single", Options: []string{"A", "B"}, AllowOther: true}, board.Answer{Selected: []string{"A"}, Text: "C"}, "not both"},
		{board.Question{Kind: "multiple", Options: []string{"A", "B"}}, board.Answer{Selected: []string{"A", "A"}}, "chosen twice"},
		{board.Question{Kind: "multiple", Options: []string{"A", "B"}}, board.Answer{Text: "C"}, "choose from the options"},
		{board.Question{Kind: "text", Options: []string{}}, board.Answer{Text: " \r\n "}, "answer this question"},
		{board.Question{Kind: "text", Options: []string{}}, board.Answer{Text: strings.Repeat("a", MaxAnswerLen+1)}, "2000 characters"},
	}
	for _, tc := range answers {
		if _, err := CheckAnswer(tc.q, tc.a); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("CheckAnswer(%v, %v) = %v, want %q", tc.q, tc.a, err, tc.want)
		}
	}
	got, err := CheckAnswer(board.Question{Kind: "multiple", Options: []string{"A", "B", "C"}, AllowOther: true},
		board.Answer{Selected: []string{"C", " A "}, Text: " also D\r\n "})
	if err != nil || strings.Join(got.Selected, ",") != "A,C" || got.Text != "also D" {
		t.Fatalf("multiple = %+v, %v; want options in question order and trimmed text", got, err)
	}

	s, b := askSession(t, "manual", 0)
	secret := formatQuestion()
	secret["why"] = "The key sk-secret-value is in the env."
	if out := call(t, s, askCall(secret, map[string]any{
		"question": "Anything else?", "kind": "text", "recommended": map[string]any{"text": "No"},
	})); out != parkedReply {
		t.Fatalf("result = %q", out)
	}
	if len(b.asked) != 1 || len(b.asked[0]) != 2 || strings.Contains(b.asked[0][0].Why, "sk-secret-value") {
		t.Fatalf("stored = %+v, want two redacted questions", b.asked)
	}
}

func TestAskUserAfterCommitRejected(t *testing.T) {
	s, b := askSession(t, "manual", 0)
	s.committed = true
	out, _, _ := s.dispatch(context.Background(), askCall(formatQuestion()))
	if out != providers.ErrorPrefix+askAfterCommit {
		t.Fatalf("result = %q", out)
	}
	if len(b.asked) != 0 || s.stop != "" {
		t.Fatalf("stored %d asks, stop %q", len(b.asked), s.stop)
	}
}

func TestQuestionRoundCap(t *testing.T) {
	s, b := askSession(t, "manual", MaxQuestionRounds)
	out, _, _ := s.dispatch(context.Background(), askCall(formatQuestion()))
	if out != providers.ErrorPrefix+askRoundCap || len(b.asked) != 0 || s.stop != "" {
		t.Fatalf("fourth round = %q, stored %d, stop %q", out, len(b.asked), s.stop)
	}

	s, b = askSession(t, "auto", MaxQuestionRounds)
	for i := 0; i < maxAutoAsks; i++ {
		out := call(t, s, askCall(formatQuestion(), formatQuestion()))
		if !strings.HasPrefix(out, autoReply) || !strings.Contains(out, "1. Which export format should the button offer? -> CSV") {
			t.Fatalf("auto ask %d = %q", i+1, out)
		}
	}
	out, _, _ = s.dispatch(context.Background(), askCall(formatQuestion()))
	if out != providers.ErrorPrefix+askAutoCap {
		t.Fatalf("fourth auto ask = %q", out)
	}
	if len(b.asked) != maxAutoAsks || b.first[1] != 2 || b.first[2] != 4 || !b.auto[0] || s.stop != "" {
		t.Fatalf("auto asks = %d first %v auto %v stop %q", len(b.asked), b.first, b.auto, s.stop)
	}
}
