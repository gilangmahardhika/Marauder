package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runtime"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readBack(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestReadFileRange(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 2500; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	writeTree(t, root, map[string]string{"big.txt": b.String(), "empty.txt": ""})

	got, err := readFile(root, "big.txt", 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := "10|line 10\n11|line 11\n12|line 12\nlines 10-12 of 2500\nmore: read from line 13"
	if got != want {
		t.Fatalf("range = %q", got)
	}
	whole, err := readFile(root, "big.txt", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(whole, "1|line 1\n") || !strings.Contains(whole, "lines 1-2000 of 2500\nmore: read from line 2001") {
		t.Fatalf("default read ends %q", whole[len(whole)-60:])
	}
	tail, err := readFile(root, "big.txt", 2499, 10)
	if err != nil || !strings.HasSuffix(tail, "lines 2499-2500 of 2500") {
		t.Fatalf("tail = %q, %v", tail, err)
	}
	if _, err := readFile(root, "big.txt", 2501, 1); err == nil || !strings.Contains(err.Error(), "2500 lines") {
		t.Fatalf("offset past the end: %v", err)
	}
	if empty, err := readFile(root, "empty.txt", 0, 0); err != nil || !strings.Contains(empty, "of 0") {
		t.Fatalf("empty = %q, %v", empty, err)
	}
	if _, err := readFile(root, "../etc/passwd", 0, 0); err == nil {
		t.Fatal("read outside the worktree")
	}
}

func TestReplaceInFileUnique(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.go": "x := 1\ny := 1\nx := 1\n"})
	if err := os.Chmod(filepath.Join(root, "a.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := replaceInFile(root, "a.go", "x := 1", "x := 2", false); err == nil || !strings.Contains(err.Error(), "occurs 2 times") {
		t.Fatalf("two matches: %v", err)
	}
	if readBack(t, root, "a.go") != "x := 1\ny := 1\nx := 1\n" {
		t.Fatal("a refused replace changed the file")
	}
	got, err := replaceInFile(root, "a.go", "y := 1", "y := 3", false)
	if err != nil || got != "replaced 1 occurrence in a.go" {
		t.Fatalf("unique = %q, %v", got, err)
	}
	got, err = replaceInFile(root, "a.go", "x := 1", "x := 2", true)
	if err != nil || got != "replaced 2 occurrences in a.go" {
		t.Fatalf("all = %q, %v", got, err)
	}
	if body := readBack(t, root, "a.go"); body != "x := 2\ny := 3\nx := 2\n" {
		t.Fatalf("file = %q", body)
	}
	info, err := os.Stat(filepath.Join(root, "a.go"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, %v", info.Mode(), err)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 1 {
		t.Fatalf("left temp files: %v", entries)
	}
}

func TestReplaceInFileNotFound(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.go": "package a\n"})
	cases := []struct{ old, new, want string }{
		{"package b", "package c", "not found"},
		{"", "x", "old is empty"},
		{"package a", "package a", "the same"},
		{"package a", placeholder1, "placeholder"},
	}
	for _, c := range cases {
		if _, err := replaceInFile(root, "a.go", c.old, c.new, false); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("replace %q: %v", c.old, err)
		}
	}
	if _, err := replaceInFile(root, "missing.go", "a", "b", false); err == nil {
		t.Fatal("replaced in a missing file")
	}
	if readBack(t, root, "a.go") != "package a\n" {
		t.Fatal("file changed")
	}
}

func TestEditRefusesPlaceholder(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{".github/workflows/lint.yml": "name: lint\n"})
	for _, text := range []string{placeholder1, "jobs:\n" + placeholder2 + "\n", ""} {
		if err := editFile(root, ".github/workflows/lint.yml", text); err == nil {
			t.Fatalf("wrote %q", text)
		}
	}
	if readBack(t, root, ".github/workflows/lint.yml") != "name: lint\n" {
		t.Fatal("refused write changed the file")
	}
	if err := editFile(root, "new/empty.txt", ""); err != nil {
		t.Fatalf("empty new file: %v", err)
	}
	if err := editFile(root, ".github/workflows/lint.yml", "name: check\n"); err != nil || readBack(t, root, ".github/workflows/lint.yml") != "name: check\n" {
		t.Fatalf("write: %v", err)
	}
}

func TestHistoryDropsEditText(t *testing.T) {
	text := "line one\nline two\n"
	s := &session{turns: []providers.Turn{{
		Calls:   []providers.ToolCall{{ID: "c1", Name: "edit_file", Args: map[string]any{"path": "a.txt", "text": text}}},
		Results: []string{"wrote a.txt"},
	}}}
	args := s.history()[0].Calls[0].Args
	if _, ok := args["text"]; ok || args["bytes"] != len(text) || args["lines"] != 2 || args["path"] != "a.txt" {
		t.Fatalf("args = %v", args)
	}
	for _, v := range args {
		if s, ok := v.(string); ok && hasPlaceholder(s) {
			t.Fatalf("placeholder in args: %v", args)
		}
	}
	if s.turns[0].Calls[0].Args["text"] != text {
		t.Fatal("history changed the stored turn")
	}
}

func TestSearchRegexGlobContext(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"src/a.go":                "package a\n\nfunc Login() {}\n\nfunc logout() {}\n",
		"src/b.ts":                "export function login() {}\n",
		"node_modules/x/login.js": "login()\n",
	})
	got, err := searchFiles(root, searchOpts{Text: `func \w+\(`, Regex: true, Glob: "*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "src/a.go:3: func Login() {}\nsrc/a.go:5: func logout() {}" {
		t.Fatalf("regex = %q", got)
	}
	got, err = searchFiles(root, searchOpts{Text: "LOGIN", IgnoreCase: true, Path: "src", Glob: "src/**/*.ts"})
	if err != nil || got != "src/b.ts:1: export function login() {}" {
		t.Fatalf("ignore case = %q, %v", got, err)
	}
	got, err = searchFiles(root, searchOpts{Text: "Login", Context: 1})
	if err != nil || got != "src/a.go-2- \nsrc/a.go:3: func Login() {}\nsrc/a.go-4- " {
		t.Fatalf("context = %q, %v", got, err)
	}
	if strings.Contains(got, "node_modules") {
		t.Fatal("searched node_modules")
	}
	if _, err := searchFiles(root, searchOpts{Text: "(", Regex: true}); err == nil {
		t.Fatal("bad regex accepted")
	}
}

