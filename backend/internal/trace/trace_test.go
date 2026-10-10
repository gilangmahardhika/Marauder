package trace_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/trace"
)

func TestRedact(t *testing.T) {
	db := boardtest.Open(t)
	const token = "fixture-deploy-token"
	ctx := context.Background()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	id, err := trace.Append(ctx, tx, trace.Event{
		Kind: "transition",
		Payload: map[string]string{
			"evidence": "deploy " + token + " end",
		},
	}, []string{token, ""})
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var payload string
	if err := db.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE id = $1`, id).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, token) {
		t.Fatalf("payload still contains the deploy token: %s", payload)
	}
	if !strings.Contains(payload, "[redacted]") {
		t.Fatalf("payload = %s, want [redacted]", payload)
	}
}
