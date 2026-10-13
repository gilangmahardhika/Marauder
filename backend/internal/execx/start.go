package execx

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ErrFrame means a child wrote a line longer than its frame cap.
var ErrFrame = errors.New("frame over the cap")

const stderrTail = 8 << 10

// StartChild is a long-lived child that reads newline-delimited frames on
// stdin and writes them on stdout. Stderr keeps only its last 8 KiB.
type StartChild struct {
	Argv     []string
	Env      []string
	MaxFrame int
}

// Process is a started child. It is killed with its process group on Close or
// when the context given to Start ends.
type Process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	out    *bufio.Reader
	max    int
	stderr *tail
	waited chan struct{}
	once   sync.Once
	wmu    sync.Mutex
	rmu    sync.Mutex
}

func Start(ctx context.Context, child StartChild) (*Process, error) {
	if len(child.Argv) == 0 || !filepath.IsAbs(child.Argv[0]) {
		return nil, ErrArgv
	}
	if _, ok := shells[filepath.Base(child.Argv[0])]; ok {
		return nil, ErrShell
	}
	if child.MaxFrame <= 0 {
		return nil, errors.New("frame cap is required")
	}
	cmd := exec.CommandContext(ctx, child.Argv[0], child.Argv[1:]...)
	cmd.Env = append([]string{}, child.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &Process{cmd: cmd, stdin: stdin, out: bufio.NewReaderSize(stdout, 64<<10), max: child.MaxFrame, stderr: &tail{}, waited: make(chan struct{})}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		_ = cmd.Wait()
		close(p.waited)
	}()
	return p, nil
}

// WriteFrame writes frame and a newline to the child's stdin.
func (p *Process) WriteFrame(frame []byte) error {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if _, err := p.stdin.Write(append(append([]byte(nil), frame...), '\n')); err != nil {
		return err
	}
	return nil
}

// ReadFrame returns the next stdout line without its newline. A line over the
// cap returns ErrFrame, after which the stream is out of step and the caller
// closes the process.
func (p *Process) ReadFrame() ([]byte, error) {
	p.rmu.Lock()
	defer p.rmu.Unlock()
	var line []byte
	for {
		chunk, err := p.out.ReadSlice('\n')
		if len(line)+len(chunk) > p.max+1 {
			return nil, ErrFrame
		}
		line = append(line, chunk...)
		switch {
		case err == nil:
			return line[:len(line)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return nil, err
		}
	}
}

// Stderr is the last 8 KiB the child wrote to stderr. It may hold secrets and
// goes to the log only, redacted.
func (p *Process) Stderr() []byte {
	return p.stderr.bytes()
}

// Close closes stdin, kills the process group, and waits for the child.
func (p *Process) Close() error {
	p.once.Do(func() {
		_ = p.stdin.Close()
		if p.cmd.Process != nil {
			_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-p.waited
	})
	return nil
}

type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if over := len(t.buf) - stderrTail; over > 0 {
		t.buf = append([]byte(nil), t.buf[over:]...)
	}
	return len(b), nil
}

func (t *tail) bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.buf...)
}
