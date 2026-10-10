package board_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/trace"
)

func TestEventsImmutable(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	id, err := trace.Append(ctx, tx, trace.Event{
		Kind:    "transition",
		Payload: map[string]string{"from": "ready", "to": "in_progress", "evidence": "recorded"},
	}, nil)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	_, err = db.ExecContext(ctx, `UPDATE events SET payload_json = '{}' WHERE id = $1`, id)
	if err == nil || !strings.Contains(err.Error(), "events are insert-only") {
		t.Fatalf("UPDATE err = %v, want events are insert-only", err)
	}
	_, err = db.ExecContext(ctx, `DELETE FROM events WHERE id = $1`, id)
	if err == nil || !strings.Contains(err.Error(), "events are insert-only") {
		t.Fatalf("DELETE err = %v, want events are insert-only", err)
	}
}
