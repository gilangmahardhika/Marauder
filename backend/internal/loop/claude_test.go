package loop

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const claudeKey = "sk-claude-test-key-0123456789"

// claudeBoard is a handoffBoard that keeps the attempt's token counts.
type claudeBoard struct {
	*handoffBoard
	input, output, cached int
}

func (b *claudeBoard) AddAttemptTokens(_ context.Context, _ string, input, output, cached int) error {
	b.input, b.output, b.cached = b.input+input, b.output+output, b.cached+cached
	return nil
}

func (b *claudeBoard) CardTokens(context.Context, string) (int, error) {
	return b.input + b.output + b.cached/10, nil
}

// fakeClaude writes a claude script that records its argv, environment, and
// stdin in its directory, then runs script, where $DIR is that directory.
// The default script prints the stream file.
func fakeClaude(t *testing.T, stream, script string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stream"), []byte(stream), 0o600); err != nil {
		t.Fatal(err)
	}
	if script == "" {
		script = `cat "$DIR/stream"`
	}
	body := "#!/bin/sh\nDIR='" + dir + "'\nprintf '%s\\n' \"$@\" > \"$DIR/args\"\nenv > \"$DIR/env\"\ncat > \"$DIR/stdin\"\n" + script + "\n"
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

func claudeSession(t *testing.T, bin string) (*session, *claudeBoard) {
	t.Helper()
	s, hb := gitSession(t, &scripted{})
	b := &claudeBoard{handoffBoard: hb}
	s.in.Board = b
	s.in.Secrets = []string{claudeKey}
	s.in.ClaudeCode = &ClaudeCode{Binary: bin, Version: "1.0.0 (Claude Code)", BaseURL: "https://api.deepseek.com/anthropic", APIKey: claudeKey, Auth: "bearer"}
	s.card.Title = "Greet from main"
	return s, b
}

