// Package claudecode runs the Claude Code CLI as an implement agent
// (RFC-121): its argv, its built environment, and its stream-json reader. It
// never touches the board, commits, or reads the host's Claude Code login.
package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/backend/internal/execx"
)

// Agent is the agent name Claude Code jobs carry and runners report.
const Agent = "claude_code"

// MaxLine is the longest stream-json line read; a longer one ends the run.
const MaxLine = 1 << 20

var (
	// ErrStop is returned by a Handler to end the process, for example at the
	// token cap.
	ErrStop = errors.New("claude code stopped by the harness")
	// ErrMissing means no claude binary was found.
	ErrMissing = errors.New("claude code is not installed")
	// ErrUnknownModel means Claude Code rejected --model (id missing or
	// inaccessible). The implement job blocks with agent_config.
	ErrUnknownModel = errors.New("claude code rejected the selected model")
)

// UnknownModel reports whether text looks like Claude Code's unknown-model
// message ("There's an issue with the selected model … may not exist").
func UnknownModel(text string) bool {
	s := strings.ToLower(text)
	return strings.Contains(s, "selected model") && strings.Contains(s, "may not exist")
}

// Spec is one Claude Code run. Auth is apiKey or bearer, already resolved.
type Spec struct {
	Binary      string
	Dir         string
	Home        string
	Prompt      string
	System      string
	Model       string
	SmallModel  string
	MaxTurns    int
	BaseURL     string
	APIKey      string
	Auth        string
	Executables []string
	AllowWeb    bool
}

// baseTools are offered on every run. Bash is offered only per executable.
var baseTools = []string{
	"Read", "Edit", "MultiEdit", "Write", "Glob", "Grep", "LS", "TodoWrite",
	"Bash(git status:*)", "Bash(git diff:*)", "Bash(git log:*)",
}

// deniedTools win over allowed ones: Claude Code never pushes, commits, or
// fetches from the network by itself.
var deniedTools = []string{
	"Bash(git push:*)", "Bash(git commit:*)", "Bash(git remote:*)", "Bash(git config:*)",
	"Bash(curl:*)", "Bash(wget:*)",
}

// Args is the argv after the binary. The prompt goes on stdin, and
// --resume and --continue are never passed (NG-008).
func Args(s Spec) []string {
	args := []string{
		"-p", "--output-format", "stream-json", "--verbose",
		"--model", s.Model,
		"--max-turns", fmt.Sprint(s.MaxTurns),
		"--permission-mode", "acceptEdits",
		"--strict-mcp-config",
	}
	if strings.TrimSpace(s.System) != "" {
		args = append(args, "--append-system-prompt", s.System)
	}
	args = append(args, "--allowedTools")
	args = append(args, AllowedTools(s.Executables, s.AllowWeb)...)
	args = append(args, "--disallowedTools")
	args = append(args, DeniedTools(s.AllowWeb)...)
	return args
}

// AllowedTools are the tools offered: the file tools, read-only git, and
// Bash for each executable name.
func AllowedTools(executables []string, allowWeb bool) []string {
	tools := append([]string{}, baseTools...)
	seen := map[string]bool{}
	for _, name := range executables {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] || strings.ContainsAny(name, "():*, \t/") {
			continue
		}
		seen[name] = true
		tools = append(tools, "Bash("+name+":*)")
	}
	if allowWeb {
		tools = append(tools, "WebFetch", "WebSearch")
	}
	return tools
}

// DeniedTools are refused even when an allow rule matches.
func DeniedTools(allowWeb bool) []string {
	tools := append([]string{}, deniedTools...)
	if !allowWeb {
		tools = append(tools, "WebFetch", "WebSearch")
	}
	return tools
}

// Env is the child's whole environment: nothing is inherited but PATH.
func Env(s Spec) []string {
	path := os.Getenv("PATH")
	if s.Binary != "" {
		path = filepath.Dir(s.Binary) + string(os.PathListSeparator) + path
	}
	small := s.SmallModel
	if small == "" {
		small = s.Model
	}
	env := []string{
		"PATH=" + path,
		"HOME=" + s.Home,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(s.Home, ".claude"),
		"TMPDIR=" + filepath.Join(s.Home, "tmp"),
		"ANTHROPIC_MODEL=" + s.Model,
		"ANTHROPIC_SMALL_FAST_MODEL=" + small,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=" + small,
		"DISABLE_AUTOUPDATER=1",
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	}
	if s.BaseURL != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+s.BaseURL)
	}
	if s.Auth == "bearer" {
		env = append(env, "ANTHROPIC_AUTH_TOKEN="+s.APIKey)
	} else {
		env = append(env, "ANTHROPIC_API_KEY="+s.APIKey)
	}
	return env
}

