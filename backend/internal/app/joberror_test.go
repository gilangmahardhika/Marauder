package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/trace"
)

func TestJobErrorCapped(t *testing.T) {
	ctx := context.Background()
	db := boardtest.Open(t)
	card, err := board.InsertCard(ctx, db, board.Card{Status: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	message := "git exit 125: head of the error " + strings.Repeat("pax\x00\x00\xff", 1<<18) + " fatal: tail of the error"
	if err := app.JobError(ctx, db, card, "", message, []string{"tail of"}); err != nil {
		t.Fatal(err)
	}
	events, err := trace.List(ctx, db, card.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %v %v", events, err)
	}
	stored := events[0].Payload
	if len(stored) > 10<<10 {
		t.Fatalf("stored payload is %d bytes", len(stored))
	}
	var payload struct{ Message string }
	if err := json.Unmarshal([]byte(stored), &payload); err != nil {
		t.Fatal(err)
	}
	got := payload.Message
	if len(got) > app.JobErrorCap+64 || !utf8.ValidString(got) || strings.ContainsRune(got, 0) {
		t.Fatalf("message is %d bytes, valid %v", len(got), utf8.ValidString(got))
	}
	if !strings.HasPrefix(got, "git exit 125: head of the error") || !strings.HasSuffix(got, "fatal: [redacted] the error") ||
		!strings.Contains(got, "bytes cut) ...") {
		t.Fatalf("message head %q tail %q", got[:40], got[len(got)-40:])
	}
}
