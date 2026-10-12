package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/sandbox"
	"orchestrator/backend/internal/trace"
)

type session struct {
	in          Input
	card        board.Card
	wt          runtime.Worktree
	attemptID   string
	system      string
	user        string
	linkedRoots map[string]string
	turns       []providers.Turn

	// evicted holds the [turn, result] pairs the history stopped sending; they
	// are never sent again, so the start of the history stays the same.
	evicted map[[2]int]bool
	reads   map[string]readMark
	nudges  int
	// committed is set once a commit call succeeds in this attempt.
	committed bool
	// passTree is the worktree's tree hash at the last run_local_check, when it
	// exited 0, and empty after a check that did not.
	passTree string
	// used counts model calls; outOfTurns is set when every turn was used.
	used       int
	outOfTurns bool
	// stop is the cap that ended the attempt: token_cap, or empty.
	stop string
	// todos is the model's current todo_write list.
	todos []todoItem
	// commands counts the run_command calls that ran in this attempt.
	commands int
}

// readOnly are the tools that change nothing, so the calls of one reply that
// use them run concurrently (RFC-082).
var readOnly = map[string]bool{"list_files": true, "search_files": true, "read_file": true, "read_linked_file": true}

// readHook, when set by a test, runs at the start of each read-only call.
var readHook func()

type callResult struct {
	summary  string
	rejected bool
	err      error
}

// readMark is where a read's result sits in the history and the hash of its text.
type readMark struct {
	turn, idx int
	hash      string
}

const (
	defaultHistory = 16 << 10
	minResultCap   = 12 << 10
	maxResultCap   = 64 << 10
	omitted        = placeholder2
	maxNudges      = 2
	minTurnsLeft   = 3
	maxCommands    = 30
)

const (
	nudgeText = "You replied without a tool call, and nothing is committed yet. The attempt ends only after commit succeeds. " +
		"Continue with the next tool call. If the change is complete and run_local_check exits 0, call commit."
	truncatedText = "Your reply was cut at the token limit before it finished. Continue in smaller steps: one tool call at a time, " +
		"and replace_in_file for each change instead of writing whole files."
)

// resultCap is the most of one result the history sends: 12 KiB, or a
// quarter of the budget when that is more, up to 64 KiB.
func resultCap(budget int) int {
	return min(maxResultCap, max(minResultCap, budget/4))
}

// history is the turns so far, sized for one request (RFC-081). The newest
// turn's results are always sent, each cut at resultCap. Older results are
// sent while their total stays under the budget left after the newest. When
// one has to go, the oldest go until the total is under three quarters of
// that, and a result once omitted stays omitted. File text in edit_file calls
// is never sent back.
func (s *session) history() []providers.Turn {
	n := len(s.turns)
	out := make([]providers.Turn, n)
	if n == 0 {
		return out
	}
	if s.evicted == nil {
		s.evicted = map[[2]int]bool{}
	}
	budget := s.in.HistoryBudget
	if budget <= 0 {
		budget = defaultHistory
	}
	rcap := resultCap(budget)
	sent := func(t, i int) string {
		turn := s.turns[t]
		return cutResult(turn.Results[i], rcap, i < len(turn.Calls) && keepsTail(turn.Calls[i].Name))
	}
	newest := 0
	for i := range s.turns[n-1].Results {
		newest += len(sent(n-1, i))
	}
	limit := max(0, budget-newest)
	total := 0
	for t := 0; t < n-1; t++ {
		for i := range s.turns[t].Results {
			if !s.evicted[[2]int{t, i}] {
				total += len(sent(t, i))
			}
		}
	}
	if total > limit {
		target := limit * 3 / 4
	evict:
		for t := 0; t < n-1; t++ {
			for i := range s.turns[t].Results {
				if s.evicted[[2]int{t, i}] {
					continue
				}
				s.evicted[[2]int{t, i}] = true
				total -= len(sent(t, i))
				if total <= target {
					break evict
				}
			}
		}
	}
	for t, turn := range s.turns {
		next := providers.Turn{Text: turn.Text, Results: make([]string, len(turn.Results)), Reasoning: turn.Reasoning, Note: turn.Note}
		for _, call := range turn.Calls {
			next.Calls = append(next.Calls, sentCall(call))
		}
		for i := range turn.Results {
			if t < n-1 && s.evicted[[2]int{t, i}] {
				next.Results[i] = omitted
				continue
			}
			next.Results[i] = sent(t, i)
		}
		out[t] = next
	}
	return out
}

