package loop

import (
	"strings"
	"testing"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/trace"
)

func TestToolArgsEditSnippets(t *testing.T) {
	secret := "super-secret-token"
	call := providers.ToolCall{
		Name: "replace_in_file",
		Args: map[string]any{
			"path": "a.ts",
			"old":  "before " + secret,
			"new":  "after " + secret,
		},
	}
	args := toolArgs(call, "", []string{secret}, "")
	if args["old"] != "before [redacted]" || args["new"] != "after [redacted]" {
		t.Fatalf("args = %v", args)
	}
	big := strings.Repeat("x", trace.DiffCap+100)
	edit := toolArgs(providers.ToolCall{
		Name: "edit_file",
		Args: map[string]any{"path": "b.ts", "text": big},
	}, "", nil, "old-"+big)
	newText, _ := edit["new"].(string)
	oldText, _ := edit["old"].(string)
	if len(newText) > trace.DiffCap+40 || !strings.Contains(newText, "bytes cut") {
		t.Fatalf("new not clipped: %d %q", len(newText), newText[:min(80, len(newText))])
	}
	if len(oldText) > trace.DiffCap+40 || !strings.Contains(oldText, "bytes cut") {
		t.Fatalf("old not clipped: %d", len(oldText))
	}
}
