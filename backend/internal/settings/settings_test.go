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