// keepsTail names the tools whose output ends with what matters: a failing
// check prints its failure last.
func keepsTail(name string) bool {
	return name == "run_local_check" || name == "run_command"
}

func cutResult(text string, limit int, tail bool) string {
	if len(text) <= limit {
		return text
	}
	if tail {
		return "truncated: head cut\n" + strings.ToValidUTF8(text[len(text)-limit:], "")
	}
	return strings.ToValidUTF8(text[:limit], "") + "\ntruncated: true"
}

// sentCall is call as the history sends it back: an edit_file call carries the
// size of its text, not the text, because the file holds it.
func sentCall(call providers.ToolCall) providers.ToolCall {
	if call.Name != "edit_file" {
		return call
	}
	text := strArg(call.Args, "text")
	args := map[string]any{}
	for k, v := range call.Args {
		if k != "text" {
			args[k] = v
		}
	}
	args["bytes"] = len(text)
	args["lines"] = countLines(text)
	call.Args = args
	return call
}

// withNote adds the turns-left reminder to the newest turn's note once
// max(3, cap/5) or fewer model turns remain. turns must come from history,
// which copies each turn.
func withNote(turns []providers.Turn, left, maxTurns int) []providers.Turn {
	if len(turns) == 0 || left > max(minTurnsLeft, maxTurns/5) {
		return turns
	}
	last := &turns[len(turns)-1]
	last.Note = joinNote(last.Note, fmt.Sprintf("%d model turns left in this attempt. If the change is done, call commit now.", left))
	return turns
}

func joinNote(note, more string) string {
	if note == "" {
		return more
	}
	return note + "\n\n" + more
}

func (s *session) loop(ctx context.Context) error {
	for turn := 0; turn < s.in.MaxTurns; turn++ {
		over, err := s.overTokens(ctx)
		if err != nil {
			return err
		}
		if over {
			s.stop = "token_cap"
			return nil
		}
		turns := withNote(withTodos(s.history(), s.todos), s.in.MaxTurns-turn, s.in.MaxTurns)
		resp, endTurn, err := complete(ctx, s.in.Provider, providers.Request{
			Model: s.in.Model, System: s.system, User: s.user, Turns: turns,
			Tools: ImplementerTools(), TaskClass: s.in.TaskClass,
		}, s.in.Live, s.card.ID, live.Frame{AttemptID: s.attemptID, Turn: turn, Role: "implementer"}, s.in.Secrets)
		if err != nil {
			return err
		}
		s.used = turn + 1
		if err := board.AddAttemptTokens(ctx, s.in.DB, s.attemptID, resp.InputTokens, resp.OutputTokens, resp.CachedInputTokens); err != nil {
			return err
		}
		modelID := resp.Model
		if modelID == "" {
			modelID = s.in.Model
		}
		err = s.event(ctx, "model_call", modelCallPayload("implementer", s.in.TaskClass, modelID, resp, s.in.Secrets))
		endTurn()
		if err != nil {
			return err
		}
		if len(resp.ToolCalls) == 0 {
			if !s.committed && s.nudges < maxNudges && turn+1 < s.in.MaxTurns {
				s.nudges++
				note := nudgeText
				if providers.Truncated(resp.StopReason) {
					note = truncatedText
				}
				s.turns = append(s.turns, providers.Turn{Text: resp.Text, Reasoning: resp.Reasoning, Note: note})
				continue
			}
			return nil
		}
		done := providers.Turn{Text: resp.Text, Reasoning: resp.Reasoning}
		calls := make([]providers.ToolCall, len(resp.ToolCalls))
		for i, call := range resp.ToolCalls {
			if call.ID == "" {
				call.ID = fmt.Sprintf("call_%d_%d", turn, i)
			}
			calls[i] = call
		}
		for i, res := range s.runCalls(ctx, calls) {
			call := calls[i]
			summary, rejected, callErr := res.summary, res.rejected, res.err
			summary = s.dedupeRead(call, summary, i)
			summary = trace.Redact(summary, s.in.Secrets)
			payload := map[string]any{
				"name": call.Name, "ok": callErr == nil && !rejected && !strings.HasPrefix(summary, providers.ErrorPrefix),
				"rejected": rejected, "summary": summary,
			}
			if args := toolArgs(call, s.card.LocalCheck, s.in.Secrets); len(args) > 0 {
				payload["args"] = args
			}
			if err := s.event(ctx, "tool_call", payload); err != nil {
				return err
			}
			if callErr != nil {
				return callErr
			}
			done.Calls = append(done.Calls, call)
			done.Results = append(done.Results, summary)
		}
		s.turns = append(s.turns, done)
	}
	s.outOfTurns = true
	return nil
}

