package runner_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/runnerapi"
	"orchestrator/backend/internal/settings"
)

const claudeKey = "sk-deepseek-claude-key"

// fakeClaude writes a claude script that answers --version, records its
// environment in dir, edits hello.txt in its working directory, and writes a
// stream-json run whose final text holds the key it was given.
func fakeClaude(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "1.0.0 (Claude Code)"; exit 0; fi
env > "` + dir + `/env"
cat > /dev/null
printf 'job claude\n' > hello.txt
printf '%s\n' '{"type":"assistant","message":{"id":"msg_1","model":"deepseek-chat","stop_reason":"tool_use","content":[{"type":"text","text":"Editing."},{"type":"tool_use","id":"tu_1","name":"Edit","input":{"file_path":"hello.txt"}}],"usage":{"input_tokens":120,"output_tokens":30}}}'
printf '{"type":"result","subtype":"success","is_error":false,"result":"Say job with Claude Code\\n\\nkey %s","num_turns":2}\n' "$ANTHROPIC_AUTH_TOKEN"
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

// useClaudeCode saves a DeepSeek anthropic family with its key and makes
// Claude Code the company's agent.
func useClaudeCode(t *testing.T, data string) {
	t.Helper()
	key := claudeKey
	families := []settings.Family{{ID: "deepseek", Protocol: "anthropic", BaseURL: "https://api.deepseek.com/anthropic", Models: []settings.Model{{ID: "deepseek-chat"}}}}
	if _, err := settings.SaveLLM(data, families, map[string]*string{"deepseek": &key}); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.SaveAgent(data, settings.AgentConfig{
		Kind: settings.AgentClaudeCode, ClaudeCode: settings.ClaudeCodeOptions{Family: "deepseek", Model: "deepseek-chat"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestClaimNeedsClaudeCodeRunner(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	useClaudeCode(t, h.data)
	ctx := context.Background()
	builtin := h.register(t, "builtin-only")
	if _, err := builtin.Heartbeat(ctx, runnerapi.Heartbeat{Version: "test", Protocol: runnerapi.Protocol}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := builtin.Claim(ctx, 0); err != nil || ok {
		t.Fatalf("a runner without Claude Code claimed: %v %v", ok, err)
	}
	claude := h.register(t, "with-claude")
	if _, err := claude.Heartbeat(ctx, runnerapi.Heartbeat{Version: "test", Protocol: runnerapi.Protocol, Agents: []string{"builtin", "claude_code"}}); err != nil {
		t.Fatal(err)
	}
	if claimed := claim(t, claude); claimed.CardID != h.card.ID {
		t.Fatalf("claimed %s, want %s", claimed.CardID, h.card.ID)
	}
}

func TestRunnerRunsClaudeCode(t *testing.T) {
	h := newHarness(t, commitScript, "true")
	useClaudeCode(t, h.data)
	ctx := context.Background()
	c := h.register(t, "agent-1")
	r, _ := h.worker(t, c)
	bin, dir := fakeClaude(t)
	r.Claude = bin
	r.Probe(ctx)
	if _, err := c.Heartbeat(ctx, runnerapi.Heartbeat{Version: "test", Protocol: runnerapi.Protocol, Agents: []string{"builtin", "claude_code"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.RunJob(ctx, claim(t, c).LeaseID); err != nil {
		t.Fatal(err)
	}
	if card := h.status(t); card.Status != "pr_open" {
		rows, _ := h.db.Query(`SELECT kind, payload_json FROM events WHERE card_id = $1 ORDER BY id`, h.card.ID)
		for rows != nil && rows.Next() {
			var kind, payload string
			_ = rows.Scan(&kind, &payload)
			t.Log(kind, payload)
		}
		t.Fatalf("card = %s %s, want pr_open", card.Status, card.BlockedReason)
	}
	if len(h.provider.seen()) != 0 {
		t.Fatalf("the control plane's model was called: %v", h.provider.seen())
	}
	env, err := os.ReadFile(filepath.Join(dir, "env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "ANTHROPIC_AUTH_TOKEN="+claudeKey) || !strings.Contains(string(env), "ANTHROPIC_BASE_URL=https://api.deepseek.com/anthropic") {
		t.Fatalf("claude env lacks the key or base URL:\n%s", env)
	}
	err = filepath.WalkDir(r.Data, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if body, err := os.ReadFile(path); err == nil && strings.Contains(string(body), claudeKey) {
			t.Errorf("%s holds the model key", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var leaks int
	if err := h.db.QueryRow(`SELECT count(*) FROM events WHERE payload_json LIKE '%' || $1 || '%'`, claudeKey).Scan(&leaks); err != nil {
		t.Fatal(err)
	}
	var runs int
	if err := h.db.QueryRow(`SELECT count(*) FROM events WHERE kind = 'agent_run' AND card_id = $1`, h.card.ID).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if leaks != 0 || runs != 1 {
		t.Fatalf("events holding the key = %d, agent_run events = %d", leaks, runs)
	}
	for route, bodies := range h.tap.all() {
		if route == "start" {
			continue
		}
		for _, body := range bodies {
			if strings.Contains(body, claudeKey) {
				t.Fatalf("the %s reply holds the model key", route)
			}
		}
	}
}
