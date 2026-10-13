package dbtools

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchestrator/backend/internal/mcp"
)

type pgClient struct {
	mu   sync.Mutex
	conn *pgx.Conn
}

// quote is a libpq keyword value in single quotes.
func quote(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
}

func openPostgres(ctx context.Context, cfg Config, password string, guard *Guard) (*pgClient, error) {
	dsn := strings.Join([]string{
		"host=" + quote(cfg.Host),
		"port=" + strconv.Itoa(cfg.Port),
		"dbname=" + quote(cfg.Database),
		"user=" + quote(cfg.User),
		"sslmode=" + quote(cfg.TLSMode),
		"connect_timeout=5",
	}, " ")
	pc, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pc.Password = password
	pc.Fallbacks = nil
	pc.RuntimeParams["default_transaction_read_only"] = "on"
	pc.RuntimeParams["application_name"] = "orchestrator-mcp"
	pc.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec
	pc.ConnectTimeout = ConnectTimeout
	pc.DialFunc = dialer{guard: guard, database: cfg.Database}.DialContext
	conn, err := pgx.ConnectConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, "SELECT 1"); err != nil {
		_ = conn.Close(context.Background())
		return nil, err
	}
	return &pgClient{conn: conn}, nil
}

func (c *pgClient) ListTools(context.Context) ([]mcp.Tool, error) {
	return Tools(), nil
}

func (c *pgClient) CallTool(ctx context.Context, name string, args map[string]any) (mcp.Result, error) {
	switch name {
	case "query":
		sql, err := stringArg(args, "sql", MaxSQL)
		if err != nil {
			return errorResult(err), nil
		}
		if strings.TrimSpace(sql) == "" {
			return errorResult(ErrStatement), nil
		}
		return c.run(ctx, sql)
	case "list_tables":
		schema, err := stringArg(args, "schema", maxName)
		if err != nil {
			return errorResult(err), nil
		}
		return c.run(ctx, `SELECT table_schema, table_name, table_type FROM information_schema.tables
			WHERE table_schema NOT IN ('pg_catalog', 'information_schema') AND ($1::text = '' OR table_schema = $1::text)
			ORDER BY table_schema, table_name`, schema)
	case "describe_table":
		schema, err := stringArg(args, "schema", maxName)
		if err != nil {
			return errorResult(err), nil
		}
		table, err := stringArg(args, "table", maxName)
		if err != nil || table == "" {
			return errorResult(errors.New("table is required")), nil
		}
		return c.run(ctx, `SELECT table_schema, column_name, data_type, is_nullable, column_default FROM information_schema.columns
			WHERE table_name = $1::text AND ($2::text = '' OR table_schema = $2::text)
			ORDER BY table_schema, ordinal_position`, table, schema)
	default:
		return mcp.Result{}, mcp.ErrProtocol
	}
}

// run reads one statement in a read-only transaction that always rolls back.
// The extended protocol refuses a second statement.
func (c *pgClient) run(ctx context.Context, sql string, args ...any) (mcp.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn.IsClosed() {
		return mcp.Result{}, mcp.ErrUnreachable
	}
	ctx, cancel := context.WithTimeout(ctx, StatementTimeout+5*time.Second)
	defer cancel()
	tx, err := c.conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return mcp.Result{}, mcp.ErrUnreachable
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = "+strconv.Itoa(int(StatementTimeout/time.Millisecond))); err != nil {
		return pgResult(err)
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return pgResult(err)
	}
	defer rows.Close()
	fields := rows.FieldDescriptions()
	columns := make([]string, len(fields))
	for i, f := range fields {
		columns[i] = f.Name
	}
	out := newTable(columns)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return pgResult(err)
		}
		if !out.add(values) {
			break
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return pgResult(err)
	}
	return mcp.Result{Text: out.String()}, nil
}

// pgResult is a statement error as the tool's text. A lost connection is a
// transport error with fixed text.
func pgResult(err error) (mcp.Result, error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return errorResult(errors.New(pgErr.Severity + ": " + pgErr.Message + " (SQLSTATE " + pgErr.Code + ")")), nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return mcp.Result{}, context.DeadlineExceeded
	}
	if pgconn.SafeToRetry(err) || errors.Is(err, context.Canceled) {
		return mcp.Result{}, mcp.ErrUnreachable
	}
	return errorResult(err), nil
}

func (c *pgClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), ConnectTimeout)
	defer cancel()
	return c.conn.Close(ctx)
}
