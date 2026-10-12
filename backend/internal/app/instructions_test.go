package app_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/settings"
)

func currentInstructions(t *testing.T, db *sql.DB, cardID string) string {
	t.Helper()
	text, err := board.CurrentInstructions(context.Background(), db, cardID)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func savedEvents(t *testing.T, db *sql.DB, cardID string) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT coalesce(actor_id, '') || ' ' || payload_json FROM events
		WHERE card_id = $1 AND kind = 'instructions_saved'`, cardID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		out = append(out, line)
	}
	return out
}

func invalidField(err error, field string) bool {
	var invalid *settings.Invalid
	return errors.As(err, &invalid) && invalid.Fields[field] != ""
}

func TestSetInstructions(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	ada := githubUser(t, db, "ada@example.com", "", 0, "")
	card, err := board.InsertCard(ctx, db, board.Card{ProjectID: "proj_default", Status: "changes_requested"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.SetInstructions(ctx, db, "proj_default", card.ID, ada.ID, "  Use the lint script.\r\nKeep tsc.  "); err != nil {
		t.Fatal(err)
	}
	if got := currentInstructions(t, db, card.ID); got != "Use the lint script.\nKeep tsc." {
		t.Fatalf("instructions = %q", got)
	}
	if n := countNotes(t, db, card.ID, "instructions"); n != 1 {
		t.Fatalf("instructions notes = %d, want 1", n)
	}
	if events := savedEvents(t, db, card.ID); len(events) != 1 || events[0] != ada.ID+` {"length":30}` {
		t.Fatalf("events = %q", events)
	}

	if _, err := app.SetInstructions(ctx, db, "proj_default", card.ID, ada.ID, "Use the lint script.\nKeep tsc."); err != nil {
		t.Fatal(err)
	}
	if events := savedEvents(t, db, card.ID); len(events) != 1 {
		t.Fatalf("unchanged text wrote events %q", events)
	}

	_, err = app.SetInstructions(ctx, db, "proj_default", card.ID, ada.ID, strings.Repeat("é", 4001))
	if !invalidField(err, "instructions") {
		t.Fatalf("long text = %v, want invalid instructions", err)
	}
	if _, err := app.SetInstructions(ctx, db, "proj_default", card.ID, ada.ID, strings.Repeat("é", 4000)); err != nil {
		t.Fatalf("4000 characters = %v", err)
	}

	if _, err := app.SetInstructions(ctx, db, "proj_default", card.ID, ada.ID, " \n "); err != nil {
		t.Fatal(err)
	}
	if got := currentInstructions(t, db, card.ID); got != "" {
		t.Fatalf("cleared instructions = %q", got)
	}
	if events := savedEvents(t, db, card.ID); len(events) != 3 || !slices.Contains(events, ada.ID+` {"length":0}`) {
		t.Fatalf("events = %q", events)
	}

	if _, err := app.SetInstructions(ctx, db, "proj_other", card.ID, ada.ID, "x"); !errors.Is(err, board.ErrNotFound) {
		t.Fatalf("other project = %v, want ErrNotFound", err)
	}
	if _, err := db.Exec(`UPDATE cards SET status = 'done' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetInstructions(ctx, db, "proj_default", card.ID, ada.ID, "x"); !errors.Is(err, app.ErrCardDone) {
		t.Fatalf("done card = %v, want ErrCardDone", err)
	}
}

func TestRunWithInstructions(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	ada := githubUser(t, db, "ada@example.com", "", 0, "")
	card, err := board.InsertCard(ctx, db, board.Card{ProjectID: "proj_default", Status: "changes_requested"})
	if err != nil {
		t.Fatal(err)
	}
	open := func() int {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM runs WHERE card_id = $1 AND processed_at IS NULL`, card.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	long := strings.Repeat("x", 4001)
	if _, err := app.QueueCardRun(ctx, db, "proj_default", card.ID, &long, ada.ID); !invalidField(err, "instructions") {
		t.Fatalf("long text = %v, want invalid instructions", err)
	}
	if open() != 0 || currentInstructions(t, db, card.ID) != "" {
		t.Fatal("a refused run wrote a row")
	}

	text := "Use the lint script."
	if _, err := app.QueueCardRun(ctx, db, "proj_default", card.ID, &text, ada.ID); err != nil {
		t.Fatal(err)
	}
	if open() != 1 || currentInstructions(t, db, card.ID) != text || len(savedEvents(t, db, card.ID)) != 1 {
		t.Fatalf("run with instructions: open %d, instructions %q", open(), currentInstructions(t, db, card.ID))
	}

	if _, err := db.Exec(`UPDATE runs SET processed_at = '2026-10-12T00:00:00Z' WHERE card_id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.QueueCardRun(ctx, db, "proj_default", card.ID, nil, ada.ID); err != nil {
		t.Fatal(err)
	}
	if currentInstructions(t, db, card.ID) != text || len(savedEvents(t, db, card.ID)) != 1 {
		t.Fatal("a run without instructions changed them")
	}

	backlog, err := board.InsertCard(ctx, db, board.Card{ProjectID: "proj_default", Status: "backlog"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.QueueCardRun(ctx, db, "proj_default", backlog.ID, &text, ada.ID); !errors.Is(err, app.ErrNotRunnable) {
		t.Fatalf("backlog run = %v, want ErrNotRunnable", err)
	}
	if currentInstructions(t, db, backlog.ID) != "" {
		t.Fatal("a refused run saved instructions")
	}
}

// firstPrompt runs one implement attempt with a review note and the given
// instructions saves, and returns the first prompt the model saw.
func firstPrompt(t *testing.T, saves ...string) fake.Prompt {
	t.Helper()
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	ctx := context.Background()
	if err := board.InsertFailureNote(ctx, f.db, f.card.ID, "att_review", "review", "Rename the greeting."); err != nil {
		t.Fatal(err)
	}
	for _, text := range saves {
		if _, err := app.SetInstructions(ctx, f.db, "proj_default", f.card.ID, "", text); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := fake.Load([]byte(commitScript))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImplementCard(ctx, f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID); err != nil {
		t.Fatal(err)
	}
	prompts := provider.Prompts()
	if len(prompts) == 0 {
		t.Fatal("no model call")
	}
	return prompts[0]
}

func TestAttemptReadsInstructionsAndNote(t *testing.T) {
	t.Run("instructions and note", func(t *testing.T) {
		prompt := firstPrompt(t, "Keep the file under ten lines.")
		block := contextpack.InstructionsHeading + "\nKeep the file under ten lines.\n" + contextpack.InstructionsEnd
		if !strings.Contains(prompt.User, "Rename the greeting.") || !strings.Contains(prompt.User, block) {
			t.Fatalf("prompt lacks the note or the instructions:\n%s", prompt.User)
		}
		if !strings.Contains(prompt.System, contextpack.InstructionsHeading) {
			t.Fatalf("system does not explain the block:\n%s", prompt.System)
		}
	})
	t.Run("cleared", func(t *testing.T) {
		prompt := firstPrompt(t, "Keep the file under ten lines.", "")
		if !strings.Contains(prompt.User, "Rename the greeting.") || strings.Contains(prompt.User, contextpack.InstructionsHeading) {
			t.Fatalf("cleared instructions still in the prompt:\n%s", prompt.User)
		}
	})
}