// overTokens reports whether the card has used its token cap (RFC-081).
func (s *session) overTokens(ctx context.Context) (bool, error) {
	if s.in.MaxTokensPerCard <= 0 {
		return false, nil
	}
	used, err := board.CardTokens(ctx, s.in.DB, s.card.ID)
	if err != nil {
		return false, err
	}
	return used >= s.in.MaxTokensPerCard, nil
}

// dedupeRead replaces a read whose text is the same as an earlier read of the
// same file and range, while the history still sends that earlier result.
func (s *session) dedupeRead(call providers.ToolCall, summary string, idx int) string {
	if call.Name != "read_file" || strings.HasPrefix(summary, providers.ErrorPrefix) {
		return summary
	}
	if s.reads == nil {
		s.reads = map[string]readMark{}
	}
	key := fmt.Sprintf("%s|%d|%d", strArg(call.Args, "path"), intArg(call.Args, "offset"), intArg(call.Args, "limit"))
	sum := hashText(summary)
	turn := len(s.turns)
	if mark, ok := s.reads[key]; ok && mark.hash == sum && !s.evicted[[2]int{mark.turn, mark.idx}] {
		return fmt.Sprintf("unchanged since turn %d; the earlier result is above", mark.turn+1)
	}
	s.reads[key] = readMark{turn: turn, idx: idx, hash: sum}
	return summary
}

// runCalls runs one reply's calls. A run of read-only calls goes concurrently;
// every other call runs alone, in order. Results keep the reply's order, and
// a call that fails the attempt ends the list.
func (s *session) runCalls(ctx context.Context, calls []providers.ToolCall) []callResult {
	out := make([]callResult, len(calls))
	concurrent := func(call providers.ToolCall) bool { return readOnly[call.Name] && call.BadArgs == "" }
	for i := 0; i < len(calls); {
		if !concurrent(calls[i]) {
			out[i].summary, out[i].rejected, out[i].err = s.dispatch(ctx, calls[i])
			if out[i].err != nil {
				return out[:i+1]
			}
			i++
			continue
		}
		j := i
		for j < len(calls) && concurrent(calls[j]) {
			j++
		}
		var wg sync.WaitGroup
		for k := i; k < j; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out[k].summary, out[k].rejected, out[k].err = s.dispatch(ctx, calls[k])
			}()
		}
		wg.Wait()
		for k := i; k < j; k++ {
			if out[k].err != nil {
				return out[:k+1]
			}
		}
		i = j
	}
	return out
}

