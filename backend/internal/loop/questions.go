package loop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/trace"
)

// Limits of ask_user and of an answer (RFC-114, RFC-115).
const (
	maxQuestions      = 5
	minOptions        = 2
	maxOptions        = 8
	maxQuestionLen    = 500
	maxOptionLen      = 120
	maxWhyLen         = 500
	MaxAnswerLen      = 2000
	maxAutoAsks       = 3
	MaxQuestionRounds = 3
)

var questionKinds = []string{"single", "multiple", "text"}

const (
	askAfterCommit = "ask_user is not available after commit"
	askRoundCap    = "ask_user: this card has asked three times; decide, and state your assumption in the commit message"
	askAutoCap     = "ask_user: this attempt has asked three times; decide, and state your assumption in the commit message"
	autoReply      = "No person answers on this card. Proceed with these answers:"
	parkedReply    = "Your questions are saved and the team is notified. This attempt ends now; the next attempt starts with the answers."
)

// ParseQuestions reads the questions argument of ask_user. Each
// recommendation must be a valid answer to its question.
func ParseQuestions(args map[string]any) ([]board.Question, error) {
	raw, ok := args["questions"].([]any)
	if !ok {
		return nil, errors.New("questions must be an array")
	}
	if len(raw) == 0 || len(raw) > maxQuestions {
		return nil, fmt.Errorf("ask 1 to %d questions", maxQuestions)
	}
	out := make([]board.Question, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("question %d must be an object", i+1)
		}
		q, err := parseQuestion(obj)
		if err != nil {
			return nil, fmt.Errorf("question %d: %w", i+1, err)
		}
		out = append(out, q)
	}
	return out, nil
}

func parseQuestion(obj map[string]any) (board.Question, error) {
	q := board.Question{
		Question:   strings.TrimSpace(strArg(obj, "question")),
		Kind:       strArg(obj, "kind"),
		AllowOther: boolArg(obj, "allowOther"),
		Why:        strings.TrimSpace(strArg(obj, "why")),
		Options:    []string{},
	}
	if q.Question == "" || utf8.RuneCountInString(q.Question) > maxQuestionLen {
		return q, fmt.Errorf("question must be 1 to %d characters", maxQuestionLen)
	}
	if !slices.Contains(questionKinds, q.Kind) {
		return q, errors.New("kind must be single, multiple, or text")
	}
	if utf8.RuneCountInString(q.Why) > maxWhyLen {
		return q, fmt.Errorf("why must be at most %d characters", maxWhyLen)
	}
	options, err := questionStrings(obj["options"], "options")
	if err != nil {
		return q, err
	}
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" || utf8.RuneCountInString(option) > maxOptionLen {
			return q, fmt.Errorf("each option must be 1 to %d characters", maxOptionLen)
		}
		if slices.Contains(q.Options, option) {
			return q, fmt.Errorf("option %q is repeated", option)
		}
		q.Options = append(q.Options, option)
	}
	if q.Kind == "text" {
		if len(q.Options) > 0 {
			return q, errors.New("a text question has no options")
		}
	} else if len(q.Options) < minOptions || len(q.Options) > maxOptions {
		return q, fmt.Errorf("a %s question needs %d to %d options", q.Kind, minOptions, maxOptions)
	}
	rec, _ := obj["recommended"].(map[string]any)
	selected, err := questionStrings(rec["selected"], "recommended.selected")
	if err != nil {
		return q, err
	}
	answer, err := CheckAnswer(q, board.Answer{Selected: selected, Text: strArg(rec, "text")})
	if err != nil {
		return q, fmt.Errorf("recommended: %w", err)
	}
	q.Recommended = answer
	return q, nil
}

func questionStrings(v any, name string) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", name)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings", name)
		}
		out = append(out, s)
	}
	return out, nil
}

