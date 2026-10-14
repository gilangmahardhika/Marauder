package settings_test

import (
	"errors"
	"testing"

	"orchestrator/backend/internal/settings"
)

func TestAgentSettingsValidate(t *testing.T) {
	dir := t.TempDir()
	key := "sk-deepseek-123456"
	families := []settings.Family{
		{ID: "deepseek", Protocol: "anthropic", BaseURL: "https://api.deepseek.com/anthropic", Models: []settings.Model{{ID: "deepseek-chat"}, {ID: "deepseek-reasoner"}}},
		{ID: "gpt", Protocol: "openai", Models: []settings.Model{{ID: "gpt-5"}}},
		{ID: "nokey", Protocol: "anthropic", Models: []settings.Model{{ID: "claude-x"}}},
	}
	if _, err := settings.SaveLLM(dir, families, map[string]*string{"deepseek": &key}); err != nil {
		t.Fatal(err)
	}
	good := settings.ClaudeCodeOptions{Family: "deepseek", Model: "deepseek-chat"}
	cases := []struct {
		name  string
		edit  func(*settings.ClaudeCodeOptions)
		field string
	}{
		{"openai family", func(o *settings.ClaudeCodeOptions) { o.Family, o.Model = "gpt", "gpt-5" }, "claudeCode.family"},
		{"no key", func(o *settings.ClaudeCodeOptions) { o.Family, o.Model = "nokey", "claude-x" }, "claudeCode.family"},
		{"unknown family", func(o *settings.ClaudeCodeOptions) { o.Family = "gone" }, "claudeCode.family"},
		{"model not in family", func(o *settings.ClaudeCodeOptions) { o.Model = "gpt-5" }, "claudeCode.model"},
		{"small model not in family", func(o *settings.ClaudeCodeOptions) { o.SmallModel = "claude-x" }, "claudeCode.smallModel"},
		{"too many turns", func(o *settings.ClaudeCodeOptions) { o.MaxTurns = 201 }, "claudeCode.maxTurns"},
		{"bad auth", func(o *settings.ClaudeCodeOptions) { o.Auth = "cookie" }, "claudeCode.auth"},
		{"relative binary", func(o *settings.ClaudeCodeOptions) { o.Binary = "bin/claude" }, "claudeCode.binary"},
	}
	for _, tc := range cases {
		opts := good
		tc.edit(&opts)
		_, err := settings.SaveAgent(dir, settings.AgentConfig{Kind: settings.AgentClaudeCode, ClaudeCode: opts})
		var invalid *settings.Invalid
		if !errors.As(err, &invalid) || invalid.Fields[tc.field] == "" {
			t.Errorf("%s: err = %v, want a %s field error", tc.name, err, tc.field)
		}
	}
	if _, err := settings.SaveAgent(dir, settings.AgentConfig{Kind: "codex"}); !errors.Is(err, settings.ErrValidation) {
		t.Errorf("unknown kind: err = %v", err)
	}

	opts := good
	opts.MaxTurns, opts.Auth, opts.Binary = 60, settings.AuthAuto, "/usr/local/bin/claude"
	cfg, err := settings.SaveAgent(dir, settings.AgentConfig{Kind: settings.AgentClaudeCode, ClaudeCode: opts})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentKind() != settings.AgentClaudeCode || cfg.Agent.ClaudeCode != opts {
		t.Fatalf("saved %+v", cfg.Agent)
	}
	cfg, _, err = settings.Load(dir)
	if err != nil || cfg.Agent.ClaudeCode.MaxTurns != 60 {
		t.Fatalf("reloaded %+v, %v", cfg.Agent, err)
	}

	stale := settings.ClaudeCodeOptions{Family: "gone", MaxTurns: 999}
	cfg, err = settings.SaveAgent(dir, settings.AgentConfig{Kind: "", ClaudeCode: stale})
	if err != nil {
		t.Fatalf("builtin with stale options: %v", err)
	}
	if cfg.AgentKind() != settings.AgentBuiltin || cfg.Agent.Kind != settings.AgentBuiltin || cfg.Agent.ClaudeCode != stale {
		t.Fatalf("builtin saved %+v", cfg.Agent)
	}
}

func TestResolveAuth(t *testing.T) {
	cases := map[[2]string]string{
		{"", ""}:                              settings.AuthAPIKey,
		{"auto", "https://api.anthropic.com"}: settings.AuthAPIKey,
		{"auto", "https://api.deepseek.com/anthropic"}: settings.AuthBearer,
		{"apiKey", "https://api.deepseek.com"}:         settings.AuthAPIKey,
		{"bearer", ""}:                                 settings.AuthBearer,
	}
	for in, want := range cases {
		if got := settings.ResolveAuth(in[0], in[1]); got != want {
			t.Errorf("ResolveAuth(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
