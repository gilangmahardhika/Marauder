package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/settings"
)

const claudeKey = "sk-deepseek-claude-key"

// fakeClaude writes a claude script that answers --version, records its
// argv in dir, edits hello.txt in its working directory, and writes a
// stream-json run that succeeds.
func fakeClaude(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "1.0.0 (Claude Code)"; exit 0; fi
printf '%s\n' "$@" > "` + dir + `/args"
cat > /dev/null
printf 'job claude\n' > hello.txt
printf '%s\n' '{"type":"assistant","message":{"id":"msg_1","model":"deepseek-chat","stop_reason":"tool_use","content":[{"type":"tool_use","id":"tu_1","name":"Edit","input":{"file_path":"hello.txt"}}],"usage":{"input_tokens":120,"output_tokens":30}}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"Say job with Claude Code\n\nChanged hello.txt.","num_turns":2}'
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

// claudeAgent saves a DeepSeek anthropic family and Claude Code as the agent.
// With no key the agent is written past SaveAgent, as if the key was cleared
// after it was saved.
func claudeAgent(t *testing.T, data, bin, key string) {
	t.Helper()
	families := []settings.Family{{ID: "deepseek", Protocol: "anthropic", BaseURL: "https://api.deepseek.com/anthropic", Models: []settings.Model{{ID: "deepseek-chat"}}}}
	keys := map[string]*string{}
	if key != "" {
		keys["deepseek"] = &key
	}
	if _, err := settings.SaveLLM(data, families, keys); err != nil {
		t.Fatal(err)
	}
	if key == "" {
		body := "agent:\n  kind: claude_code\n  claudeCode:\n    family: deepseek\n    model: deepseek-chat\n    binary: " + bin + "\n"
		raw, err := os.ReadFile(filepath.Join(data, "config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(data, "config.yaml"), string(raw)+body)
		return
	}
	if _, err := settings.SaveAgent(data, settings.AgentConfig{
		Kind: settings.AgentClaudeCode, ClaudeCode: settings.ClaudeCodeOptions{Family: "deepseek", Model: "deepseek-chat", MaxTurns: 9, Binary: bin},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestImplementUsesClaudeCode(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	bin, dir := fakeClaude(t)
	claudeAgent(t, f.data, bin, claudeKey)
	card := f.run(t, `{"implement":[]}`)
	if card.Status != "pr_open" {
		t.Fatalf("card = %s %s, want pr_open", card.Status, card.BlockedReason)
	}
	message, err := exec.Command(lookGit(t), "-C", f.repo, "log", "-1", "--format=%B", "orch/"+card.ID).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(message), "Say job with Claude Code") || !strings.Contains(string(message), "claude code") {
		t.Fatalf("commit = %q", message)
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--max-turns\n9\n") || !strings.Contains(string(args), "--model\ndeepseek-chat\n") {
		t.Fatalf("argv lacks the agent's turn cap or model:\n%s", args)
	}
	var model string
	var input, output int
	if err := f.db.QueryRow(`SELECT model_id, input_tokens, output_tokens FROM attempts WHERE card_id = $1`, card.ID).Scan(&model, &input, &output); err != nil {
		t.Fatal(err)
	}
	if model != "deepseek-chat" || input != 120 || output != 30 {
		t.Fatalf("attempt = %s %d/%d", model, input, output)
	}
	if runs := payloads(t, f, "agent_run"); len(runs) != 1 || runs[0]["exitCode"] != float64(0) {
		t.Fatalf("agent_run = %v", runs)
	}
}

func TestImplementClaudeCodeWithoutKeyBlocks(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	bin, dir := fakeClaude(t)
	claudeAgent(t, f.data, bin, "")
	card := f.run(t, `{"implement":[]}`)
	blockedWith(t, card, "agent_config", 0)
	if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
		t.Fatal("claude ran without a key")
	}
}
