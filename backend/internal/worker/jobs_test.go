package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/worker"
)

// gatedJobs records started cards and holds each job until release is closed.
type gatedJobs struct {
	mu      sync.Mutex
	started []string
	release chan struct{}
}

func (g *gatedJobs) jobs() worker.Jobs {
	return worker.Jobs{Implement: func(ctx context.Context, projectID, cardID string) (string, error) {
		g.mu.Lock()
		g.started = append(g.started, cardID)
		g.mu.Unlock()
		select {
		case <-g.release:
		case <-ctx.Done():
		}
		return "pr_open", nil
	}}
}

func (g *gatedJobs) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.started)
}

func TestPassStartsQueuedCardsUnderCap(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	data := dataDir(t)
	if err := os.WriteFile(filepath.Join(data, "projects", testProject, "config.yaml"), []byte("scheduler:\n  implementationCap: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i, at := range []string{"2026-10-11T00:00:01Z", "2026-10-11T00:00:02Z", "2026-10-11T00:00:03Z"} {
		card, err := board.InsertCard(ctx, db, board.Card{Title: "queued", Status: "ready"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, card.ID)
		if _, err := db.Exec(`
			INSERT INTO runs (id, trigger, created_at, project_id, card_id) VALUES ($1, 'api', $2, $3, $4)`,
			"run_card0000000"+string(rune('a'+i)), at, testProject, card.ID); err != nil {
			t.Fatal(err)
		}
	}
	gate := &gatedJobs{release: make(chan struct{})}
	slots := worker.NewSlots()
	started, err := worker.StartQueued(ctx, db, data, testProject, gate.jobs(), slots, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if started != 2 {
		t.Fatalf("started = %d, want the cap of 2", started)
	}
	waitFor(t, "two running jobs", func() bool { return gate.count() == 2 })
	var open int
	if err := db.QueryRow(`SELECT count(*) FROM runs WHERE card_id IS NOT NULL AND processed_at IS NULL`).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 1 {
		t.Fatalf("open card requests = %d, want 1", open)
	}
	again, err := worker.StartQueued(ctx, db, data, testProject, gate.jobs(), slots, t.Logf)
	if err != nil || again != 0 {
		t.Fatalf("start at cap = %d %v", again, err)
	}
	close(gate.release)
	slots.Wait()
	last, err := worker.StartQueued(ctx, db, data, testProject, gate.jobs(), slots, t.Logf)
	if err != nil || last != 1 {
		t.Fatalf("start after release = %d %v", last, err)
	}
	slots.Wait()
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if len(gate.started) != 3 || gate.started[2] != ids[2] {
		t.Fatalf("started = %v, want the oldest first and %s last", gate.started, ids[2])
	}
}

func TestPassSkipsUnqueuedReady(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	data := dataDir(t)
	if _, err := board.InsertCard(ctx, db, board.Card{Title: "ready only", Status: "ready"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO runs (id, trigger, created_at, project_id) VALUES ('run_accept000001', 'accept', '2026-10-11T00:00:00Z', $1)`, testProject); err != nil {
		t.Fatal(err)
	}
	gate := &gatedJobs{release: make(chan struct{})}
	close(gate.release)
	startWorkerJobs(t, db, data, gate.jobs())
	waitFor(t, "the project request to be processed", func() bool {
		open, err := worker.Wake(ctx, db, testProject)
		return err == nil && !open
	})
	time.Sleep(100 * time.Millisecond)
	if n := gate.count(); n != 0 {
		t.Fatalf("started %d jobs for a ready card nobody queued", n)
	}
}