// dispatch runs one tool call. A result that reports a failure starts with
// providers.ErrorPrefix. rejected marks a tool outside the profile.
func (s *session) dispatch(ctx context.Context, call providers.ToolCall) (string, bool, error) {
	if !allowedTool(call.Name) {
		return providers.ErrorPrefix + "not in profile", true, nil
	}
	if call.BadArgs != "" {
		return providers.ErrorPrefix + "the arguments were not valid JSON (" + call.BadArgs + "); send the call again with a JSON object", false, nil
	}
	if readOnly[call.Name] && readHook != nil {
		readHook()
	}
	text, err := s.run(ctx, call)
	if err != nil {
		return providers.ErrorPrefix + err.Error(), false, nil
	}
	return text, false, nil
}

func (s *session) run(ctx context.Context, call providers.ToolCall) (string, error) {
	args := call.Args
	switch call.Name {
	case "list_files":
		return listFiles(s.wt.Path, strArg(args, "path"), strArg(args, "glob"), intArg(args, "depth"))
	case "search_files":
		return searchFiles(s.wt.Path, searchOpts{
			Text: strArg(args, "text"), Regex: boolArg(args, "regex"), Path: strArg(args, "path"),
			Glob: strArg(args, "glob"), IgnoreCase: boolArg(args, "ignoreCase"), Context: intArg(args, "context"),
		})
	case "read_file":
		return readFile(s.wt.Path, strArg(args, "path"), intArg(args, "offset"), intArg(args, "limit"))
	case "read_linked_file":
		root, ok := s.linkedRoots[strArg(args, "project")]
		if !ok {
			return "", errors.New("not a linked project with a clone")
		}
		return readFile(root, strArg(args, "path"), intArg(args, "offset"), intArg(args, "limit"))
	case "replace_in_file":
		return replaceInFile(s.wt.Path, strArg(args, "path"), strArg(args, "old"), strArg(args, "new"), boolArg(args, "replaceAll"))
	case "edit_file":
		if err := editFile(s.wt.Path, strArg(args, "path"), strArg(args, "text")); err != nil {
			return "", err
		}
		return "wrote " + strArg(args, "path"), nil
	case "delete_file":
		if err := deleteFile(s.wt.Path, strArg(args, "path")); err != nil {
			return "", err
		}
		return "deleted " + strArg(args, "path"), nil
	case "move_file":
		if err := moveFile(s.wt.Path, strArg(args, "from"), strArg(args, "to")); err != nil {
			return "", err
		}
		return "moved " + strArg(args, "from") + " to " + strArg(args, "to"), nil
	case "todo_write":
		items, err := parseTodos(args)
		if err != nil {
			return "", err
		}
		s.todos = items
		return todoSummary(items), nil
	case "run_local_check":
		exit, tail, err := s.localCheck(ctx)
		if err != nil {
			return "", err
		}
		return fmtInt(exit) + "\n" + tail, nil
	case "run_command":
		return s.runCommand(ctx, args)
	case "commit":
		message := commitMessage(strArg(args, "message"), s.card.ID, s.attemptID, s.in.Model)
		if err := s.in.Git.Commit(ctx, s.wt, message); err != nil {
			return "", err
		}
		s.committed = true
		return "committed", nil
	default:
		return "", errors.New("not in profile")
	}
}

// localCheck runs the card's check on a copy of the worktree. An exit of 0
// records the tree it ran on, so autoCommit can commit that tree.
func (s *session) localCheck(ctx context.Context) (int, string, error) {
	s.passTree = ""
	tree, err := s.in.Git.TreeHash(ctx, s.wt)
	if err != nil {
		return 0, "", err
	}
	exports := filepath.Join(s.in.Git.Data, "exports")
	dest := filepath.Join(exports, s.attemptID)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return 0, "", err
	}
	defer os.RemoveAll(dest)
	if err := sandbox.CheckExport(exports, dest, s.wt.Path); err != nil {
		return 0, "", err
	}
	if err := runtime.CopyWorktree(s.wt.Path, dest); err != nil {
		return 0, "", err
	}
	res, executed, err := sandbox.ExecuteCheck(ctx, sandbox.ExecuteOpts{
		Command: s.card.LocalCheck,
		Allow:   s.in.Allow,
		Fixture: runtime.FixtureMode(s.in.Git.Repo),
		Engine:  s.in.Engine,
		Image:   s.in.Image,
		Dir:     dest,
		Timeout: 10 * time.Minute,
		MaxOut:  1 << 20,
	}, s.exec())
	if err != nil {
		return 0, "", err
	}
	if !executed {
		return 0, "", fmt.Errorf("check was not executed")
	}
	if res.Exit == 0 {
		s.passTree = tree
	}
	return res.Exit, lastLines(string(res.Out), 200), nil
}

