package board_test

import (
	"context"
	"errors"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
)

func TestHandoffAppliedOnce(t *testing.T) {
	ctx := context.Background()
	db := boardtest.Open(t)
	card, err := board.InsertCard(ctx, db, board.Card{ProjectID: "proj_default", Status: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	attempt := func(n int) string {
		id, err := board.NewID("att_")
		if err != nil {
			t.Fatal(err)
		}
		if err := board.InsertAttempt(ctx, db, board.Attempt{ID: id, CardID: card.ID, N: n, TaskClass: "implement"}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := attempt(1)
	id, err := board.SaveHandoff(ctx, db, board.Handoff{
		CardID: card.ID, AttemptID: first, Summary: "Done: read main.go", SummarySource: "model",
		Todos: []board.Todo{{Text: "fix the handler", Status: "in_progress"}}, Patch: "diff --git a/x b/x\n", PatchFiles: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := board.SaveHandoff(ctx, db, board.Handoff{CardID: card.ID, AttemptID: first, SummarySource: "harness"}); !errors.Is(err, board.ErrConflict) {
		t.Fatalf("second save = %v, want a conflict", err)
	}
	h, ok, err := board.PendingHandoff(ctx, db, card.ID)
	if err != nil || !ok || h.ID != id || h.ProjectID != "proj_default" || len(h.Todos) != 1 || h.Todos[0].Text != "fix the handler" {
		t.Fatalf("pending = %+v %v %v", h, ok, err)
	}
	if err := board.MarkHandoffApplied(ctx, db, "card_000000000000", id, true, ""); !errors.Is(err, board.ErrConflict) {
		t.Fatalf("mark from another card = %v", err)
	}
	if err := board.MarkHandoffApplied(ctx, db, card.ID, id, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := board.MarkHandoffApplied(ctx, db, card.ID, id, true, ""); !errors.Is(err, board.ErrConflict) {
		t.Fatalf("second mark = %v, want a conflict", err)
	}
	if _, ok, err := board.PendingHandoff(ctx, db, card.ID); err != nil || ok {
		t.Fatalf("an applied handoff is still pending: %v %v", ok, err)
	}

	second := attempt(2)
	if _, err := board.SaveHandoff(ctx, db, board.Handoff{CardID: card.ID, AttemptID: second, SummarySource: "harness"}); err != nil {
		t.Fatal(err)
	}
	attempt(3)
	if _, ok, err := board.PendingHandoff(ctx, db, card.ID); err != nil || ok {
		t.Fatalf("the handoff of an older attempt is pending: %v %v", ok, err)
	}
}
