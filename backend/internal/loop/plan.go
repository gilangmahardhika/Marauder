package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/trace"
)

var planWriteTools = map[string]bool{
	"replace_in_file": true, "edit_file": true, "delete_file": true, "move_file": true,
	"run_local_check": true, "run_command": true, "commit": true,
}

// planningBlocked returns an error when a planning-phase attempt may not run
// the named tool.
func (s *session) planningBlocked(name string) error {
	if !s.in.Planning || !planWriteTools[name] {
		return nil
	}
	return errors.New(planPhaseBlock)
}

// submitPlan stores an open plan and parks the attempt (RFC-128).
func (s *session) submitPlan(ctx context.Context, args map[string]any) (string, error) {
	if s.committed {
		return "", errors.New(planAfterCommit)
	}
	if !s.in.Planning {
		return "", errors.New("submit_plan: this card already has an accepted plan or plan mode is off")
	}
	title, steps, err := ParsePlanArgs(args)
	if err != nil {
		return "", fmt.Errorf("submit_plan: %w", err)
	}
	title = trace.Redact(title, s.in.Secrets)
	for i := range steps {
		steps[i].Text = trace.Redact(steps[i].Text, s.in.Secrets)
		steps[i].Detail = trace.Redact(steps[i].Detail, s.in.Secrets)
	}
	if err := s.in.board().SubmitPlan(ctx, s.attemptID, s.card.ID, title, steps); err != nil {
		return "", err
	}
	shown := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		shown = append(shown, map[string]any{"text": step.Text, "detail": step.Detail})
	}
	s.askEvent = map[string]any{"title": title, "steps": shown}
	s.stop = "plan"
	return planParkedReply, nil
}

// Limits of submit_plan (RFC-128, RFC-129).
const (
	maxPlanTitle  = 200
	minPlanSteps  = 1
	maxPlanSteps  = 30
	maxStepText   = 500
	maxStepDetail = 1000
)

const (
	planAfterCommit = "submit_plan is not available after commit"
	planPhaseBlock  = "planning phase: submit a plan before editing"
	planParkedReply = "Your plan is saved and the team is notified. This attempt ends now; the next attempt starts after they accept the plan."
)

// ParsePlanArgs reads submit_plan's title and steps.
func ParsePlanArgs(args map[string]any) (string, []board.PlanStep, error) {
	title := strings.TrimSpace(strArg(args, "title"))
	raw, ok := args["steps"].([]any)
	if !ok {
		return "", nil, errors.New("steps must be an array")
	}
	steps := make([]board.PlanStep, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("step %d must be an object", i+1)
		}
		steps = append(steps, board.PlanStep{
			ID:     strings.TrimSpace(strArg(obj, "id")),
			Text:   strings.TrimSpace(strArg(obj, "text")),
			Detail: strings.TrimSpace(strArg(obj, "detail")),
		})
	}
	return NormalizePlan(title, steps)
}

// NormalizePlan checks and trims a plan's title and steps.
func NormalizePlan(title string, steps []board.PlanStep) (string, []board.PlanStep, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > maxPlanTitle {
		return "", nil, fmt.Errorf("title must be 1 to %d characters", maxPlanTitle)
	}
	if len(steps) < minPlanSteps || len(steps) > maxPlanSteps {
		return "", nil, fmt.Errorf("plan needs %d to %d steps", minPlanSteps, maxPlanSteps)
	}
	out := make([]board.PlanStep, 0, len(steps))
	for i, step := range steps {
		text := strings.TrimSpace(step.Text)
		detail := strings.TrimSpace(step.Detail)
		if text == "" || utf8.RuneCountInString(text) > maxStepText {
			return "", nil, fmt.Errorf("step %d text must be 1 to %d characters", i+1, maxStepText)
		}
		if utf8.RuneCountInString(detail) > maxStepDetail {
			return "", nil, fmt.Errorf("step %d detail must be at most %d characters", i+1, maxStepDetail)
		}
		out = append(out, board.PlanStep{ID: strings.TrimSpace(step.ID), Text: text, Detail: detail})
	}
	return title, out, nil
}
