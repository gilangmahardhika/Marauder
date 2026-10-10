package intake

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var ErrUnknownID = errors.New("unknown requirement id")

// Apply upserts parsed records on projectID. A manual row is never
// overwritten. Its id is returned in skipped (RFC-053).
func Apply(ctx context.Context, db *sql.DB, projectID string, result Result) (skipped []string, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, req := range result.Requirements {
		acceptance := req.Acceptance
		if acceptance == nil {
			acceptance = []string{}
		}
		encoded, err := json.Marshal(acceptance)
		if err != nil {
			return nil, err
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO requirements (
				id, source_kind, source_path, heading, statement,
				acceptance_json, non_goals_json, content_hash, project_id
			) VALUES ($1, $2, $3, $4, $5, $6, '[]', $7, $8)
			ON CONFLICT (project_id, id) DO UPDATE SET
				source_kind = EXCLUDED.source_kind,
				source_path = EXCLUDED.source_path,
				heading = EXCLUDED.heading,
				statement = EXCLUDED.statement,
				acceptance_json = EXCLUDED.acceptance_json,
				non_goals_json = EXCLUDED.non_goals_json,
				content_hash = EXCLUDED.content_hash,
				accepted_at = CASE
					WHEN requirements.content_hash IS NOT DISTINCT FROM EXCLUDED.content_hash THEN requirements.accepted_at
					ELSE NULL
				END,
				accepted_by = CASE
					WHEN requirements.content_hash IS NOT DISTINCT FROM EXCLUDED.content_hash THEN requirements.accepted_by
					ELSE NULL
				END
			WHERE requirements.source_kind <> 'manual'`,
			req.ID, req.SourceKind, req.SourcePath, req.Heading, req.Statement, string(encoded), req.Hash, projectID,
		)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n == 0 {
			skipped = append(skipped, req.ID)
		}
	}
	for _, dec := range result.Decisions {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO decisions (id, source_path, statement, content_hash, project_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (project_id, id) DO UPDATE SET
				source_path = EXCLUDED.source_path,
				statement = EXCLUDED.statement,
				content_hash = EXCLUDED.content_hash`,
			dec.ID, dec.SourcePath, dec.Statement, dec.Hash, projectID,
		)
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE intake_state SET draft_pending = $1 WHERE project_id = $2`, result.DraftPending, projectID)
	if err != nil {
		return nil, err
	}
	return skipped, tx.Commit()
}

func Accept(ctx context.Context, db *sql.DB, projectID, operator string, ids []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range ids {
		res, err := tx.ExecContext(ctx, `
			UPDATE requirements SET accepted_at = $1, accepted_by = $2 WHERE id = $3 AND project_id = $4`,
			now, operator, id, projectID,
		)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrUnknownID
		}
	}
	runID, err := newID("run_")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO runs (id, trigger, created_at, project_id) VALUES ($1, 'accept', $2, $3)`,
		runID, now, projectID,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func newID(prefix string) (string, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf[:]), nil
}
