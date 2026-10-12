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
	// ErrRefineProposal means the model's finished text fails the save rules.
	ErrRefineProposal = errors.New("the model did not return a usable refinement")
	// ErrRefineProvider means the request failed on both models or timed out.
	ErrRefineProvider = errors.New("the refine request to the model failed")
)

const (
	refineTimeout   = 2 * time.Minute
	refineMaxTokens = 4096
	maxRefineGuide  = 1000
)

// RefineInput is the draft and the field to refine. RequirementID is empty for
// a new requirement.
type RefineInput struct {
	Field         string
	RequirementID string
	Statement     string
	Acceptance    []string
	Instructions  string
}

// RefineDone is the finished text of one field. It is never saved here.
// Acceptance holds the cleaned lines when Field is acceptance.
type RefineDone struct {
	Text       string   `json:"text"`
	Acceptance []string `json:"acceptance,omitempty"`
	ModelID    string   `json:"modelId"`
}

// modelStreamer is a provider that also names the model that answers, right
// before its first text.
type modelStreamer interface {
	streamFrom(ctx context.Context, req providers.Request, onStart func(model string), onText func(string)) (providers.Response, error)
}

// RefineField asks the implement route's model to rewrite one field of a draft
// and streams the text as it arrives (RFC-087). emit gets "start" with the
// modelId before the first text, a "delta" per piece of redacted text, then
// "done" with a RefineDone. Errors return before any emit for bad input or a
// missing route; a provider error, a cancel, or text that fails the save rules
// returns after. It writes no requirement, only a requirement_refined event
// without text. provider, when set, replaces the routed provider in tests.
func RefineField(ctx context.Context, data string, db *sql.DB, projectID, actorID string, in RefineInput, provider providers.Provider, emit func(kind string, payload any)) error {
	if in.Field != "statement" && in.Field != "acceptance" {
		return &intake.Invalid{Fields: map[string]string{"field": "Choose statement or acceptance to refine."}}
	}
	draft, err := intake.Validate(intake.Draft{Statement: in.Statement, Acceptance: in.Acceptance})
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(in.Instructions) > maxRefineGuide {
		return &intake.Invalid{Fields: map[string]string{
			"instructions": fmt.Sprintf("Keep what to improve to %d characters or fewer.", maxRefineGuide),
		}}
	}
	cfg, secrets, err := settings.Load(data)
	if err != nil {
		return err
	}
	route := cfg.Router.Routes["implement"]
	if route.Model == "" {
		return ErrRefineRoute
	}
	if provider == nil {
		if _, _, _, err := llm.ForModel(cfg, secrets, route.Model); err != nil {
			return ErrRefineRoute
		}
		provider = RoutedProvider(cfg, secrets, route.Model, route.Fallback)
	}
	system := contextpack.RefineStatementSystem
	if in.Field == "acceptance" {
		system = contextpack.RefineAcceptanceSystem
	}
	req := providers.Request{
		Model:     route.Model,
		System:    system,
		User:      contextpack.RefineUser(in.Field, in.RequirementID, draft.Statement, draft.Acceptance, in.Instructions),
		MaxTokens: refineMaxTokens,
		TaskClass: "refine_requirement",
	}

	redact := modelKeys(secrets)
	modelID := route.Model
	started := false
	start := func(model string) {
		if !started {
			started = true
			modelID = model
			emit("start", map[string]string{"modelId": model})
		}
	}
	out := trace.NewStreamRedactor(redact, func(text string) {
		start(modelID)
		emit("delta", map[string]string{"text": text})
	})
	ctx, cancel := context.WithTimeout(ctx, refineTimeout)
	defer cancel()
	var resp providers.Response
	switch p := provider.(type) {
	case modelStreamer:
		resp, err = p.streamFrom(ctx, req, start, out.Write)
	case providers.Streamer:
		resp, err = p.Stream(ctx, req, func(text string) {
			start(modelID)
			out.Write(text)
		})
	default:
		resp, err = p.Complete(ctx, req)
		if err == nil {
			out.Write(resp.Text)
		}
	}
	if err != nil {
		if parent := context.Cause(ctx); errors.Is(parent, context.Canceled) {
			return parent
		}
		return fmt.Errorf("%w: %s", ErrRefineProvider, trace.Redact(err.Error(), redact))
	}
	out.Flush()
	if resp.Model != "" {
		modelID = resp.Model
	}
	start(modelID)

	requirementID := in.RequirementID
	if requirementID == "" {
		requirementID = "new"
	}
	_, _ = trace.Append(ctx, db, trace.Event{
		Kind: "requirement_refined", ActorID: actorID, ProjectID: projectID,
		Payload: map[string]any{
			"requirementId": requirementID, "field": in.Field, "modelId": modelID,
			"inputTokens": resp.InputTokens, "outputTokens": resp.OutputTokens, "cachedInputTokens": resp.CachedInputTokens,
		},
	}, redact)

	done, err := finished(in.Field, draft, trace.Redact(resp.Text, redact))
	if err != nil {
		return err
	}
	done.ModelID = modelID
	emit("done", done)
	return nil
}

// finished checks the model's whole text for field with the save rules.
func finished(field string, draft intake.Draft, text string) (RefineDone, error) {
	check := intake.Draft{Statement: text, Acceptance: draft.Acceptance}
	if field == "acceptance" {
		var lines []string
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			return RefineDone{}, fmt.Errorf("%w: the reply has no acceptance lines", ErrRefineProposal)
		}
		check = intake.Draft{Statement: draft.Statement, Acceptance: lines}
	}
	clean, err := intake.Validate(check)
	if err != nil {
		var invalid *intake.Invalid
		if errors.As(err, &invalid) {
			var msgs []string
			for _, msg := range invalid.Fields {
				msgs = append(msgs, msg)
			}
			sort.Strings(msgs)
			return RefineDone{}, fmt.Errorf("%w: %s", ErrRefineProposal, strings.Join(msgs, " "))
		}
		return RefineDone{}, err
	}
	if field == "acceptance" {
		return RefineDone{Text: strings.Join(clean.Acceptance, "\n"), Acceptance: clean.Acceptance}, nil
	}
	return RefineDone{Text: clean.Statement}, nil
}
