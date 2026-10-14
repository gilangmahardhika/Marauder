package board

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// LeaseTTL is how long a lease lives without a heartbeat from its runner.
const LeaseTTL = 60 * time.Second

// ClaimRun leases the oldest open card-scoped run request to an active
// runner with a free slot (RFC-107). Concurrent claims skip rows another claim
// holds. A card whose affinity names another active runner with a live
// heartbeat is left for that runner. capFor returns a project's
// implementationCap, counted against its open leases. Nothing to claim is
// ErrNotFound.
func ClaimRun(ctx context.Context, db *sql.DB, runnerID string, capFor func(projectID string) int, offline time.Duration) (Lease, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Lease{}, err
	}
	defer tx.Rollback()
	var slots, open int
	var state string
	err = tx.QueryRowContext(ctx, `SELECT slots, state FROM runners WHERE id = $1 FOR UPDATE`, runnerID).Scan(&slots, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, err
	}
	if state != "active" {
		return Lease{}, ErrNotFound
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM runner_leases WHERE runner_id = $1 AND ended_at IS NULL`, runnerID).Scan(&open); err != nil {
		return Lease{}, err
	}
	if open >= slots {
		return Lease{}, ErrNotFound
	}
	now := time.Now()
	cutoff := stamp(now.Add(-offline))
	full := []string{}
	for {
		var runID, cardID, projectID string
		err := tx.QueryRowContext(ctx, `
			SELECT r.id, r.card_id, r.project_id FROM runs r
			JOIN cards c ON c.id = r.card_id
			WHERE r.processed_at IS NULL AND r.card_id IS NOT NULL
				AND r.project_id <> ALL($2::text[])
				AND NOT EXISTS (SELECT 1 FROM runner_leases l WHERE l.card_id = r.card_id AND l.ended_at IS NULL)
				AND (c.runner_affinity IS NULL OR c.runner_affinity = $1 OR NOT EXISTS (
					SELECT 1 FROM runners a WHERE a.id = c.runner_affinity AND a.state = 'active' AND a.heartbeat_at > $3))
			ORDER BY r.created_at, r.id
			LIMIT 1
			FOR UPDATE OF r SKIP LOCKED`, runnerID, full, cutoff).Scan(&runID, &cardID, &projectID)
		if errors.Is(err, sql.ErrNoRows) {
			return Lease{}, ErrNotFound
		}
		if err != nil {
			return Lease{}, err
		}
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('runner_lease:' || $1))`, projectID); err != nil {
			return Lease{}, err
		}
		var leased int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM runner_leases WHERE project_id = $1 AND ended_at IS NULL`, projectID).Scan(&leased); err != nil {
			return Lease{}, err
		}
		if leased >= capFor(projectID) {
			full = append(full, projectID)
			continue
		}
		id, err := NewID("lse_")
		if err != nil {
			return Lease{}, err
		}
		lease := Lease{
			ID: id, RunID: runID, RunnerID: runnerID, CardID: cardID, ProjectID: projectID,
			LeasedUntil: stamp(now.Add(LeaseTTL)), CreatedAt: stamp(now),
		}
		if _, err := tx.ExecContext(ctx, `UPDATE runs SET processed_at = $2 WHERE id = $1`, runID, stamp(now)); err != nil {
			return Lease{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runner_leases (id, run_id, runner_id, card_id, project_id, leased_until, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			lease.ID, lease.RunID, lease.RunnerID, lease.CardID, lease.ProjectID, lease.LeasedUntil, lease.CreatedAt); err != nil {
			return Lease{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE cards SET runner_affinity = $2 WHERE id = $1`, cardID, runnerID); err != nil {
			return Lease{}, err
		}
		return lease, tx.Commit()
	}
}

