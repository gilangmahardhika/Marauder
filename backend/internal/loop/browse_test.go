package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/providers"
)

func TestHistoryKeepsNewestResults(t *testing.T) {
	s := &session{user: "CARD"}
	if len(s.history()) != 0 {
		t.Fatalf("history = %v", s.history())
	}
	big := strings.Repeat("x", 10<<10)
	read := func(id, path string) providers.ToolCall {
		return providers.ToolCall{ID: id, Name: "read_file", Args: map[string]any{"path": path}}
	}
	s.turns = []providers.Turn{
		{Calls: []providers.ToolCall{read("c1", "a.txt"), read("c2", "b.txt")}, Results: []string{big, big}},
		{Text: "editing", Calls: []providers.ToolCall{{ID: "c3", Name: "edit_file", Args: map[string]any{"path": "b.txt", "text": big}}}, Results: []string{"wrote b.txt"}},
		{Calls: []providers.ToolCall{{ID: "c4", Name: "list_files", Args: map[string]any{}}}, Results: []string{strings.Repeat("p\n", 20<<10)}},
	}
	got := s.history()
	if len(got) != 3 || got[1].Text != "editing" || got[0].Calls[1].ID != "c2" {
		t.Fatalf("history shape = %+v", got)
	}
	if got[0].Results[0] != omitted || got[0].Results[1] != omitted {
		t.Fatalf("old results kept: %.40q %.40q", got[0].Results[0], got[0].Results[1])
	}
	if got[1].Results[0] != "wrote b.txt" || !strings.HasSuffix(got[2].Results[0], "truncated: true") {
		t.Fatalf("newest results = %q, %.40q", got[1].Results[0], got[2].Results[0])
	}
	if _, ok := got[1].Calls[0].Args["text"]; ok || got[1].Calls[0].Args["path"] != "b.txt" || got[1].Calls[0].Args["bytes"] != len(big) {
		t.Fatalf("edit args = %v", got[1].Calls[0].Args)
	}
	if s.turns[1].Calls[0].Args["text"] != big {
		t.Fatal("history changed the stored turn")
	}
	if warned := withNote(s.history(), 9, 40); warned[2].Note != "" {
		t.Fatal("warned with turns to spare")
	}
	if warned := withNote(s.history(), 3, 12); !strings.Contains(warned[2].Note, "3 model turns left") {
		t.Fatalf("no warning at 3 of 12: %q", warned[2].Note)
	}
	warned := withNote(s.history(), 8, 40)
	if !strings.Contains(warned[2].Note, "8 model turns left") || s.turns[2].Note != "" || strings.Contains(warned[2].Results[0], "turns left") {
		t.Fatalf("warning = %q, stored note %q", warned[2].Note, s.turns[2].Note)
	}
	size := 0
	for _, turn := range got {
		for _, r := range turn.Results {
			size += len(r)
		}
	}
	if size > defaultHistory {
		t.Fatalf("results are %d bytes", size)
	}
}

func TestListAndSearchFiles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write := func(dir, rel, text string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(root, "resources/js/Pages/Auth/Login.tsx", "<InputLabel value=\"Username atau Email\" />\n")
	write(root, "app/Login.php", "'Username atau Email tidak boleh kosong.'\n")
	write(root, ".git/config", "Username atau Email\n")
	write(outside, "secret.txt", "Username atau Email\n")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	list, err := listFiles(root, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "resources/js/Pages/Auth/Login.tsx") || strings.Contains(list, ".git") || strings.Contains(list, "secret.txt") {
		t.Fatalf("list = %s", list)
	}
	sub, err := listFiles(root, "resources", "", 0)
	if err != nil || strings.Contains(sub, "app/Login.php") {
		t.Fatalf("sub list = %s, %v", sub, err)
	}
	if _, err := listFiles(root, "../", "", 0); err == nil {
		t.Fatal("list outside the worktree")
	}

	found, err := searchFiles(root, searchOpts{Text: "Username atau Email"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(found, "resources/js/Pages/Auth/Login.tsx:1: <InputLabel") || !strings.Contains(found, "app/Login.php:1:") {
		t.Fatalf("search = %s", found)
	}
	if strings.Contains(found, ".git") || strings.Contains(found, "secret.txt") {
		t.Fatalf("search left the worktree: %s", found)
	}
	if none, _ := searchFiles(root, searchOpts{Text: "not here"}); none != "no matches for not here" {
		t.Fatalf("search = %s", none)
	}
}
