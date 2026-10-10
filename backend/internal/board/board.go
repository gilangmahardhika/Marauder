package board

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"orchestrator/backend/migrations"
)

const maxConns = 4

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrLocked   = errors.New("locked")
)

type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Card struct {
	ID             string
	Title          string
	Status         string
	Risk           string
	Manual         bool
	AttemptsUsed   int
	MaxAttempts    int
	Branch         string
	WorktreePath   string
	WorktreeGitDir string
	PRURL          string
	HeadSHA        string
	ApprovedSHA    string
	DeployedSHA    string
	BlockedReason  string
	FailureReason  string
	LocalCheck     string
	DevCheck       string
	RequirementIDs []string
	CreatedAt      string
	UpdatedAt      string
}

type StatusWrite struct {
	ID               string
	From             string
	To               string
	UpdatedAt        string
	ApprovedSHA      sql.NullString
	SetApprovedSHA   bool
	BlockedReason    sql.NullString
	SetBlockedReason bool
	PRURL            sql.NullString
	SetPRURL         bool
}

type Gate struct {
	ID          string
	CardID      string
	AttemptID   string
	Name        string
	Passed      bool
	EvidenceRef string
	CreatedAt   string
}

func NewID(prefix string) (string, error) {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf[:]), nil
}

func Open(url string) (*sql.DB, error) {
	return OpenInSchema(url, "")
}

func OpenInSchema(url, schema string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	// The events_immutable migration sends the function and its trigger as one
	// query. The extended protocol rejects that.
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if schema != "" {
		cfg.RuntimeParams["search_path"] = schema
	}
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return err
	}
	_, err = provider.Up(context.Background())
	return err
}

func InsertCard(ctx context.Context, q DBTX, card Card) (Card, error) {
	if card.ID == "" {
		id, err := NewID("card_")
		if err != nil {
			return Card{}, err
		}
		card.ID = id
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if card.CreatedAt == "" {
		card.CreatedAt = now
	}
	if card.UpdatedAt == "" {
		card.UpdatedAt = now
	}
	if card.Title == "" {
		card.Title = "card"
	}
	if card.Risk == "" {
		card.Risk = "low"
	}
	if card.MaxAttempts == 0 {
		card.MaxAttempts = 3
	}
	if card.RequirementIDs == nil {
		card.RequirementIDs = []string{}
	}
	reqJSON, err := json.Marshal(card.RequirementIDs)
	if err != nil {
		return Card{}, err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO cards (
			id, title, requirement_ids_json, decision_ids_json, depends_on_json,
			risk, service, status, manual, local_check, dev_check,
			attempts_used, max_attempts, environment, created_at, updated_at
		) VALUES (
			$1, $2, $3, '[]', '[]',
			$4, 'default', $5, $6, $7, $8,
			$9, $10, 'dev', $11, $12
		)`,
		card.ID, card.Title, string(reqJSON), card.Risk, card.Status, card.Manual,
		nullString(card.LocalCheck), nullString(card.DevCheck),
		card.AttemptsUsed, card.MaxAttempts, card.CreatedAt, card.UpdatedAt,
	)
	if err != nil {
		return Card{}, err
	}
	return card, nil
}

func GetCard(ctx context.Context, q DBTX, id string) (Card, error) {
	var card Card
	var reqJSON string
	err := q.QueryRowContext(ctx, `
		SELECT id, title, status, risk, manual, attempts_used, max_attempts,
			COALESCE(branch, ''), COALESCE(worktree_path, ''), COALESCE(worktree_gitdir, ''),
			COALESCE(pr_url, ''), COALESCE(head_sha, ''),
			COALESCE(approved_sha, ''), COALESCE(deployed_sha, ''),
			COALESCE(blocked_reason, ''), COALESCE(failure_reason, ''),
			COALESCE(local_check, ''), COALESCE(dev_check, ''),
			requirement_ids_json, created_at, updated_at
		FROM cards WHERE id = $1`, id).Scan(
		&card.ID, &card.Title, &card.Status, &card.Risk, &card.Manual,
		&card.AttemptsUsed, &card.MaxAttempts, &card.Branch, &card.WorktreePath,
		&card.WorktreeGitDir, &card.PRURL, &card.HeadSHA, &card.ApprovedSHA,
		&card.DeployedSHA, &card.BlockedReason, &card.FailureReason,
		&card.LocalCheck, &card.DevCheck, &reqJSON, &card.CreatedAt, &card.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	if err != nil {
		return Card{}, err
	}
	if reqJSON != "" {
		if err := json.Unmarshal([]byte(reqJSON), &card.RequirementIDs); err != nil {
			return Card{}, err
		}
	}
	return card, nil
}

func CompareAndSwap(ctx context.Context, q DBTX, w StatusWrite) (int64, error) {
	sets := []string{"status = $1", "updated_at = $2"}
	args := []any{w.To, w.UpdatedAt}
	add := func(column string, value any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)+1))
		args = append(args, value)
	}
	if w.SetApprovedSHA {
		add("approved_sha", nullValue(w.ApprovedSHA))
	}
	if w.SetBlockedReason {
		add("blocked_reason", nullValue(w.BlockedReason))
	}
	if w.SetPRURL {
		add("pr_url", nullValue(w.PRURL))
	}
	idPos := len(args) + 1
	fromPos := idPos + 1
	args = append(args, w.ID, w.From)
	query := fmt.Sprintf(
		"UPDATE cards SET %s WHERE id = $%d AND status = $%d",
		strings.Join(sets, ", "), idPos, fromPos,
	)
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func InsertGate(ctx context.Context, q DBTX, gate Gate) error {
	if gate.ID == "" {
		id, err := NewID("gate_")
		if err != nil {
			return err
		}
		gate.ID = id
	}
	if gate.CreatedAt == "" {
		gate.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	var attempt any
	if gate.AttemptID != "" {
		attempt = gate.AttemptID
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO gates (id, card_id, attempt_id, name, passed, evidence_ref, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		gate.ID, gate.CardID, attempt, gate.Name, gate.Passed, gate.EvidenceRef, gate.CreatedAt,
	)
	return err
}

func ClearApprovals(ctx context.Context, q DBTX, cardID, clearedAt string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE approvals SET cleared_at = $1
		WHERE card_id = $2 AND cleared_at IS NULL`,
		clearedAt, cardID,
	)
	return err
}

func CountCards(ctx context.Context, q DBTX) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM cards`).Scan(&n)
	return n, err
}

func CountGates(ctx context.Context, q DBTX, cardID string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM gates WHERE card_id = $1`, cardID).Scan(&n)
	return n, err
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullValue(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}
