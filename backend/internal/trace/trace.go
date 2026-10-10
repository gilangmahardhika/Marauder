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
	"route":               {},
	"model_call":          {},
	"tool_call":           {},
	"gate":                {},
	"transition":          {},
	"transition_rejected": {},
	"deploy":              {},
	"health":              {},
	"verify":              {},
	"budget_stop":         {},
	"config_changed":      {},
	"assertion_failed":    {},
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
	_, err = q.ExecContext(ctx, `
		INSERT INTO events (id, ts, card_id, attempt_id, kind, payload_json, actor_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		ev.ID, ev.TS, cardID, attemptID, ev.Kind, stored, actorID,
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
