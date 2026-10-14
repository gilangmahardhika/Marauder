package notify

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
)

// BodyMax is the most runes of a notification body: the card title and the
// project name, nothing else.
const BodyMax = 120

// ListMax is the most notifications one read returns.
const ListMax = 50

var ErrNotFound = errors.New("notification not found")

// Row is one stored notification, as the API returns it.
type Row struct {
	ID        string  `json:"id"`
	UserID    string  `json:"-"`
	Kind      string  `json:"kind"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	ProjectID string  `json:"projectId"`
	CardID    string  `json:"cardId"`
	URL       string  `json:"url"`
	Push      bool    `json:"-"`
	CreatedAt string  `json:"createdAt"`
	ReadAt    *string `json:"readAt"`
}

// Notifier writes the notifications of new events and hands the rows this
// process wrote, and that should reach a browser, to Push.
type Notifier struct {
	DB   *sql.DB
	Data string
	Push func(context.Context, []Row)
	Logf func(string, ...any)
}

// Fanout is the listener's Events hook.
func (n *Notifier) Fanout(ctx context.Context, events []Event) {
	var pushed []Row
	for _, ev := range events {
		if ev.CardID == "" || (ev.Kind != "transition" && ev.Kind != "review_failed") {
			continue
		}
		rows, err := n.write(ctx, ev)
		if err != nil {
			if n.Logf != nil {
				n.Logf("notify: %s: %v", ev.ID, err)
			}
			continue
		}
		for _, row := range rows {
			if row.Push {
				pushed = append(pushed, row)
			}
		}
	}
	if len(pushed) > 0 && n.Push != nil {
		n.Push(ctx, pushed)
	}
}

// write inserts one row per recipient and returns the rows it inserted. A
// row another process or an earlier read inserted is skipped, so it is sent
// once.
func (n *Notifier) write(ctx context.Context, ev Event) ([]Row, error) {
	card, err := board.GetCard(ctx, n.DB, ev.CardID)
	if errors.Is(err, board.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	notes, err := Recipients(ctx, n.DB, n.Data, ev, card)
	if err != nil || len(notes) == 0 {
		return nil, err
	}
	var project string
	if err := n.DB.QueryRowContext(ctx, `SELECT name FROM projects WHERE id = $1`, card.ProjectID).Scan(&project); err != nil {
		return nil, err
	}
	body := clip(card.Title+" · "+project, BodyMax)
	url := "/cards/" + card.ID + "?project=" + card.ProjectID
	now := time.Now().UTC().Format(time.RFC3339)
	var out []Row
	for _, note := range notes {
		id, err := newID()
		if err != nil {
			return out, err
		}
		res, err := n.DB.ExecContext(ctx, `
			INSERT INTO notifications (id, event_id, user_id, kind, project_id, card_id, title, body, url, push, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (event_id, user_id, kind) DO NOTHING`,
			id, ev.ID, note.UserID, note.Kind, card.ProjectID, card.ID, note.Title, body, url, note.Push, now)
		if err != nil {
			return out, err
		}
		if inserted, _ := res.RowsAffected(); inserted == 1 {
			out = append(out, Row{ID: id, UserID: note.UserID, Kind: note.Kind, Title: note.Title, Body: body,
				ProjectID: card.ProjectID, CardID: card.ID, URL: url, Push: note.Push, CreatedAt: now})
		}
	}
	return out, nil
}

// visible keeps the rows of projects the user can still see.
const visible = `
	FROM notifications n
	JOIN projects p ON p.id = n.project_id
	JOIN tribe_members m ON m.tribe_id = p.tribe_id AND m.user_id = n.user_id
	WHERE n.user_id = $1`

// List returns the user's newest notifications, unread ones only when
// unread is set, and the unread count.
func List(ctx context.Context, db *sql.DB, userID string, unread bool) ([]Row, int, error) {
	filter := ""
	if unread {
		filter = " AND n.read_at IS NULL"
	}
	rows, err := db.QueryContext(ctx, `
		SELECT n.id, n.kind, n.title, n.body, n.project_id, COALESCE(n.card_id, ''), n.url, n.created_at, n.read_at`+
		visible+filter+`
		ORDER BY n.created_at DESC, n.id DESC LIMIT $2`, userID, ListMax)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		row, err := scanRow(rows)
		if err != nil {
			return nil, 0, err
		}
		row.UserID = userID
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	count, err := Unread(ctx, db, userID)
	return out, count, err
}

// Unread counts the user's unread notifications in projects they see.
func Unread(ctx context.Context, db *sql.DB, userID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*)`+visible+` AND n.read_at IS NULL`, userID).Scan(&n)
	return n, err
}

// Get returns one of the user's notifications in a project they see.
func Get(ctx context.Context, db *sql.DB, userID, id string) (Row, error) {
	row, err := scanRow(db.QueryRowContext(ctx, `
		SELECT n.id, n.kind, n.title, n.body, n.project_id, COALESCE(n.card_id, ''), n.url, n.created_at, n.read_at`+
		visible+` AND n.id = $2`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Row{}, ErrNotFound
	}
	row.UserID = userID
	return row, err
}

// MarkRead marks the given notifications of the user read, or all of them.
func MarkRead(ctx context.Context, db *sql.DB, userID string, ids []string, all bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if all {
		_, err := db.ExecContext(ctx, `UPDATE notifications SET read_at = $2 WHERE user_id = $1 AND read_at IS NULL`, userID, now)
		return err
	}
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, `
			UPDATE notifications SET read_at = $3 WHERE user_id = $1 AND id = $2 AND read_at IS NULL`, userID, id, now); err != nil {
			return err
		}
	}
	return nil
}

type scanner interface {
	Scan(...any) error
}

func scanRow(s scanner) (Row, error) {
	var row Row
	var readAt sql.NullString
	err := s.Scan(&row.ID, &row.Kind, &row.Title, &row.Body, &row.ProjectID, &row.CardID, &row.URL, &row.CreatedAt, &readAt)
	if readAt.Valid {
		row.ReadAt = &readAt.String
	}
	return row, err
}

func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

func newID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "ntf_" + hex.EncodeToString(buf[:]), nil
}
