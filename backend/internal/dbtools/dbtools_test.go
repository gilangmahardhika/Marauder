package dbtools_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/dbtools"
	"orchestrator/backend/internal/envfile"
	"orchestrator/backend/internal/mcp"
)

// testDB is the test database as a connector config, with a fresh schema
// holding table t of 250 rows.
func testDB(t *testing.T) (dbtools.Config, string, *sql.DB, string) {
	t.Helper()
	if err := envfile.Load(); err != nil {
		t.Fatal(err)
	}
	raw := os.Getenv("ORCHESTRATOR_TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("ORCHESTRATOR_TEST_DATABASE_URL is required")
	}
	pc, err := pgx.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	tlsMode := "disable"
	if pc.TLSConfig != nil {
		tlsMode = "require"
	}
	cfg := dbtools.Config{Host: pc.Host, Port: int(pc.Port), Database: pc.Database, User: pc.User, TLSMode: tlsMode, ReadOnlyLogin: true}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	schema := "dbt" + hex.EncodeToString(buf[:])
	admin, err := board.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	})
	for _, stmt := range []string{
		"CREATE SCHEMA " + schema,
		"CREATE TABLE " + schema + ".t (id integer PRIMARY KEY, name text NOT NULL)",
		"INSERT INTO " + schema + ".t SELECT g, 'row ' || g FROM generate_series(1, 250) g",
	} {
		if _, err := admin.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return cfg, pc.Password, admin, schema
}

