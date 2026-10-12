package loop

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/sandbox"
	"orchestrator/backend/internal/trace"
)

const readCap = 256 << 10
const writeCap = 1 << 20

var ErrRequirement = errors.New("unknown or unaccepted requirement")

// ErrOwners rejects a card whose requirements were brought in by different users (RFC-054).
var ErrOwners = errors.New("requirements have different creators; create one card per creator")

// ErrSkill rejects a card that names a skill the project has not enabled (RFC-056).
var ErrSkill = errors.New("skill is not enabled for this project")

func ImplementerTools() []providers.Tool {
	return []providers.Tool{
		{
			Name:        "list_files",
			Description: "List the files under a directory of the card worktree, up to 500 paths.",
			Parameters:  object(map[string]any{"path": str("Repository-relative directory. Empty or . for the root")}),
		},
		{
			Name:        "search_files",
			Description: "Find lines that contain the exact text in the card worktree. Returns path:line: text, up to 100 matches.",
			Parameters:  object(map[string]any{"text": str("Exact text to find, case sensitive")}, "text"),
		},
		{
			Name:        "read_file",
			Description: "Read a file from the card worktree. Paths are relative to the repository root.",
			Parameters:  object(map[string]any{"path": str("Repository-relative path")}, "path"),
		},
		{
			Name:        "read_linked_file",
			Description: "Read a file from a linked project by its project id. Linked projects are read only.",
			Parameters: object(map[string]any{
				"project": str("Linked project id"),
				"path":    str("Path relative to the linked repository root"),
			}, "project", "path"),
		},
		{
			Name:        "edit_file",
			Description: "Replace the whole content of a file in the card worktree, creating it when missing.",
			Parameters: object(map[string]any{
				"path": str("Repository-relative path"),
				"text": str("The complete new file content"),
			}, "path", "text"),
		},
		{
			Name:        "run_local_check",
			Description: "Run the card's local check on a copy of the worktree. Returns the exit code and the last 200 lines.",
			Parameters:  object(map[string]any{}),
		},
		{
			Name:        "commit",
			Description: "Commit every change in the worktree. Call it once the work is done and the local check passes.",
			Parameters:  object(map[string]any{"message": str("Subject of at most 72 characters in the imperative mood, a blank line, then what changed and why")}, "message"),
		},
	}
}

