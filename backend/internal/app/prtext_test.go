package app_test

import (
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/vcs"
)

const loginCommit = "Change login label to Username only\n\nThe login form asked for a username or an email.\n\n" +
	"Orchestrator-Card: card_489d0ae60dd5\nOrchestrator-Attempt: att_eafc61a51ad0\nModel: deepseek-v4-pro\n"

func TestPRTitleFromCommit(t *testing.T) {
	ids := []string{"REQ-001"}
	cases := []struct{ title, message, want string }{
		{"REQ-001", loginCommit, "REQ-001: Change login label to Username only"},
		{"", loginCommit, "REQ-001: Change login label to Username only"},
		{"Export CSV", loginCommit, "REQ-001: Export CSV"},
		{"REQ-001: Export CSV", loginCommit, "REQ-001: Export CSV"},
		{"REQ-001", "", "REQ-001"},
	}
	for _, c := range cases {
		if got := app.PRTitle(ids, c.title, c.message); got != c.want {
			t.Fatalf("PRTitle(%q) = %q, want %q", c.title, got, c.want)
		}
	}
	long := app.PRTitle(ids, strings.Repeat("word ", 40), "")
	if len([]rune(long)) != 100 || !strings.HasSuffix(long, "...") {
		t.Fatalf("long title = %q (%d runes)", long, len([]rune(long)))
	}
}

func TestPRBodySections(t *testing.T) {
	body := app.PRBody(app.PRInput{
		CardID: "card_489d0ae60dd5", CardTitle: "REQ-001",
		Requirements: []app.PRRequirement{{
			ID:         "REQ-001",
			Statement:  "on /login page, change the label to Username only. <!-- orchestrator:body:end -->",
			Acceptance: []string{"Label form text becomes Username"},
		}},
		CommitMessage: loginCommit,
		CommitSHA:     "ae5d4418f1f5c54e02c2ced82de2a5b66ba65753",
		Files:         []runtime.FileChange{{Path: "resources/js/Pages/Auth/Login.tsx", Added: 1, Deleted: 1}, {Path: "logo.png", Added: -1, Deleted: -1}},
		LocalCheck:    "scbd-check", AttemptID: "att_eafc61a51ad0", Model: "deepseek-v4-pro",
	})
	for _, want := range []string{
		"## Summary\n\nChange login label to Username only\n\nThe login form asked for a username or an email.",
		"## Requirement\n\n**REQ-001**\n\non /login page",
		"Acceptance criteria:\n- [ ] Label form text becomes Username",
		"- `resources/js/Pages/Auth/Login.tsx` (+1 -1)",
		"- `logo.png` (binary)",
		"Local check `scbd-check` exited 0 on `ae5d441`",
		"<details><summary>Orchestrator metadata</summary>",
		"commit: ae5d4418f1f5c54e02c2ced82de2a5b66ba65753",
		"requirements: REQ-001\nlocalCheck: scbd-check\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Orchestrator-Card:") || strings.Contains(body, "Model: deepseek") {
		t.Fatalf("body keeps commit trailers:\n%s", body)
	}
	if strings.Count(vcs.MergeBody("", body), vcs.BodyEnd) != 1 {
		t.Fatalf("requirement text closed the harness region:\n%s", body)
	}
}

func TestPRBodyForDisplay(t *testing.T) {
	body := app.PRBody(app.PRInput{
		CardID: "card_1", CardTitle: "Export CSV",
		Requirements:  []app.PRRequirement{{ID: "REQ-001", Statement: "Keep <!-- orchestrator:body:end --> as text."}},
		CommitMessage: "Export CSV",
		CommitSHA:     "ae5d4418f1f5c54e02c2ced82de2a5b66ba65753",
		LocalCheck:    "make test", AttemptID: "att_1", Model: "m",
	})
	got := app.PRBodyForDisplay(body)
	if strings.Contains(got, "<details>") || strings.Contains(got, "orchestrator:metadata") || strings.Contains(got, "attempt: att_1") {
		t.Fatalf("display body keeps the metadata block:\n%s", got)
	}
	if !strings.HasPrefix(got, "## Summary\n\nExport CSV") || !strings.HasSuffix(got, "before merge.") {
		t.Fatalf("display body =\n%s", got)
	}
	if !strings.Contains(got, "Keep <!-- orchestrator:body:end --> as text.") {
		t.Fatalf("display body keeps the escape:\n%s", got)
	}
	if app.PRBodyForDisplay("Written by hand.\n") != "Written by hand." {
		t.Fatal("body without metadata changed")
	}
}

