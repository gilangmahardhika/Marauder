// Package ghcli runs the gh CLI with one GitHub token in the child
// environment. The company account and each card owner share it.
package ghcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/trace"
)

var (
	ErrGitHub = errors.New("github call failed")
	// ErrNotFound wraps ErrGitHub when GitHub answered 404.
	ErrNotFound = errors.New("github returned not found")
)

const (
	timeout = 30 * time.Second
	MaxOut  = 4 << 20
)

type Runner struct {
	Bin       string
	ConfigDir string
	Exec      func(context.Context, execx.Child) (execx.Result, error)
}

// API runs `gh api --hostname host args...`.
func (g Runner) API(ctx context.Context, host, token string, args ...string) ([]byte, error) {
	return g.Run(ctx, host, token, append([]string{"api", "--hostname", host}, args...)...)
}

// Run runs gh with args. The token is refused in argv and redacted from errors.
func (g Runner) Run(ctx context.Context, host, token string, args ...string) ([]byte, error) {
	bin := g.Bin
	if bin == "" {
		found, err := exec.LookPath("gh")
		if err != nil {
			return nil, fmt.Errorf("%w: gh is not installed", ErrGitHub)
		}
		bin = found
	}
	run := g.Exec
	if run == nil {
		run = execx.Command
	}
	configDir := g.ConfigDir
	if configDir == "" {
		dir, err := os.MkdirTemp("", "orchestrator-gh-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		configDir = dir
	}
	argv := append([]string{bin}, args...)
	for _, arg := range argv {
		if token != "" && strings.Contains(arg, token) {
			return nil, fmt.Errorf("%w: token in argv", ErrGitHub)
		}
	}
	env := []string{
		"GH_TOKEN=" + token,
		"GH_HOST=" + host,
		"GH_CONFIG_DIR=" + configDir,
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
		"NO_COLOR=1",
		"HOME=" + configDir,
	}
	if host != "github.com" {
		env = append(env, "GH_ENTERPRISE_TOKEN="+token)
	}
	res, err := run(ctx, execx.Child{Argv: argv, Env: env, Dir: configDir, Timeout: timeout, MaxOut: MaxOut})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrGitHub, trace.Redact(err.Error(), []string{token}))
	}
	if res.Exit != 0 {
		msg := FirstLine(trace.Redact(string(res.Out), []string{token}))
		if strings.Contains(string(res.Out), "(HTTP 404)") {
			return nil, fmt.Errorf("%w: %w: %s", ErrGitHub, ErrNotFound, msg)
		}
		return nil, fmt.Errorf("%w: %s", ErrGitHub, msg)
	}
	return res.Out, nil
}

// FirstLine trims gh output to one short line for an error message.
func FirstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	if s == "" {
		return "gh exited non-zero"
	}
	return s
}
