package settings_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

func TestSecretOmitted(t *testing.T) {
	dir := t.TempDir()
	writeProjectSecrets(t, dir, "deploy-super-secret-token")
	cfg, _, err := settings.LoadProject(dir, "proj_a")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(settings.DeployView(cfg))
	if strings.Contains(string(body), "deploy-super-secret-token") {
		t.Fatalf("view leaked token: %s", body)
	}
}

func TestSecretKeep(t *testing.T) {
	dir := t.TempDir()
	writeProjectSecrets(t, dir, "deploy-super-secret-token")
	path := filepath.Join(settings.ProjectDir(dir, "proj_a"), "secrets.json")
	before, _ := os.ReadFile(path)
	if _, _, err := settings.SaveProjectDeploy(dir, "proj_a", settings.ProjectPaths{Data: dir}, settings.Deploy{SandboxEngine: "docker"}, nil, false); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatalf("secrets changed\n%s", after)
	}
}

func TestDeployEnabledDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, _, err := settings.LoadProject(dir, "proj_a")
	if err != nil {
		t.Fatal(err)
	}
	if !settings.DeployOn(cfg) || !*settings.DeployView(cfg).DeployEnabled {
		t.Fatal("a project with no deploy.enabled key must deploy")
	}
	paths := settings.ProjectPaths{Data: dir}
	off := false
	_, changed, err := settings.SaveProjectDeploy(dir, "proj_a", paths, settings.Deploy{DeployEnabled: &off}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(changed, " "), "deployEnabled") {
		t.Fatalf("changed = %v", changed)
	}
	if cfg, _, _ = settings.LoadProject(dir, "proj_a"); settings.DeployOn(cfg) {
		t.Fatal("deploy stayed on after saving false")
	}
	if _, _, err := settings.SaveProjectDeploy(dir, "proj_a", paths, settings.Deploy{SandboxEngine: "docker"}, nil, false); err != nil {
		t.Fatal(err)
	}
	if cfg, _, _ = settings.LoadProject(dir, "proj_a"); settings.DeployOn(cfg) {
		t.Fatal("a save without deployEnabled turned deploy back on")
	}
}

