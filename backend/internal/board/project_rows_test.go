package board_test

import (
	"context"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
)

func TestCardRowsKeepCardProject(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	if _, err := db.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_gadget', 'tribe_default', 'Gadget', '/tmp/gadget', '2026-10-12T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(ctx, db, board.Card{ProjectID: "proj_gadget", Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	attempt := board.Attempt{ID: "att_gadget00001", CardID: card.ID, N: 1, TaskClass: "implement", ModelID: "m", MaxTurns: 1}
	if err := board.InsertAttempt(ctx, db, attempt); err != nil {
		t.Fatal(err)
	}
	if err := board.AcquireLock(ctx, db, card.ID, "worker"); err != nil {
		t.Fatal(err)
	}
	if err := board.InsertFailureNote(ctx, db, card.ID, attempt.ID, "local_check", "exit 1"); err != nil {
		t.Fatal(err)
	}
	if err := board.InsertGate(ctx, db, board.Gate{CardID: card.ID, AttemptID: attempt.ID, Name: "local_checks", Passed: true}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"attempts", "locks", "failure_notes", "gates"} {
		var project string
		if err := db.QueryRow(`SELECT project_id FROM `+table+` WHERE card_id = $1`, card.ID).Scan(&project); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if project != "proj_gadget" {
			t.Errorf("%s project_id = %q, want proj_gadget", table, project)
		}
	}
}