func (s *session) exec() func(context.Context, execx.Child) (execx.Result, error) {
	if s.in.Exec != nil {
		return s.in.Exec
	}
	return execx.Command
}

// runCommand runs an allowlisted argv on a fresh copy of the worktree that is
// deleted afterwards (RFC-084). Its exit is never check evidence, so it leaves
// passTree alone.
func (s *session) runCommand(ctx context.Context, args map[string]any) (string, error) {
	if len(s.in.Commands) == 0 {
		return "", errors.New("run_command is not enabled for this project")
	}
	raw, ok := args["argv"].([]any)
	if !ok {
		return "", errors.New("argv must be an array of strings")
	}
	argv := make([]string, len(raw))
	for i, arg := range raw {
		text, ok := arg.(string)
		if !ok {
			return "", errors.New("argv must be an array of strings")
		}
		argv[i] = text
	}
	if err := sandbox.ParseArgv(argv, s.in.Commands); err != nil {
		return "", err
	}
	if runtime.FixtureMode(s.in.Git.Repo) {
		return "", errors.New("run_command needs the container sandbox")
	}
	if s.commands >= maxCommands {
		return "", fmt.Errorf("run_command has run %d times in this attempt, the most it may", maxCommands)
	}
	s.commands++
	exports := filepath.Join(s.in.Git.Data, "exports")
	if err := os.MkdirAll(exports, 0o700); err != nil {
		return "", err
	}
	dest, err := os.MkdirTemp(exports, s.attemptID+"-cmd-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dest)
	if err := sandbox.CheckExport(exports, dest, s.wt.Path); err != nil {
		return "", err
	}
	if err := runtime.CopyWorktree(s.wt.Path, dest); err != nil {
		return "", err
	}
	res, executed, err := sandbox.ExecuteArgv(ctx, argv, sandbox.ExecuteOpts{
		Engine:  s.in.Engine,
		Image:   s.in.Image,
		Dir:     dest,
		Timeout: 5 * time.Minute,
		MaxOut:  1 << 20,
	}, s.exec())
	if err != nil {
		return "", err
	}
	if !executed {
		return "", errors.New("the command was not executed")
	}
	return trace.Redact(fmtInt(res.Exit)+"\n"+lastLines(string(res.Out), 200), s.in.Secrets), nil
}

// autoCommitTrailer marks a commit the harness made for the model (RFC-081).
const autoCommitTrailer = "Committed-By: harness after passing local check"

// autoCommit commits the worktree when it has changes and its tree is the one
// the last passing run_local_check ran on.
func (s *session) autoCommit(ctx context.Context) error {
	if s.passTree == "" {
		return nil
	}
	clean, err := s.in.Git.Clean(ctx, s.wt)
	if err != nil || clean {
		return err
	}
	tree, err := s.in.Git.TreeHash(ctx, s.wt)
	if err != nil || tree != s.passTree {
		return err
	}
	subject := strings.TrimSpace(s.card.Title)
	if subject == "" {
		subject = "Apply the card's change"
	}
	message := commitMessage(subject, s.card.ID, s.attemptID, s.in.Model) + autoCommitTrailer + "\n"
	if err := s.in.Git.Commit(ctx, s.wt, message); err != nil {
		return err
	}
	return s.event(ctx, "tool_call", map[string]any{
		"name": "commit", "ok": true, "rejected": false, "summary": "committed by the harness after a passing local check",
		"args": map[string]any{"message": trace.Clip(s.card.Title, s.in.Secrets, trace.ArgCap), "auto": true},
	})
}