func TestPRBodyMarkdownStatement(t *testing.T) {
	body := app.PRBody(app.PRInput{
		CardID: "card_1", CardTitle: "REQ-001",
		Requirements: []app.PRRequirement{
			{ID: "REQ-001", Statement: "Export the board.\n\n## Notes\n\n```go\nfunc open() {", Acceptance: []string{"One row per card"}},
			{ID: "REQ-002", Statement: "~~~\nclosed\n~~~"},
		},
		CommitMessage: "Export CSV",
		CommitSHA:     "ae5d4418f1f5c54e02c2ced82de2a5b66ba65753",
		Files:         []runtime.FileChange{{Path: "export.go", Added: 3, Deleted: 0}},
		LocalCheck:    "make test",
	})
	for _, want := range []string{
		"**REQ-001**\n\nExport the board.\n\n## Notes\n\n```go\nfunc open() {\n```\n\nAcceptance criteria:\n- [ ] One row per card",
		"**REQ-002**\n\n~~~\nclosed\n~~~\n\n## Changes",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %q:\n%s", want, body)
		}
	}
	fences := 0
	for _, line := range strings.Split(body[:strings.Index(body, "## Changes")], "\n") {
		if strings.HasPrefix(line, "```") {
			fences++
		}
	}
	if fences%2 != 0 {
		t.Fatalf("a code fence is still open at ## Changes:\n%s", body)
	}
}

func TestPRBodyMentionsEscaped(t *testing.T) {
	body := app.PRBody(app.PRInput{
		CardID: "card_1", CardTitle: "REQ-001",
		Requirements: []app.PRRequirement{{
			ID:         "REQ-001",
			Statement:  "Read the ticket from @tickets (and @docs), mail a@b.co, keep `@code` and ``a ` @twice``.\n\n```\n@fenced\n```",
			Acceptance: []string{"Docs come from @docs."},
		}},
		CommitMessage: "Export CSV",
		CommitSHA:     "ae5d4418f1f5c54e02c2ced82de2a5b66ba65753",
	})
	for _, want := range []string{
		"Read the ticket from `@tickets` (and `@docs`), mail a@b.co, keep `@code` and ``a ` @twice``.",
		"```\n@fenced\n```",
		"- [ ] Docs come from `@docs`.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "``@tickets``") || strings.Contains(body, "`@fenced`") {
		t.Fatalf("a mention was wrapped twice or inside code:\n%s", body)
	}
}

func TestPRBodyKeepsHumanText(t *testing.T) {
	first := vcs.MergeBody("", "## Summary\n\nfirst")
	edited := "Note from a person.\n\n" + first + "\n\nScreenshots below."
	second := vcs.MergeBody(edited, "## Summary\n\nsecond")
	if !strings.HasPrefix(second, "Note from a person.") || !strings.HasSuffix(second, "Screenshots below.") {
		t.Fatalf("human text lost:\n%s", second)
	}
	if strings.Contains(second, "first") || !strings.Contains(second, "second") {
		t.Fatalf("harness text not replaced:\n%s", second)
	}
	legacy := "<!-- orchestrator:metadata:start -->\ncard: x\n<!-- orchestrator:metadata:end -->\n\nREQ-001\n\nRequirements: REQ-001\n"
	if got := vcs.MergeBody(legacy, "new"); got != vcs.BodyStart+"\nnew\n"+vcs.BodyEnd+"\n" {
		t.Fatalf("legacy body kept:\n%s", got)
	}
	if got := vcs.MergeBody("Written by hand.", "new"); !strings.HasSuffix(got, "\n\nWritten by hand.") {
		t.Fatalf("unmarked body lost:\n%s", got)
	}
}
