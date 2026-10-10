package deploy_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/deploy"
	"orchestrator/backend/internal/verify"
)

const sha = "0123456789abcdef0123456789abcdef01234567"

func TestDeployFailed(t *testing.T) {
	db := boardtest.Open(t)
	card := cardWith(t, db, "deploying_dev")
	rollbacks := 0
	next, err := deploy.Run(context.Background(), deploy.Job{
		DB: db, Card: card, Command: "/opt/deploy", Rollback: "/opt/rollback",
		Exec: func(argv []string) int {
			if argv[0] == "/opt/deploy" {
				return 1
			}
			return 0
		},
		RollbackN: &rollbacks,
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "blocked" || next.BlockedReason != "deploy_failed" || rollbacks != 1 {
		t.Fatalf("card=%+v rollbacks=%d", next, rollbacks)
	}
}

func TestHealthFail(t *testing.T) {
	db := boardtest.Open(t)
	card := cardWith(t, db, "deploying_dev")
	rollbacks := 0
	next, err := deploy.Run(context.Background(), deploy.Job{
		DB: db, Card: card, Command: "/opt/deploy", Rollback: "/opt/rollback",
		Exec:      func([]string) int { return 0 },
		ProbeFn:   func() error { return errors.New("500") },
		RollbackN: &rollbacks,
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "blocked" || next.BlockedReason != "health" || rollbacks != 1 {
		t.Fatalf("card=%+v rollbacks=%d", next, rollbacks)
	}
	var verifies int
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'verify'`, card.ID).Scan(&verifies); err != nil {
		t.Fatal(err)
	}
	if verifies != 0 {
		t.Fatalf("verify events = %d", verifies)
	}
}

func TestHealthOnly(t *testing.T) {
	db := boardtest.Open(t)
	card := cardWith(t, db, "verifying")
	next, err := verify.HealthOnly(context.Background(), db, card)
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "done" {
		t.Fatalf("status = %s err path", next.Status)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'verify'`, card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, "health_only") {
		t.Fatalf("payload = %s", payload)
	}
}

func TestDevFailThenPass(t *testing.T) {
	db := boardtest.Open(t)
	card := cardWith(t, db, "verifying")
	if _, err := db.Exec(`UPDATE cards SET pr_url = 'https://github.com/acme/widget/pull/1' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	card, err := board.GetCard(context.Background(), db, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := verify.Finish(context.Background(), db, card, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "changes_requested" || failed.PRURL != card.PRURL {
		t.Fatalf("failed = %+v", failed)
	}
	var notes int
	if err := db.QueryRow(`SELECT count(*) FROM failure_notes WHERE card_id = $1 AND source = 'dev_check'`, card.ID).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	if notes != 1 {
		t.Fatalf("notes = %d", notes)
	}
	again := cardWith(t, db, "verifying")
	passed, err := verify.Finish(context.Background(), db, again, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if passed.Status != "done" {
		t.Fatalf("status = %s", passed.Status)
	}
}

func TestReconcileDeploy(t *testing.T) {
	db := boardtest.Open(t)
	card := cardWith(t, db, "deploying_dev")
	if _, err := db.Exec(`INSERT INTO events (id, ts, card_id, kind, payload_json) VALUES ('evt_0123456789ab', '2026-01-01T00:00:00Z', $1, 'deploy', '{}')`, card.ID); err != nil {
		t.Fatal(err)
	}
	deploys := 7
	if err := deploy.Reconcile(context.Background(), db, card, &deploys, func([]string) int { return 0 }); err != nil {
		t.Fatal(err)
	}
	if deploys != 0 {
		t.Fatalf("deploy execs = %d", deploys)
	}
	var healths int
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'health'`, card.ID).Scan(&healths); err != nil {
		t.Fatal(err)
	}
	if healths != 1 {
		t.Fatalf("health = %d", healths)
	}
}

func cardWith(t *testing.T, db *sql.DB, status string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: status, Title: status})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cards SET approved_sha = $2, deployed_sha = $2 WHERE id = $1`, card.ID, sha); err != nil {
		t.Fatal(err)
	}
	got, err := board.GetCard(context.Background(), db, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
