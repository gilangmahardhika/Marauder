package sandbox

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"orchestrator/backend/internal/execx"
)

// MCPLabel marks the containers of stdio MCP servers. Its value is the card
// or project the server runs for, so a sweep removes only that scope's
// leftovers.
const MCPLabel = "orchestrator.mcp"

// MaxStdioFrame is the longest line read from a stdio server.
const MaxStdioFrame = 1 << 20

var (
	ErrStdioFixture = errors.New("stdio MCP servers need the container sandbox")
	ErrStdioEnv     = errors.New("stdio environment name is not allowed")
	ErrStdioName    = errors.New("stdio container name or label is not allowed")

	containerName = regexp.MustCompile(`^orch-mcp-[a-z0-9-]{1,80}$`)
	labelValue    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	envName       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

// ReservedEnv names variables the engine or the container already sets.
func ReservedEnv(name string) bool {
	switch strings.ToUpper(name) {
	case "PATH", "HOME", "CI", "XDG_RUNTIME_DIR":
		return true
	}
	return false
}

// StdioOpts start one stdio MCP server. Env values reach the container
// through the engine's environment, so the argv names them and never holds a
// value.
type StdioOpts struct {
	Engine  string
	Image   string
	Argv    []string
	Env     map[string]string
	Name    string
	Label   string
	Fixture bool
}

// StdioArgv is the engine argv and environment of a stdio server: the check
// container's limits with stdin open, no network, no mount, and a name and
// label for cleanup.
func StdioArgv(opts StdioOpts) (argv, env []string, err error) {
	if opts.Fixture {
		return nil, nil, ErrStdioFixture
	}
	if err := CheckEngine(opts.Engine); err != nil {
		return nil, nil, err
	}
	if !PinnedImage(opts.Image) {
		return nil, nil, ErrImage
	}
	if len(opts.Argv) == 0 {
		return nil, nil, ErrArgv
	}
	if err := ParseArgv(opts.Argv, opts.Argv[:1]); err != nil {
		return nil, nil, err
	}
	if !containerName.MatchString(opts.Name) || !labelValue.MatchString(opts.Label) {
		return nil, nil, ErrStdioName
	}
	argv = []string{opts.Engine, "run", "--rm", "-i", "--pull=never", "--network=none",
		"--name", opts.Name, "--label", MCPLabel + "=" + opts.Label}
	argv = append(argv, hardening()...)
	env = EngineEnv(opts.Engine)
	names := make([]string, 0, len(opts.Env))
	for name := range opts.Env {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !envName.MatchString(name) || ReservedEnv(name) {
			return nil, nil, ErrStdioEnv
		}
		argv = append(argv, "--env", name)
		env = append(env, name+"="+opts.Env[name])
	}
	argv = append(argv, opts.Image)
	argv = append(argv, opts.Argv...)
	return argv, env, nil
}

// Stdio is a running stdio server. Close kills it and removes its container.
type Stdio struct {
	*execx.Process
	rm   func()
	once sync.Once
}

func (s *Stdio) Close() error {
	s.once.Do(func() {
		_ = s.Process.Close()
		s.rm()
	})
	return nil
}

// StartArgv starts a stdio server with start and removes its container with
// run on Close.
func StartArgv(ctx context.Context, opts StdioOpts,
	start func(context.Context, execx.StartChild) (*execx.Process, error),
	run func(context.Context, execx.Child) (execx.Result, error),
) (*Stdio, error) {
	argv, env, err := StdioArgv(opts)
	if err != nil {
		return nil, err
	}
	rm := func() {
		_, _ = run(context.Background(), execx.Child{
			Argv: []string{opts.Engine, "rm", "-f", opts.Name}, Env: EngineEnv(opts.Engine),
			Timeout: 30 * time.Second, MaxOut: 64 << 10,
		})
	}
	proc, err := start(ctx, execx.StartChild{Argv: argv, Env: env, MaxFrame: MaxStdioFrame})
	if err != nil {
		rm()
		return nil, err
	}
	return &Stdio{Process: proc, rm: rm}, nil
}

// SweepMCP removes leftover stdio server containers labelled with label.
func SweepMCP(ctx context.Context, engine, label string, run func(context.Context, execx.Child) (execx.Result, error)) error {
	if err := CheckEngine(engine); err != nil {
		return err
	}
	if !labelValue.MatchString(label) {
		return ErrStdioName
	}
	res, err := run(ctx, execx.Child{
		Argv: []string{engine, "ps", "-aq", "--filter", "label=" + MCPLabel + "=" + label},
		Env:  EngineEnv(engine), Timeout: 30 * time.Second, MaxOut: 64 << 10,
	})
	if err != nil || res.Exit != 0 {
		return err
	}
	ids := strings.Fields(string(res.Out))
	if len(ids) == 0 {
		return nil
	}
	_, err = run(ctx, execx.Child{
		Argv: append([]string{engine, "rm", "-f"}, ids...),
		Env:  EngineEnv(engine), Timeout: 30 * time.Second, MaxOut: 64 << 10,
	})
	return err
}