func TestListGlobDepth(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md":            "",
		"src/main.go":          "",
		"src/pkg/util.go":      "",
		"src/pkg/util_test.go": "",
		"vendor/x/x.go":        "",
		"dist/app.js":          "",
		"node_modules/y/y.js":  "",
	})
	got, err := listFiles(root, "", "**/*.go", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "src/main.go\nsrc/pkg/util.go\nsrc/pkg/util_test.go" {
		t.Fatalf("glob = %q", got)
	}
	got, err = listFiles(root, "", "", 2)
	if err != nil || got != "README.md\nsrc/main.go" {
		t.Fatalf("depth = %q, %v", got, err)
	}
	got, err = listFiles(root, "src", "*_test.go", 0)
	if err != nil || got != "src/pkg/util_test.go" {
		t.Fatalf("base glob = %q, %v", got, err)
	}
	if got, _ := listFiles(root, "vendor", "", 0); got != "vendor/x/x.go" {
		t.Fatalf("an explicit vendor path = %q", got)
	}
}

func TestDeleteMoveSafePath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "a", "b.txt": "b"})
	writeTree(t, outside, map[string]string{"keep.txt": "keep"})
	if err := os.Symlink(filepath.Join(outside, "keep.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(root, "a.txt", "sub/c.txt"); err != nil {
		t.Fatal(err)
	}
	if readBack(t, root, "sub/c.txt") != "a" {
		t.Fatal("move lost the text")
	}
	if err := moveFile(root, "sub/c.txt", "b.txt"); err == nil {
		t.Fatal("move over an existing file")
	}
	if err := moveFile(root, "b.txt", "../b.txt"); err == nil {
		t.Fatal("move outside the worktree")
	}
	if err := moveFile(root, "b.txt", ".git/b.txt"); err == nil {
		t.Fatal("move into .git")
	}
	if err := deleteFile(root, "link.txt"); err == nil {
		t.Fatal("deleted a symlink")
	}
	if err := deleteFile(root, "../"+filepath.Base(outside)+"/keep.txt"); err == nil {
		t.Fatal("deleted outside the worktree")
	}
	if err := deleteFile(root, "b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("b.txt still there")
	}
	if readBack(t, outside, "keep.txt") != "keep" {
		t.Fatal("outside file changed")
	}
}

func TestToolErrorsFlagged(t *testing.T) {
	root := t.TempDir()
	s := &session{wt: runtime.Worktree{Path: root}}
	for _, call := range []providers.ToolCall{
		{Name: "read_file", Args: map[string]any{"path": "missing.txt"}},
		{Name: "replace_in_file", Args: map[string]any{"path": "missing.txt", "old": "a", "new": "b"}},
		{Name: "deploy"},
		{Name: "read_file", BadArgs: "unexpected end of JSON input"},
	} {
		got, _, err := s.dispatch(t.Context(), call)
		if err != nil || !strings.HasPrefix(got, providers.ErrorPrefix) {
			t.Fatalf("%s = %q, %v", call.Name, got, err)
		}
	}
}
