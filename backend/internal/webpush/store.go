package webpush

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"orchestrator/backend/internal/notify"
)

// MaxFailures in a row delete a subscription.
const MaxFailures = 5

// sendFor bounds one batch of sends, so a slow push service cannot hold the
// listener.
const sendFor = 30 * time.Second

// Save stores the user's subscription. An endpoint another user had moves
// to this user, since a browser holds one endpoint per site.
func Save(ctx context.Context, db *sql.DB, userID, endpoint, p256dh, auth string) error {
	if _, err := CheckEndpoint(endpoint); err != nil {
		return err
	}
	if err := CheckKeys(p256dh, auth); err != nil {
		return err
	}
	id, err := newID()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, failures, created_at)
		VALUES ($1, $2, $3, $4, $5, 0, $6)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh,
			auth = EXCLUDED.auth, failures = 0`,
		id, userID, endpoint, p256dh, auth, time.Now().UTC().Format(time.RFC3339))
	return err
}

// Delete removes the user's subscription with endpoint.
func Delete(ctx context.Context, db *sql.DB, userID, endpoint string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, userID, endpoint)
	return err
}

// Count is how many subscriptions the user has.
func Count(ctx context.Context, db *sql.DB, userID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

func forUser(ctx context.Context, db *sql.DB, userID string) ([]Subscription, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256dh, &s.Auth); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Pusher sends notification rows to their users' browsers.
type Pusher struct {
	DB     *sql.DB
	Sender *Sender
	Logf   func(string, ...any)
}

// Push is the notifier's Push hook. It sends in the background.
func (p *Pusher) Push(ctx context.Context, rows []notify.Row) {
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendFor)
		defer cancel()
		p.Send(ctx, rows)
	}()
}

// Send delivers each row to every subscription of its user. A gone
// subscription is deleted, and so is one that failed MaxFailures times in a
// row.
func (p *Pusher) Send(ctx context.Context, rows []notify.Row) {
	for _, row := range rows {
		subs, err := forUser(ctx, p.DB, row.UserID)
		if err != nil {
			p.logf("push: %v", err)
			continue
		}
		msg := Message{Title: row.Title, Body: row.Body, URL: row.URL, Tag: row.CardID}
		for _, sub := range subs {
			p.deliver(ctx, sub, msg)
		}
	}
}

// SendTest sends a test message to every subscription of the user and
// reports how many took it.
func (p *Pusher) SendTest(ctx context.Context, userID string) (int, error) {
	subs, err := forUser(ctx, p.DB, userID)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, sub := range subs {
		if p.deliver(ctx, sub, Message{Title: "Test notification", Body: "Browser notifications are on.", URL: "/account", Tag: "test"}) {
			sent++
		}
	}
	return sent, nil
}

func (p *Pusher) deliver(ctx context.Context, sub Subscription, msg Message) bool {
	err := p.Sender.Send(ctx, sub, msg)
	switch {
	case err == nil:
		_, _ = p.DB.ExecContext(ctx, `UPDATE push_subscriptions SET failures = 0 WHERE id = $1`, sub.ID)
		return true
	case errors.Is(err, ErrGone), errors.Is(err, ErrEndpoint), errors.Is(err, ErrKeys):
		_, _ = p.DB.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, sub.ID)
	default:
		p.logf("push: %v", err)
		var failures int
		if err := p.DB.QueryRowContext(ctx, `
			UPDATE push_subscriptions SET failures = failures + 1 WHERE id = $1 RETURNING failures`, sub.ID).Scan(&failures); err == nil && failures >= MaxFailures {
			_, _ = p.DB.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, sub.ID)
		}
	}
	return false
}

func (p *Pusher) logf(format string, args ...any) {
	if p.Logf != nil {
		p.Logf(format, args...)
	}
}

func newID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "psub_" + hex.EncodeToString(buf[:]), nil
}
