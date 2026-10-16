package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

// GivenAnswer is one entry of the answers route body (RFC-114).
type GivenAnswer struct {
	QuestionID string   `json:"questionId"`
	Selected   []string `json:"selected"`
	Text       string   `json:"text"`
}

var questionModes = []string{"manual", "auto"}

// CanAnswer reports whether the actor may answer a card's questions and set
// its question mode: the roles that may run a card, a tribe admin of the
// project's tribe, or a company admin (RFC-114, RFC-115).
func CanAnswer(ctx context.Context, db *sql.DB, actor User, projectID string) (bool, error) {
	rights, err := CardContextRightsFor(ctx, db, actor, projectID)
	return rights.Change, err
}

// AnswerQuestions answers every open question of a waiting_answer card, moves
// it to ready, and queues its next attempt, all in one transaction. With
// useSuggestions set each question takes its recommendation and answers is
// ignored.
func AnswerQuestions(ctx context.Context, db *sql.DB, actor User, projectID, cardID string, answers []GivenAnswer, useSuggestions bool) (board.Card, error) {
	ok, err := CanAnswer(ctx, db, actor, projectID)
	if err != nil {
		return board.Card{}, err
	}
	if !ok {
		return board.Card{}, identity.ErrForbidden
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	card, err := lockCard(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	if card.Status != "waiting_answer" {
		return card, board.ErrConflict
	}
	if _, err := board.OpenPlan(ctx, tx, projectID, cardID); err == nil {
		return card, board.ErrConflict
	} else if !errors.Is(err, board.ErrNotFound) {
		return card, err
	}
	open, err := board.OpenQuestions(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	checked, source, err := checkAnswers(open, answers, useSuggestions)
	if err != nil {
		return card, err
	}
	ids := make([]string, 0, len(open))
	for _, q := range open {
		if err := board.SaveAnswer(ctx, tx, projectID, cardID, q.ID, checked[q.ID], source, actor.ID); err != nil {
			return card, err
		}
		ids = append(ids, q.ID)
	}
	next, err := machine.Transition(ctx, tx, card, "ready", machine.Evidence{ActorID: actor.ID}, nil)
	if err != nil {
		return card, err
	}
	if _, err := insertCardRun(ctx, tx, projectID, cardID, "api"); err != nil {
		return card, err
	}
	if _, err := trace.Append(ctx, tx, trace.Event{
		CardID: card.ID, ProjectID: card.ProjectID, ActorID: actor.ID, Kind: "questions_answered",
		Payload: map[string]any{"questionIds": ids, "source": source},
	}, nil); err != nil {
		return card, err
	}
	return next, tx.Commit()
}

// checkAnswers matches each open question to exactly one entry and checks it.
// A bad entry is a field error on answers.{n}; a missing, repeated, or unknown
// question id is one on answers.
func checkAnswers(open []board.Question, answers []GivenAnswer, useSuggestions bool) (map[string]board.Answer, string, error) {
	out := make(map[string]board.Answer, len(open))
	if useSuggestions {
		for _, q := range open {
			out[q.ID] = q.Recommended
		}
		return out, "suggestion", nil
	}
	byID := make(map[string]board.Question, len(open))
	for _, q := range open {
		byID[q.ID] = q
	}
	fields := map[string]string{}
	seen := map[string]bool{}
	for i, given := range answers {
		q, ok := byID[given.QuestionID]
		if !ok || seen[given.QuestionID] {
			return nil, "", &settings.Invalid{Fields: map[string]string{"answers": "Answer each open question once."}}
		}
		seen[given.QuestionID] = true
		answer, err := loop.CheckAnswer(q, board.Answer{Selected: given.Selected, Text: given.Text})
		if err != nil {
			fields[fmt.Sprintf("answers.%d", i)] = sentence(err.Error())
			continue
		}
		out[q.ID] = answer
	}
	if len(seen) != len(open) {
		return nil, "", &settings.Invalid{Fields: map[string]string{"answers": "Answer each open question once."}}
	}
	if len(fields) > 0 {
		return nil, "", &settings.Invalid{Fields: fields}
	}
	return out, "person", nil
}

func sentence(msg string) string {
	r, size := utf8.DecodeRuneInString(msg)
	msg = string(unicode.ToUpper(r)) + msg[size:]
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return msg
}

// SetQuestionMode stores whether a person or the agent answers the card's
// questions (RFC-115). It writes no status. An attempt already running keeps
// the mode it started with.
func SetQuestionMode(ctx context.Context, db *sql.DB, actor User, projectID, cardID, mode string) (board.Card, error) {
	if !slices.Contains(questionModes, mode) {
		return board.Card{}, &settings.Invalid{Fields: map[string]string{"questionMode": "Choose manual or auto."}}
	}
	ok, err := CanAnswer(ctx, db, actor, projectID)
	if err != nil {
		return board.Card{}, err
	}
	if !ok {
		return board.Card{}, identity.ErrForbidden
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	card, err := lockCard(ctx, tx, projectID, cardID)
	if err != nil {
		return card, err
	}
	if card.Status == "done" || card.Status == "wont_fix" {
		return card, board.ErrConflict
	}
	old, err := board.SetQuestionMode(ctx, tx, projectID, cardID, mode)
	if err != nil {
		return card, err
	}
	if old != mode {
		if _, err := trace.Append(ctx, tx, trace.Event{
			CardID: card.ID, ProjectID: card.ProjectID, ActorID: actor.ID, Kind: "question_mode_changed",
			Payload: map[string]any{"from": old, "to": mode},
		}, nil); err != nil {
			return card, err
		}
	}
	if card, err = board.GetCard(ctx, tx, cardID); err != nil {
		return card, err
	}
	return card, tx.Commit()
}
