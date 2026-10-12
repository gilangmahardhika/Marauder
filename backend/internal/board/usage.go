package board

import (
	"context"
	"database/sql"
)

// UsageRow is the token usage of one day, project, model, task type, and user
// (RFC-089). UserID is empty for a card with no owner.
type UsageRow struct {
	Day               string
	ProjectID         string
	ProjectName       string
	ModelID           string
	Task              string
	UserID            string
	UserName          string
	UserEmail         string
	Calls             int64
	InputTokens       int64
	OutputTokens      int64
	CachedInputTokens int64
}

// UsageRows totals the model_call and requirement_refined events with ts in
// [from, to), across every project. The task type is the event's taskClass,
// else refine_requirement for a refine, review for a reviewer call, and the
// attempt's task class otherwise. A call on a card counts against the card's
// owner, and a refine against its actor.
func UsageRows(ctx context.Context, db *sql.DB, from, to string) ([]UsageRow, error) {
	rows, err := db.QueryContext(ctx, `
		WITH calls AS (
			SELECT substr(e.ts, 1, 10) AS day, e.project_id, e.payload_json::jsonb AS p,
				CASE
					WHEN e.kind = 'requirement_refined' THEN 'refine_requirement'
					WHEN COALESCE(e.payload_json::jsonb->>'taskClass', '') <> '' THEN e.payload_json::jsonb->>'taskClass'
					WHEN e.payload_json::jsonb->>'role' = 'reviewer' THEN 'review'
					ELSE COALESCE(a.task_class, 'implement')
				END AS task,
				CASE WHEN e.kind = 'requirement_refined' THEN e.actor_id ELSE c.owner_id END AS user_id
			FROM events e
			LEFT JOIN attempts a ON a.id = e.attempt_id
			LEFT JOIN cards c ON c.id = e.card_id
			WHERE e.kind IN ('model_call', 'requirement_refined') AND e.ts >= $1 AND e.ts < $2
		)
		SELECT calls.day, calls.project_id, COALESCE(pr.name, ''), COALESCE(calls.p->>'modelId', ''), calls.task,
			COALESCE(calls.user_id, ''), COALESCE(u.name, ''), COALESCE(u.email, ''),
			count(*),
			COALESCE(SUM((calls.p->>'inputTokens')::bigint), 0),
			COALESCE(SUM((calls.p->>'outputTokens')::bigint), 0),
			COALESCE(SUM((calls.p->>'cachedInputTokens')::bigint), 0)
		FROM calls
		LEFT JOIN projects pr ON pr.id = calls.project_id
		LEFT JOIN users u ON u.id = calls.user_id
		GROUP BY calls.day, calls.project_id, pr.name, calls.p->>'modelId', calls.task, calls.user_id, u.name, u.email
		ORDER BY calls.day, calls.project_id, calls.p->>'modelId', calls.task, calls.user_id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageRow{}
	for rows.Next() {
		var r UsageRow
		if err := rows.Scan(&r.Day, &r.ProjectID, &r.ProjectName, &r.ModelID, &r.Task,
			&r.UserID, &r.UserName, &r.UserEmail, &r.Calls, &r.InputTokens, &r.OutputTokens, &r.CachedInputTokens); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
