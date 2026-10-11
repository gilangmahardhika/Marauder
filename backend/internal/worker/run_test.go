package worker_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/worker"
)

func cardCount(t *testing.T, db *sql.DB, project string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM cards WHERE project_id = $1`, project).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func acceptRow(t *testing.T, db *sql.DB, id, project string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO runs (id, trigger, created_at, project_id) VALUES ($1, 'accept', '2026-10-11T00:00:00Z', $2)`,
		id, project); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func startWorker(t *testing.T, db *sql.DB, data string) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		worker.Run(ctx, db, data, 20*time.Millisecond, t.Logf)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("worker did not stop")
		}
	})
	return cancel
}

func TestWorkerCreatesCardsAfterAccept(t *testing.T) {
	db := boardtest.Open(t)
	data := dataDir(t)
	insertRequirement(t, db, testProject, "REQ-001", "Alpha", true, `["line"]`, "")
	acceptRow(t, db, "run_accept000001", testProject)
	startWorker(t, db, data)

	waitFor(t, "a ready card", func() bool {
		var n int
		_ = db.QueryRow(`SELECT count(*) FROM cards WHERE project_id = $1 AND status = 'ready'`, testProject).Scan(&n)
		return n == 1
	})
	waitFor(t, "the run row to be processed", func() bool {
		open, err := worker.Pending(context.Background(), db, testProject)
		return err == nil && !open
	})
	if n := cardCount(t, db, testProject); n != 1 {
		t.Fatalf("cards = %d, want 1", n)
	}
}

func TestProjectLockSkips(t *testing.T) {
	db := boardtest.Open(t)
	data := dataDir(t)
	insertProject(t, db, "proj_other")
	insertRequirement(t, db, testProject, "REQ-001", "Locked", true, `["line"]`, "")
	insertRequirement(t, db, "proj_other", "REQ-001", "Free", true, `["line"]`, "")
	acceptRow(t, db, "run_accept000001", testProject)
	acceptRow(t, db, "run_accept000002", "proj_other")
	lock, err := worker.LockProject(data, testProject)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	startWorker(t, db, data)

	waitFor(t, "a card in the free project", func() bool { return cardCount(t, db, "proj_other") == 1 })
	time.Sleep(100 * time.Millisecond)
	if n := cardCount(t, db, testProject); n != 0 {
		t.Fatalf("locked project got %d cards", n)
	}
}
