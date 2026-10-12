package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/llm"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

var (
	// ErrRefineRoute means the implement route has no usable model (RFC-086).
	ErrRefineRoute = errors.New("set a model for the implement route in Router settings to refine requirements")
	// ErrRefineProposal means the model's reply was not a valid proposal.
	ErrRefineProposal = errors.New("the model did not return a usable refinement")
	// ErrRefineProvider means the request failed on both models or timed out.
	ErrRefineProvider = errors.New("the refine request to the model failed")
)

const (
	refineTimeout   = 2 * time.Minute
	refineMaxTokens = 4096
	maxRefineNotes  = 2000
	maxRefineGuide  = 1000
)

// RefineInput is the text to refine. RequirementID is empty for a new one.
type RefineInput struct {
	RequirementID string
	Statement     string
	Acceptance    []string
	Instructions  string
}

// Refinement is the model's proposal. It is never saved here.
type Refinement struct {
	Statement  string   `json:"statement"`
	Acceptance []string `json:"acceptance"`
	Notes      string   `json:"notes"`
	ModelID    string   `json:"modelId"`
}

// RefineTools is the one tool a refinement request offers.
func RefineTools() []providers.Tool {
	return []providers.Tool{{
		Name:        contextpack.RefineTool,
		Description: "Return the refined requirement. Call it exactly once.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"statement": map[string]any{"type": "string", "description": "The refined statement, in markdown"},
				"acceptance": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
					"description": "One observable check per item, with no list marker"},
				"notes": map[string]any{"type": "string", "description": "What changed and why, and any open question"},
			},
			"required": []string{"statement", "acceptance", "notes"},
		},
	}}
}

// RefineRequirement asks the implement route's model to refine a draft
// (RFC-086). It writes no requirement, only a requirement_refined event
// without text. provider, when set, replaces the routed provider in tests.
func RefineRequirement(ctx context.Context, data string, db *sql.DB, projectID, actorID string, in RefineInput, provider providers.Provider) (Refinement, error) {
	draft, err := intake.Validate(intake.Draft{Statement: in.Statement, Acceptance: in.Acceptance})
	if err != nil {
		return Refinement{}, err
	}
	if utf8.RuneCountInString(in.Instructions) > maxRefineGuide {
		return Refinement{}, &intake.Invalid{Fields: map[string]string{
			"instructions": fmt.Sprintf("Keep what to improve to %d characters or fewer.", maxRefineGuide),
		}}
	}
	cfg, secrets, err := settings.Load(data)
	if err != nil {
		return Refinement{}, err
	}
	route := cfg.Router.Routes["implement"]
	if route.Model == "" {
		return Refinement{}, ErrRefineRoute
	}
	if provider == nil {
		if _, _, _, err := llm.ForModel(cfg, secrets, route.Model); err != nil {
			return Refinement{}, ErrRefineRoute
		}
		provider = RoutedProvider(cfg, secrets, route.Model, route.Fallback)
	}
	redact := modelKeys(secrets)
	ctx, cancel := context.WithTimeout(ctx, refineTimeout)
	defer cancel()
	resp, err := provider.Complete(ctx, providers.Request{
		Model:     route.Model,
		System:    contextpack.RefineSystem,
		User:      contextpack.RefineUser(in.RequirementID, draft.Statement, draft.Acceptance, in.Instructions),
		Tools:     RefineTools(),
		MaxTokens: refineMaxTokens,
		TaskClass: "refine_requirement",
	})
	if err != nil {
		return Refinement{}, fmt.Errorf("%w: %s", ErrRefineProvider, trace.Redact(err.Error(), redact))
	}
	modelID := resp.Model
	if modelID == "" {
		modelID = route.Model
	}
	requirementID := in.RequirementID
	if requirementID == "" {
		requirementID = "new"
	}
	_, _ = trace.Append(ctx, db, trace.Event{
		Kind: "requirement_refined", ActorID: actorID, ProjectID: projectID,
		Payload: map[string]any{
			"requirementId": requirementID, "modelId": modelID,
			"inputTokens": resp.InputTokens, "outputTokens": resp.OutputTokens, "cachedInputTokens": resp.CachedInputTokens,
		},
	}, redact)
	out, err := proposal(resp)
	if err != nil {
		return Refinement{}, err
	}
	out.Statement = trace.Redact(out.Statement, redact)
	for i, line := range out.Acceptance {
		out.Acceptance[i] = trace.Redact(line, redact)
	}
	out.Notes = trace.Redact(out.Notes, redact)
	out.ModelID = modelID
	return out, nil
}

// proposal reads the one propose_refinement call and checks it like a save.
func proposal(resp providers.Response) (Refinement, error) {
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != contextpack.RefineTool || resp.ToolCalls[0].BadArgs != "" {
		return Refinement{}, ErrRefineProposal
	}
	args := resp.ToolCalls[0].Args
	statement, _ := args["statement"].(string)
	notes, _ := args["notes"].(string)
	raw, ok := args["acceptance"].([]any)
	if !ok {
		return Refinement{}, ErrRefineProposal
	}
	lines := make([]string, 0, len(raw))
	for _, item := range raw {
		line, ok := item.(string)
		if !ok {
			return Refinement{}, ErrRefineProposal
		}
		lines = append(lines, line)
	}
	draft, err := intake.Validate(intake.Draft{Statement: statement, Acceptance: lines})
	if err != nil {
		var invalid *intake.Invalid
		if errors.As(err, &invalid) {
			var msgs []string
			for _, msg := range invalid.Fields {
				msgs = append(msgs, msg)
			}
			sort.Strings(msgs)
			return Refinement{}, fmt.Errorf("%w: %s", ErrRefineProposal, strings.Join(msgs, " "))
		}
		return Refinement{}, err
	}
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > maxRefineNotes {
		notes = string([]rune(notes)[:maxRefineNotes]) + "…"
	}
	return Refinement{Statement: draft.Statement, Acceptance: draft.Acceptance, Notes: notes}, nil
}
