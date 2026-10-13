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

const cardNoteLimit = 10

// cardDetail is the single-card body: the board fields plus the card's
// requirements, the rules its packs carry, its newest failure notes, and its
// context (RFC-105). The board list leaves these out.
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
	if view, err := app.CardContextFor(r.Context(), s.DB, card); err == nil {
		body["context"] = view
	}
	return body
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
