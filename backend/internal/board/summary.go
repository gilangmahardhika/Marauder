package board

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"time"
)

// StatusGroup maps a card status to the board group used by the API (RFC-116).
func StatusGroup(status string) string {
	switch status {
	case "backlog", "ready":
		return "waiting"
	case "waiting_answer":
		return "question"
	case "in_progress", "self_check":
		return "running"
	case "human_review", "pr_open", "agent_review", "changes_requested":
		return "review"
	case "approved", "deploying_dev", "verifying":
		return "shipping"
	case "done":
		return "finished"
	case "blocked":
		return "stopped"
	case "wont_fix":
		return "closed"
	default:
		return "waiting"
	}
}

// GroupCounts is WIP by board group.
type GroupCounts struct {
	Waiting  int `json:"waiting"`
	Question int `json:"question"`
	Running  int `json:"running"`
	Review   int `json:"review"`
	Shipping int `json:"shipping"`
	Finished int `json:"finished"`
	Stopped  int `json:"stopped"`
	Closed   int `json:"closed"`
}

// ReasonCount is one blocked_reason (or budget cap) and how many.
type ReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// CapCount is one budget_stop cap and how many events.
type CapCount struct {
	Cap   string `json:"cap"`
	Count int    `json:"count"`
}

// AgingCard is an open card ordered by age for the summary list.
type AgingCard struct {
	CardID    string `json:"cardId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

// AnswerSources totals answered questions by answer_source in the range.
type AnswerSources struct {
	Person     int `json:"person"`
	Suggestion int `json:"suggestion"`
	Auto       int `json:"auto"`
}

// Attention is live queue pressure on a project (or rolled up for a tribe).
type Attention struct {
	ReadyCount         int           `json:"readyCount"`
	WaitingAnswer      int           `json:"waitingAnswer"`
	HumanReview        int           `json:"humanReview"`
	Blocked            []ReasonCount `json:"blocked"`
	OpenRuns           int           `json:"openRuns"`
	Locked             int           `json:"locked"`
	ImplementationCap  int           `json:"implementationCap"`
	OpenQuestions      int           `json:"openQuestions"`
}

// RangeInsights totals events and answers in [from, to).
type RangeInsights struct {
	Done               int           `json:"done"`
	WontFix            int           `json:"wontFix"`
	Rework             int           `json:"rework"`
	BudgetStops        []CapCount    `json:"budgetStops"`
	AnswerSources      AnswerSources `json:"answerSources"`
	MedianAnswerHours  *float64      `json:"medianAnswerHours"`
}

// MCPHealth counts tribe MCP trouble in the range (RFC-094, RFC-102).
type MCPHealth struct {
	Changed         int `json:"changed"`
	ToolErrors      int `json:"toolErrors"`
	MissingMentions int `json:"missingMentions"`
}

// ProjectSnapshot is the live board view for one project.
type ProjectSnapshot struct {
	Groups    GroupCounts `json:"groups"`
	Attention Attention   `json:"attention"`
	Aging     []AgingCard `json:"aging"`
}

// EmptyGroupCounts returns zeros so JSON always has every key.
func EmptyGroupCounts() GroupCounts {
	return GroupCounts{}
}

// CountGroups fills GroupCounts and Attention status fields from card rows of one project.
func CountGroups(ctx context.Context, db *sql.DB, projectID string) (GroupCounts, Attention, error) {
	groups := EmptyGroupCounts()
	att := Attention{Blocked: []ReasonCount{}}
	rows, err := db.QueryContext(ctx, `
		SELECT status, COALESCE(blocked_reason, ''), count(*)
		FROM cards WHERE project_id = $1
		GROUP BY status, COALESCE(blocked_reason, '')`, projectID)
	if err != nil {
		return groups, att, err
	}
	defer rows.Close()
	blocked := map[string]int{}
	for rows.Next() {
		var status, reason string
		var n int
		if err := rows.Scan(&status, &reason, &n); err != nil {
			return groups, att, err
		}
		switch StatusGroup(status) {
		case "waiting":
			groups.Waiting += n
		case "question":
			groups.Question += n
			att.WaitingAnswer += n
		case "running":
			groups.Running += n
		case "review":
			groups.Review += n
			if status == "human_review" {
				att.HumanReview += n
			}
		case "shipping":
			groups.Shipping += n
		case "finished":
			groups.Finished += n
		case "stopped":
			groups.Stopped += n
			if reason != "" {
				blocked[reason] += n
			} else {
				blocked["unknown"] += n
			}
		case "closed":
			groups.Closed += n
		}
	}
	if err := rows.Err(); err != nil {
		return groups, att, err
	}
	att.Blocked = reasonCounts(blocked)
	return groups, att, nil
}

func reasonCounts(m map[string]int) []ReasonCount {
	out := make([]ReasonCount, 0, len(m))
	for reason, n := range m {
		out = append(out, ReasonCount{Reason: reason, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Reason < out[j].Reason
	})
	if out == nil {
		return []ReasonCount{}
	}
	return out
}

// CountOpenRuns returns runs of the project that are not processed yet.
func CountOpenRuns(ctx context.Context, db *sql.DB, projectID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM runs
		WHERE project_id = $1 AND processed_at IS NULL`, projectID).Scan(&n)
	return n, err
}