func object(props map[string]any, required ...string) map[string]any {
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func str(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

type Settings struct {
	LocalCheck  string
	DevCheck    string
	Allow       []string
	MaxAttempts int
}

type Input struct {
	DB         *sql.DB
	Git        runtime.Git
	BaseBranch string
	Linked     []LinkedSource
	CardID     string
	Model      string
	TaskClass  string
	MaxTurns   int
	Provider   providers.Provider
	Secrets    []string
	Allow      []string
	Engine     string
	Image      string
}

type Result struct {
	AttemptID string
	Outcome   string
	Status    string
}

func Run(ctx context.Context, in Input) (Result, error) {
	if in.TaskClass == "" {
		in.TaskClass = "implement"
	}
	if in.MaxTurns <= 0 {
		in.MaxTurns = 12
	}
	card, err := board.GetCard(ctx, in.DB, in.CardID)
	if err != nil {
		return Result{}, err
	}
	if card.Status == "ready" || card.Status == "changes_requested" {
		card, err = move(ctx, in.DB, card, "in_progress", machine.Evidence{}, in.Secrets)
		if err != nil {
			return Result{}, err
		}
	}
	base := in.BaseBranch
	if base == "" {
		base = "main"
	}
	wt, err := in.Git.EnsureWorktree(ctx, card.ID, base)
	if err != nil {
		return Result{}, err
	}
	if err := board.StoreWorktree(ctx, in.DB, card.ID, wt.Branch, wt.Path, wt.GitDir); err != nil {
		return Result{}, err
	}
	holder, err := board.NewID("lok_")
	if err != nil {
		return Result{}, err
	}
	if err := board.AcquireLock(ctx, in.DB, card.ID, holder); err != nil {
		return Result{}, err
	}
	defer board.ReleaseLock(ctx, in.DB, card.ID, holder)

	if err := in.Git.DiscardChanges(ctx, wt); err != nil {
		return Result{}, err
	}
	baseSHA, err := in.Git.WorktreeHead(ctx, wt)
	if err != nil {
		return Result{}, err
	}
	statements, err := statements(ctx, in.DB, card.ProjectID, card.RequirementIDs)
	if err != nil {
		return Result{}, err
	}
	linked, linkedRoots, linkedRefs, err := prepareLinked(ctx, in.DB, filepath.Join(in.Git.Data, "exports"), in.Linked)
	if err != nil {
		return Result{}, err
	}
	note, err := latestNote(ctx, in.DB, card.ID)
	if err != nil {
		return Result{}, err
	}
	instructions, err := board.CurrentInstructions(ctx, in.DB, card.ID)
	if err != nil {
		return Result{}, err
	}
	convs, convItems, err := CardConventions(ctx, in.DB, card.ProjectID, card.ID)
	if err != nil {
		return Result{}, err
	}
	system, user, systemHash, err := contextpack.BuildImplementer(contextpack.Input{
		Card: contextpack.Card{
			ID: card.ID, Title: card.Title, Status: card.Status, LocalCheck: card.LocalCheck,
		},
		Requirements: statements,
		FailureNote:  note,
		Instructions: instructions,
		Linked:       linked,
		Conventions:  convs,
		Secrets:      in.Secrets,
	})
	if err != nil {
		return Result{}, err
	}
	n, parent, err := board.NextAttempt(ctx, in.DB, card.ID)
	if err != nil {
		return Result{}, err
	}
	attemptID, err := board.NewID("att_")
	if err != nil {
		return Result{}, err
	}
	if err := board.InsertAttempt(ctx, in.DB, board.Attempt{
		ID: attemptID, CardID: card.ID, N: n, ParentID: parent,
		TaskClass: in.TaskClass, ModelID: in.Model, PromptHash: hashText(user),
		SystemHash: systemHash, BaseSHA: baseSHA, MaxTurns: in.MaxTurns,
	}); err != nil {
		return Result{}, err
	}

	session := &session{
		in: in, card: card, wt: wt, attemptID: attemptID,
		system: system, user: user, linkedRoots: linkedRoots,
	}
	if err := session.event(ctx, "tool_call", map[string]any{
		"name": "conventions_context", "ok": true, "rejected": false,
		"summary": fmt.Sprintf("%d rules and skills", len(convItems)), "items": convItems,
	}); err != nil {
		return Result{}, err
	}
	if len(linkedRefs) > 0 {
		if err := session.event(ctx, "tool_call", map[string]any{
			"name": "linked_context", "ok": true, "rejected": false,
			"summary": fmt.Sprintf("%d linked projects", len(linkedRefs)), "links": linkedRefs,
		}); err != nil {
			return Result{}, err
		}
	}
	if err := session.loop(ctx); err != nil {
		_ = board.FinishAttempt(ctx, in.DB, attemptID, "error", "")
		return Result{}, err
	}
	head, err := in.Git.WorktreeHead(ctx, wt)
	if err != nil {
		return Result{}, err
	}
	if head == baseSHA {
		if err := board.InsertFailureNote(ctx, in.DB, card.ID, attemptID, "no_commit", "no commit"); err != nil {
			return Result{}, err
		}
		if err := board.AddAttemptUsed(ctx, in.DB, card.ID); err != nil {
			return Result{}, err
		}
		if err := board.FinishAttempt(ctx, in.DB, attemptID, "no_commit", ""); err != nil {
			return Result{}, err
		}
		fresh, err := board.GetCard(ctx, in.DB, card.ID)
		if err != nil {
			return Result{}, err
		}
		return Result{AttemptID: attemptID, Outcome: "no_commit", Status: fresh.Status}, nil
	}
	if err := board.FinishAttempt(ctx, in.DB, attemptID, "", head); err != nil {
		return Result{}, err
	}
	fresh, err := board.GetCard(ctx, in.DB, card.ID)
	if err != nil {
		return Result{}, err
	}
	return Result{AttemptID: attemptID, Status: fresh.Status}, nil
}

type session struct {
	in          Input
	card        board.Card
	wt          runtime.Worktree
	attemptID   string
	system      string
	user        string
	linkedRoots map[string]string
	turns       []providers.Turn
}

// Free model tiers refuse a request over about 7000 tokens, so the history
// carries the newest results first and drops the text of older ones.
const (
	resultCap  = 12 << 10
	historyCap = 16 << 10
	omitted    = "(older result omitted)"
	written    = "(omitted: already written to the file)"
	lastTurns  = 3
)

// history is the turns so far, sized for one request. The newest result is
// always sent, cut at resultCap. Older results are sent while their total stays
// under historyCap, then replaced by omitted. File text in edit_file calls is
// never sent back, because the result already says the file was written.
func (s *session) history() []providers.Turn {
	out := make([]providers.Turn, len(s.turns))
	used, full, newest := 0, true, true
	for t := len(s.turns) - 1; t >= 0; t-- {
		turn := s.turns[t]
		next := providers.Turn{Text: turn.Text, Results: make([]string, len(turn.Results))}
		for _, call := range turn.Calls {
			if call.Name == "edit_file" {
				args := map[string]any{}
				for k, v := range call.Args {
					args[k] = v
				}
				args["text"] = written
				call.Args = args
			}
			next.Calls = append(next.Calls, call)
		}
		for i := len(turn.Results) - 1; i >= 0; i-- {
			text := turn.Results[i]
			if len(text) > resultCap {
				text = strings.ToValidUTF8(text[:resultCap], "") + "\ntruncated: true"
			}
			if !newest && (!full || used+len(text) > historyCap) {
				full = false
				text = omitted
			}
			newest = false
			used += len(text)
			next.Results[i] = text
		}
		out[t] = next
	}
	return out
}

// warnTurnsLeft adds a reminder to the newest result when lastTurns or fewer
// model turns remain. turns must come from history, which copies the results.
func warnTurnsLeft(turns []providers.Turn, left int) []providers.Turn {
	if left > lastTurns || len(turns) == 0 {
		return turns
	}
	last := turns[len(turns)-1].Results
	if n := len(last); n > 0 {
		last[n-1] += fmt.Sprintf("\n\n%d model turns left in this attempt. If the change is done, call commit now.", left)
	}
	return turns
}

func (s *session) loop(ctx context.Context) error {
	for turn := 0; turn < s.in.MaxTurns; turn++ {
		turns := warnTurnsLeft(s.history(), s.in.MaxTurns-turn)
		resp, err := s.in.Provider.Complete(ctx, providers.Request{
			Model: s.in.Model, System: s.system, User: s.user, Turns: turns,
			Tools: ImplementerTools(), TaskClass: s.in.TaskClass,
		})
		if err != nil {
			return err
		}
		if err := board.AddAttemptTokens(ctx, s.in.DB, s.attemptID, resp.InputTokens, resp.OutputTokens); err != nil {
			return err
		}
		modelID := resp.Model
		if modelID == "" {
			modelID = s.in.Model
		}
		if err := s.event(ctx, "model_call", map[string]any{
			"role": "implementer", "modelId": modelID, "stopReason": resp.StopReason,
			"inputTokens": resp.InputTokens, "outputTokens": resp.OutputTokens,
		}); err != nil {
			return err
		}
		if len(resp.ToolCalls) == 0 {
			return nil
		}
		done := providers.Turn{Text: resp.Text}
		for i, call := range resp.ToolCalls {
			if call.ID == "" {
				call.ID = fmt.Sprintf("call_%d_%d", turn, i)
			}
			summary, rejected, callErr := s.dispatch(ctx, call)
			payload := map[string]any{
				"name": call.Name, "ok": callErr == nil && !rejected, "rejected": rejected, "summary": summary,
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
	return nil
}

func (s *session) dispatch(ctx context.Context, call providers.ToolCall) (string, bool, error) {
	if !allowedTool(call.Name) {
		return "not in profile", true, nil
	}
	switch call.Name {
	case "list_files":
		text, err := listFiles(s.wt.Path, strArg(call.Args, "path"))
		if err != nil {
			return err.Error(), false, nil
		}
		return text, false, nil
	case "search_files":
		text, err := searchFiles(s.wt.Path, strArg(call.Args, "text"))
		if err != nil {
			return err.Error(), false, nil
		}
		return text, false, nil
	case "read_file":
		text, truncated, err := readFile(s.wt.Path, strArg(call.Args, "path"))
		if err != nil {
			return err.Error(), false, nil
		}
		if truncated {
			return text + "\ntruncated: true", false, nil
		}
		return text, false, nil
	case "read_linked_file":
		root, ok := s.linkedRoots[strArg(call.Args, "project")]
		if !ok {
			return "not a linked project with a clone", false, nil
		}
		text, truncated, err := readFile(root, strArg(call.Args, "path"))
		if err != nil {
			return err.Error(), false, nil
		}
		if truncated {
			return text + "\ntruncated: true", false, nil
		}
		return text, false, nil
	case "edit_file":
		if err := editFile(s.wt.Path, strArg(call.Args, "path"), strArg(call.Args, "text")); err != nil {
			return err.Error(), false, nil
		}
		return "wrote " + strArg(call.Args, "path"), false, nil
	case "run_local_check":
		exit, tail, err := s.localCheck(ctx)
		if err != nil {
			return err.Error(), false, nil
		}
		return fmtInt(exit) + "\n" + tail, false, nil
	case "commit":
		message := commitMessage(strArg(call.Args, "message"), s.card.ID, s.attemptID, s.in.Model)
		if err := s.in.Git.Commit(ctx, s.wt, message); err != nil {
			return err.Error(), false, nil
		}
		return "committed", false, nil
	default:
		return "not in profile", true, nil
	}
}

func (s *session) localCheck(ctx context.Context) (int, string, error) {
	dest := filepath.Join(s.in.Git.Data, "exports", s.attemptID)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return 0, "", err
	}
	defer os.RemoveAll(dest)
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
	}, execx.Command)
	if err != nil {
		return 0, "", err
	}
	if !executed {
		return 0, "", fmt.Errorf("check was not executed")
	}
	return res.Exit, lastLines(string(res.Out), 200), nil
}

func (s *session) event(ctx context.Context, kind string, payload map[string]any) error {
	_, err := trace.Append(ctx, s.in.DB, trace.Event{
		CardID: s.card.ID, AttemptID: s.attemptID, ProjectID: s.card.ProjectID, Kind: kind, Payload: payload,
	}, s.in.Secrets)
	return err
}

func allowedTool(name string) bool {
	for _, tool := range ImplementerTools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func readFile(root, requested string) (string, bool, error) {
	full, err := runtime.SafePath(root, requested)
	if err != nil {
		return "", false, err
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return "", false, err
	}
	if len(body) > readCap {
		return string(body[:readCap]), true, nil
	}
	return string(body), false, nil
}

func editFile(root, requested, text string) error {
	if len(text) > writeCap {
		return fmt.Errorf("write exceeds 1 MiB")
	}
	full, err := runtime.SafePath(root, requested)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(full); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", requested)
	}
	if err := os.MkdirAll(dirOf(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(text), 0o644)
}

func commitMessage(message, cardID, attemptID, modelID string) string {
	return message + "\n\nOrchestrator-Card: " + cardID + "\nOrchestrator-Attempt: " + attemptID + "\nModel: " + modelID + "\n"
}

func strArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	switch v := args[key].(type) {
	case string:
		return v
	default:
		return ""
	}
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func dirOf(path string) string {
	return filepath.Dir(path)
}

func fmtInt(n int) string {
	return fmt.Sprintf("exit %d", n)
}

func move(ctx context.Context, db *sql.DB, card board.Card, to string, ev machine.Evidence, secrets []string) (board.Card, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return board.Card{}, err
	}
	defer tx.Rollback()
	next, err := machine.Transition(ctx, tx, card, to, ev, secrets)
	if err != nil {
		if errors.Is(err, machine.ErrIllegal) || errors.Is(err, machine.ErrEvidence) {
			if cerr := tx.Commit(); cerr != nil {
				return board.Card{}, cerr
			}
		}
		return board.Card{}, err
	}
	if err := tx.Commit(); err != nil {
		return board.Card{}, err
	}
	return next, nil
}
