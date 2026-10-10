package deploy

import (
	"context"
	"database/sql"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/trace"
)

type Exec func(argv []string) int

type Job struct {
	DB        *sql.DB
	Card      board.Card
	Command   string
	Rollback  string
	HealthURL string
	Hosts     []string
	Fixture   bool
	Exec      Exec
	RollbackN *int
	DeployN   *int
	ProbeFn   func() error
}

func Run(ctx context.Context, job Job) (board.Card, error) {
	if err := ValidSHA(job.Card.ApprovedSHA); err != nil {
		return job.Card, err
	}
	if job.DeployN != nil {
		*job.DeployN++
	}
	code := job.Exec([]string{job.Command, job.Card.ApprovedSHA})
	_ = traceEvent(ctx, job.DB, job.Card.ID, "deploy", map[string]any{
		"requestedSha": job.Card.ApprovedSHA, "previousSha": "", "devUrl": job.HealthURL, "exitCode": code,
	})
	if code != 0 {
		return fail(ctx, job, "deploy_failed")
	}
	probe := job.ProbeFn
	if probe == nil {
		probe = func() error { return Probe(job.HealthURL, job.Hosts, job.Fixture, nil, nil) }
	}
	if err := probe(); err != nil {
		return fail(ctx, job, "health")
	}
	_ = traceEvent(ctx, job.DB, job.Card.ID, "health", map[string]any{"passed": true, "status": 200})
	if _, err := job.DB.ExecContext(ctx, `UPDATE cards SET deployed_sha = $2 WHERE id = $1`, job.Card.ID, job.Card.ApprovedSHA); err != nil {
		return job.Card, err
	}
	job.Card.DeployedSHA = job.Card.ApprovedSHA
	return move(ctx, job.DB, job.Card, "verifying", machine.Evidence{SHA: job.Card.ApprovedSHA})
}

func fail(ctx context.Context, job Job, reason string) (board.Card, error) {
	if job.RollbackN != nil {
		*job.RollbackN++
	}
	exit := 0
	if job.Rollback != "" && job.Exec != nil {
		exit = job.Exec([]string{job.Rollback})
	}
	_ = traceEvent(ctx, job.DB, job.Card.ID, "health", map[string]any{"passed": false, "rollbackExit": exit})
	return move(ctx, job.DB, job.Card, "blocked", machine.Evidence{Reason: reason})
}

func Reconcile(ctx context.Context, db *sql.DB, card board.Card, execs *int, rollback Exec) error {
	var deploys, healths int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'deploy'`, card.ID).Scan(&deploys)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'health'`, card.ID).Scan(&healths)
	if deploys > healths {
		if execs != nil {
			*execs = 0
		}
		if rollback != nil {
			rollback([]string{"rollback"})
		}
		_ = traceEvent(ctx, db, card.ID, "health", map[string]any{"passed": false, "rollbackExit": 0})
	}
	return nil
}

func move(ctx context.Context, db *sql.DB, card board.Card, to string, ev machine.Evidence) (board.Card, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return card, err
	}
	defer tx.Rollback()
	next, err := machine.Transition(ctx, tx, card, to, ev, nil)
	if err != nil {
		return card, err
	}
	if err := tx.Commit(); err != nil {
		return card, err
	}
	return next, nil
}

func traceEvent(ctx context.Context, db *sql.DB, cardID, kind string, payload map[string]any) error {
	_, err := trace.Append(ctx, db, trace.Event{CardID: cardID, Kind: kind, Payload: payload}, nil)
	return err
}
