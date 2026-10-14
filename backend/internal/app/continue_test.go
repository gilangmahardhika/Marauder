package app_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
)

func TestUnblockContinuesWork(t *testing.T) {
	f := newJobFixture(t, "true", 1, routerConfig, fixtureProject)
	card, _ := f.runWith(t, `{"implement":[
		{"tool":"todo_write","items":[{"text":"edit hello.txt","status":"done"},{"text":"commit","status":"pending"}]},
		{"tool":"edit_file","path":"hello.txt","text":"partial\n"},
		{"text":"pausing"},{"text":"still pausing"},{"text":"stopping"},
		{"text":"Goal: say partial\nDone: hello.txt says partial\nNext steps: commit"}]}`)
	if card.Status != "blocked" || card.BlockedReason != "attempt_cap" {
		t.Fatalf("card = %s %s, want blocked at the attempt cap", card.Status, card.BlockedReason)
	}
	var source string
	var files int
	if err := f.db.QueryRow(`SELECT summary_source, patch_files FROM attempt_handoffs WHERE card_id = $1`, card.ID).Scan(&source, &files); err != nil {
		t.Fatalf("handoff row: %v", err)
	}
	if source != "model" || files != 1 {
		t.Fatalf("handoff = %s with %d files", source, files)
	}

	if _, err := app.Unblock(context.Background(), f.db, card.ID, false); err != nil {
		t.Fatal(err)
	}
	card, provider := f.runWith(t, `{"implement_hard":[{"tool":"commit","message":"Say partial"}]}`)
	if card.Status != "pr_open" || card.MaxAttempts != 2 {
		t.Fatalf("card = %s %s max %d, want pr_open after one more attempt", card.Status, card.BlockedReason, card.MaxAttempts)
	}
	first := provider.Prompts()[0]
	for _, want := range []string{"WORK SO FAR", "Done: hello.txt says partial", "[pending] commit", "is in the worktree: 1 files"} {
		if !strings.Contains(first.User, want) {
			t.Fatalf("the next attempt's pack lacks %q:\n%s", want, first.User)
		}
	}
	out, err := exec.Command(lookGit(t), "-C", f.repo, "show", "refs/heads/orch/"+card.ID+":hello.txt").Output()
	if err != nil || string(out) != "partial\n" {
		t.Fatalf("committed hello.txt = %q %v, want the last attempt's edit", out, err)
	}
	var applied bool
	if err := f.db.QueryRow(`SELECT applied_ok FROM attempt_handoffs WHERE card_id = $1`, card.ID).Scan(&applied); err != nil || !applied {
		t.Fatalf("handoff applied = %v %v", applied, err)
	}
	if got := payloads(t, f, "handoff_applied"); len(got) != 1 || got[0]["ok"] != true {
		t.Fatalf("handoff_applied = %v", got)
	}
}
