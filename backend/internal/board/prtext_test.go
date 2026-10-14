package board_test

import (
	"context"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
)

func TestLatestPRText(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	card, err := board.InsertCard(ctx, db, board.Card{ProjectID: "proj_default", Status: "pr_open"})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := board.LatestPRText(ctx, db, "proj_default", card.ID)
	if err != nil || empty.Title != "" {
		t.Fatalf("before publish = %+v, %v", empty, err)
	}
	for n, id := range []string{"att_prtext00001", "att_prtext00002", "att_prtext00003"} {
		if err := board.InsertAttempt(ctx, db, board.Attempt{ID: id, CardID: card.ID, N: n + 1, TaskClass: "implement", ModelID: "m", MaxTurns: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := board.SetAttemptPRText(ctx, db, "att_prtext00001", "First", "one"); err != nil {
		t.Fatal(err)
	}
	if err := board.SetAttemptPRText(ctx, db, "att_prtext00002", "Second", "two"); err != nil {
		t.Fatal(err)
	}
	got, err := board.LatestPRText(ctx, db, "proj_default", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Second" || got.Body != "two" || got.AttemptN != 2 {
		t.Fatalf("latest = %+v, want the newest attempt with text", got)
	}
	other, err := board.LatestPRText(ctx, db, "proj_other", card.ID)
	if err != nil || other.Title != "" {
		t.Fatalf("other project = %+v, %v", other, err)
	}
}
