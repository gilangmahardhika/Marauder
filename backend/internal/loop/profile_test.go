package loop_test

import (
	"strings"
	"testing"

	"orchestrator/backend/internal/loop"
)

func TestImplementerTools(t *testing.T) {
	got := map[string]bool{}
	for _, tool := range loop.ImplementerTools() {
		if tool.Name == "deploy" || tool.Name == "verdict" {
			t.Fatalf("profile includes %s", tool.Name)
		}
		got[tool.Name] = true
	}
	for _, want := range []string{"list_files", "search_files", "read_file", "read_linked_file", "edit_file", "run_local_check", "commit"} {
		if !got[want] {
			t.Fatalf("missing %s in %s", want, strings.Join(keys(got), ", "))
		}
	}
}

func TestReviewerTools(t *testing.T) {
	var got []string
	for _, tool := range loop.ReviewerTools() {
		got = append(got, tool.Name)
	}
	if strings.Join(got, ",") != "read_diff,comment_pr,verdict" {
		t.Fatalf("reviewer tools = %v", got)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}
