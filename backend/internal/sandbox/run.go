package sandbox

import (
	"context"
	"time"

	"orchestrator/backend/internal/execx"
)

type ExecuteOpts struct {
	Command string
	Allow   []string
	Fixture bool
	Dir     string
	Timeout time.Duration
	MaxOut  int
}

func ExecuteCheck(ctx context.Context, opts ExecuteOpts, run func(context.Context, execx.Child) (execx.Result, error)) (execx.Result, bool, error) {
	argv, err := ParseCheck(opts.Command, opts.Allow)
	if err != nil {
		return execx.Result{}, false, err
	}
	if !opts.Fixture {
		return execx.Result{}, false, nil
	}
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