// Usage is one assistant message's tokens. Input excludes the cache.
type Usage struct {
	Input      int `json:"input_tokens"`
	Output     int `json:"output_tokens"`
	CacheRead  int `json:"cache_read_input_tokens"`
	CacheWrite int `json:"cache_creation_input_tokens"`
}

// ToolUse is one tool call Claude Code made.
type ToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

// Message is one assistant stream line: its text and tool calls. Claude Code
// writes a message's blocks on lines that share its id; Repeat is set on all
// but the first, whose Usage is the message's (the others carry none).
type Message struct {
	ID         string
	Model      string
	StopReason string
	Text       string
	Tools      []ToolUse
	Usage      Usage
	Repeat     bool
}

// Result is the stream's last line. Subtype is success, error_max_turns, or
// error_during_execution.
type Result struct {
	Subtype string
	IsError bool
	Text    string
	Turns   int
}

// Outcome is how the process ended. Result is nil when the stream had none,
// and Stopped is set when a Handler returned ErrStop.
type Outcome struct {
	Result  *Result
	Exit    int
	Stderr  string
	Stopped bool
}

// Handler gets each assistant line as it is read.
type Handler func(Message) error

type line struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Message struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
		Usage *Usage `json:"usage"`
	} `json:"message"`
	Result   string `json:"result"`
	IsError  bool   `json:"is_error"`
	NumTurns int    `json:"num_turns"`
}

// Run starts Claude Code in s.Dir with the prompt on stdin and reads its
// stream until it exits. A cancelled ctx kills its process group.
func Run(ctx context.Context, s Spec, handle Handler) (Outcome, error) {
	if err := os.MkdirAll(filepath.Join(s.Home, "tmp"), 0o700); err != nil {
		return Outcome{}, err
	}
	p, err := execx.Start(ctx, execx.StartChild{
		Argv: append([]string{s.Binary}, Args(s)...), Env: Env(s), Dir: s.Dir, MaxFrame: MaxLine,
	})
	if err != nil {
		return Outcome{}, err
	}
	defer p.Close()
	go func() {
		_ = p.WriteFrame([]byte(s.Prompt))
		_ = p.CloseInput()
	}()

	var out Outcome
	seen := map[string]bool{}
	stop := func(err error) (Outcome, error) {
		_ = p.Close()
		out.Exit = p.Wait()
		out.Stderr = string(p.Stderr())
		if errors.Is(err, ErrStop) {
			out.Stopped = true
			return out, nil
		}
		return out, err
	}
read:
	for {
		raw, err := p.ReadFrame()
		if err != nil {
			if errors.Is(err, execx.ErrFrame) {
				return stop(fmt.Errorf("claude code wrote a line over %d bytes", MaxLine))
			}
			if !errors.Is(err, io.EOF) {
				return stop(err)
			}
			break
		}
		var l line
		if json.Unmarshal(raw, &l) != nil {
			continue
		}
		switch l.Type {
		case "assistant":
			m := Message{ID: l.Message.ID, Model: l.Message.Model, StopReason: l.Message.StopReason, Repeat: l.Message.ID != "" && seen[l.Message.ID]}
			seen[l.Message.ID] = true
			if l.Message.Usage != nil && !m.Repeat {
				m.Usage = *l.Message.Usage
			}
			var texts []string
			for _, block := range l.Message.Content {
				switch block.Type {
				case "text":
					if block.Text != "" {
						texts = append(texts, block.Text)
					}
				case "tool_use":
					m.Tools = append(m.Tools, ToolUse{ID: block.ID, Name: block.Name, Input: block.Input})
				}
			}
			m.Text = strings.Join(texts, "\n")
			if err := handle(m); err != nil {
				return stop(err)
			}
		case "result":
			// The last line: a tool's child that keeps stdout open must not
			// hold the attempt.
			out.Result = &Result{Subtype: l.Subtype, IsError: l.IsError, Text: l.Result, Turns: l.NumTurns}
			break read
		}
	}
	out.Exit = p.Wait()
	out.Stderr = string(p.Stderr())
	if err := ctx.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// Find returns the claude binary: binary when it is set, else claude on PATH.
func Find(binary string) (string, error) {
	if binary == "" {
		found, err := exec.LookPath("claude")
		if err != nil {
			return "", ErrMissing
		}
		binary = found
	}
	abs, err := filepath.Abs(binary)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%w at %s", ErrMissing, abs)
	}
	return abs, nil
}

// Version runs claude --version and returns its first line.
func Version(ctx context.Context, binary string) (string, error) {
	res, err := execx.Command(ctx, execx.Child{
		Argv: []string{binary, "--version"}, Env: []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.TempDir(), "DISABLE_AUTOUPDATER=1"},
		Timeout: 15 * time.Second, MaxOut: 4 << 10,
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(res.Out))
	if res.Exit != 0 {
		return "", fmt.Errorf("claude --version exited %d: %s", res.Exit, firstLine(text))
	}
	return firstLine(text), nil
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(line)
}
