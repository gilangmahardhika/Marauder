package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Runner is a registered remote runner (RFC-106). Its status is derived from
// HeartbeatAt, State, and its open leases; it is never stored.
type Runner struct {
	ID          string
	Name        string
	Labels      []string
	Slots       int
	Version     string
	Protocol    int
	State       string
	HeartbeatAt string
	CreatedAt   string
	CreatedBy   string
}

// Lease is one implement job a runner holds (RFC-107).
type Lease struct {
	ID              string
	RunID           string
	RunnerID        string
	CardID          string
	ProjectID       string
	AttemptID       string
	ModelID         string
	FallbackModelID string
	TaskClass       string
	LeasedUntil     string
	CreatedAt       string
	EndedAt         string
	Outcome         string
}

func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// InsertRegistration stores a registration token by its hash.
func InsertRegistration(ctx context.Context, db DBTX, createdBy, tokenHash string, expires time.Time) error {
	id, err := NewID("rrg_")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO runner_registrations (id, token_hash, created_by, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, id, tokenHash, createdBy, stamp(time.Now()), stamp(expires))
	return err
}

// ConsumeRegistration marks an unused, unexpired token used and returns the
// user who minted it. Any other token is ErrNotFound.
func ConsumeRegistration(ctx context.Context, db DBTX, tokenHash, runnerID string) (string, error) {
	now := stamp(time.Now())
	var createdBy string
	err := db.QueryRowContext(ctx, `
		UPDATE runner_registrations SET used_at = $2, runner_id = $3
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
		RETURNING created_by`, tokenHash, now, runnerID).Scan(&createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return createdBy, err
}

// InsertRunner stores a new active runner with its secret hash.
func InsertRunner(ctx context.Context, db DBTX, r Runner, secretHash string) error {
	labels, err := json.Marshal(nonNilStrings(r.Labels))
	if err != nil {
		return err
	}
	now := stamp(time.Now())
	_, err = db.ExecContext(ctx, `
		INSERT INTO runners (id, name, labels_json, slots, version, protocol, secret_hash, state, heartbeat_at, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'active', $8, $8, $9)`,
		r.ID, r.Name, string(labels), r.Slots, r.Version, r.Protocol, secretHash, now, nullable(r.CreatedBy))
	return err
}

const runnerColumns = `id, name, labels_json, slots, version, protocol, state, COALESCE(heartbeat_at, ''), created_at, COALESCE(created_by, '')`

func scanRunner(scan func(...any) error) (Runner, error) {
	var r Runner
	var labels string
	if err := scan(&r.ID, &r.Name, &labels, &r.Slots, &r.Version, &r.Protocol, &r.State, &r.HeartbeatAt, &r.CreatedAt, &r.CreatedBy); err != nil {
		return Runner{}, err
	}
	if err := json.Unmarshal([]byte(labels), &r.Labels); err != nil {
		return Runner{}, err
	}
	r.Labels = nonNilStrings(r.Labels)
	return r, nil
}

// RunnerBySecret finds the runner that is not removed with that secret hash.
func RunnerBySecret(ctx context.Context, db DBTX, secretHash string) (Runner, error) {
	r, err := scanRunner(db.QueryRowContext(ctx, `
		SELECT `+runnerColumns+` FROM runners WHERE secret_hash = $1 AND state <> 'removed'`, secretHash).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Runner{}, ErrNotFound
	}
	return r, err
}

// GetRunner reads a runner that is not removed.
func GetRunner(ctx context.Context, db DBTX, id string) (Runner, error) {
	r, err := scanRunner(db.QueryRowContext(ctx, `
		SELECT `+runnerColumns+` FROM runners WHERE id = $1 AND state <> 'removed'`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Runner{}, ErrNotFound
	}
	return r, err
}

// ListRunners lists runners that are not removed, by name.
func ListRunners(ctx context.Context, db *sql.DB) ([]Runner, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+runnerColumns+` FROM runners WHERE state <> 'removed' ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Runner
	for rows.Next() {
		r, err := scanRunner(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Heartbeat records that the runner is alive with its version and protocol.
func Heartbeat(ctx context.Context, db DBTX, id, version string, protocol int) error {
	_, err := db.ExecContext(ctx, `
		UPDATE runners SET heartbeat_at = $2, version = $3, protocol = $4
		WHERE id = $1 AND state <> 'removed'`, id, stamp(time.Now()), version, protocol)
	return err
}

// SetRunnerState moves a runner from one state to another as a
// compare-and-swap; a runner in another state is ErrConflict.
func SetRunnerState(ctx context.Context, db DBTX, id, from, to string) error {
	res, err := db.ExecContext(ctx, `UPDATE runners SET state = $3 WHERE id = $1 AND state = $2`, id, from, to)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

const leaseColumns = `id, run_id, runner_id, card_id, project_id, COALESCE(attempt_id, ''), model_id, fallback_model_id, task_class,
	leased_until, created_at, COALESCE(ended_at, ''), COALESCE(outcome, '')`

func scanLease(scan func(...any) error) (Lease, error) {
	var l Lease
	err := scan(&l.ID, &l.RunID, &l.RunnerID, &l.CardID, &l.ProjectID, &l.AttemptID, &l.ModelID, &l.FallbackModelID, &l.TaskClass,
		&l.LeasedUntil, &l.CreatedAt, &l.EndedAt, &l.Outcome)
	return l, err
}

// OpenLeases lists every open lease, oldest first. An empty runnerID lists
// the leases of every runner.
func OpenLeases(ctx context.Context, db *sql.DB, runnerID string) ([]Lease, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+leaseColumns+` FROM runner_leases
		WHERE ended_at IS NULL AND ($1 = '' OR runner_id = $1)
		ORDER BY created_at, id`, runnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lease
	for rows.Next() {
		l, err := scanLease(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
