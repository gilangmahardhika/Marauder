package worker

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"orchestrator/backend/internal/settings"
)

const defaultImplementationCap = 2

// Jobs are the job runners the worker starts. worker cannot import app, so
// serve and run-next pass them in. A nil runner starts no job of its kind.
type Jobs struct {
	Implement func(ctx context.Context, projectID, cardID string) (status string, err error)
	Review    func(ctx context.Context, projectID, cardID string) (status string, err error)
	Refresh   func(ctx context.Context, projectID, cardID string) (status string, err error)
	// Reap ends expired runner leases and returns how many it ended. When nil
	// the worker ends them on the board alone.
	Reap func(ctx context.Context) (int, error)
}

// Slots tracks one project's implement jobs running in this process.
type Slots struct {
	mu      sync.Mutex
	running map[string]bool
	wg      sync.WaitGroup
}

func NewSlots() *Slots {
	return &Slots{running: map[string]bool{}}
}

func (s *Slots) Running() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}

// Wait returns when every job started through these slots has returned.
func (s *Slots) Wait() {
	s.wg.Wait()
}

func (s *Slots) take(cardID string, limit int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[cardID] || len(s.running) >= limit {
		return false
	}
	s.running[cardID] = true
	s.wg.Add(1)
	return true
}

func (s *Slots) release(cardID string) {
	s.mu.Lock()
	delete(s.running, cardID)
	s.mu.Unlock()
	s.wg.Done()
}

// StartQueued starts an implement job for each open card-scoped run request
// of the project (RFC-061), oldest first, while the project's running jobs are
// under scheduler.implementationCap (RFC-063). A request is marked processed
// when its job starts. Ready cards without a request are not started. When
// jobs.executor is runners the requests are left for runners to claim.
func StartQueued(ctx context.Context, db *sql.DB, dataRoot, projectID string, jobs Jobs, slots *Slots, logf func(string, ...any)) (int, error) {
	if jobs.Implement == nil {
		return 0, nil
	}
	if dataRoot != "" {
		cfg, _, err := settings.Load(dataRoot)
		if err != nil {
			return 0, err
		}
		if cfg.Executor() == settings.ExecutorRunners {
			return 0, nil
		}
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	limit, err := implementationCap(dataRoot, projectID)
	if err != nil {
		return 0, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT r.id, r.card_id FROM runs r
		WHERE r.project_id = $1 AND r.card_id IS NOT NULL AND r.processed_at IS NULL
		ORDER BY r.created_at, r.id`, projectID)
	if err != nil {
		return 0, err
	}
	type request struct{ id, cardID string }
	var queued []request
	for rows.Next() {
		var req request
		if err := rows.Scan(&req.id, &req.cardID); err != nil {
			rows.Close()
			return 0, err
		}
		queued = append(queued, req)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	started := 0
	for _, req := range queued {
		if !slots.take(req.cardID, limit) {
			continue
		}
		claimed, err := claim(ctx, db, req.id)
		if err != nil || !claimed {
			slots.release(req.cardID)
			if err != nil {
				return started, err
			}
			continue
		}
		started++
		logf("worker: card %s started", req.cardID)
		go func(cardID string) {
			defer slots.release(cardID)
			status, err := jobs.Implement(ctx, projectID, cardID)
			if err != nil && ctx.Err() == nil {
				logf("worker: card %s: %v", cardID, err)
				return
			}
			if status != "" {
				logf("worker: card %s -> %s", cardID, status)
			}
		}(req.cardID)
	}
	return started, nil
}

func claim(ctx context.Context, db *sql.DB, runID string) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE runs SET processed_at = $2 WHERE id = $1 AND processed_at IS NULL`,
		runID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func implementationCap(dataRoot, projectID string) (int, error) {
	if dataRoot == "" {
		return defaultImplementationCap, nil
	}
	cfg, _, err := settings.LoadProject(dataRoot, projectID)
	if err != nil {
		return 0, err
	}
	if cfg.Scheduler.ImplementationCap > 0 {
		return cfg.Scheduler.ImplementationCap, nil
	}
	return defaultImplementationCap, nil
}
