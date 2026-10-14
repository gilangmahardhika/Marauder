package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Channel is the NOTIFY channel the migration 31 triggers send on.
const Channel = "orchestrator"

// Listener timing (RFC-113); tests shorten them.
var (
	// Overlap reads events again whose transaction may have committed after
	// a later seq was read.
	Overlap = 10 * time.Second
	// ScanEvery reads events even when no NOTIFY arrived.
	ScanEvery = 30 * time.Second
	// Debounce gathers the changes of one burst of writes.
	Debounce = 250 * time.Millisecond
	// seenFor keeps event ids already handed on for longer than Overlap.
	seenFor = time.Minute
	// pageSize is how many events one read takes after the cursor.
	pageSize = 500
)

// Event is one stored event read after the cursor. ProjectID is the card's
// project, since older transition events carry proj_default.
type Event struct {
	ID        string
	Seq       int64
	TS        string
	CardID    string
	ProjectID string
	ActorID   string
	Kind      string
	Payload   string
}

// Listener wakes on NOTIFY from any process that writes the board, hands new
// events to Events, and tells the hub which projects changed. NOTIFY only
// wakes it: the seq cursor in notify_cursor decides which events are new.
type Listener struct {
	DB *sql.DB
	// Connect opens the dedicated connection that runs LISTEN.
	Connect func(context.Context) (*pgx.Conn, error)
	Hub     *Hub
	// Events receives each new event once per process; nil drops them.
	Events func(context.Context, []Event)
	Logf   func(string, ...any)

	schema string
	seen   map[string]time.Time
}

type wake struct {
	Table     string `json:"t"`
	Schema    string `json:"s"`
	ProjectID string `json:"projectId"`
	CardID    string `json:"cardId"`
	UserID    string `json:"userId"`
	ID        string `json:"id"`
}

// Run listens until ctx ends, reconnecting with a backoff. After a
// reconnect every stream resyncs, since changes made meanwhile sent no
// NOTIFY it heard.
func (l *Listener) Run(ctx context.Context) {
	if err := l.DB.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&l.schema); err != nil && ctx.Err() == nil {
		l.logf("notify: schema: %v", err)
	}
	delay := time.Second
	for first := true; ; first = false {
		err := l.session(ctx, !first)
		if ctx.Err() != nil {
			return
		}
		l.logf("notify: listener: %v", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, 30*time.Second)
	}
}

func (l *Listener) session(ctx context.Context, resync bool) error {
	conn, err := l.Connect(ctx)
	if err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		conn.Close(context.Background())
		return err
	}
	wctx, cancel := context.WithCancel(ctx)
	notes := make(chan string)
	errc := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			n, err := conn.WaitForNotification(wctx)
			if err != nil {
				errc <- err
				return
			}
			select {
			case notes <- n.Payload:
			case <-wctx.Done():
				errc <- wctx.Err()
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-done
		conn.Close(context.Background())
	}()
	if resync && l.Hub != nil {
		l.Hub.Resync()
	}
	l.Scan(ctx)

	tick := time.NewTicker(ScanEvery)
	defer tick.Stop()
	var flush <-chan time.Time
	pending := map[string]bool{}
	scanDue := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errc:
			return err
		case payload := <-notes:
			var w wake
			if json.Unmarshal([]byte(payload), &w) != nil || w.Schema != l.schema {
				continue
			}
			switch w.Table {
			case "notifications":
				if l.Hub != nil {
					l.Hub.Notified(w.UserID, w.ID)
				}
				continue
			case "events":
				scanDue = true
			}
			if w.ProjectID != "" {
				pending[w.ProjectID] = true
			}
			if flush == nil {
				flush = time.After(Debounce)
			}
		case <-flush:
			flush = nil
			if scanDue {
				scanDue = false
				l.Scan(ctx)
			}
			l.changed(pending)
			clear(pending)
		case <-tick.C:
			l.changed(l.Scan(ctx))
		}
	}
}

func (l *Listener) changed(projects map[string]bool) {
	if l.Hub == nil {
		return
	}
	for id := range projects {
		l.Hub.Changed(id)
	}
}

// Scan hands on the events after the cursor, and those within Overlap that
// it has not handed on yet, then moves the cursor. It returns the projects
// of the events it handed on.
func (l *Listener) Scan(ctx context.Context) map[string]bool {
	if l.seen == nil {
		l.seen = map[string]time.Time{}
	}
	now := time.Now()
	for id, at := range l.seen {
		if now.Sub(at) > seenFor {
			delete(l.seen, id)
		}
	}
	var cursor int64
	if err := l.DB.QueryRowContext(ctx, `SELECT seq FROM notify_cursor WHERE id = 1`).Scan(&cursor); err != nil {
		l.logf("notify: cursor: %v", err)
		return nil
	}
	since := now.UTC().Add(-Overlap).Format(time.RFC3339)
	events, err := l.read(ctx, `e.seq <= $1 AND e.ts >= $2 ORDER BY e.seq`, cursor, since)
	if err != nil {
		l.logf("notify: events: %v", err)
		return nil
	}
	last := cursor
	for {
		page, err := l.read(ctx, `e.seq > $1 ORDER BY e.seq LIMIT $2`, last, pageSize)
		if err != nil {
			l.logf("notify: events: %v", err)
			return nil
		}
		events = append(events, page...)
		if len(page) > 0 {
			last = page[len(page)-1].Seq
		}
		if len(page) < pageSize {
			break
		}
	}
	fresh := events[:0:0]
	projects := map[string]bool{}
	for _, ev := range events {
		if _, ok := l.seen[ev.ID]; ok {
			continue
		}
		l.seen[ev.ID] = now
		fresh = append(fresh, ev)
		projects[ev.ProjectID] = true
	}
	if len(fresh) > 0 && l.Events != nil {
		l.Events(ctx, fresh)
	}
	if last > cursor {
		if _, err := l.DB.ExecContext(ctx, `UPDATE notify_cursor SET seq = GREATEST(seq, $1) WHERE id = 1`, last); err != nil {
			l.logf("notify: cursor: %v", err)
		}
	}
	return projects
}

func (l *Listener) read(ctx context.Context, where string, args ...any) ([]Event, error) {
	rows, err := l.DB.QueryContext(ctx, `
		SELECT e.id, e.seq, e.ts, COALESCE(e.card_id, ''), COALESCE(c.project_id, e.project_id),
			COALESCE(e.actor_id, ''), e.kind, e.payload_json
		FROM events e LEFT JOIN cards c ON c.id = e.card_id
		WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.Seq, &ev.TS, &ev.CardID, &ev.ProjectID, &ev.ActorID, &ev.Kind, &ev.Payload); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (l *Listener) logf(format string, args ...any) {
	if l.Logf != nil {
		l.Logf(format, args...)
	}
}
