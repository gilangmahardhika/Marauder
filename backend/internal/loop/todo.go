package loop

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"orchestrator/backend/internal/providers"
)

const (
	maxTodos    = 30
	maxTodoText = 200
	// MaxTodos and MaxTodoText are the todo_write limits, which a runner's
	// handoff is held to (RFC-118).
	MaxTodos    = maxTodos
	MaxTodoText = maxTodoText
)

var todoStatuses = []string{"pending", "in_progress", "done"}

type todoItem struct {
	Text, Status string
}

// parseTodos reads todo_write's items (RFC-082): up to maxTodos, each with
// text of 1 to maxTodoText bytes and a known status.
func parseTodos(args map[string]any) ([]todoItem, error) {
	raw, ok := args["items"].([]any)
	if !ok {
		return nil, errors.New("items must be an array of {text, status}")
	}
	if len(raw) > maxTodos {
		return nil, fmt.Errorf("the list has %d items; the most is %d", len(raw), maxTodos)
	}
	items := make([]todoItem, 0, len(raw))
	for i, entry := range raw {
		obj, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("item %d is not an object with text and status", i+1)
		}
		text := strings.TrimSpace(strArg(obj, "text"))
		status := strArg(obj, "status")
		switch {
		case text == "":
			return nil, fmt.Errorf("item %d has no text", i+1)
		case len(text) > maxTodoText:
			return nil, fmt.Errorf("item %d text is %d bytes; the most is %d", i+1, len(text), maxTodoText)
		case !slices.Contains(todoStatuses, status):
			return nil, fmt.Errorf("item %d status %q is not pending, in_progress, or done", i+1, status)
		}
		items = append(items, todoItem{Text: text, Status: status})
	}
	return items, nil
}

func todoSummary(items []todoItem) string {
	done := 0
	for _, item := range items {
		if item.Status == "done" {
			done++
		}
	}
	return fmt.Sprintf("todo list: %d items, %d done", len(items), done)
}

func renderTodos(items []todoItem) string {
	var b strings.Builder
	b.WriteString("Your todo list:")
	for _, item := range items {
		fmt.Fprintf(&b, "\n[%s] %s", item.Status, item.Text)
	}
	return b.String()
}

// withTodos puts the current todo list in the newest turn's note, outside the
// history budget. turns must come from history, which copies each turn.
func withTodos(turns []providers.Turn, items []todoItem) []providers.Turn {
	if len(turns) == 0 || len(items) == 0 {
		return turns
	}
	last := &turns[len(turns)-1]
	last.Note = joinNote(last.Note, renderTodos(items))
	return turns
}
