package tenancy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/dbtools"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/netguard"
	"orchestrator/backend/internal/sandbox"
)

// Limits of a tribe's MCP servers (RFC-092).
const (
	MaxMCPServers      = 8
	MaxMCPTools        = 32
	MaxMCPSecrets      = 8
	MaxSchemaBytes     = 8 << 10
	MaxSchemaDepth     = 5
	MaxToolDescription = 512
	minSecret          = 8
)

var (
	MCPSlugPattern = regexp.MustCompile(`^[a-z][a-z0-9]{1,15}$`)
	MCPToolPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	headerPattern  = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	envPattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

	// reservedHeaders are set by the MCP client itself.
	reservedHeaders = map[string]bool{
		"host": true, "content-type": true, "content-length": true, "accept": true, "connection": true,
		"mcp-session-id": true, "mcp-protocol-version": true, "transfer-encoding": true,
	}
)

// MCPTool is one allowlisted tool. Description is the admin's text; the
// server's own description is never sent to a model.
type MCPTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Schema       map[string]any `json:"schema"`
	SchemaSHA256 string         `json:"schemaSha256"`
}

// MCPServer is a tribe's server as the API returns it. SecretsSet names the
// headers (http), environment variables (stdio), or PASSWORD (a database)
// that hold a value. Config is a database connector's connection, without
// its password, and nil for the other transports.
type MCPServer struct {
	ID         string          `json:"id"`
	TribeID    string          `json:"tribeId"`
	Slug       string          `json:"slug"`
	Transport  string          `json:"transport"`
	URL        string          `json:"url"`
	Image      string          `json:"image"`
	Argv       []string        `json:"argv"`
	Config     *dbtools.Config `json:"config"`
	Enabled    bool            `json:"enabled"`
	SecretsSet []string        `json:"secretsSet"`
	Tools      []MCPTool       `json:"tools"`
	UpdatedAt  string          `json:"updatedAt"`
}

// MCPServerInput is a create or save. A Secrets entry with an empty value
// keeps the stored value; Clear names values to delete.
type MCPServerInput struct {
	Slug      string            `json:"slug"`
	Transport string            `json:"transport"`
	URL       string            `json:"url"`
	Image     string            `json:"image"`
	Argv      []string          `json:"argv"`
	Config    *dbtools.Config   `json:"config"`
	Enabled   *bool             `json:"enabled"`
	Secrets   map[string]string `json:"secrets"`
	Clear     []string          `json:"clear"`
	Tools     []MCPTool         `json:"tools"`
}

// MCPConn is a server with its opened secrets, for a job or discovery. It is
// never encoded to a client.
type MCPConn struct {
	MCPServer
	Secrets map[string]string `json:"-"`
}

// SchemaHash is the SHA-256 of the schema's JSON with sorted keys.
func SchemaHash(schema map[string]any) string {
	raw, _ := json.Marshal(normalSchema(schema))
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func normalSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return schema
}

// schemaDepth counts nested schemas. A map of named schemas, such as
// properties, is not a level of its own.
func schemaDepth(v any) int {
	switch t := v.(type) {
	case map[string]any:
		deepest := 0
		for key, child := range t {
			named, ok := child.(map[string]any)
			if ok && (key == "properties" || key == "patternProperties" || key == "$defs" || key == "definitions") {
				for _, schema := range named {
					deepest = max(deepest, schemaDepth(schema))
				}
				continue
			}
			deepest = max(deepest, schemaDepth(child))
		}
		return deepest + 1
	case []any:
		deepest := 0
		for _, child := range t {
			deepest = max(deepest, schemaDepth(child))
		}
		return deepest
	default:
		return 0
	}
}

// ListMCPServers returns the tribe's servers for an admin of it.
func ListMCPServers(ctx context.Context, db *sql.DB, actor identity.User, tribeID string) ([]MCPServer, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return nil, err
	}
	conns, err := loadMCP(ctx, db, nil, `s.tribe_id = $1`, tribeID)
	if err != nil {
		return nil, err
	}
	out := make([]MCPServer, len(conns))
	for i, c := range conns {
		out[i] = c.MCPServer
	}
	return out, nil
}

// OpenMCPServer returns one of the tribe's servers with its secrets, for
// discovery by an admin of the tribe.
func OpenMCPServer(ctx context.Context, db *sql.DB, actor identity.User, key []byte, tribeID, serverID string) (MCPConn, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return MCPConn{}, err
	}
	conns, err := loadMCP(ctx, db, key, `s.tribe_id = $1 AND s.id = $2`, tribeID, serverID)
	if err != nil {
		return MCPConn{}, err
	}
	if len(conns) == 0 {
		return MCPConn{}, identity.ErrNotFound
	}
	return conns[0], nil
}

