package sandbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/backend/internal/execx"
)

// ErrEngine means the sandbox engine path is not an absolute podman or docker binary.
var ErrEngine = errors.New("sandbox engine is not set")

// ErrEngineDown means the engine is installed but not reachable.
var ErrEngineDown = errors.New("sandbox engine is not reachable")

const probeTimeout = 10 * time.Second

type ExecuteOpts struct {
	Command string
	Allow   []string
	Fixture bool
	Engine  string
	Image   string
	Dir     string
	Timeout time.Duration
	MaxOut  int
}

// ExecuteCheck runs a check on an export. In fixture mode it runs on the host
// from the fixture path. Otherwise it runs in the pinned image with the export
// mounted at /work and no network.
func ExecuteCheck(ctx context.Context, opts ExecuteOpts, run func(context.Context, execx.Child) (execx.Result, error)) (execx.Result, bool, error) {
	argv, err := ParseCheck(opts.Command, opts.Allow)
	if err != nil {
		return execx.Result{}, false, err
	}
	if opts.Fixture {
		bin, err := ResolveFixture(argv[0])
		if err != nil {
			return execx.Result{}, false, err
		}
		res, err := run(ctx, execx.Child{
			Argv:    append([]string{bin}, argv[1:]...),
			Env:     ContainerEnv(),
			Dir:     opts.Dir,
			Timeout: opts.Timeout,
			MaxOut:  opts.MaxOut,
		})
		return res, true, err
	}
	if err := CheckEngine(opts.Engine); err != nil {
		return execx.Result{}, false, err
	}
	container, err := LocalArgv(opts.Engine, opts.Image, opts.Dir, argv)
	if err != nil {
		return execx.Result{}, false, err
	}
	res, err := run(ctx, execx.Child{
		Argv:    container,
		Env:     EngineEnv(opts.Engine),
		Timeout: opts.Timeout,
		MaxOut:  opts.MaxOut,
	})
	return res, true, err
}

// ProbeEngine runs `<engine> version`, which fails when the engine cannot
// reach its service, such as a stopped Podman machine. It returns
// ErrEngineDown with the first line of the engine's output.
func ProbeEngine(ctx context.Context, engine string, run func(context.Context, execx.Child) (execx.Result, error)) error {
	if err := CheckEngine(engine); err != nil {
		return err
	}
	res, err := run(ctx, execx.Child{
		Argv:    []string{engine, "version"},
		Env:     EngineEnv(engine),
		Timeout: probeTimeout,
		MaxOut:  64 << 10,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEngineDown, err)
	}
	if res.Exit != 0 {
		line, _, _ := strings.Cut(strings.TrimSpace(string(res.Out)), "\n")
		return fmt.Errorf("%w: %s exited %d: %s", ErrEngineDown, filepath.Base(engine), res.Exit, line)
	}
	return nil
}

// CheckEngine accepts an absolute path whose basename is podman or docker.
func CheckEngine(engine string) error {
	if !filepath.IsAbs(engine) {
		return ErrEngine
	}
	switch filepath.Base(engine) {
	case "podman", "docker":
		return nil
	default:
		return ErrEngine
	}
}
