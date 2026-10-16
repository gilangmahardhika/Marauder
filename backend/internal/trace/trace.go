package trace

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

var kinds = map[string]struct{}{
	"route":                 {},
	"model_call":            {},
	"tool_call":             {},
	"gate":                  {},
	"transition":            {},
	"transition_rejected":   {},
	"deploy":                {},
	"merge":                 {},
	"wont_fix":              {},
	"health":                {},
	"verify":                {},
	"budget_stop":           {},
	"context_compacted":     {},
	"handoff_saved":         {},
	"handoff_applied":       {},
	"agent_run":             {},
	"config_changed":        {},
	"assertion_failed":      {},
	"reviewers_requested":   {},
	"stacks_detected":       {},
	"job_error":             {},
	"instructions_saved":    {},
	"context_changed":       {},
	"requirement_refined":   {},
	"review_failed":         {},
	"questions_answered":    {},
	"question_mode_changed": {},
	"plan_accepted":         {},
	"plan_skipped":          {},
	"plan_mode_changed":     {},
}

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Event struct {
	ID        string
	TS        string
	CardID    string
	AttemptID string
	ActorID   string
	ProjectID string
	Kind      string
	Payload   any
}

func Append(ctx context.Context, q queryer, ev Event, secrets []string) (string, error) {
	if _, ok := kinds[ev.Kind]; !ok {
		return "", fmt.Errorf("unknown event kind %q", ev.Kind)
	}
	if ev.ID == "" {
		id, err := newID("evt_")
		if err != nil {
			return "", err
		}
		ev.ID = id
	}
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339)
	}
	payload := ev.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	stored := Redact(string(raw), secrets)
	var cardID, attemptID any
	if ev.CardID != "" {
		cardID = ev.CardID
	}
	if ev.AttemptID != "" {
		attemptID = ev.AttemptID
	}
	var actorID any
	if ev.ActorID != "" {
		actorID = ev.ActorID
	}
	projectID := ev.ProjectID
	if projectID == "" {
		projectID = "proj_default"
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO events (id, ts, card_id, attempt_id, kind, payload_json, actor_id, project_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ev.ID, ev.TS, cardID, attemptID, ev.Kind, stored, actorID, projectID,
	)
	if err != nil {
		return "", err
	}
	return ev.ID, nil
}

func Count(ctx context.Context, q queryer, cardID, kind string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `
		SELECT count(*) FROM events WHERE card_id = $1 AND kind = $2`,
		cardID, kind,
	).Scan(&n)
	return n, err
}

// Listed is one stored event of a card, with its payload as stored.
type Listed struct {
	ID        string
	Seq       int64
	TS        string
	AttemptID string
	Kind      string
	Payload   string
}

type lister interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// List returns the card's events oldest first. Events in the same second keep
// the order they were inserted in.
func List(ctx context.Context, q lister, cardID string) ([]Listed, error) {
	return scanListed(q.QueryContext(ctx, `
		SELECT id, seq, ts, COALESCE(attempt_id, ''), kind, payload_json
		FROM events WHERE card_id = $1 ORDER BY ts, seq`, cardID))
}

// ListSince returns the card's events with ts at or after since, oldest
// first. An empty since returns every event. The caller checks the card is in
// the project; events are read by card id because older ones may carry
// proj_default.
func ListSince(ctx context.Context, q lister, cardID, since string) ([]Listed, error) {
	return scanListed(q.QueryContext(ctx, `
		SELECT id, seq, ts, COALESCE(attempt_id, ''), kind, payload_json
		FROM events WHERE card_id = $1 AND ($2 = '' OR ts >= $2)
		ORDER BY ts, seq`, cardID, since))
}

func scanListed(rows *sql.Rows, err error) ([]Listed, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Listed{}
	for rows.Next() {
		var ev Listed
		if err := rows.Scan(&ev.ID, &ev.Seq, &ev.TS, &ev.AttemptID, &ev.Kind, &ev.Payload); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// TextCap is the most bytes of a model reply kept in model_call.text, and
// ArgCap the most bytes of each value in tool_call.args.
const (
	TextCap = 4 << 10
	ArgCap  = 1 << 10
)

// Clip makes text safe for one payload field: NUL bytes are dropped, invalid
// UTF-8 is replaced, secrets are redacted, and text over max bytes keeps its
// head and tail around a cut marker. Redacting before the cut keeps a secret
// that crosses the cut from surviving in part.
func Clip(text string, secrets []string, max int) string {
	text = Redact(strings.ToValidUTF8(strings.ReplaceAll(text, "\x00", ""), "\uFFFD"), secrets)
	if len(text) <= max {
		return text
	}
	half := max / 2
	head := strings.ToValidUTF8(text[:half], "")
	tail := strings.ToValidUTF8(text[len(text)-half:], "")
	cut := len(text) - len(head) - len(tail)
	return fmt.Sprintf("%s\n... (%d bytes cut) ...\n%s", head, cut, tail)
}

func Redact(s string, secrets []string) string {
	ordered := append([]string(nil), secrets...)
	sort.Slice(ordered, func(i, j int) bool {
		return len(ordered[i]) > len(ordered[j])
	})
	for _, secret := range ordered {
		if secret == "" {
			continue
		}
		s = strings.ReplaceAll(s, secret, "[redacted]")
	}
	return s
}

func newID(prefix string) (string, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf[:]), nil
}