// CountLocks returns cards of the project that hold a lock row.
func CountLocks(ctx context.Context, db *sql.DB, projectID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM locks WHERE project_id = $1`, projectID).Scan(&n)
	return n, err
}

// CountOpenQuestions returns unanswered card_questions of the project.
func CountOpenQuestions(ctx context.Context, db *sql.DB, projectID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM card_questions
		WHERE project_id = $1 AND answered_at IS NULL`, projectID).Scan(&n)
	return n, err
}

// AgingCards returns up to limit non-terminal cards oldest by updated_at.
func AgingCards(ctx context.Context, db *sql.DB, projectID string, limit int) ([]AgingCard, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, title, status, updated_at FROM cards
		WHERE project_id = $1 AND status NOT IN ('done', 'wont_fix')
		ORDER BY updated_at ASC, id ASC
		LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgingCard{}
	for rows.Next() {
		var c AgingCard
		if err := rows.Scan(&c.CardID, &c.Title, &c.Status, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ProjectRange totals transitions, budget stops, and answers for one project in [from, to).
func ProjectRange(ctx context.Context, db *sql.DB, projectID, from, to string) (RangeInsights, error) {
	r := RangeInsights{BudgetStops: []CapCount{}, AnswerSources: AnswerSources{}}
	if err := db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE payload_json::jsonb->>'to' = 'done'),
			count(*) FILTER (WHERE payload_json::jsonb->>'to' = 'wont_fix'),
			count(*) FILTER (WHERE payload_json::jsonb->>'to' = 'changes_requested')
		FROM events
		WHERE project_id = $1 AND kind = 'transition' AND ts >= $2 AND ts < $3`,
		projectID, from, to).Scan(&r.Done, &r.WontFix, &r.Rework); err != nil {
		return r, err
	}
	stops, err := budgetStops(ctx, db, `
		SELECT COALESCE(payload_json::jsonb->>'cap', 'unknown'), count(*)
		FROM events
		WHERE project_id = $1 AND kind = 'budget_stop' AND ts >= $2 AND ts < $3
		GROUP BY 1`, projectID, from, to)
	if err != nil {
		return r, err
	}
	r.BudgetStops = stops
	if err := db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE answer_source = 'person'),
			count(*) FILTER (WHERE answer_source = 'suggestion'),
			count(*) FILTER (WHERE answer_source = 'auto')
		FROM card_questions
		WHERE project_id = $1 AND answered_at IS NOT NULL AND answered_at >= $2 AND answered_at < $3`,
		projectID, from, to).Scan(&r.AnswerSources.Person, &r.AnswerSources.Suggestion, &r.AnswerSources.Auto); err != nil {
		return r, err
	}
	median, err := medianAnswerHours(ctx, db, projectID, from, to)
	if err != nil {
		return r, err
	}
	r.MedianAnswerHours = median
	return r, nil
}

func budgetStops(ctx context.Context, db *sql.DB, query string, args ...any) ([]CapCount, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CapCount{}
	for rows.Next() {
		var c CapCount
		if err := rows.Scan(&c.Cap, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Cap < out[j].Cap
	})
	return out, rows.Err()
}

func medianAnswerHours(ctx context.Context, db *sql.DB, projectID, from, to string) (*float64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT created_at, answered_at FROM card_questions
		WHERE project_id = $1 AND answered_at IS NOT NULL AND answered_at >= $2 AND answered_at < $3`,
		projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hours []float64
	for rows.Next() {
		var created, answered string
		if err := rows.Scan(&created, &answered); err != nil {
			return nil, err
		}
		c, err1 := time.Parse(time.RFC3339, created)
		a, err2 := time.Parse(time.RFC3339, answered)
		if err1 != nil || err2 != nil {
			continue
		}
		h := a.Sub(c).Hours()
		if h < 0 {
			continue
		}
		hours = append(hours, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(hours) == 0 {
		return nil, nil
	}
	sort.Float64s(hours)
	var med float64
	mid := len(hours) / 2
	if len(hours)%2 == 0 {
		med = (hours[mid-1] + hours[mid]) / 2
	} else {
		med = hours[mid]
	}
	med = math.Round(med*10) / 10
	return &med, nil
}

// TribeProjectIDs lists projects of a tribe ordered by name.
func TribeProjectIDs(ctx context.Context, db *sql.DB, tribeID string) ([]struct {
	ID   string
	Name string
}, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name FROM projects WHERE tribe_id = $1 ORDER BY name, id`, tribeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID   string
		Name string
	}
	for rows.Next() {
		var p struct {
			ID   string
			Name string
		}
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TribeMCPHealth counts MCP trouble events in tribe projects in [from, to).
func TribeMCPHealth(ctx context.Context, db *sql.DB, tribeID, from, to string) (MCPHealth, error) {
	var h MCPHealth
	err := db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE e.kind = 'assertion_failed' AND e.payload_json::jsonb->>'name' = 'mcp_changed'),
			count(*) FILTER (WHERE e.kind = 'tool_call'
				AND e.payload_json::jsonb->>'name' LIKE 'mcp\_%' ESCAPE '\'
				AND e.payload_json::jsonb->>'name' <> 'mcp_context'
				AND (e.payload_json::jsonb->>'ok') = 'false'),
			count(*) FILTER (WHERE e.kind = 'tool_call'
				AND e.payload_json::jsonb->>'name' = 'mcp_context'
				AND jsonb_array_length(COALESCE(e.payload_json::jsonb->'missing', '[]'::jsonb)) > 0)
		FROM events e
		JOIN projects p ON p.id = e.project_id
		WHERE p.tribe_id = $1 AND e.ts >= $2 AND e.ts < $3`, tribeID, from, to).Scan(&h.Changed, &h.ToolErrors, &h.MissingMentions)
	return h, err
}

// AddGroups adds b into a.
func AddGroups(a, b GroupCounts) GroupCounts {
	return GroupCounts{
		Waiting: a.Waiting + b.Waiting, Question: a.Question + b.Question,
		Running: a.Running + b.Running, Review: a.Review + b.Review,
		Shipping: a.Shipping + b.Shipping, Finished: a.Finished + b.Finished,
		Stopped: a.Stopped + b.Stopped, Closed: a.Closed + b.Closed,
	}
}

// MergeBlocked merges blocked reason maps.
func MergeBlocked(into []ReasonCount, add []ReasonCount) []ReasonCount {
	m := map[string]int{}
	for _, r := range into {
		m[r.Reason] += r.Count
	}
	for _, r := range add {
		m[r.Reason] += r.Count
	}
	return reasonCounts(m)
}

// MergeCaps merges budget stop lists.
func MergeCaps(into []CapCount, add []CapCount) []CapCount {
	m := map[string]int{}
	for _, c := range into {
		m[c.Cap] += c.Count
	}
	for _, c := range add {
		m[c.Cap] += c.Count
	}
	out := make([]CapCount, 0, len(m))
	for cap, n := range m {
		out = append(out, CapCount{Cap: cap, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Cap < out[j].Cap
	})
	if out == nil {
		return []CapCount{}
	}
	return out
}
