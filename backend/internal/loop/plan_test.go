package loop

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/providers"
)

type planBoard struct {
	Board
	title string
	steps []board.PlanStep
}

func (b *planBoard) SubmitPlan(_ context.Context, _, _ string, title string, steps []board.PlanStep) error {
	b.title, b.steps = title, steps
	return nil
}

func planSession(t *testing.T, planning bool) (*session, *planBoard) {
	t.Helper()
	s := commandSession(t, nil, refuseExec(t))
	b := &planBoard{}
	s.in.Board = b
	s.in.Planning = planning
	s.card = board.Card{ID: "card_plan"}
	s.tools = ImplementerTools()
	s.allowed = map[string]providers.Tool{}
	for _, tool := range s.tools {
		s.allowed[tool.Name] = tool
	}
	return s, b
}

func planCall(title string, steps ...map[string]any) providers.ToolCall {
	list := make([]any, len(steps))
	for i, step := range steps {
		list[i] = step
	}
	return providers.ToolCall{Name: "submit_plan", Args: map[string]any{"title": title, "steps": list}}
}

func TestPlanStepValidation(t *testing.T) {
	s, b := planSession(t, true)
	out, _, _ := s.dispatch(context.Background(), planCall("Title"))
	if !strings.Contains(out, "plan needs 1 to 30 steps") || b.title != "" {
		t.Fatalf("empty steps = %q stored %q", out, b.title)
	}
	out, _, _ = s.dispatch(context.Background(), planCall(strings.Repeat("t", 201), map[string]any{"text": "Do it"}))
	if !strings.Contains(out, "title must be 1 to 200") {
		t.Fatalf("long title = %q", out)
	}
	out, _, _ = s.dispatch(context.Background(), planCall("Ok", map[string]any{"text": strings.Repeat("x", 501)}))
	if !strings.Contains(out, "text must be 1 to 500") {
		t.Fatalf("long step = %q", out)
	}
}

func TestWriteToolsBlockedUntilPlanAccepted(t *testing.T) {
	s, _ := planSession(t, true)
	out, _, _ := s.dispatch(context.Background(), providers.ToolCall{
		Name: "edit_file", Args: map[string]any{"path": "a.txt", "text": "hi"},
	})
	if !strings.Contains(out, planPhaseBlock) {
		t.Fatalf("edit during planning = %q", out)
	}
	out, _, _ = s.dispatch(context.Background(), providers.ToolCall{
		Name: "commit", Args: map[string]any{"message": "done"},
	})
	if !strings.Contains(out, planPhaseBlock) {
		t.Fatalf("commit during planning = %q", out)
	}
}

func TestSubmitPlanParksInSession(t *testing.T) {
	s, b := planSession(t, true)
	out, _, _ := s.dispatch(context.Background(), planCall("Ship it", map[string]any{"text": "Write the export", "detail": "CSV"}))
	if !strings.Contains(out, "plan is saved") || s.stop != "plan" {
		t.Fatalf("submit = %q stop %q", out, s.stop)
	}
	if b.title != "Ship it" || len(b.steps) != 1 || b.steps[0].Text != "Write the export" {
		t.Fatalf("stored = %+v", b)
	}
}
