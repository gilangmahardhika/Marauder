package dbtools

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"

	mssql "github.com/microsoft/go-mssqldb"

	"orchestrator/backend/internal/mcp"
)

type msClient struct {
	mu   sync.Mutex
	db   *sql.DB
	conn *sql.Conn
}

func openSQLServer(ctx context.Context, cfg Config, password string, guard *Guard) (*msClient, error) {
	q := url.Values{}
	q.Set("database", cfg.Database)
	q.Set("app name", "orchestrator-mcp")
	q.Set("dial timeout", "5")
	switch cfg.TLSMode {
	case "disable":
		q.Set("encrypt", "disable")
	case "require":
		q.Set("encrypt", "true")
		q.Set("TrustServerCertificate", "true")
	default:
		q.Set("encrypt", "true")
	}
	u := url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(cfg.User, password),
		Host:     net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		RawQuery: q.Encode(),
	}
	connector, err := mssql.NewConnector(u.String())
	if err != nil {
		return nil, mcp.ErrProtocol
	}
	connector.Dialer = dialer{guard: guard, database: cfg.Database}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "SELECT 1"); err != nil {
		conn.Close()
		db.Close()
		return nil, err
	}
	return &msClient{db: db, conn: conn}, nil
}

func (c *msClient) ListTools(context.Context) ([]mcp.Tool, error) {
	return Tools(), nil
}

func (c *msClient) CallTool(ctx context.Context, name string, args map[string]any) (mcp.Result, error) {
	switch name {
	case "query":
		text, err := stringArg(args, "sql", MaxSQL)
		if err != nil {
			return errorResult(err), nil
		}
		if err := CheckSelect(text); err != nil {
			return errorResult(err), nil
		}
		return c.run(ctx, text)
	case "list_tables":
		schema, err := stringArg(args, "schema", maxName)
		if err != nil {
			return errorResult(err), nil
		}
		return c.run(ctx, `SELECT TABLE_SCHEMA, TABLE_NAME, TABLE_TYPE FROM INFORMATION_SCHEMA.TABLES
			WHERE (@p1 = '' OR TABLE_SCHEMA = @p1) ORDER BY TABLE_SCHEMA, TABLE_NAME`, schema)
	case "describe_table":
		schema, err := stringArg(args, "schema", maxName)
		if err != nil {
			return errorResult(err), nil
		}
		table, err := stringArg(args, "table", maxName)
		if err != nil || table == "" {
			return errorResult(errors.New("table is required")), nil
		}
		return c.run(ctx, `SELECT TABLE_SCHEMA, COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_DEFAULT FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_NAME = @p1 AND (@p2 = '' OR TABLE_SCHEMA = @p2) ORDER BY TABLE_SCHEMA, ORDINAL_POSITION`, table, schema)
	default:
		return mcp.Result{}, mcp.ErrProtocol
	}
}

// run reads one statement in a transaction that always rolls back. SQL
// Server has no read-only transaction; the login and CheckSelect guard it.
func (c *msClient) run(ctx context.Context, text string, args ...any) (mcp.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, StatementTimeout)
	defer cancel()
	tx, err := c.conn.BeginTx(ctx, nil)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return mcp.Result{}, context.DeadlineExceeded
		}
		return mcp.Result{}, mcp.ErrUnreachable
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, text, args...)
	if err != nil {
		return msResult(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return msResult(err)
	}
	out := newTable(columns)
	values := make([]any, len(columns))
	ptrs := make([]any, len(columns))
	for i := range values {
		ptrs[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return msResult(err)
		}
		if !out.add(values) {
			break
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return msResult(err)
	}
	return mcp.Result{Text: out.String()}, nil
}

func msResult(err error) (mcp.Result, error) {
	var msErr mssql.Error
	if errors.As(err, &msErr) {
		return errorResult(errors.New(msErr.Message + " (error " + strconv.Itoa(int(msErr.Number)) + ")")), nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return mcp.Result{}, context.DeadlineExceeded
	}
	if errors.Is(err, sql.ErrConnDone) || errors.Is(err, context.Canceled) {
		return mcp.Result{}, mcp.ErrUnreachable
	}
	return errorResult(err), nil
}

func (c *msClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.Close()
	return c.db.Close()
}

// keywords that end a read: writes, DDL, and server-side calls. T-SQL lets a
// second statement follow without a semicolon, so they are refused anywhere.
var refusedWords = map[string]bool{
	"INTO": true, "EXEC": true, "EXECUTE": true, "OPENROWSET": true, "OPENQUERY": true, "OPENDATASOURCE": true,
	"INSERT": true, "UPDATE": true, "DELETE": true, "MERGE": true, "DROP": true, "ALTER": true, "CREATE": true,
	"TRUNCATE": true, "GRANT": true, "REVOKE": true, "DENY": true, "BACKUP": true, "RESTORE": true,
	"SHUTDOWN": true, "KILL": true, "DBCC": true, "USE": true, "DECLARE": true, "SET": true,
	"WAITFOR": true, "RECONFIGURE": true, "BULK": true,
}

// CheckSelect allows one SELECT or WITH statement, with strings, quoted
// names, and comments skipped. A trailing semicolon is allowed.
func CheckSelect(text string) error {
	if len(text) > MaxSQL {
		return ErrStatement
	}
	var words []string
	ended := false
	r := []rune(text)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case c == '-' && i+1 < len(r) && r[i+1] == '-':
			for i < len(r) && r[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(r) && r[i+1] == '*':
			depth := 0
			for i < len(r) {
				if r[i] == '/' && i+1 < len(r) && r[i+1] == '*' {
					depth++
					i += 2
					continue
				}
				if r[i] == '*' && i+1 < len(r) && r[i+1] == '/' {
					depth--
					i += 2
					if depth == 0 {
						break
					}
					continue
				}
				i++
			}
			if depth != 0 {
				return ErrStatement
			}
		case c == '\'' || c == '"' || c == '[':
			if ended {
				return ErrStatement
			}
			closer := c
			if c == '[' {
				closer = ']'
			}
			i++
			closed := false
			for i < len(r) {
				if r[i] == closer {
					if i+1 < len(r) && r[i+1] == closer {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return ErrStatement
			}
		case c == ';':
			ended = true
			i++
		case isWordRune(c):
			if ended {
				return ErrStatement
			}
			start := i
			for i < len(r) && isWordRune(r[i]) {
				i++
			}
			words = append(words, strings.ToUpper(string(r[start:i])))
		default:
			if ended && !isSpace(c) {
				return ErrStatement
			}
			i++
		}
	}
	if len(words) == 0 || (words[0] != "SELECT" && words[0] != "WITH") {
		return ErrStatement
	}
	for _, w := range words {
		if refusedWords[w] {
			return ErrStatement
		}
	}
	return nil
}

func isWordRune(c rune) bool {
	return c == '_' || c == '@' || c == '#' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c > 127 && !isSpace(c)
}

func isSpace(c rune) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' || c == 0xa0
}
