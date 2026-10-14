package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Answer is a person's or the agent's answer to one question (RFC-114).
type Answer struct {
	Selected []string `json:"selected"`
	Text     string   `json:"text"`
}

// Question is one card_questions row. Answer is nil while the question is
// open. AnsweredBy is the user id, or "" for an auto answer.
type Question struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"projectId"`
	CardID       string   `json:"cardId"`
	AttemptID    string   `json:"attemptId"`
	AttemptN     int      `json:"attemptN"`
	Position     int      `json:"position"`
	Question     string   `json:"question"`
	Kind         string   `json:"kind"`
	Options      []string `json:"options"`
	AllowOther   bool     `json:"allowOther"`
	Recommended  Answer   `json:"recommended"`
	Why          string   `json:"why"`
	Answer       *Answer  `json:"answer"`
	AnswerSource string   `json:"answerSource"`
	AnsweredBy   string   `json:"answeredBy"`
	AnsweredAt   string   `json:"answeredAt"`
	CreatedAt    string   `json:"createdAt"`
}

// InsertQuestions stores an attempt's questions on the card, numbered from
// first. With auto set each is answered at once with its recommendation.
func InsertQuestions(ctx context.Context, q DBTX, cardID, attemptID string, first int, questions []Question, auto bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for i, question := range questions {
		id, err := NewID("qst_")
		if err != nil {
			return err
		}
		options := question.Options
		if options == nil {
			options = []string{}
		}
		optionsJSON, err := json.Marshal(options)
		if err != nil {
			return err
		}
		recommended, err := answerJSON(question.Recommended)
		if err != nil {
			return err
		}
		var answer, source, answeredAt any
		if auto {
			answer, source, answeredAt = recommended, "auto", now
		}
		if _, err := q.ExecContext(ctx, `
			INSERT INTO card_questions (
				id, project_id, card_id, attempt_id, position, question, kind, options_json, allow_other,
				recommended_json, why, answer_json, answer_source, answered_at, created_at
			) VALUES (
				$1, (SELECT project_id FROM cards WHERE id = $2), $2, $3, $4, $5, $6, $7, $8,
				$9, $10, $11, $12, $13, $14
			)`,
			id, cardID, attemptID, first+i, question.Question, question.Kind, string(optionsJSON), question.AllowOther,
			recommended, question.Why, answer, source, answeredAt, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func answerJSON(a Answer) (string, error) {
	if a.Selected == nil {
		a.Selected = []string{}
	}
	raw, err := json.Marshal(a)
	return string(raw), err
}

// CardQuestions lists the card's questions in the project: open ones first,
// then answered ones, each oldest first.
func CardQuestions(ctx context.Context, db *sql.DB, projectID, cardID string) ([]Question, error) {
	return queryQuestions(ctx, db, `
		SELECT q.id, q.project_id, q.card_id, q.attempt_id, COALESCE(a.n, 0), q.position, q.question, q.kind,
			q.options_json, q.allow_other, q.recommended_json, q.why, q.answer_json,
			COALESCE(q.answer_source, ''), COALESCE(q.answered_by, ''), COALESCE(q.answered_at, ''), q.created_at
		FROM card_questions q LEFT JOIN attempts a ON a.id = q.attempt_id
		WHERE q.card_id = $1 AND q.project_id = $2
		ORDER BY q.answer_json IS NOT NULL, q.created_at, a.n, q.position`, cardID, projectID)
}

// AnsweredQuestions lists the card's answered questions, oldest first, for
// the next attempt's pack.
func AnsweredQuestions(ctx context.Context, db *sql.DB, cardID string) ([]Question, error) {
	return queryQuestions(ctx, db, `
		SELECT q.id, q.project_id, q.card_id, q.attempt_id, COALESCE(a.n, 0), q.position, q.question, q.kind,
			q.options_json, q.allow_other, q.recommended_json, q.why, q.answer_json,
			COALESCE(q.answer_source, ''), COALESCE(q.answered_by, ''), COALESCE(q.answered_at, ''), q.created_at
		FROM card_questions q LEFT JOIN attempts a ON a.id = q.attempt_id
		WHERE q.card_id = $1 AND q.answer_json IS NOT NULL
		ORDER BY q.created_at, a.n, q.position`, cardID)
}

// OpenQuestions lists the card's unanswered questions, oldest first. q may be
// a transaction that holds the card row lock.
func OpenQuestions(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, projectID, cardID string) ([]Question, error) {
	return queryQuestions(ctx, q, `
		SELECT q.id, q.project_id, q.card_id, q.attempt_id, COALESCE(a.n, 0), q.position, q.question, q.kind,
			q.options_json, q.allow_other, q.recommended_json, q.why, q.answer_json,
			COALESCE(q.answer_source, ''), COALESCE(q.answered_by, ''), COALESCE(q.answered_at, ''), q.created_at
		FROM card_questions q LEFT JOIN attempts a ON a.id = q.attempt_id
		WHERE q.card_id = $1 AND q.project_id = $2 AND q.answer_json IS NULL
		ORDER BY q.created_at, a.n, q.position`, cardID, projectID)
}

func queryQuestions(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, query string, args ...any) ([]Question, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Question{}
	for rows.Next() {
		var question Question
		var optionsJSON, recommendedJSON string
		var answer sql.NullString
		if err := rows.Scan(&question.ID, &question.ProjectID, &question.CardID, &question.AttemptID, &question.AttemptN,
			&question.Position, &question.Question, &question.Kind, &optionsJSON, &question.AllowOther, &recommendedJSON,
			&question.Why, &answer, &question.AnswerSource, &question.AnsweredBy, &question.AnsweredAt, &question.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(optionsJSON), &question.Options); err != nil {
			return nil, fmt.Errorf("question %s options: %w", question.ID, err)
		}
		if err := json.Unmarshal([]byte(recommendedJSON), &question.Recommended); err != nil {
			return nil, fmt.Errorf("question %s recommendation: %w", question.ID, err)
		}
		if answer.Valid {
			var a Answer
			if err := json.Unmarshal([]byte(answer.String), &a); err != nil {
				return nil, fmt.Errorf("question %s answer: %w", question.ID, err)
			}
			question.Answer = &a
		}
		if question.Options == nil {
			question.Options = []string{}
		}
		if question.Recommended.Selected == nil {
			question.Recommended.Selected = []string{}
		}
		out = append(out, question)
	}
	return out, rows.Err()
}

// SaveAnswer answers one open question. Zero rows means it was answered or
// is not the card's, which is a conflict.
func SaveAnswer(ctx context.Context, q DBTX, projectID, cardID, questionID string, answer Answer, source, userID string) error {
	raw, err := answerJSON(answer)
	if err != nil {
		return err
	}
	var by any
	if userID != "" {
		by = userID
	}
	res, err := q.ExecContext(ctx, `
		UPDATE card_questions SET answer_json = $1, answer_source = $2, answered_by = $3, answered_at = $4
		WHERE id = $5 AND card_id = $6 AND project_id = $7 AND answer_json IS NULL`,
		raw, source, by, time.Now().UTC().Format(time.RFC3339), questionID, cardID, projectID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}

// QuestionRounds counts the card's attempts that ended to wait for answers.
func QuestionRounds(ctx context.Context, q DBTX, cardID string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `
		SELECT count(*) FROM attempts WHERE card_id = $1 AND outcome = 'question'`, cardID).Scan(&n)
	return n, err
}

// SetQuestionMode stores the card's answer mode and returns the old one.
// ErrNotFound means no card has that id in the project.
func SetQuestionMode(ctx context.Context, q DBTX, projectID, cardID, mode string) (string, error) {
	var old string
	err := q.QueryRowContext(ctx, `
		SELECT question_mode FROM cards WHERE id = $1 AND project_id = $2 FOR UPDATE`, cardID, projectID).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil || old == mode {
		return old, err
	}
	_, err = q.ExecContext(ctx, `
		UPDATE cards SET question_mode = $1, updated_at = $2 WHERE id = $3 AND project_id = $4`,
		mode, time.Now().UTC().Format(time.RFC3339), cardID, projectID)
	return old, err
}