// CheckAnswer checks a against q and returns it trimmed, with CRLF as LF and
// the options in the question's order. The answers route and ask_user's
// recommendation both use it.
func CheckAnswer(q board.Question, a board.Answer) (board.Answer, error) {
	text := strings.TrimSpace(strings.ReplaceAll(a.Text, "\r\n", "\n"))
	if utf8.RuneCountInString(text) > MaxAnswerLen {
		return board.Answer{}, fmt.Errorf("keep the answer to %d characters or fewer", MaxAnswerLen)
	}
	picked := map[string]bool{}
	for _, s := range a.Selected {
		s = strings.TrimSpace(s)
		if !slices.Contains(q.Options, s) {
			return board.Answer{}, fmt.Errorf("%q is not one of the options", s)
		}
		if picked[s] {
			return board.Answer{}, fmt.Errorf("%q is chosen twice", s)
		}
		picked[s] = true
	}
	selected := []string{}
	for _, option := range q.Options {
		if picked[option] {
			selected = append(selected, option)
		}
	}
	if len(selected) == 0 && text == "" {
		return board.Answer{}, errors.New("answer this question")
	}
	switch q.Kind {
	case "single":
		if len(selected) > 1 {
			return board.Answer{}, errors.New("choose one option")
		}
		if text != "" && !q.AllowOther {
			return board.Answer{}, errors.New("choose one of the options")
		}
		if text != "" && len(selected) > 0 {
			return board.Answer{}, errors.New("choose an option or write your own answer, not both")
		}
	case "multiple":
		if text != "" && !q.AllowOther {
			return board.Answer{}, errors.New("choose from the options")
		}
	case "text":
		if len(selected) > 0 || text == "" {
			return board.Answer{}, errors.New("write an answer")
		}
	}
	return board.Answer{Selected: selected, Text: text}, nil
}

// AnswerText is an answer as one line of text: the chosen options, then the
// written answer.
func AnswerText(a board.Answer) string {
	parts := append([]string(nil), a.Selected...)
	if a.Text != "" {
		parts = append(parts, a.Text)
	}
	return strings.Join(parts, "; ")
}

// askUser runs ask_user (RFC-114). On a manual card it stores the questions
// open and stops the attempt; on an auto card it stores them answered with
// their recommendations and returns those.
func (s *session) askUser(ctx context.Context, args map[string]any) (string, error) {
	if s.committed {
		return "", errors.New(askAfterCommit)
	}
	auto := s.in.QuestionMode == "auto"
	if auto && s.asks >= maxAutoAsks {
		return "", errors.New(askAutoCap)
	}
	if !auto && s.in.QuestionRounds >= MaxQuestionRounds {
		return "", errors.New(askRoundCap)
	}
	questions, err := ParseQuestions(args)
	if err != nil {
		return "", fmt.Errorf("ask_user: %w", err)
	}
	for i := range questions {
		questions[i] = redactQuestion(questions[i], s.in.Secrets)
	}
	if err := s.in.board().AskQuestions(ctx, s.attemptID, s.card.ID, s.asked, questions, auto); err != nil {
		return "", err
	}
	s.asks++
	s.asked += len(questions)
	shown := make([]map[string]any, 0, len(questions))
	for _, q := range questions {
		item := map[string]any{
			"question": q.Question, "kind": q.Kind, "options": q.Options, "allowOther": q.AllowOther,
			"recommended": AnswerText(q.Recommended), "why": q.Why,
		}
		if auto {
			item["answer"] = AnswerText(q.Recommended)
		}
		shown = append(shown, item)
	}
	mode := "manual"
	if auto {
		mode = "auto"
	}
	s.askEvent = map[string]any{"questions": shown, "mode": mode}
	if !auto {
		s.stop = "question"
		return parkedReply, nil
	}
	lines := []string{autoReply}
	for i, q := range questions {
		lines = append(lines, fmt.Sprintf("%d. %s -> %s", i+1, q.Question, AnswerText(q.Recommended)))
	}
	return strings.Join(lines, "\n"), nil
}

func redactQuestion(q board.Question, secrets []string) board.Question {
	r := func(s string) string { return trace.Redact(s, secrets) }
	q.Question = r(q.Question)
	q.Why = r(q.Why)
	options := make([]string, len(q.Options))
	for i, o := range q.Options {
		options[i] = r(o)
	}
	q.Options = options
	selected := make([]string, len(q.Recommended.Selected))
	for i, o := range q.Recommended.Selected {
		selected[i] = r(o)
	}
	q.Recommended = board.Answer{Selected: selected, Text: r(q.Recommended.Text)}
	return q
}
