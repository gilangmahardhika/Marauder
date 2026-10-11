package worker

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"orchestrator/backend/internal/settings"
)

// Run starts one loop per project and picks up projects created later. It
// returns when ctx is cancelled and every project loop has stopped.
func Run(ctx context.Context, db *sql.DB, dataRoot string, interval time.Duration, logf func(string, ...any)) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	var wg sync.WaitGroup
	started := map[string]bool{}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		projects, err := projectIDs(ctx, db)
		if err != nil && ctx.Err() == nil {
			logf("worker: list projects: %v", err)
		}
		for _, id := range projects {
			if started[id] {
				continue
			}
			started[id] = true
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				projectLoop(ctx, db, dataRoot, id, interval, logf)
			}(id)
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-ticker.C:
		}
	}
}

// LockProject takes the project's worker lock at <data>/projects/<id>/worker.lock.
func LockProject(dataRoot, projectID string) (*os.File, error) {
	dir := settings.ProjectDir(dataRoot, projectID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return Lock(filepath.Join(dir, "worker.lock"))
}

// Drain runs passes until one changes nothing, and returns the totals.
func Drain(ctx context.Context, db *sql.DB, dataRoot, projectID string) (Result, error) {
	var total Result
	for ctx.Err() == nil {
		result, err := Pass(ctx, db, dataRoot, projectID)
		total.Created += result.Created
		total.Promoted += result.Promoted
		if err != nil || !result.Changed() {
			return total, err
		}
	}
	return total, ctx.Err()
}

func projectLoop(ctx context.Context, db *sql.DB, dataRoot, projectID string, interval time.Duration, logf func(string, ...any)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lock *os.File
	defer func() {
		if lock != nil {
			lock.Close()
		}
	}()
	logged := false
	for {
		if lock == nil {
			var err error
			lock, err = LockProject(dataRoot, projectID)
			if err != nil {
				lock = nil
				if !logged {
					if errors.Is(err, ErrBusy) {
						logf("worker: project %s is locked by another worker, skipped", projectID)
					} else {
						logf("worker: project %s lock: %v", projectID, err)
					}
					logged = true
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				continue
			}
		}
		open, err := Pending(ctx, db, projectID)
		if err != nil && ctx.Err() == nil {
			logf("worker: project %s: %v", projectID, err)
		}
		if open {
			if result, err := Drain(ctx, db, dataRoot, projectID); err != nil && ctx.Err() == nil {
				logf("worker: project %s pass: %v", projectID, err)
			} else if result.Changed() {
				logf("worker: project %s created %d promoted %d", projectID, result.Created, result.Promoted)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func projectIDs(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