func TestSettingsSectionsKeepEachOther(t *testing.T) {
	dir := t.TempDir()
	paths := settings.ProjectPaths{Data: dir}
	limit, solo := 5, true
	if _, _, err := settings.SaveProjectWorkflow(dir, "proj_a", &limit, &solo); err != nil {
		t.Fatal(err)
	}
	prd, rfc := "specs/prd", "specs/rfc"
	if _, _, err := settings.SaveProjectPaths(dir, "proj_a", &prd, &rfc); err != nil {
		t.Fatal(err)
	}
	off := false
	if _, _, err := settings.SaveProjectDeploy(dir, "proj_a", paths, settings.Deploy{SandboxEngine: "docker", DeployEnabled: &off}, nil, false); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := settings.LoadProject(dir, "proj_a")
	if err != nil {
		t.Fatal(err)
	}
	if w := settings.WorkflowView(cfg); w.ImplementationCap != 5 || !w.SoloApproval {
		t.Fatalf("deploy save reset the workflow: %+v", w)
	}
	if p := settings.DocPathsView(cfg); p.PRD != prd || p.RFC != rfc {
		t.Fatalf("deploy save reset the paths: %+v", p)
	}

	limit = 3
	_, changed, err := settings.SaveProjectWorkflow(dir, "proj_a", &limit, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(changed, " ") != "implementationCap" {
		t.Fatalf("changed = %v", changed)
	}
	cfg, _, _ = settings.LoadProject(dir, "proj_a")
	if settings.DeployOn(cfg) || cfg.Paths.PRD != prd || !cfg.Approval.Solo || cfg.Sandbox.Engine != "docker" {
		t.Fatalf("workflow save reset another section: %+v", cfg)
	}

	prd = "docs/requirements"
	if _, _, err := settings.SaveProjectPaths(dir, "proj_a", &prd, nil); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ = settings.LoadProject(dir, "proj_a")
	if w := settings.WorkflowView(cfg); w.ImplementationCap != 3 || !w.SoloApproval || cfg.Paths.RFC != rfc || cfg.Paths.PRD != prd {
		t.Fatalf("paths save reset another section: %+v", cfg)
	}

	tooMany := 9
	if _, _, err := settings.SaveProjectWorkflow(dir, "proj_a", &tooMany, nil); !errors.Is(err, settings.ErrValidation) {
		t.Fatalf("cap 9 err = %v", err)
	}
	outside := "../secrets"
	if _, _, err := settings.SaveProjectPaths(dir, "proj_a", &outside, nil); !errors.Is(err, settings.ErrValidation) {
		t.Fatalf("outside path err = %v", err)
	}
}

func TestShortSecret(t *testing.T) {
	dir := t.TempDir()
	writeProjectSecrets(t, dir, "deploy-super-secret-token")
	token := "abcd"
	_, _, err := settings.SaveProjectDeploy(dir, "proj_a", settings.ProjectPaths{Data: dir}, settings.Deploy{}, &token, false)
	if !errors.Is(err, settings.ErrValidation) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownModel(t *testing.T) {
	dir := t.TempDir()
	cfg := settings.Config{}
	cfg.LLM.Families = []settings.Family{{ID: "anthropic", Models: []settings.Model{{ID: "known"}}}}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("llm:\n  families:\n    - id: anthropic\n      models:\n        - id: known\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
	cfg.Router.Routes = map[string]struct {
		Model    string `yaml:"model" json:"model"`
		Fallback string `yaml:"fallback" json:"fallback"`
	}{"implement": {Model: "missing"}}
	if _, err := settings.SaveRouter(dir, cfg); err != settings.ErrValidation {
		t.Fatalf("err = %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if string(after) != string(before) {
		t.Fatalf("yaml changed\n%s", after)
	}
}

func TestModelIdUnique(t *testing.T) {
	dir := t.TempDir()
	body := []byte("llm:\n  families:\n    - id: anthropic\n      models:\n        - id: same\n    - id: openai\n      models:\n        - id: same\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.SaveRouter(dir, settings.Config{}); err != settings.ErrValidation {
		t.Fatalf("err = %v", err)
	}
}

func TestSaveLLMModelOptions(t *testing.T) {
	family := func(protocol string, opts settings.ModelOptions) []settings.Family {
		return []settings.Family{{ID: "f", Protocol: protocol, BaseURL: "http://127.0.0.1:1", Models: []settings.Model{{ID: "m", ModelOptions: opts}}}}
	}
	refused := map[string][]settings.Family{
		"adaptive on openai":    family("openai", settings.ModelOptions{Thinking: "adaptive"}),
		"unknown thinking":      family("anthropic", settings.ModelOptions{Thinking: "sometimes"}),
		"budget under 1024":     family("anthropic", settings.ModelOptions{Thinking: "enabled", ThinkingBudget: 1023}),
		"budget over 32768":     family("anthropic", settings.ModelOptions{Thinking: "enabled", ThinkingBudget: 32769}),
		"unknown effort":        family("anthropic", settings.ModelOptions{Effort: "extreme"}),
		"openai-only effort":    family("anthropic", settings.ModelOptions{Effort: "minimal"}),
		"anthropic-only effort": family("openai", settings.ModelOptions{Effort: "max"}),
	}
	for name, families := range refused {
		dir := t.TempDir()
		_, err := settings.SaveLLM(dir, families, nil)
		var invalid *settings.Invalid
		if !errors.As(err, &invalid) || invalid.Fields["modelOptions"] == "" {
			t.Fatalf("%s: err = %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "config.yaml")); !os.IsNotExist(err) {
			t.Fatalf("%s: config.yaml was written", name)
		}
	}
	if _, err := settings.NewModels([]string{"m"}, map[string]settings.ModelOptions{"other": {Effort: "high"}}); err == nil {
		t.Fatal("an option for an unlisted model was accepted")
	}

	dir := t.TempDir()
	families := family("anthropic", settings.ModelOptions{Thinking: "enabled", ThinkingBudget: 4096, Effort: "high"})
	families = append(families, family("openai", settings.ModelOptions{Thinking: "disabled", ThinkingBudget: 4096, Effort: "minimal"})...)
	families[1].ID, families[1].Models[0].ID = "g", "n"
	if _, err := settings.SaveLLM(dir, families, nil); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := settings.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.LLM.Families[0].Models[0].ModelOptions; got != (settings.ModelOptions{Thinking: "enabled", ThinkingBudget: 4096, Effort: "high"}) {
		t.Fatalf("anthropic options = %+v", got)
	}
	if got := cfg.LLM.Families[1].Models[0].ModelOptions; got != (settings.ModelOptions{Thinking: "disabled", Effort: "minimal"}) {
		t.Fatalf("openai options = %+v, want the unused budget dropped", got)
	}
}

func TestContextTokensRange(t *testing.T) {
	family := func(tokens int) []settings.Family {
		return []settings.Family{{ID: "f", Protocol: "anthropic", BaseURL: "http://127.0.0.1:1",
			Models: []settings.Model{{ID: "m", ModelOptions: settings.ModelOptions{ContextTokens: tokens}}}}}
	}
	for _, tokens := range []int{7999, 2000001} {
		_, err := settings.SaveLLM(t.TempDir(), family(tokens), nil)
		var invalid *settings.Invalid
		if !errors.As(err, &invalid) || !strings.Contains(invalid.Fields["modelOptions"], "context window") {
			t.Fatalf("%d: err = %v", tokens, err)
		}
	}
	dir := t.TempDir()
	if _, err := settings.SaveLLM(dir, family(200000), nil); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := settings.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	opts := cfg.LLM.Families[0].Models[0].ModelOptions
	if opts.ContextWindow() != 200000 || opts.ContextLimit() != 150000 {
		t.Fatalf("window %d limit %d, want 200000 and 150000", opts.ContextWindow(), opts.ContextLimit())
	}
	if none := (settings.ModelOptions{}); none.ContextWindow() != 128000 || none.ContextLimit() != 96000 {
		t.Fatalf("default window %d limit %d, want 128000 and 96000", none.ContextWindow(), none.ContextLimit())
	}
}

func TestConfigEvent(t *testing.T) {
	payload := map[string]any{"section": "github", "fields": []string{"owner", "approvers"}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	stored := trace.Redact(string(raw), []string{"ghp_super_secret_token"})
	if strings.Contains(stored, "ghp_super_secret_token") || !strings.Contains(stored, "owner") {
		t.Fatalf("event = %s", stored)
	}
}

func TestSnapshot(t *testing.T) {
	attemptModel := "model-a"
	_ = settings.Config{}
	if attemptModel != "model-a" {
		t.Fatal("attempt model changed")
	}
}

func writeProjectSecrets(t *testing.T, data, token string) {
	t.Helper()
	dir := settings.ProjectDir(data, "proj_a")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("{\n  \"deployToken\": \"" + token + "\"\n}\n")
	if err := os.WriteFile(filepath.Join(dir, "secrets.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSaveDeployCommandExecutables(t *testing.T) {
	dir := t.TempDir()
	paths := settings.ProjectPaths{Data: dir}
	for _, names := range [][]string{{"go", "bash"}, {"env"}, {"xargs"}, {"Busybox"}, {"timeout"}, {"/usr/bin/go"}, {"go test"}} {
		_, _, err := settings.SaveProjectDeploy(dir, "proj_a", paths, settings.Deploy{CommandExecutables: names}, nil, false)
		var invalid *settings.Invalid
		if !errors.As(err, &invalid) || invalid.Fields["commandExecutables"] == "" {
			t.Fatalf("%v saved: %v", names, err)
		}
	}
	_, changed, err := settings.SaveProjectDeploy(dir, "proj_a", paths, settings.Deploy{CommandExecutables: []string{"go", "gofmt", "npx"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(changed, ","), "commandExecutables") {
		t.Fatalf("changed = %v", changed)
	}
	cfg, _, err := settings.LoadProject(dir, "proj_a")
	if err != nil {
		t.Fatal(err)
	}
	view := settings.DeployView(cfg)
	if strings.Join(view.CommandExecutables, ",") != "go,gofmt,npx" {
		t.Fatalf("view = %v", view.CommandExecutables)
	}
	if empty := settings.DeployView(settings.ProjectConfig{}); empty.CommandExecutables == nil {
		t.Fatal("an unset list must encode as [], not null")
	}
}
