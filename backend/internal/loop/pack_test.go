package loop

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runtime"
)

func TestPackPathMap(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"go.mod":                        "module x",
		"cmd/app/main.go":               "package main",
		"a/b/c/d.go":                    "package c",
		"a/b/c/deep/e.go":               "package deep",
		"a/b/c/deep/f.go":               "package deep",
		"node_modules/pkg/index.js":     "x",
		"web/node_modules/pkg/index.js": "x",
		".git/config":                   "x",
		".orchestrator/state":           "x",
	})
	got, err := pathMap(root)
	if err != nil {
		t.Fatal(err)
	}
	want := "a/b/c/d.go\na/b/c/deep/ (2 files)\ncmd/app/main.go\ngo.mod"
	if got != want {
		t.Fatalf("path map =\n%s\nwant\n%s", got, want)
	}

	linked := t.TempDir()
	writeTree(t, linked, map[string]string{".git": "gitdir: /elsewhere", "main.go": "package main"})
	if got, err := pathMap(linked); err != nil || got != "main.go" {
		t.Fatalf("worktree map = %q, %v; the .git file must not show", got, err)
	}
	if got, err := listFiles(linked, "", "", 0); err != nil || got != "main.go" {
		t.Fatalf("worktree list = %q, %v; the .git file must not show", got, err)
	}

	big := t.TempDir()
	files := map[string]string{}
	for i := 0; i < 400; i++ {
		files[fmt.Sprintf("pkg%03d/sub/file_with_a_long_name_%03d.go", i, i)] = "x"
	}
	writeTree(t, big, files)
	got, err = pathMap(big)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > pathMapCap || !strings.Contains(got, "pkg000/sub/ (1 files)") || strings.Contains(got, "long_name") {
		t.Fatalf("depth did not drop to fit: %d bytes\n%.200s", len(got), got)
	}

	huge := t.TempDir()
	files = map[string]string{}
	for i := 0; i < 900; i++ {
		files[fmt.Sprintf("top_level_file_with_a_long_name_%04d.go", i)] = "x"
	}
	writeTree(t, huge, files)
	got, err = pathMap(huge)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > pathMapCap || !strings.HasSuffix(got, "\ntruncated: true") {
		t.Fatalf("map not cut: %d bytes", len(got))
	}
}

func TestTodoWriteCap(t *testing.T) {
	items := func(n int, text, status string) map[string]any {
		var list []any
		for i := 0; i < n; i++ {
			list = append(list, map[string]any{"text": text, "status": status})
		}
		return map[string]any{"items": list}
	}
	if got, err := parseTodos(items(30, "step", "pending")); err != nil || len(got) != 30 {
		t.Fatalf("30 items = %d, %v", len(got), err)
	}
	for name, args := range map[string]map[string]any{
		"31 items":   items(31, "step", "pending"),
		"long text":  items(1, strings.Repeat("x", 201), "pending"),
		"empty text": items(1, "  ", "done"),
		"bad status": items(1, "step", "blocked"),
		"not array":  {"items": "step"},
	} {
		if _, err := parseTodos(args); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	s := &session{}
	if _, err := s.run(context.Background(), providers.ToolCall{Name: "todo_write", Args: items(31, "step", "pending")}); err == nil || len(s.todos) != 0 {
		t.Fatalf("over the cap replaced the list: %v %v", err, s.todos)
	}
}

func TestReadOnlyCallsConcurrent(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "alpha\n", "b.txt": "beta\n"})
	s := &session{wt: runtime.Worktree{Path: root}}

	const reads = 3
	var mu sync.Mutex
	arrived := 0
	all := make(chan struct{})
	timedOut := false
	readHook = func() {
		mu.Lock()
		arrived++
		if arrived == reads {
			close(all)
		}
		mu.Unlock()
		select {
		case <-all:
		case <-time.After(2 * time.Second):
			mu.Lock()
			timedOut = true
			mu.Unlock()
		}
	}
	defer func() { readHook = nil }()

	call := func(name string, args map[string]any) providers.ToolCall {
		return providers.ToolCall{Name: name, Args: args}
	}
	out := s.runCalls(context.Background(), []providers.ToolCall{
		call("read_file", map[string]any{"path": "a.txt"}),
		call("list_files", map[string]any{}),
		call("search_files", map[string]any{"text": "beta"}),
	})
	if timedOut {
		t.Fatal("read-only calls of one reply ran one at a time")
	}
	if len(out) != 3 || !strings.HasPrefix(out[0].summary, "1|alpha") || !strings.Contains(out[1].summary, "b.txt") || !strings.Contains(out[2].summary, "b.txt:1: beta") {
		t.Fatalf("results out of order: %+v", out)
	}

	readHook = nil
	out = s.runCalls(context.Background(), []providers.ToolCall{
		call("read_file", map[string]any{"path": "a.txt"}),
		call("replace_in_file", map[string]any{"path": "a.txt", "old": "alpha", "new": "omega"}),
		call("read_file", map[string]any{"path": "a.txt"}),
	})
	if !strings.HasPrefix(out[0].summary, "1|alpha") || !strings.HasPrefix(out[2].summary, "1|omega") {
		t.Fatalf("write did not run between the reads: %q, %q", out[0].summary, out[2].summary)
	}
}
