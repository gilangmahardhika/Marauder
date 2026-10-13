package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/dbtools"
	"orchestrator/backend/internal/envfile"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/tenancy"
)

// testPGConfig is the test database as a connector, and its password.
func testPGConfig(t *testing.T) (dbtools.Config, string) {
	t.Helper()
	if err := envfile.Load(); err != nil {
		t.Fatal(err)
	}
	pc, err := pgx.ParseConfig(os.Getenv("ORCHESTRATOR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if pc.Password != "" && len(pc.Password) < 8 {
		t.Skip("the test database password is shorter than a saved secret may be")
	}
	tlsMode := "disable"
	if pc.TLSConfig != nil {
		tlsMode = "require"
	}
	return dbtools.Config{Host: pc.Host, Port: int(pc.Port), Database: pc.Database, User: pc.User, TLSMode: tlsMode, ReadOnlyLogin: true}, pc.Password
}

// seedPG adds the "pg" connector to the tribe, allowlisting its fixed tools,
// and a table dbt in the fixture's schema. It returns the schema.
func seedPG(t *testing.T, f jobFixture) (string, string) {
	t.Helper()
	cfg, password := testPGConfig(t)
	var schema string
	if err := f.db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE dbt (id integer PRIMARY KEY, name text NOT NULL)`,
		`INSERT INTO dbt VALUES (1, 'alpha'), (2, 'beta')`,
	} {
		if _, err := f.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	var tools []tenancy.MCPTool
	for _, tool := range dbtools.Tools() {
		tools = append(tools, tenancy.MCPTool{Name: tool.Name, Description: tool.Description, Schema: tool.Schema})
	}
	in := app.MCPServerInput{Slug: "pg", Transport: dbtools.Postgres, Config: &cfg, Tools: tools}
	if password != "" {
		in.Secrets = map[string]string{dbtools.PasswordSecret: password}
	}
	if _, _, err := app.CreateMCPServer(context.Background(), f.db, mcpAdmin(t, f), tokenKey, mcpDialer, "tribe_default", in); err != nil {
		t.Fatal(err)
	}
	return schema, password
}

func TestDBConfigValidation(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	admin := mcpAdmin(t, f)
	cfg := dbtools.Config{Host: "10.0.0.5", Port: 5432, Database: "app", User: "reader", TLSMode: "require"}
	_, _, err := app.CreateMCPServer(context.Background(), f.db, admin, tokenKey, mcpDialer, "tribe_default", app.MCPServerInput{
		Slug: "pg", Transport: dbtools.Postgres, URL: "https://db.example.com/mcp", Config: &cfg,
		Secrets: map[string]string{"Authorization": "Bearer abcdefgh123"},
	})
	var v *identity.Validation
	if !errors.As(err, &v) {
		t.Fatalf("err = %v", err)
	}
	for _, key := range []string{"transport", "config.readOnlyLogin", "secrets"} {
		if v.Fields[key] == "" {
			t.Fatalf("missing %s in %v", key, v.Fields)
		}
	}
	_, _, err = app.CreateMCPServer(context.Background(), f.db, admin, tokenKey, mcpDialer, "tribe_default", app.MCPServerInput{
		Slug: "mssql", Transport: dbtools.SQLServer,
	})
	if !errors.As(err, &v) || v.Fields["config"] == "" {
		t.Fatalf("missing config err = %v", err)
	}

	cfg.ReadOnlyLogin = true
	srv, fields, err := app.CreateMCPServer(context.Background(), f.db, admin, tokenKey, mcpDialer, "tribe_default", app.MCPServerInput{
		Slug: "mssql", Transport: dbtools.SQLServer, Config: &cfg,
		Secrets: map[string]string{dbtools.PasswordSecret: "db-password-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if srv.Config == nil || *srv.Config != cfg || len(srv.SecretsSet) != 1 || srv.SecretsSet[0] != dbtools.PasswordSecret {
		t.Fatalf("saved = %+v", srv)
	}
	if !strings.Contains(strings.Join(fields, ","), "config") {
		t.Fatalf("changed fields = %v", fields)
	}
	list, err := app.ListMCPServers(context.Background(), f.db, admin, "tribe_default")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "db-password-1") || !strings.Contains(string(raw), `"host":"10.0.0.5"`) {
		t.Fatalf("list = %s", raw)
	}
	var stored string
	if err := f.db.QueryRow(`SELECT config_json FROM tribe_mcp_servers WHERE id = $1`, srv.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "db-password-1") {
		t.Fatal("password stored in config_json")
	}
}

func TestImplementerDBTools(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	schema, _ := seedPG(t, f)
	provider, card := f.runMCP(t, `{"implement":[
		{"tool":"mcp_pg_query","sql":"SELECT name FROM `+schema+`.dbt ORDER BY id"},
		{"tool":"mcp_pg_query","sql":"DELETE FROM `+schema+`.dbt"},
		{"tool":"edit_file","path":"hello.txt","text":"job\n"},
		{"tool":"commit","message":"Job commit"}]}`, nil)
	if card.Status != "pr_open" {
		t.Fatalf("card = %s", card.Status)
	}
	prompts := provider.Prompts()
	tools := prompts[0].Tools
	if tools[len(tools)-1] != "mcp_pg_query" || tools[len(tools)-3] != "mcp_pg_describe_table" {
		t.Fatalf("tools = %v", tools)
	}
	read := prompts[1].Turns[len(prompts[1].Turns)-1].Results[0]
	if !strings.Contains(read, contextpack.MarkerStart+"\nMCP RESULT pg/query\n") || !strings.Contains(read, "name\nalpha\nbeta\n(2 rows)") {
		t.Fatalf("query result = %q", read)
	}
	write := prompts[2].Turns[len(prompts[2].Turns)-1].Results[0]
	if !strings.Contains(write, "read-only") {
		t.Fatalf("delete result = %q", write)
	}
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM dbt`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("rows = %d, %v", n, err)
	}
	ctxEvents := named(cardEvents(t, f, "tool_call"), "mcp_context")
	servers, _ := ctxEvents[0]["servers"].([]any)
	desc, _ := servers[0].(map[string]any)
	cfg, _ := testPGConfig(t)
	if desc["transport"] != "postgres" || desc["database"] != cfg.Database || !strings.HasSuffix(desc["host"].(string), ":"+strconv.Itoa(cfg.Port)) {
		t.Fatalf("mcp_context = %v", desc)
	}
}

func TestDBPasswordRedacted(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	_, password := seedPG(t, f)
	if password == "" {
		t.Skip("the test database has no password")
	}
	codes := make([]string, 0, len(password))
	for _, r := range password {
		codes = append(codes, "chr("+strconv.Itoa(int(r))+")")
	}
	provider, _ := f.runMCP(t, `{"implement":[
		{"tool":"mcp_pg_query","sql":"SELECT `+strings.Join(codes, " || ")+` AS echo"},
		{"tool":"edit_file","path":"hello.txt","text":"job\n"},
		{"tool":"commit","message":"Job commit"}]}`, nil)
	result := provider.Prompts()[1].Turns[len(provider.Prompts()[1].Turns)-1].Results[0]
	if strings.Contains(result, password) || !strings.Contains(result, "echo\n") {
		t.Fatalf("model saw the password: %q", result)
	}
	var leaks int
	if err := f.db.QueryRow(`SELECT count(*) FROM events WHERE payload_json LIKE '%' || $1 || '%'`, password).Scan(&leaks); err != nil {
		t.Fatal(err)
	}
	if leaks != 0 {
		t.Fatalf("%d events hold the password", leaks)
	}
}