func openPG(t *testing.T, cfg dbtools.Config, password string) mcp.Client {
	t.Helper()
	client, err := dbtools.Open(context.Background(), dbtools.Postgres, cfg, password, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func call(t *testing.T, c mcp.Client, name string, args map[string]any) mcp.Result {
	t.Helper()
	res, err := c.CallTool(context.Background(), name, args)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func rowCount(t *testing.T, db *sql.DB, schema string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + schema + ".t").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPostgresToolReadOnly(t *testing.T) {
	cfg, password, admin, schema := testDB(t)
	client := openPG(t, cfg, password)
	for _, stmt := range []string{
		"INSERT INTO " + schema + ".t VALUES (999, 'x')",
		"UPDATE " + schema + ".t SET name = 'y'",
		"WITH d AS (DELETE FROM " + schema + ".t RETURNING id) SELECT count(*) FROM d",
		"SELECT nextval('" + schema + ".nope')",
	} {
		res := call(t, client, "query", map[string]any{"sql": stmt})
		if !res.IsError {
			t.Fatalf("%s ran: %q", stmt, res.Text)
		}
	}
	res := call(t, client, "query", map[string]any{"sql": "INSERT INTO " + schema + ".t VALUES (999, 'x')"})
	if !strings.Contains(res.Text, "read-only") || !strings.Contains(res.Text, "25006") {
		t.Fatalf("insert error = %q", res.Text)
	}
	if n := rowCount(t, admin, schema); n != 250 {
		t.Fatalf("rows = %d", n)
	}
	ok := call(t, client, "query", map[string]any{"sql": "SELECT name FROM " + schema + ".t WHERE id = 7"})
	if ok.IsError || !strings.Contains(ok.Text, "row 7") || !strings.HasSuffix(ok.Text, "(1 rows)") {
		t.Fatalf("select = %q", ok.Text)
	}
}

func TestPostgresSingleStatement(t *testing.T) {
	cfg, password, admin, schema := testDB(t)
	client := openPG(t, cfg, password)
	res := call(t, client, "query", map[string]any{"sql": "SELECT 1; DROP TABLE " + schema + ".t"})
	if !res.IsError {
		t.Fatalf("two statements ran: %q", res.Text)
	}
	res = call(t, client, "query", map[string]any{"sql": "COMMIT; SET transaction_read_only = off"})
	if !res.IsError {
		t.Fatalf("commit ran: %q", res.Text)
	}
	if n := rowCount(t, admin, schema); n != 250 {
		t.Fatalf("rows = %d", n)
	}
	if res := call(t, client, "query", map[string]any{"sql": "SELECT 2 AS two"}); res.IsError || !strings.Contains(res.Text, "two\n2\n") {
		t.Fatalf("connection unusable after refusal: %q", res.Text)
	}
}

func TestPostgresRowCap(t *testing.T) {
	cfg, password, _, schema := testDB(t)
	client := openPG(t, cfg, password)
	res := call(t, client, "query", map[string]any{"sql": "SELECT id, name FROM " + schema + ".t ORDER BY id"})
	lines := strings.Split(res.Text, "\n")
	if lines[0] != "id | name" || lines[1] != "1 | row 1" || len(lines) != dbtools.MaxRows+2 {
		t.Fatalf("table = %d lines, head %q", len(lines), lines[:2])
	}
	if !strings.HasSuffix(res.Text, "(200 rows shown; more rows were cut)") {
		t.Fatalf("tail = %q", lines[len(lines)-1])
	}
	long := call(t, client, "query", map[string]any{"sql": "SELECT repeat('x', 5000) AS v, E'a\\nb' AS w"})
	if strings.Contains(long.Text, strings.Repeat("x", dbtools.MaxCell+1)) || !strings.Contains(long.Text, `a\nb`) {
		t.Fatalf("cell not clipped: %d bytes", len(long.Text))
	}
	tables := call(t, client, "list_tables", map[string]any{"schema": schema})
	if !strings.Contains(tables.Text, schema+" | t | BASE TABLE") || !strings.HasSuffix(tables.Text, "(1 rows)") {
		t.Fatalf("list_tables = %q", tables.Text)
	}
	cols := call(t, client, "describe_table", map[string]any{"schema": schema, "table": "t"})
	if !strings.Contains(cols.Text, "name | text | NO") || !strings.HasSuffix(cols.Text, "(2 rows)") {
		t.Fatalf("describe_table = %q", cols.Text)
	}
	tools, _ := client.ListTools(context.Background())
	if len(tools) != 3 || tools[0].Name != "describe_table" || tools[2].Name != "query" {
		t.Fatalf("tools = %v", tools)
	}
}

func TestDBConnectorRefusesBoard(t *testing.T) {
	cfg, password, _, _ := testDB(t)
	guard, err := dbtools.BoardGuard(os.Getenv("ORCHESTRATOR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbtools.Open(context.Background(), dbtools.Postgres, cfg, password, guard); !errors.Is(err, mcp.ErrBlocked) {
		t.Fatalf("board connector err = %v", err)
	}
	upper := cfg
	upper.Database = strings.ToUpper(cfg.Database)
	upper.Host = "localhost"
	if _, err := dbtools.Open(context.Background(), dbtools.Postgres, upper, password, guard); !errors.Is(err, mcp.ErrBlocked) {
		t.Fatalf("board by another name err = %v", err)
	}
	if mcp.Failure(mcp.ErrBlocked) != "the server address was refused" {
		t.Fatal("blocked failure text changed")
	}
	wrong := cfg
	wrong.Port = 1
	if _, err := dbtools.Open(context.Background(), dbtools.Postgres, wrong, password, guard); !errors.Is(err, mcp.ErrUnreachable) {
		t.Fatalf("closed port err = %v", err)
	}
}

func TestDBConfigCheck(t *testing.T) {
	good := dbtools.Config{Host: "db.internal", Port: 5432, Database: "app", User: "reader", TLSMode: "require", ReadOnlyLogin: true}
	if fields := dbtools.CheckConfig(good); len(fields) != 0 {
		t.Fatalf("good config = %v", fields)
	}
	for _, host := range []string{"10.0.0.5", "127.0.0.1", "::1", "localhost"} {
		c := good
		c.Host = host
		if fields := dbtools.CheckConfig(c); len(fields) != 0 {
			t.Fatalf("host %s refused: %v", host, fields)
		}
	}
	bad := dbtools.CheckConfig(dbtools.Config{Host: "/var/run/postgresql", Port: 0, TLSMode: "prefer"})
	for _, key := range []string{"config.host", "config.port", "config.database", "config.user", "config.tlsMode", "config.readOnlyLogin"} {
		if bad[key] == "" {
			t.Fatalf("missing %s in %v", key, bad)
		}
	}
	for _, host := range []string{"a,b", "db host", "db\\inst", "x@y"} {
		c := good
		c.Host = host
		if dbtools.CheckConfig(c)["config.host"] == "" {
			t.Fatalf("host %q allowed", host)
		}
	}
}

func TestSQLServerStatementCheck(t *testing.T) {
	allowed := []string{
		"SELECT 1",
		"select top 10 * from dbo.Orders;",
		"WITH x AS (SELECT 1 AS a) SELECT a FROM x",
		"SELECT 'a;b' AS s -- trailing; comment",
		"SELECT [update], [delete] FROM t /* insert into x */",
		"SELECT N'it''s; fine' AS s ;  ",
		"SELECT \"set\" FROM t",
	}
	for _, q := range allowed {
		if err := dbtools.CheckSelect(q); err != nil {
			t.Fatalf("refused %q", q)
		}
	}
	refused := []string{
		"",
		"INSERT INTO t VALUES (1)",
		"SELECT 1; DROP TABLE t",
		"SELECT 1 DROP TABLE t",
		"SELECT * INTO copy FROM t",
		"EXEC sp_who",
		"SELECT * FROM OPENROWSET('SQLNCLI', 'x', 'SELECT 1')",
		"SELECT * FROM OPENQUERY(srv, 'SELECT 1')",
		"WITH d AS (SELECT 1 AS a) DELETE FROM t",
		"SELECT 1; SELECT 2",
		"SELECT 'unclosed",
		"SELECT 1 /* open",
		"UPDATE t SET a = 1",
		"SELECT 1 WAITFOR DELAY '00:01'",
		"DECLARE @x int SELECT @x",
		strings.Repeat("SELECT 1 ", dbtools.MaxSQL),
	}
	for _, q := range refused {
		if err := dbtools.CheckSelect(q); !errors.Is(err, dbtools.ErrStatement) {
			t.Fatalf("allowed %q", q)
		}
	}
}

// TestSQLServerLive runs against ORCH_TEST_MSSQL, a URL such as
// sqlserver://reader:secret@127.0.0.1:1433?database=app&encrypt=disable.
func TestSQLServerLive(t *testing.T) {
	_ = envfile.Load()
	raw := os.Getenv("ORCH_TEST_MSSQL")
	if raw == "" {
		t.Skip("ORCH_TEST_MSSQL is not set")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 1433
	}
	password, _ := u.User.Password()
	tlsMode := "require"
	if u.Query().Get("encrypt") == "disable" {
		tlsMode = "disable"
	}
	cfg := dbtools.Config{Host: u.Hostname(), Port: port, Database: u.Query().Get("database"), User: u.User.Username(), TLSMode: tlsMode, ReadOnlyLogin: true}
	client, err := dbtools.Open(context.Background(), dbtools.SQLServer, cfg, password, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	res := call(t, client, "query", map[string]any{"sql": "SELECT 1 AS one"})
	if res.IsError || !strings.Contains(res.Text, "one\n1\n") {
		t.Fatalf("select = %q", res.Text)
	}
	if res := call(t, client, "query", map[string]any{"sql": "CREATE TABLE orch_probe (a int)"}); !res.IsError {
		t.Fatalf("create ran: %q", res.Text)
	}
	if res := call(t, client, "list_tables", nil); res.IsError {
		t.Fatalf("list_tables = %q", res.Text)
	}
}
