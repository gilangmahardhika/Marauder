package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/llm"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

const (
	planChatMaxTokens = 4096
	planChatTimeout   = 2 * time.Minute
	planChatGuideCap  = 2000
)

// PlanChatInput is the body of POST .../plan/chat (RFC-130).
type PlanChatInput struct {
	Message  string
	Title    string
	Steps    []board.PlanStep
	Model    string
	Thinking string
	Effort   string
}

// PlanChat streams a plan Q&A or revision. emit gets start, delta, done (with
// optional revision), or the caller maps errors before the stream starts.
func PlanChat(ctx context.Context, data string, db *sql.DB, actor User, projectID, cardID string, in PlanChatInput, provider providers.Provider, emit func(kind string, payload any)) error {
	ok, err := CanAnswer(ctx, db, actor, projectID)
	if err != nil {
		return err
	}
	if !ok {
		return identity.ErrForbidden
	}
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return err
	}
	if card.ProjectID != projectID || card.Status != "waiting_answer" {
		return board.ErrConflict
	}
	if _, err := board.OpenPlan(ctx, db, projectID, cardID); err != nil {
		if errors.Is(err, board.ErrNotFound) {
			return board.ErrConflict
		}
		return err
	}
	title, steps, err := loop.NormalizePlan(in.Title, in.Steps)
	if err != nil {
		return planFieldErr(err)
	}
	msg := strings.TrimSpace(in.Message)
	if utf8.RuneCountInString(msg) > planChatGuideCap {
		return &settings.Invalid{Fields: map[string]string{
			"message": fmt.Sprintf("Keep the message to %d characters or fewer.", planChatGuideCap),
		}}
	}
	if msg == "" {
		msg = "Review this plan and suggest clearer steps if needed."
	}
	cfg, secrets, err := settings.Load(data)
	if err != nil {
		return err
	}
	route := cfg.Router.Routes["plan"]
	model := route.Model
	if in.Model != "" {
		model = in.Model
	}
	if model == "" {
		return ErrPlanRoute
	}
	pinned := model != route.Model || in.Thinking != "" || in.Effort != ""
	var opts settings.ModelOptions
	if pinned {
		if opts, err = refineOptions(cfg, model, in.Thinking, in.Effort); err != nil {
			return err
		}
	}
	if provider == nil {
		if _, _, _, err := llm.ForModel(cfg, secrets, model); err != nil {
			if pinned {
				return &settings.Invalid{Fields: map[string]string{"model": "This model's LLM family has no usable key."}}
			}
			return ErrPlanRoute
		}
		if pinned {
			provider = PinnedProvider(cfg, secrets, model, opts)
		} else {
			provider = RoutedProvider(cfg, secrets, route.Model, route.Fallback)
		}
	}
	var stepLines strings.Builder
	for i, step := range steps {
		fmt.Fprintf(&stepLines, "%d. %s", i+1, step.Text)
		if step.Detail != "" {
			fmt.Fprintf(&stepLines, " (%s)", step.Detail)
		}
		stepLines.WriteByte('\n')
	}
	system := "You help a person review an execution plan for a coding card. " +
		"Answer questions clearly. When they ask to change the plan, end your reply with a JSON object alone on the last lines: " +
		`{"title":"...","steps":[{"text":"...","detail":"..."}]}` +
		". Otherwise answer in prose only. Do not invent tools or status changes."
	user := fmt.Sprintf("Plan title: %s\nSteps:\n%s\nPerson: %s", title, stepLines.String(), msg)
	req := providers.Request{
		Model: model, System: system, User: user, MaxTokens: planChatMaxTokens, TaskClass: "plan_chat", ToolChoice: "none",
	}
	if pinned {
		req = llm.WithOptions(req, opts)
	}
	redact := modelKeys(secrets)
	modelID := model
	started := false
	start := func(m string) {
		if !started {
			started = true
			modelID = m
			emit("start", map[string]string{"modelId": modelID})
		}
	}
	ctx, cancel := context.WithTimeout(ctx, planChatTimeout)
	defer cancel()
	var full strings.Builder
	out := trace.NewStreamRedactor(redact, func(text string) {
		start(modelID)
		full.WriteString(text)
		emit("delta", map[string]string{"text": text})
	})
	resp, err := refineSend(ctx, provider, req, start, out, &modelID)
	if err != nil {
		if parent := context.Cause(ctx); errors.Is(parent, context.Canceled) {
			return parent
		}
		return fmt.Errorf("%w: %s", ErrRefineProvider, trace.Redact(err.Error(), redact))
	}
	_ = resp
	out.Flush()
	done := map[string]any{"modelId": modelID}
	if rev, ok := parsePlanRevision(full.String()); ok {
		done["revision"] = rev
	}
	emit("done", done)
	return nil
}

func parsePlanRevision(text string) (map[string]any, bool) {
	text = strings.TrimSpace(text)
	for start := strings.LastIndex(text, "{"); start >= 0; start = strings.LastIndex(text[:start], "{") {
		raw := text[start:]
		var parsed struct {
			Title string `json:"title"`
			Steps []struct {
				Text   string `json:"text"`
				Detail string `json:"detail"`
			} `json:"steps"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil || parsed.Title == "" || len(parsed.Steps) == 0 {
			continue
		}
		steps := make([]board.PlanStep, 0, len(parsed.Steps))
		for _, s := range parsed.Steps {
			steps = append(steps, board.PlanStep{Text: s.Text, Detail: s.Detail})
		}
		title, steps, err := loop.NormalizePlan(parsed.Title, steps)
		if err != nil {
			continue
		}
		outSteps := make([]map[string]string, 0, len(steps))
		for _, s := range steps {
			outSteps = append(outSteps, map[string]string{"text": s.Text, "detail": s.Detail})
		}
		return map[string]any{"title": title, "steps": outSteps}, true
	}
	return nil, false
}