func streamLines(lines ...map[string]any) string {
	var b strings.Builder
	for _, line := range lines {
		raw, _ := json.Marshal(line)
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

func assistant(id string, block map[string]any, usage map[string]any) map[string]any {
	msg := map[string]any{"id": id, "model": "deepseek-chat", "content": []any{block}}
	if usage != nil {
		msg["usage"] = usage
	}
	return map[string]any{"type": "assistant", "message": msg}
}

func result(subtype, text string, turns int) map[string]any {
	return map[string]any{"type": "result", "subtype": subtype, "is_error": subtype != "success", "result": text, "num_turns": turns}
}

func readFileText(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestClaudeCodeArgsAndEnv(t *testing.T) {
	t.Setenv("ORCH_PARENT_ONLY", "leak")
	bin, dir := fakeClaude(t, streamLines(result("success", "Nothing to change", 1)), "")
	s, _ := claudeSession(t, bin)
	s.in.Allow, s.in.Commands = []string{"npm", "go"}, []string{"go", "make"}
	s.in.ClaudeCode.Instructions = "Prefer table tests."
	s.user = "CARD\nid: card_0123456789ab\n"
	if err := s.claude(context.Background()); err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(readFileText(t, filepath.Join(dir, "args"))), "\n")
	joined := "\n" + strings.Join(args, "\n") + "\n"
	for _, want := range []string{
		"\n-p\n", "\n--output-format\nstream-json\n", "\n--verbose\n", "\n--model\nm\n", "\n--max-turns\n5\n",
		"\n--permission-mode\nacceptEdits\n", "\n--strict-mcp-config\n",
		"\nBash(npm:*)\n", "\nBash(go:*)\n", "\nBash(make:*)\n", "\nBash(git push:*)\n", "\nWebFetch\n",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv lacks %q:\n%s", strings.TrimSpace(want), joined)
		}
	}
	if strings.Count(joined, "\nBash(go:*)\n") != 1 {
		t.Errorf("go is offered more than once:\n%s", joined)
	}
	for _, refused := range []string{"--resume", "--continue", "--dangerously-skip-permissions"} {
		if strings.Contains(joined, refused) {
			t.Errorf("argv has %s", refused)
		}
	}
	if !strings.Contains(joined, "Prefer table tests.") || !strings.Contains(joined, "WORK SO FAR") {
		t.Errorf("the appended system prompt lacks the instructions or the WORK SO FAR rule")
	}
	if got := readFileText(t, filepath.Join(dir, "stdin")); got != s.user+"\n" {
		t.Errorf("stdin = %q, want the pack", got)
	}
	env := readFileText(t, filepath.Join(dir, "env"))
	home := filepath.Join(s.in.Git.Data, "claude", s.attemptID)
	for _, want := range []string{
		"HOME=" + home, "CLAUDE_CONFIG_DIR=" + filepath.Join(home, ".claude"), "ANTHROPIC_BASE_URL=https://api.deepseek.com/anthropic",
		"ANTHROPIC_AUTH_TOKEN=" + claudeKey, "ANTHROPIC_MODEL=m", "DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	} {
		if !strings.Contains(env, want+"\n") {
			t.Errorf("env lacks %s", want)
		}
	}
	if strings.Contains(env, "ORCH_PARENT_ONLY") || strings.Contains(env, "ANTHROPIC_API_KEY=") {
		t.Errorf("env has a parent variable or the wrong key variable:\n%s", env)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Errorf("the attempt's HOME was not removed: %v", err)
	}
}

func TestClaudeCodeCommits(t *testing.T) {
	stream := streamLines(
		assistant("msg_1", map[string]any{"type": "text", "text": "I will greet."}, map[string]any{"input_tokens": 100, "output_tokens": 20, "cache_read_input_tokens": 50, "cache_creation_input_tokens": 10}),
		assistant("msg_1", map[string]any{"type": "tool_use", "id": "t1", "name": "Edit", "input": map[string]any{"file_path": "main.go", "old_string": "func main() {}", "new_string": "func main() { println(\"hi\") }"}}, map[string]any{"input_tokens": 100, "output_tokens": 20, "cache_read_input_tokens": 50, "cache_creation_input_tokens": 10}),
		map[string]any{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": "ok"}}}},
		assistant("msg_2", map[string]any{"type": "tool_use", "id": "t2", "name": "TodoWrite", "input": map[string]any{"todos": []any{
			map[string]any{"content": "edit main", "status": "completed"}, map[string]any{"content": "run tests", "status": "in_progress"},
		}}}, map[string]any{"input_tokens": 30, "output_tokens": 5}),
		result("success", "## Say hi from main\n\nmain.go prints hi.", 2),
	)
	bin, _ := fakeClaude(t, stream, `printf 'package main\n\nfunc main() { println("hi") }\n' > main.go
cat "$DIR/stream"`)
	s, b := claudeSession(t, bin)
	base, _ := s.in.Git.WorktreeHead(context.Background(), s.wt)
	if err := s.claude(context.Background()); err != nil {
		t.Fatal(err)
	}
	head, _ := s.in.Git.WorktreeHead(context.Background(), s.wt)
	if head == base || !s.committed {
		t.Fatal("no commit after a successful result with changes")
	}
	out, err := exec.Command(s.in.Git.Bin, "-C", s.wt.Path, "log", "-1", "--format=%B").Output()
	if err != nil {
		t.Fatal(err)
	}
	msg := string(out)
	if !strings.HasPrefix(msg, "Say hi from main\n") || !strings.Contains(msg, "Orchestrator-Attempt: att_handoff") || !strings.Contains(msg, claudeTrailer) {
		t.Fatalf("commit message:\n%s", msg)
	}
	if b.input != 140 || b.output != 25 || b.cached != 50 {
		t.Fatalf("tokens = %d in, %d out, %d cached; want each message once", b.input, b.output, b.cached)
	}
	if calls := b.payloads("model_call"); len(calls) != 2 || calls[0]["text"] != "I will greet." || calls[0]["inputTokens"] != 110 {
		t.Fatalf("model_call = %v", calls)
	}
	var names []string
	for _, call := range b.payloads("tool_call") {
		names = append(names, call["name"].(string))
	}
	if strings.Join(names, ",") != "Edit,TodoWrite,commit" {
		t.Fatalf("tool calls = %v", names)
	}
	edit := b.payloads("tool_call")[0]["args"].(map[string]any)
	if edit["path"] != "main.go" || edit["newBytes"] == nil {
		t.Fatalf("Edit args = %v", edit)
	}
	if len(s.todos) != 2 || s.todos[0].Status != "done" || s.todos[1] != (todoItem{Text: "run tests", Status: "in_progress"}) {
		t.Fatalf("todos = %+v", s.todos)
	}
	run := b.payloads("agent_run")
	if len(run) != 1 || run[0]["agent"] != "claude_code" || run[0]["turns"] != 2 || run[0]["exitCode"] != 0 || run[0]["version"] != "1.0.0 (Claude Code)" {
		t.Fatalf("agent_run = %v", run)
	}
}

func TestClaudeCodeTokenCapKills(t *testing.T) {
	first := streamLines(assistant("msg_1", map[string]any{"type": "tool_use", "id": "t1", "name": "Read", "input": map[string]any{"file_path": "main.go"}},
		map[string]any{"input_tokens": 900, "output_tokens": 200}))
	later := streamLines(result("success", "never read", 1))
	bin, dir := fakeClaude(t, first, `printf 'package main\n\n// half done\n' > main.go
cat "$DIR/stream"
sleep 30
cat "$DIR/later"`)
	if err := os.WriteFile(filepath.Join(dir, "later"), []byte(later), 0o600); err != nil {
		t.Fatal(err)
	}
	s, b := claudeSession(t, bin)
	s.in.MaxTokensPerCard = 1000
	start := time.Now()
	if err := s.claude(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.stop != "token_cap" || time.Since(start) > 10*time.Second {
		t.Fatalf("stop = %q after %v", s.stop, time.Since(start))
	}
	if s.committed {
		t.Fatal("a stopped run committed")
	}
	if run := b.payloads("agent_run"); len(run) != 1 || run[0]["exitCode"] == 0 {
		t.Fatalf("agent_run = %v", run)
	}
	s.saveHandoff(context.Background(), s.stop, "main")
	if len(b.saved) != 1 || b.saved[0].PatchFiles != 1 || b.saved[0].SummarySource != "harness" {
		t.Fatalf("handoff = %+v", b.saved)
	}
}

func TestClaudeCodeMaxTurns(t *testing.T) {
	bin, _ := fakeClaude(t, streamLines(result("error_max_turns", "", 5)), `printf 'package main\n// partial\n' > main.go
cat "$DIR/stream"`)
	s, _ := claudeSession(t, bin)
	if err := s.claude(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s.outOfTurns || s.committed || s.stop != "" {
		t.Fatalf("outOfTurns %v committed %v stop %q", s.outOfTurns, s.committed, s.stop)
	}
}

func TestClaudeCodeErrorExit(t *testing.T) {
	bin, _ := fakeClaude(t, "", `echo "401: invalid key `+claudeKey+`" >&2
exit 3`)
	s, b := claudeSession(t, bin)
	editMain(t, s, "package main\n// before the failure\n")
	err := s.claude(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exited 3") || !strings.Contains(err.Error(), "401: invalid key") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), claudeKey) {
		t.Fatal("the error holds the key")
	}
	s.saveHandoff(context.Background(), "error", "main")
	if len(b.saved) != 1 || b.saved[0].SummarySource != "harness" {
		t.Fatalf("handoff = %+v", b.saved)
	}
}

func TestClaudeCodeHandoffSaved(t *testing.T) {
	bin, _ := fakeClaude(t, streamLines(result("success", "Goal: greet\nDone: nothing, main.go already greets", 1)), "")
	s, b := claudeSession(t, bin)
	if err := s.claude(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.committed {
		t.Fatal("a run without changes committed")
	}
	s.saveHandoff(context.Background(), "no_commit", "main")
	if len(b.saved) != 1 || b.saved[0].SummarySource != "model" || !strings.Contains(b.saved[0].Summary, "main.go already greets") {
		t.Fatalf("handoff = %+v", b.saved)
	}
}

func TestClaudeCodeTraceRedactsKey(t *testing.T) {
	stream := streamLines(
		assistant("msg_1", map[string]any{"type": "text", "text": "The key is " + claudeKey}, map[string]any{"input_tokens": 1, "output_tokens": 1}),
		assistant("msg_1", map[string]any{"type": "tool_use", "id": "t1", "name": "Bash", "input": map[string]any{"command": "echo " + claudeKey}}, nil),
		result("success", "Used "+claudeKey, 1),
	)
	bin, _ := fakeClaude(t, stream, "")
	s, b := claudeSession(t, bin)
	if err := s.claude(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(b.events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), claudeKey) || strings.Contains(s.finalText, claudeKey) {
		t.Fatalf("the key reached the trace or the summary:\n%s", raw)
	}
	if calls := b.payloads("tool_call"); len(calls) != 1 || calls[0]["name"] != "Bash" {
		t.Fatalf("tool calls = %v", calls)
	}
}
