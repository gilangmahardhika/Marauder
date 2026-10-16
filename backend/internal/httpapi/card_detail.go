package httpapi

import (
	"context"
	"database/sql"
	"net/http"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/conventions"
)

type cardRequirement struct {
	ID         string `json:"id"`
	Heading    string `json:"heading"`
	Statement  string `json:"statement"`
	SourceKind string `json:"sourceKind"`
	SourcePath string `json:"sourcePath"`
	Accepted   bool   `json:"accepted"`
}

type cardRule struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

type cardNote struct {
	Source    string `json:"source"`
	Note      string `json:"note"`
	CreatedAt string `json:"createdAt"`
	AttemptN  int    `json:"attemptN"`
}

// cardPullRequest is the pull request text the card last published (RFC-110).
type cardPullRequest struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	AttemptN int    `json:"attemptN"`
	SHA      string `json:"sha"`
}

const cardNoteLimit = 10

// cardDetail is the single-card body: the board fields plus the card's
// requirements, the rules its packs carry, its newest failure notes, its
// context (RFC-105), and its published pull request text (RFC-110). The board
// list leaves these out.
func (s *Server) cardDetail(r *http.Request, card board.Card) map[string]any {
	body := s.cardBody(r, card)
	reqs, err := cardRequirements(r.Context(), s.DB, card.ProjectID, card.RequirementIDs)
	if err != nil {
		reqs = []cardRequirement{}
	}
	body["requirements"] = reqs
	rules := []cardRule{}
	if items, err := conventions.ForCard(r.Context(), s.DB, card.ProjectID, card.ID); err == nil {
		for _, it := range items {
			if it.Kind == "rule" {
				rules = append(rules, cardRule{ID: it.ID, Title: it.Title, Label: it.Label, Category: it.Category})
			}
		}
	}
	body["rules"] = rules
	notes := []cardNote{}
	if items, err := board.CardNotes(r.Context(), s.DB, card.ProjectID, card.ID, cardNoteLimit); err == nil {
		for _, n := range items {
			notes = append(notes, cardNote{Source: n.Source, Note: n.Note, CreatedAt: n.CreatedAt, AttemptN: n.AttemptN})
		}
	}
	body["notes"] = notes
	var instructions any
	if text, err := board.CurrentInstructions(r.Context(), s.DB, card.ID); err == nil && text != "" {
		instructions = text
	}
	body["instructions"] = instructions
	var pr any
	if text, err := board.LatestPRText(r.Context(), s.DB, card.ProjectID, card.ID); err == nil && text.Title != "" {
		pr = cardPullRequest{Title: text.Title, Body: app.PRBodyForDisplay(text.Body), AttemptN: text.AttemptN, SHA: text.CommitSHA}
	}
	body["pullRequest"] = pr
	if view, err := app.CardContextFor(r.Context(), s.DB, card); err == nil {
		body["context"] = view
	}
	questions, err := cardQuestions(r.Context(), s.DB, card.ProjectID, card.ID)
	if err != nil {
		questions = []map[string]any{}
	}
	body["questions"] = questions
	if plan, err := board.CardPlanForDisplay(r.Context(), s.DB, card.ProjectID, card.ID); err == nil {
		var acceptedBy any
		if plan.AcceptedBy != "" {
			if found, ok, err := app.OwnerOf(r.Context(), s.DB, plan.AcceptedBy); err == nil && ok {
				acceptedBy = map[string]any{"id": found.ID, "name": found.Name, "email": found.Email}
			}
		}
		steps := make([]map[string]any, 0, len(plan.Steps))
		for _, step := range plan.Steps {
			steps = append(steps, map[string]any{"id": step.ID, "text": step.Text, "detail": step.Detail})
		}
		body["plan"] = map[string]any{
			"id": plan.ID, "attemptN": plan.AttemptN, "title": plan.Title, "steps": steps,
			"status": plan.Status, "acceptedBy": acceptedBy, "acceptedAt": nullString(plan.AcceptedAt),
			"createdAt": plan.CreatedAt,
		}
	} else {
		body["plan"] = nil
	}
	return body
}

// cardQuestions is the card's questions as the card page shows them, with the
// answering person as {id, name, email} (RFC-114).
func cardQuestions(ctx context.Context, db *sql.DB, projectID, cardID string) ([]map[string]any, error) {
	items, err := board.CardQuestions(ctx, db, projectID, cardID)
	if err != nil {
		return nil, err
	}
	people := map[string]any{}
	out := make([]map[string]any, 0, len(items))
	for _, q := range items {
		var answer, source, answeredBy, answeredAt any
		if q.Answer != nil {
			selected := q.Answer.Selected
			if selected == nil {
				selected = []string{}
			}
			answer = board.Answer{Selected: selected, Text: q.Answer.Text}
			source, answeredAt = q.AnswerSource, q.AnsweredAt
		}
		if q.AnsweredBy != "" {
			person, ok := people[q.AnsweredBy]
			if !ok {
				if found, ok, err := app.OwnerOf(ctx, db, q.AnsweredBy); err == nil && ok {
					person = map[string]any{"id": found.ID, "name": found.Name, "email": found.Email}
				}
				people[q.AnsweredBy] = person
			}
			answeredBy = person
		}
		out = append(out, map[string]any{
			"id": q.ID, "attemptN": q.AttemptN, "question": q.Question, "kind": q.Kind, "options": q.Options,
			"allowOther": q.AllowOther, "recommended": q.Recommended, "why": q.Why, "answer": answer,
			"answerSource": source, "answeredBy": answeredBy, "answeredAt": answeredAt, "createdAt": q.CreatedAt,
		})
	}
	return out, nil
}

// cardRequirements reads the named requirements of one project in the card's
// order. An id with no row in that project is skipped.
func cardRequirements(ctx context.Context, db *sql.DB, projectID string, ids []string) ([]cardRequirement, error) {
	out := []cardRequirement{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, heading, statement, source_kind, source_path, COALESCE(accepted_at, '') <> ''
		FROM requirements WHERE project_id = $1 AND id = ANY($2)`, projectID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]cardRequirement{}
	for rows.Next() {
		var req cardRequirement
		if err := rows.Scan(&req.ID, &req.Heading, &req.Statement, &req.SourceKind, &req.SourcePath, &req.Accepted); err != nil {
			return nil, err
		}
		found[req.ID] = req
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if req, ok := found[id]; ok {
			out = append(out, req)
		}
	}
	return out, nil
}
