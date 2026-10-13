package sandbox_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/sandbox"
)

const (
	stdioImage  = "ghcr.io/acme/docs@sha256:" + "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	stdioSecret = "docs-secret-value-123"
)

// TestMain runs the test binary as a fake engine when it is called podman.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "podman" {
		os.Exit(fakeEngine(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeEngine appends each call to $HOME/engine.log. `run` serves MCP on
// stdin and stdout; `ps` lists one container.
func fakeEngine(args []string) int {
	logPath := filepath.Join(os.Getenv("HOME"), "engine.log")
	logf := func(format string, a ...any) {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, format+"\n", a...)
			f.Close()
		}
	}
	logf("%s", strings.Join(args, " "))
	switch args[0] {
	case "ps":
		fmt.Println("abc123")
		return 0
	case "rm":
		return 0
	case "run":
	default:
		return 1
	}
	fmt.Fprintln(os.Stderr, "server log with "+os.Getenv("DOCS_TOKEN"))
	out := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	var pending map[string]any
	for scanner.Scan() {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			return 2
		}
		if msg.Method == "" {
			if msg.Error != nil {
				logf("reply %d", msg.Error.Code)
			}
			if pending != nil {
				_ = out.Encode(pending)
				pending = nil
			}
			continue
		}
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": mcp.ProtocolVersion, "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "docs"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "lookup", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/message", "params": map[string]any{}})
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": "srv-1", "method": "roots/list"})
			text := fmt.Sprintf("token %s id %v", os.Getenv("DOCS_TOKEN"), msg.Params.Arguments["id"])
			pending = map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}}
			continue
		default:
			continue
		}
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}
	return 0
}

func fakePodman(t *testing.T) (engine, home string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	engine = filepath.Join(dir, "podman")
	if err := os.Symlink(self, engine); err != nil {
		t.Fatal(err)
	}
	home = t.TempDir()
	t.Setenv("HOME", home)
	return engine, home
}

func engineLog(t *testing.T, home string) string {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(home, "engine.log"))
	return string(raw)
}

func stdioOpts(engine string) sandbox.StdioOpts {
	return sandbox.StdioOpts{
		Engine: engine, Image: stdioImage, Argv: []string{"docs-mcp", "--stdio"},
		Env: map[string]string{"DOCS_TOKEN": stdioSecret}, Name: "orch-mcp-abc123", Label: "card_1",
	}
}

