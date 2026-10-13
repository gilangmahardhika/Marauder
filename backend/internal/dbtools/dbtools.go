// Package dbtools is the built-in read-only database connectors of a tribe
// (RFC-097). A connector answers the MCP client's list and call requests, so
// a job treats it like any other MCP server.
package dbtools

import (
	"context"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"orchestrator/backend/internal/mcp"
)

const (
	Postgres  = "postgres"
	SQLServer = "sqlserver"

	// PasswordSecret is the only secret name of a connector.
	PasswordSecret = "PASSWORD"

	MaxSQL           = 4000
	MaxRows          = 200
	MaxCell          = 1 << 10
	MaxError         = 300
	maxName          = 128
	ConnectTimeout   = 5 * time.Second
	StatementTimeout = 15 * time.Second
)

var (
	ErrBoard     = errors.New("the board database is refused")
	ErrStatement = errors.New("only one SELECT or WITH statement is allowed")

	hostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,62})(\.[A-Za-z0-9]([A-Za-z0-9-]{0,62}))*$`)
)

// Config is a connector's connection without its password.
type Config struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Database      string `json:"database"`
	User          string `json:"user"`
	TLSMode       string `json:"tlsMode"`
	ReadOnlyLogin bool   `json:"readOnlyLogin"`
}

// IsDatabase reports whether transport is a built-in connector.
func IsDatabase(transport string) bool {
	return transport == Postgres || transport == SQLServer
}

// DefaultPort is the engine's usual port.
func DefaultPort(transport string) int {
	if transport == SQLServer {
		return 1433
	}
	return 5432
}

// CheckConfig returns field errors keyed "config.<field>".
func CheckConfig(c Config) map[string]string {
	fields := map[string]string{}
	host := strings.TrimSpace(c.Host)
	if host != c.Host || !validHost(host) {
		fields["config.host"] = "host is a name or an IP address"
	}
	if c.Port < 1 || c.Port > 65535 {
		fields["config.port"] = "port is 1 to 65535"
	}
	if c.Database == "" || len(c.Database) > maxName || strings.ContainsAny(c.Database, "\x00\r\n") {
		fields["config.database"] = "database is 1 to 128 characters"
	}
	if c.User == "" || len(c.User) > maxName || strings.ContainsAny(c.User, "\x00\r\n") {
		fields["config.user"] = "user is 1 to 128 characters"
	}
	switch c.TLSMode {
	case "disable", "require", "verify-full":
	default:
		fields["config.tlsMode"] = "TLS mode is disable, require, or verify-full"
	}
	if !c.ReadOnlyLogin {
		fields["config.readOnlyLogin"] = "confirm that this login can only read"
	}
	return fields
}

func validHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	return hostName.MatchString(host)
}

var (
	schemaArg = map[string]any{"type": "string", "maxLength": maxName, "description": "Schema name. Empty means every schema."}
	toolList  = []mcp.Tool{
		{
			Name:        "describe_table",
			Description: "List a table's columns with their types, nullability, and defaults.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"schema": schemaArg,
					"table":  map[string]any{"type": "string", "maxLength": maxName, "description": "Table or view name."},
				},
				"required":             []any{"table"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "list_tables",
			Description: "List the tables and views, optionally in one schema.",
			Schema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"schema": schemaArg},
				"additionalProperties": false,
			},
		},
		{
			Name:        "query",
			Description: "Run one read-only SQL statement and return at most 200 rows as a text table.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{"type": "string", "maxLength": MaxSQL, "description": "One SELECT statement."},
				},
				"required":             []any{"sql"},
				"additionalProperties": false,
			},
		},
	}
)

// Tools are a connector's fixed tools in name order. Their schemas never
// change, so their hashes hold across releases.
func Tools() []mcp.Tool {
	raw, _ := json.Marshal(toolList)
	var out []mcp.Tool
	_ = json.Unmarshal(raw, &out)
	return out
}

// Open connects with a 5 second limit and checks the login with SELECT 1.
// Errors are mcp errors, so callers report fixed text.
func Open(ctx context.Context, transport string, cfg Config, password string, guard *Guard) (mcp.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()
	var client mcp.Client
	var err error
	switch transport {
	case Postgres:
		client, err = openPostgres(ctx, cfg, password, guard)
	case SQLServer:
		client, err = openSQLServer(ctx, cfg, password, guard)
	default:
		return nil, mcp.ErrProtocol
	}
	if err == nil {
		return client, nil
	}
	switch {
	case errors.Is(err, ErrBoard):
		return nil, mcp.ErrBlocked
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
		return nil, context.DeadlineExceeded
	default:
		return nil, mcp.ErrUnreachable
	}
}

// args reads a tool's string arguments.
func stringArg(args map[string]any, key string, limit int) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok || len(s) > limit {
		return "", fmt.Errorf("%s must be a string of at most %d characters", key, limit)
	}
	return s, nil
}

func errorResult(err error) mcp.Result {
	return mcp.Result{Text: clip(err.Error(), MaxError), IsError: true}
}

func clip(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return strings.ToValidUTF8(s[:limit], "")
}

// table renders rows as a text table: the header, then one line per row,
// at most MaxRows rows and mcp.MaxResponse bytes.
type table struct {
	b     strings.Builder
	rows  int
	cut   bool
	limit int
}

func newTable(columns []string) *table {
	t := &table{limit: mcp.MaxResponse}
	t.b.WriteString(strings.Join(columns, " | "))
	t.b.WriteString("\n")
	return t
}

// add appends one row. It returns false once the table is full.
func (t *table) add(values []any) bool {
	if t.rows >= MaxRows {
		t.cut = true
		return false
	}
	cells := make([]string, len(values))
	for i, v := range values {
		cells[i] = cell(v)
	}
	line := strings.Join(cells, " | ") + "\n"
	if t.b.Len()+len(line) > t.limit {
		t.cut = true
		return false
	}
	t.b.WriteString(line)
	t.rows++
	return true
}

func (t *table) String() string {
	if t.cut {
		return t.b.String() + "(" + strconv.Itoa(t.rows) + " rows shown; more rows were cut)"
	}
	return t.b.String() + "(" + strconv.Itoa(t.rows) + " rows)"
}

func cell(v any) string {
	var s string
	switch t := v.(type) {
	case nil:
		return "NULL"
	case string:
		s = t
	case []byte:
		if utf8.Valid(t) {
			s = string(t)
		} else {
			s = `\x` + hex.EncodeToString(t)
		}
	case time.Time:
		s = t.Format(time.RFC3339Nano)
	case [16]byte:
		h := hex.EncodeToString(t[:])
		s = h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	case map[string]any, []any:
		raw, _ := json.Marshal(t)
		s = string(raw)
	case driver.Valuer:
		dv, err := t.Value()
		if err != nil {
			s = fmt.Sprint(t)
		} else {
			return cell(dv)
		}
	default:
		s = fmt.Sprint(t)
	}
	s = strings.NewReplacer("\r", "", "\n", `\n`).Replace(s)
	return clip(s, MaxCell)
}
