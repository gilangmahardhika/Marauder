package vcs_test

import (
	"strings"
	"testing"

	"orchestrator/backend/internal/vcs"
)

func TestPushURL(t *testing.T) {
	cmd, err := vcs.Push("/usr/bin/git", "/tmp/askpass", "https://github.com/acme/widget.git", "ghp_secret_token", "card_0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Argv, " ")
	if strings.Contains(joined, "origin") {
		t.Fatalf("argv uses origin: %s", joined)
	}
	if !strings.Contains(joined, "https://github.com/acme/widget.git") {
		t.Fatalf("argv = %s", joined)
	}
	if !strings.Contains(joined, "refs/heads/orch/card_0123456789ab:refs/heads/orch/card_0123456789ab") {
		t.Fatalf("refspec = %s", joined)
	}
}

func TestAskpass(t *testing.T) {
	const token = "ghp_secret_token"
	cmd, err := vcs.Push("/usr/bin/git", "/tmp/askpass", "https://github.com/acme/widget.git", token, "card_0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(cmd.Argv, " "), token) {
		t.Fatal("token in argv")
	}
	found := false
	for _, env := range cmd.Env {
		if env == "GH_TOKEN="+token {
			found = true
		}
	}
	if !found {
		t.Fatalf("env = %v", cmd.Env)
	}
}

func TestPROnce(t *testing.T) {
	creates := 0
	summary := "<summary written by the implementer>"
	first := vcs.EnsurePR("", "", summary, "requirements: REQ-001", &creates)
	second := vcs.EnsurePR(first.URL, first.Body, summary, "requirements: REQ-001", &creates)
	if creates != 1 || first.URL != second.URL {
		t.Fatalf("creates %d urls %s %s", creates, first.URL, second.URL)
	}
}

func TestMetadataRewrite(t *testing.T) {
	creates := 0
	summary := "<summary written by the implementer>"
	first := vcs.EnsurePR("", "", summary, "requirements: REQ-001", &creates)
	second := vcs.EnsurePR(first.URL, first.Body, summary, "requirements: REQ-002", &creates)
	if !strings.Contains(second.Body, "requirements: REQ-002") || !strings.Contains(second.Body, summary) {
		t.Fatalf("body = %s", second.Body)
	}
	if strings.Contains(second.Body, "requirements: REQ-001") {
		t.Fatalf("old metadata kept: %s", second.Body)
	}
}
