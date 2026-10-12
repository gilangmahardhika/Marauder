package trace_test

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/trace"
)

func TestTraceClip(t *testing.T) {
	const secret = "postgres://u:p&ss@db/board"
	if got := trace.Clip("short\x00 text "+secret, []string{secret}, 64); got != "short text [redacted]" {
		t.Fatalf("short = %q", got)
	}
	long := "HEAD " + secret + " " + strings.Repeat("ü", 2000) + "\xff" + strings.Repeat("ß", 2000) + " TAIL"
	got := trace.Clip(long, []string{secret}, 1024)
	if !utf8.ValidString(got) || strings.Contains(got, "p&ss") || len(got) > 1024+64 {
		t.Fatalf("clip is %d bytes, valid %v: %q", len(got), utf8.ValidString(got), got[:80])
	}
	if !strings.HasPrefix(got, "HEAD [redacted] ü") || !strings.HasSuffix(got, "ß TAIL") || !strings.Contains(got, "bytes cut) ...") {
		t.Fatalf("head %q tail %q", got[:30], got[len(got)-30:])
	}
	// A secret across the cut is still redacted, because the cut comes last.
	across := strings.Repeat("a", 500) + secret + strings.Repeat("b", 500)
	if got := trace.Clip(across, []string{secret}, 1000); strings.Contains(got, "p&s") || strings.Contains(got, "postgres") {
		t.Fatalf("secret across the cut survived: %q", got)
	}
}

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
