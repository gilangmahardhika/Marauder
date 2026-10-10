package execx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

var (
	ErrShell = errors.New("refusing shell")
	ErrArgv  = errors.New("argv must start with an absolute path")
)

var shells = map[string]struct{}{
	"sh":         {},
	"bash":       {},
	"zsh":        {},
	"dash":       {},
	"fish":       {},
	"cmd":        {},
	"powershell": {},
}

type Child struct {
	Argv    []string
	Env     []string
	Dir     string
	Timeout time.Duration
	MaxOut  int
}

type Result struct {
	Exit int
	Out  []byte
}

func Command(ctx context.Context, child Child) (Result, error) {
	if len(child.Argv) == 0 || !filepath.IsAbs(child.Argv[0]) {
		return Result{}, ErrArgv
	}
	if _, ok := shells[filepath.Base(child.Argv[0])]; ok {
		return Result{}, ErrShell
	}
	if child.Timeout <= 0 {
		return Result{}, errors.New("timeout is required")
	}
	if child.MaxOut <= 0 {
		return Result{}, errors.New("output cap is required")
	}

	runCtx, cancel := context.WithTimeout(ctx, child.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, child.Argv[0], child.Argv[1:]...)
	cmd.Env = append([]string{}, child.Env...)
	cmd.Dir = child.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second

	buf := &capBuf{max: child.MaxOut, hit: make(chan struct{})}
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	select {
	case err := <-waitDone:
		if runCtx.Err() != nil {
			res, _ := finish(buf, nil, false)
			res.Exit = 124
			return res, nil
		}
		return finish(buf, err, false)
	case <-buf.hit:
		cancel()
		<-waitDone
		res, _ := finish(buf, nil, true)
		res.Exit = 125
		return res, nil
	case <-runCtx.Done():
		<-waitDone
		res, _ := finish(buf, nil, false)
		res.Exit = 124
		return res, nil
	}
}

func finish(buf *capBuf, err error, oversize bool) (Result, error) {
	out := buf.bytes()
	if oversize {
		return Result{Exit: 125, Out: out}, nil
	}
	if err == nil {
		return Result{Exit: 0, Out: out}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return Result{Exit: exitErr.ExitCode(), Out: out}, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Result{Exit: 124, Out: out}, nil
	}
	return Result{Out: out}, err
}

type capBuf struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	max  int
	hit  chan struct{}
	once sync.Once
}

func (c *capBuf) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buf.Len() >= c.max {
		c.trip()
		return 0, io.ErrShortWrite
	}
	remain := c.max - c.buf.Len()
	if len(p) > remain {
		c.buf.Write(p[:remain])
		c.trip()
		return remain, io.ErrShortWrite
	}
	_, _ = c.buf.Write(p)
	return len(p), nil
}

func (c *capBuf) trip() {
	c.once.Do(func() { close(c.hit) })
}

func (c *capBuf) bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}
