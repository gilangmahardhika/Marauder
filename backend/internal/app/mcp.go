package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"orchestrator/backend/internal/dbtools"
	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/netguard"
	"orchestrator/backend/internal/tenancy"
)

type MCPServer = tenancy.MCPServer
type MCPServerInput = tenancy.MCPServerInput
type MCPTool = tenancy.MCPTool
type MCPConn = tenancy.MCPConn

// ErrMCPStdio is returned for a stdio server when no sandbox engine can run it.
var ErrMCPStdio = errors.New("stdio servers need the container sandbox")

const discoverTimeout = 30 * time.Second

// MCPDialer opens a tribe's MCP server (RFC-093, RFC-097). The zero value
// reaches public https servers and any database, and refuses stdio servers.
type MCPDialer struct {
	Policy   netguard.Policy
	Resolver netguard.Resolver
	// Stdio starts a stdio server in the sandbox; nil refuses stdio.
	Stdio func(ctx context.Context, conn MCPConn) (mcp.Client, error)
	// Board refuses a database connector to the board database (RFC-097).
	Board *dbtools.Guard
}

// Open connects to one server and runs initialize.
func (d MCPDialer) Open(ctx context.Context, conn MCPConn) (mcp.Client, error) {
	switch conn.Transport {
	case "http":
		if _, err := netguard.CheckURL(conn.URL, d.Policy); err != nil {
			return nil, mcp.ErrBlocked
		}
		return mcp.DialHTTP(ctx, conn.URL, conn.Secrets, netguard.Client(d.Policy, d.Resolver))
	case "stdio":
		if d.Stdio == nil {
			return nil, ErrMCPStdio
		}
		return d.Stdio(ctx, conn)
	case dbtools.Postgres, dbtools.SQLServer:
		if conn.Config == nil {
			return nil, mcp.ErrProtocol
		}
		return dbtools.Open(ctx, conn.Transport, *conn.Config, conn.Secrets[dbtools.PasswordSecret], d.Board)
	default:
		return nil, mcp.ErrProtocol
	}
}

// MCPFailure is a fixed message for an MCP error. It never carries the
// server's response text.
func MCPFailure(err error) string {
	if errors.Is(err, ErrMCPStdio) {
		return ErrMCPStdio.Error()
	}
	return mcp.Failure(err)
}

// DiscoverError is a failed discovery; Reason is from MCPFailure.
type DiscoverError struct {
	Reason string
}

func (e *DiscoverError) Error() string { return e.Reason }

// DiscoveredTool is one tool a server lists. Valid is false for a name the
// allowlist cannot take.
type DiscoveredTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Schema       map[string]any `json:"schema"`
	SchemaSHA256 string         `json:"schemaSha256"`
	Valid        bool           `json:"valid"`
}

func ListMCPServers(ctx context.Context, db *sql.DB, actor User, tribeID string) ([]MCPServer, error) {
	return tenancy.ListMCPServers(ctx, db, actor, tribeID)
}

func CreateMCPServer(ctx context.Context, db *sql.DB, actor User, key []byte, d MCPDialer, tribeID string, in MCPServerInput) (MCPServer, []string, error) {
	return tenancy.CreateMCPServer(ctx, db, actor, key, d.Policy, tribeID, in)
}

func SaveMCPServer(ctx context.Context, db *sql.DB, actor User, key []byte, d MCPDialer, tribeID, serverID string, in MCPServerInput) (MCPServer, []string, error) {
	return tenancy.SaveMCPServer(ctx, db, actor, key, d.Policy, tribeID, serverID, in)
}

func DeleteMCPServer(ctx context.Context, db *sql.DB, actor User, tribeID, serverID string) error {
	return tenancy.DeleteMCPServer(ctx, db, actor, tribeID, serverID)
}

// DiscoverMCPTools connects to one of the tribe's servers and lists its
// tools for the admin to allowlist.
func DiscoverMCPTools(ctx context.Context, db *sql.DB, actor User, key []byte, d MCPDialer, tribeID, serverID string) ([]DiscoveredTool, error) {
	conn, err := tenancy.OpenMCPServer(ctx, db, actor, key, tribeID, serverID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()
	client, err := d.Open(ctx, conn)
	if err != nil {
		return nil, &DiscoverError{Reason: MCPFailure(err)}
	}
	defer client.Close()
	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, &DiscoverError{Reason: MCPFailure(err)}
	}
	out := make([]DiscoveredTool, 0, len(tools))
	for _, t := range tools {
		desc := t.Description
		if len(desc) > tenancy.MaxToolDescription {
			desc = strings.ToValidUTF8(desc[:tenancy.MaxToolDescription], "")
		}
		out = append(out, DiscoveredTool{
			Name: t.Name, Description: desc, Schema: t.Schema,
			SchemaSHA256: tenancy.SchemaHash(t.Schema), Valid: tenancy.MCPToolPattern.MatchString(t.Name),
		})
	}
	return out, nil
}