func TestStdioNoNetworkNoArgvSecret(t *testing.T) {
	argv, env, err := sandbox.StdioArgv(stdioOpts("/usr/bin/podman"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(argv, " ")
	for _, want := range []string{"/usr/bin/podman run --rm -i --pull=never --network=none", "--name orch-mcp-abc123", "--label orchestrator.mcp=card_1",
		"--cap-drop=ALL", "--read-only", "--user 65532:65532", "--env DOCS_TOKEN " + stdioImage + " docs-mcp --stdio"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv lacks %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, stdioSecret) || slices.Contains(argv, "--mount") || strings.Contains(joined, "--network=host") {
		t.Fatalf("argv = %s", joined)
	}
	if !slices.Contains(env, "DOCS_TOKEN="+stdioSecret) {
		t.Fatalf("engine env = %v", env)
	}

	bad := map[string]func(*sandbox.StdioOpts){
		"reserved env": func(o *sandbox.StdioOpts) { o.Env = map[string]string{"PATH": "/x"} },
		"env name":     func(o *sandbox.StdioOpts) { o.Env = map[string]string{"A=B": "x"} },
		"floating":     func(o *sandbox.StdioOpts) { o.Image = "ghcr.io/acme/docs:latest" },
		"name":         func(o *sandbox.StdioOpts) { o.Name = "web" },
		"label":        func(o *sandbox.StdioOpts) { o.Label = "a b" },
		"engine":       func(o *sandbox.StdioOpts) { o.Engine = "podman" },
	}
	for name, change := range bad {
		opts := stdioOpts("/usr/bin/podman")
		change(&opts)
		if _, _, err := sandbox.StdioArgv(opts); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

func TestStdioRefusesShell(t *testing.T) {
	for _, argv := range [][]string{{"sh", "-c", "docs-mcp"}, {"bash"}, {"env", "docs-mcp"}, {"/usr/bin/docs-mcp"}, {}} {
		opts := stdioOpts("/usr/bin/podman")
		opts.Argv = argv
		_, _, err := sandbox.StdioArgv(opts)
		if !errors.Is(err, sandbox.ErrWrapper) && !errors.Is(err, sandbox.ErrBare) && !errors.Is(err, sandbox.ErrArgv) {
			t.Fatalf("%v: err = %v", argv, err)
		}
	}
	if _, err := execx.Start(context.Background(), execx.StartChild{Argv: []string{"/bin/sh", "-c", "true"}, MaxFrame: 1}); !errors.Is(err, execx.ErrShell) {
		t.Fatalf("execx.Start shell err = %v", err)
	}
}

func TestStdioFixtureRefused(t *testing.T) {
	opts := stdioOpts("/usr/bin/podman")
	opts.Fixture = true
	started := false
	_, err := sandbox.StartArgv(context.Background(), opts, func(context.Context, execx.StartChild) (*execx.Process, error) {
		started = true
		return nil, errors.New("unreachable")
	}, execx.Command)
	if !errors.Is(err, sandbox.ErrStdioFixture) || started {
		t.Fatalf("err = %v started %v", err, started)
	}
}

func TestStdioContainerRemoved(t *testing.T) {
	engine, home := fakePodman(t)
	srv, err := sandbox.StartArgv(context.Background(), stdioOpts(engine), execx.Start, execx.Command)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	_ = srv.Close()
	log := engineLog(t, home)
	if strings.Count(log, "rm -f orch-mcp-abc123") != 1 {
		t.Fatalf("engine log:\n%s", log)
	}

	failed := stdioOpts(engine)
	failed.Name = "orch-mcp-failed"
	if _, err := sandbox.StartArgv(context.Background(), failed, func(context.Context, execx.StartChild) (*execx.Process, error) {
		return nil, errors.New("engine refused")
	}, execx.Command); err == nil || !strings.Contains(engineLog(t, home), "rm -f orch-mcp-failed") {
		t.Fatalf("failed start: err %v, log:\n%s", err, engineLog(t, home))
	}

	if err := sandbox.SweepMCP(context.Background(), engine, "card_1", execx.Command); err != nil {
		t.Fatal(err)
	}
	log = engineLog(t, home)
	if !strings.Contains(log, "ps -aq --filter label=orchestrator.mcp=card_1") || !strings.Contains(log, "rm -f abc123") {
		t.Fatalf("sweep log:\n%s", log)
	}
}

func TestStdioListAndCall(t *testing.T) {
	engine, home := fakePodman(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	srv, err := sandbox.StartArgv(ctx, stdioOpts(engine), execx.Start, execx.Command)
	if err != nil {
		t.Fatal(err)
	}
	client, err := mcp.DialStdio(ctx, srv)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil || len(tools) != 1 || tools[0].Name != "lookup" {
		t.Fatalf("tools = %v err %v", tools, err)
	}
	res, err := client.CallTool(ctx, "lookup", map[string]any{"id": "T-1"})
	if err != nil || res.Text != "token "+stdioSecret+" id T-1" {
		t.Fatalf("result = %+v err %v", res, err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	log := engineLog(t, home)
	if !strings.Contains(log, "reply -32601") || !strings.Contains(log, "rm -f orch-mcp-abc123") || strings.Contains(log, stdioSecret) {
		t.Fatalf("engine log:\n%s", log)
	}
	if !strings.Contains(string(srv.Stderr()), "server log with") {
		t.Fatalf("stderr = %q", srv.Stderr())
	}
	if _, err := client.ListTools(ctx); err == nil {
		t.Fatal("closed client still answers")
	}
}
