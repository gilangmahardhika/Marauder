package app

import (
	"context"
	"errors"
	"log"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/sandbox"
	"orchestrator/backend/internal/trace"
)

// StdioOpener starts stdio servers in engine's sandbox, labelled with label
// so SweepMCP can remove what a crash left (RFC-093). A server lives until
// its client is closed, not until the context of its handshake ends.
func StdioOpener(engine string, fixture bool, label string, tools Tools) func(context.Context, MCPConn) (mcp.Client, error) {
	run := tools.Exec
	if run == nil {
		run = execx.Command
	}
	return func(ctx context.Context, conn MCPConn) (mcp.Client, error) {
		name, err := board.NewID("orch-mcp-")
		if err != nil {
			return nil, err
		}
		srv, err := sandbox.StartArgv(context.WithoutCancel(ctx), sandbox.StdioOpts{
			Engine: engine, Image: conn.Image, Argv: conn.Argv, Env: conn.Secrets,
			Name: name, Label: label, Fixture: fixture,
		}, execx.Start, run)
		if errors.Is(err, sandbox.ErrStdioFixture) || errors.Is(err, sandbox.ErrEngine) {
			return nil, ErrMCPStdio
		}
		if err != nil {
			return nil, mcp.ErrUnreachable
		}
		client, err := mcp.DialStdio(ctx, srv)
		if err != nil {
			logStderr(conn, srv)
			return nil, err
		}
		return &stdioClient{Client: client, conn: conn, srv: srv}, nil
	}
}

type stdioClient struct {
	mcp.Client
	conn MCPConn
	srv  *sandbox.Stdio
}

func (c *stdioClient) Close() error {
	err := c.Client.Close()
	logStderr(c.conn, c.srv)
	return err
}

// logStderr writes a server's stderr tail to the process log, redacted. It
// never reaches a model, an event, or the API.
func logStderr(conn MCPConn, srv *sandbox.Stdio) {
	_ = srv.Close()
	text := strings.TrimSpace(string(srv.Stderr()))
	if text == "" {
		return
	}
	secrets := make([]string, 0, len(conn.Secrets))
	for _, value := range conn.Secrets {
		secrets = append(secrets, value)
	}
	log.Printf("mcp %s stderr: %s", conn.Slug, trace.Redact(text, secrets))
}