// ProjectMCPServers returns the enabled servers of the project's tribe, in
// slug order with tools in name order, with their secrets. It is unchecked:
// a job of the project calls it.
func ProjectMCPServers(ctx context.Context, db *sql.DB, key []byte, projectID string) ([]MCPConn, error) {
	return loadMCP(ctx, db, key, `s.enabled AND s.tribe_id = (SELECT tribe_id FROM projects WHERE id = $1)`, projectID)
}

// MCPMention is what a project member may see of a tribe server to mention
// it in a requirement (RFC-101): no transport, host or secret.
type MCPMention struct {
	Slug      string `json:"slug"`
	Enabled   bool   `json:"enabled"`
	ToolCount int    `json:"toolCount"`
}

// ProjectMCPMentions lists every server of the project's tribe, enabled or
// not, in slug order. It is unchecked: the caller already sees the project.
func ProjectMCPMentions(ctx context.Context, db *sql.DB, projectID string) ([]MCPMention, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.slug, s.enabled, COUNT(t.name)
		FROM tribe_mcp_servers s LEFT JOIN tribe_mcp_tools t ON t.server_id = s.id
		WHERE s.tribe_id = (SELECT tribe_id FROM projects WHERE id = $1)
		GROUP BY s.id, s.slug, s.enabled ORDER BY s.slug`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MCPMention{}
	for rows.Next() {
		var m MCPMention
		if err := rows.Scan(&m.Slug, &m.Enabled, &m.ToolCount); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadMCP(ctx context.Context, db *sql.DB, key []byte, where string, args ...any) ([]MCPConn, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.tribe_id, s.slug, s.transport, s.url, s.image, s.argv_json, s.config_json, s.enabled,
			s.secret_names_json, s.secrets_ciphertext, s.updated_at
		FROM tribe_mcp_servers s WHERE `+where+` ORDER BY s.slug`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MCPConn
	for rows.Next() {
		var c MCPConn
		var argv, config, names, cipher string
		if err := rows.Scan(&c.ID, &c.TribeID, &c.Slug, &c.Transport, &c.URL, &c.Image, &argv, &config, &c.Enabled, &names, &cipher, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if dbtools.IsDatabase(c.Transport) {
			c.Config = &dbtools.Config{}
			if err := json.Unmarshal([]byte(config), c.Config); err != nil {
				return nil, err
			}
		}
		if err := json.Unmarshal([]byte(argv), &c.Argv); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(names), &c.SecretsSet); err != nil {
			return nil, err
		}
		c.Secrets = map[string]string{}
		if key != nil && cipher != "" {
			plain, err := identity.OpenToken(key, cipher)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal([]byte(plain), &c.Secrets); err != nil {
				return nil, err
			}
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range out {
		tools, err := loadMCPTools(ctx, db, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Tools = tools
		if out[i].Argv == nil {
			out[i].Argv = []string{}
		}
		if out[i].SecretsSet == nil {
			out[i].SecretsSet = []string{}
		}
	}
	return out, nil
}

func loadMCPTools(ctx context.Context, db *sql.DB, serverID string) ([]MCPTool, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, description, schema_json, schema_sha256 FROM tribe_mcp_tools
		WHERE server_id = $1 ORDER BY name`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tools := []MCPTool{}
	for rows.Next() {
		var t MCPTool
		var schema string
		if err := rows.Scan(&t.Name, &t.Description, &schema, &t.SchemaSHA256); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(schema), &t.Schema); err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, rows.Err()
}

// CreateMCPServer adds a server to the tribe. It returns the saved server and
// the names of the fields it set, for config_changed.
func CreateMCPServer(ctx context.Context, db *sql.DB, actor identity.User, key []byte, policy netguard.Policy, tribeID string, in MCPServerInput) (MCPServer, []string, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return MCPServer{}, nil, err
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribe_mcp_servers WHERE tribe_id = $1`, tribeID).Scan(&n); err != nil {
		return MCPServer{}, nil, err
	}
	if n >= MaxMCPServers {
		return MCPServer{}, nil, &identity.Validation{Fields: map[string]string{"slug": "a tribe has at most 8 MCP servers"}}
	}
	id, err := board.NewID("mcp_")
	if err != nil {
		return MCPServer{}, nil, err
	}
	return saveMCP(ctx, db, actor, key, policy, tribeID, id, MCPConn{Secrets: map[string]string{}}, true, in)
}

// SaveMCPServer replaces one of the tribe's servers.
func SaveMCPServer(ctx context.Context, db *sql.DB, actor identity.User, key []byte, policy netguard.Policy, tribeID, serverID string, in MCPServerInput) (MCPServer, []string, error) {
	current, err := OpenMCPServer(ctx, db, actor, key, tribeID, serverID)
	if err != nil {
		return MCPServer{}, nil, err
	}
	return saveMCP(ctx, db, actor, key, policy, tribeID, serverID, current, false, in)
}

// DeleteMCPServer removes one of the tribe's servers and its allowlist.
func DeleteMCPServer(ctx context.Context, db *sql.DB, actor identity.User, tribeID, serverID string) error {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `DELETE FROM tribe_mcp_servers WHERE tribe_id = $1 AND id = $2`, tribeID, serverID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return identity.ErrNotFound
	}
	return nil
}

func saveMCP(ctx context.Context, db *sql.DB, actor identity.User, key []byte, policy netguard.Policy, tribeID, id string, current MCPConn, create bool, in MCPServerInput) (MCPServer, []string, error) {
	fields := map[string]string{}
	in.Slug = strings.TrimSpace(in.Slug)
	in.URL = strings.TrimSpace(in.URL)
	in.Image = strings.TrimSpace(in.Image)
	if !MCPSlugPattern.MatchString(in.Slug) {
		fields["slug"] = "slug is 2 to 16 lowercase letters and digits, starting with a letter"
	}
	if in.Argv == nil {
		in.Argv = []string{}
	}
	switch in.Transport {
	case "http":
		if _, err := netguard.CheckURL(in.URL, policy); err != nil {
			fields["url"] = err.Error()
		}
		if in.Image != "" || len(in.Argv) > 0 {
			fields["transport"] = "an http server has a URL, not an image or argv"
		}
	case "stdio":
		if !sandbox.PinnedImage(in.Image) {
			fields["image"] = "image must be pinned by @sha256: digest"
		}
		if len(in.Argv) == 0 || sandbox.ParseArgv(in.Argv, in.Argv[:1]) != nil {
			fields["argv"] = "argv is 1 to 64 strings; the first is a bare program name and not a shell"
		}
		if in.URL != "" {
			fields["transport"] = "a stdio server has an image and argv, not a URL"
		}
	case dbtools.Postgres, dbtools.SQLServer:
		if in.URL != "" || in.Image != "" || len(in.Argv) > 0 {
			fields["transport"] = "a database server has a connection, not a URL, image, or argv"
		}
		if in.Config == nil {
			fields["config"] = "a database server needs a host, port, database, and user"
		} else {
			for k, v := range dbtools.CheckConfig(*in.Config) {
				fields[k] = v
			}
		}
	default:
		fields["transport"] = "transport is http, stdio, postgres, or sqlserver"
	}
	configJSON := "{}"
	if dbtools.IsDatabase(in.Transport) && in.Config != nil {
		raw, _ := json.Marshal(in.Config)
		configJSON = string(raw)
	}

	secrets := map[string]string{}
	for name, value := range current.Secrets {
		secrets[name] = value
	}
	var touched []string
	for _, name := range in.Clear {
		if _, ok := secrets[name]; ok {
			delete(secrets, name)
			touched = append(touched, "secrets."+name)
		}
	}
	for name, value := range in.Secrets {
		if !secretNameOK(in.Transport, name) {
			fields["secrets"] = "secret name " + name + " is not a valid header, environment, or PASSWORD name"
			continue
		}
		if value == "" {
			continue
		}
		if len(value) < minSecret {
			fields["secrets."+name] = "value is too short"
			continue
		}
		secrets[name] = value
		touched = append(touched, "secrets."+name)
	}
	if !create && in.Transport != current.Transport && len(in.Secrets) == 0 {
		for name := range secrets {
			if !secretNameOK(in.Transport, name) {
				delete(secrets, name)
				touched = append(touched, "secrets."+name)
			}
		}
	}
	if len(secrets) > MaxMCPSecrets {
		fields["secrets"] = "a server has at most 8 secrets"
	}

	tools, toolErr := checkTools(in.Tools)
	if toolErr != "" {
		fields["tools"] = toolErr
	}
	if in.Slug != "" && fields["slug"] == "" {
		var other int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribe_mcp_servers WHERE tribe_id = $1 AND slug = $2 AND id <> $3`, tribeID, in.Slug, id).Scan(&other); err != nil {
			return MCPServer{}, nil, err
		}
		if other > 0 {
			fields["slug"] = "slug is already used in this tribe"
		}
	}
	if len(fields) > 0 {
		return MCPServer{}, nil, &identity.Validation{Fields: fields}
	}

	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	namesJSON, _ := json.Marshal(names)
	argvJSON, _ := json.Marshal(in.Argv)
	cipher := ""
	if len(secrets) > 0 {
		plain, _ := json.Marshal(secrets)
		sealed, err := identity.Seal(key, plain)
		if err != nil {
			return MCPServer{}, nil, err
		}
		cipher = sealed
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	} else if !create {
		enabled = current.Enabled
	}
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return MCPServer{}, nil, err
	}
	defer tx.Rollback()
	if create {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO tribe_mcp_servers (id, tribe_id, slug, transport, url, image, argv_json, config_json, enabled,
				secret_names_json, secrets_ciphertext, updated_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)`,
			id, tribeID, in.Slug, in.Transport, in.URL, in.Image, string(argvJSON), configJSON, enabled, string(namesJSON), cipher, nullable(actor.ID), now)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE tribe_mcp_servers SET slug = $3, transport = $4, url = $5, image = $6, argv_json = $7, config_json = $8,
				enabled = $9, secret_names_json = $10, secrets_ciphertext = $11, updated_by = $12, updated_at = $13
			WHERE tribe_id = $1 AND id = $2`,
			tribeID, id, in.Slug, in.Transport, in.URL, in.Image, string(argvJSON), configJSON, enabled, string(namesJSON), cipher, nullable(actor.ID), now)
	}
	if err != nil {
		return MCPServer{}, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tribe_mcp_tools WHERE server_id = $1`, id); err != nil {
		return MCPServer{}, nil, err
	}
	for _, t := range tools {
		schema, _ := json.Marshal(t.Schema)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tribe_mcp_tools (server_id, name, description, schema_json, schema_sha256)
			VALUES ($1, $2, $3, $4, $5)`, id, t.Name, t.Description, string(schema), t.SchemaSHA256); err != nil {
			return MCPServer{}, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return MCPServer{}, nil, err
	}
	changed := []string{"slug", "transport", "url", "image", "argv", "config", "enabled", "tools"}
	changed = append(changed, touched...)
	conns, err := loadMCP(ctx, db, nil, `s.tribe_id = $1 AND s.id = $2`, tribeID, id)
	if err != nil {
		return MCPServer{}, nil, err
	}
	if len(conns) == 0 {
		return MCPServer{}, nil, errors.New("saved server is missing")
	}
	return conns[0].MCPServer, changed, nil
}

func nullable(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func secretNameOK(transport, name string) bool {
	if dbtools.IsDatabase(transport) {
		return name == dbtools.PasswordSecret
	}
	if transport == "stdio" {
		return envPattern.MatchString(name) && !sandbox.ReservedEnv(name)
	}
	return headerPattern.MatchString(name) && !reservedHeaders[strings.ToLower(name)]
}

// checkTools validates the allowlist and fills each schema hash.
func checkTools(in []MCPTool) ([]MCPTool, string) {
	if len(in) > MaxMCPTools {
		return nil, "a server has at most 32 tools"
	}
	var seen []string
	out := make([]MCPTool, 0, len(in))
	for _, t := range in {
		if !MCPToolPattern.MatchString(t.Name) {
			return nil, "tool name " + t.Name + " must be 1 to 40 letters, digits, '_', or '-'"
		}
		if slices.Contains(seen, t.Name) {
			return nil, "tool " + t.Name + " is listed twice"
		}
		seen = append(seen, t.Name)
		desc := strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' {
				return -1
			}
			return r
		}, t.Description))
		if len(desc) > MaxToolDescription {
			return nil, "tool " + t.Name + " description is over 512 bytes"
		}
		schema := normalSchema(t.Schema)
		raw, err := json.Marshal(schema)
		if err != nil || len(raw) > MaxSchemaBytes {
			return nil, "tool " + t.Name + " schema is over 8 KiB"
		}
		if schemaDepth(schema) > MaxSchemaDepth {
			return nil, "tool " + t.Name + " schema is more than 5 levels deep"
		}
		if typ, ok := schema["type"]; ok && typ != "object" {
			return nil, "tool " + t.Name + " schema must be an object"
		}
		out = append(out, MCPTool{Name: t.Name, Description: desc, Schema: schema, SchemaSHA256: SchemaHash(schema)})
	}
	return out, ""
}