// ExtendLeases moves the expiry of a runner's open leases to LeaseTTL from now.
func ExtendLeases(ctx context.Context, db DBTX, runnerID string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE runner_leases SET leased_until = $2 WHERE runner_id = $1 AND ended_at IS NULL`,
		runnerID, stamp(time.Now().Add(LeaseTTL)))
	return err
}

// OpenLease reads a lease the runner holds and has not ended.
func OpenLease(ctx context.Context, db DBTX, runnerID, leaseID string) (Lease, error) {
	l, err := scanLease(db.QueryRowContext(ctx, `
		SELECT `+leaseColumns+` FROM runner_leases
		WHERE id = $1 AND runner_id = $2 AND ended_at IS NULL`, leaseID, runnerID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	return l, err
}

// SetLeaseRoute records the model route the control plane chose for a lease.
func SetLeaseRoute(ctx context.Context, db DBTX, leaseID, model, fallback, class string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE runner_leases SET model_id = $2, fallback_model_id = $3, task_class = $4 WHERE id = $1`,
		leaseID, model, fallback, class)
	return err
}

// SetLeaseAttempt records the attempt a lease is running.
func SetLeaseAttempt(ctx context.Context, db DBTX, leaseID, attemptID string) error {
	_, err := db.ExecContext(ctx, `UPDATE runner_leases SET attempt_id = $2 WHERE id = $1`, leaseID, attemptID)
	return err
}

// EndLease ends an open lease with outcome done and drops locks it held.
func EndLease(ctx context.Context, db *sql.DB, leaseID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cardID string
	err = tx.QueryRowContext(ctx, `
		UPDATE runner_leases SET ended_at = $2, outcome = 'done' WHERE id = $1 AND ended_at IS NULL
		RETURNING card_id`, leaseID, stamp(time.Now())).Scan(&cardID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM locks WHERE card_id = $1 AND holder = $2`, cardID, leaseID); err != nil {
		return err
	}
	return tx.Commit()
}

// ReapFilter picks the open leases ReapLeases ends. Empty fields match all.
type ReapFilter struct {
	RunnerID string
	LeaseID  string
	// Expired matches leases past leased_until or held by a removed runner.
	Expired bool
}

// ReapLeases ends the matching open leases with outcome (lost or removed). For
// each it cancels the card's open attempt, deletes the card lock the lease
// held, clears the card's affinity to that runner, and opens the run request
// again when the card can still start. Card status is never changed; that is
// machine.Transition's job. It returns the ids of the leases it ended.
func ReapLeases(ctx context.Context, db *sql.DB, f ReapFilter, outcome string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+leaseColumns+` FROM runner_leases
		WHERE ended_at IS NULL AND ($1 = '' OR runner_id = $1) AND ($2 = '' OR id = $2)
			AND (NOT $3 OR leased_until < $4
				OR runner_id IN (SELECT id FROM runners WHERE state = 'removed'))`,
		f.RunnerID, f.LeaseID, f.Expired, stamp(time.Now()))
	if err != nil {
		return nil, err
	}
	var leases []Lease
	for rows.Next() {
		l, err := scanLease(rows.Scan)
		if err != nil {
			rows.Close()
			return nil, err
		}
		leases = append(leases, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []string
	for _, l := range leases {
		ended, err := reapLease(ctx, db, l, outcome)
		if err != nil {
			return out, err
		}
		if ended {
			out = append(out, l.ID)
		}
	}
	return out, nil
}

func reapLease(ctx context.Context, db *sql.DB, l Lease, outcome string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := stamp(time.Now())
	res, err := tx.ExecContext(ctx, `
		UPDATE runner_leases SET ended_at = $2, outcome = $3 WHERE id = $1 AND ended_at IS NULL`, l.ID, now, outcome)
	if err != nil {
		return false, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE attempts SET ended_at = $2, outcome = 'cancelled' WHERE card_id = $1 AND ended_at IS NULL`, l.CardID, now); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM locks WHERE card_id = $1 AND holder = $2`, l.CardID, l.ID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cards SET runner_affinity = NULL WHERE id = $1 AND runner_affinity = $2`, l.CardID, l.RunnerID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE runs SET processed_at = NULL
		WHERE id = $1
			AND EXISTS (SELECT 1 FROM cards WHERE id = $2 AND status IN ('ready', 'changes_requested', 'in_progress'))
			AND NOT EXISTS (SELECT 1 FROM runs WHERE card_id = $2 AND processed_at IS NULL)`, l.RunID, l.CardID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
