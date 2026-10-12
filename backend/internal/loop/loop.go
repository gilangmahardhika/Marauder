package loop

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/trace"
)

const readCap = 256 << 10

var ErrRequirement = errors.New("unknown or unaccepted requirement")

// ErrOwners rejects a card whose requirements were brought in by different users (RFC-054).
var ErrOwners = errors.New("requirements have different creators; create one card per creator")

// ErrSkill rejects a card that names a skill the project has not enabled (RFC-056).
var ErrSkill = errors.New("skill is not enabled for this project")

func ImplementerTools() []providers.Tool {
	return []providers.Tool{
		{
			Name:        "list_files",
			Description: "List the files under a directory of the card worktree, up to 500 paths. Skips .git, node_modules, vendor, and dist below the start.",
			Parameters: object(map[string]any{
				"path":  str("Repository-relative directory. Empty or . for the root"),
				"glob":  str("Only paths matching this glob, such as **/*.go or *.ts. A glob without / matches the file name"),
				"depth": integer("How many directories deep to list below path. 0 lists everything"),
			}),
		},
		{
			Name:        "search_files",
			Description: "Find lines in the card worktree that contain the text, or match it as an RE2 regular expression when regex is true. Returns path:line: text, and path-line- text for context lines, up to 100 matches.",
			Parameters: object(map[string]any{
				"text":       str("Text to find. Exact and case sensitive unless regex or ignoreCase is set"),
				"regex":      boolean("Treat text as an RE2 regular expression"),
				"path":       str("Repository-relative directory or file to search. Empty for the root"),
				"glob":       str("Only files whose path matches this glob, such as *.go"),
				"ignoreCase": boolean("Match without regard to case"),
				"context":    integer("Lines of context before and after each match, up to 5"),
			}, "text"),
		},
		{
			Name:        "read_file",
			Description: "Read a file from the card worktree as numbered lines N|text. Reads up to 2000 lines from offset; the result says the total and where to read next.",
			Parameters: object(map[string]any{
				"path":   str("Repository-relative path"),
				"offset": integer("First line to read, from 1"),
				"limit":  integer("Number of lines to read, up to 2000"),
			}, "path"),
		},
		{
			Name:        "read_linked_file",
			Description: "Read a file from a linked project by its project id, as numbered lines like read_file. Linked projects are read only.",
			Parameters: object(map[string]any{
				"project": str("Linked project id"),
				"path":    str("Path relative to the linked repository root"),
				"offset":  integer("First line to read, from 1"),
				"limit":   integer("Number of lines to read, up to 2000"),
			}, "project", "path"),
		},
		{
			Name:        "replace_in_file",
			Description: "Replace exact text in a file of the card worktree. old must occur exactly once unless replaceAll is true. Copy old from read_file output without the N| prefixes, with its indentation. Prefer this to edit_file for any change to an existing file.",
			Parameters: object(map[string]any{
				"path":       str("Repository-relative path"),
				"old":        str("The exact text to replace"),
				"new":        str("The text to put in its place"),
				"replaceAll": boolean("Replace every occurrence"),
			}, "path", "old", "new"),
		},
		{
			Name:        "edit_file",
			Description: "Write the whole content of a file in the card worktree, creating it when missing. Use it for new files; change an existing file with replace_in_file.",
			Parameters: object(map[string]any{
				"path": str("Repository-relative path"),
				"text": str("The complete new file content"),
			}, "path", "text"),
		},
		{
			Name:        "delete_file",
			Description: "Delete one file from the card worktree.",
			Parameters:  object(map[string]any{"path": str("Repository-relative path")}, "path"),
		},
		{
			Name:        "move_file",
			Description: "Move or rename one file in the card worktree. The destination must not exist.",
			Parameters: object(map[string]any{
				"from": str("Repository-relative path of the file"),
				"to":   str("Repository-relative path to move it to"),
			}, "from", "to"),
		},
		{
			Name:        "todo_write",
			Description: "Replace your todo list for this attempt. Up to 30 items, each with text of up to 200 bytes and a status. The list is shown to you again after every turn. It changes no file.",
			Parameters: object(map[string]any{
				"items": array(object(map[string]any{
					"text":   str("What to do"),
					"status": map[string]any{"type": "string", "enum": todoStatuses, "description": "pending, in_progress, or done"},
				}, "text", "status"), "The whole list, in order"),
			}, "items"),
		},
		{
			Name:        "run_local_check",
			Description: "Run the card's local check on a copy of the worktree. Returns the exit code and the last 200 lines.",
			Parameters:  object(map[string]any{}),
		},
		{
			Name: "run_command",
			Description: "Run one program from the project's allowed commands on a fresh throwaway copy of the worktree, in the sandbox with no network, " +
				"such as a single test file, a type checker, or a linter. argv is the program name and its arguments, never a shell string; there is no shell, so pipes, redirects, and globs do not work. " +
				"Every change the command makes to files is discarded, and its exit code does not count as the local check. Returns the exit code and the last 200 lines. At most 30 calls per attempt.",
			Parameters: object(map[string]any{
				"argv": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "maxItems": 64,
					"description": "Program name, then each argument as its own string, such as [\"go\", \"test\", \"./pkg/...\"]"},
			}, "argv"),
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

func integer(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func boolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func array(items map[string]any, description string) map[string]any {
	return map[string]any{"type": "array", "items": items, "description": description}
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
	// Commands are the names run_command may start; empty disables it (RFC-084).
	Commands []string
	// Exec runs sandbox children; nil means execx.Command. Tests set it.
	Exec func(context.Context, execx.Child) (execx.Result, error)
	// HistoryBudget is the routed model's history budget in bytes; zero means
	// 16 KiB (RFC-081). MaxTokensPerCard of zero sets no token cap.
	HistoryBudget    int
	MaxTokensPerCard int
	// Live, when set, gets each turn's reply text as the model writes it.
	Live live.Publisher
}

// Result is how the attempt ended. Outcome is empty for a new commit,
// no_commit, no_change for a branch already ahead of its base, or budget with
// the cap in Reason.
type Result struct {
	AttemptID string
	Outcome   string
	Reason    string
	Status    string
}

// DefaultTurns is the turn cap of an attempt whose input sets none (RFC-081).
const DefaultTurns = 40

func Run(ctx context.Context, in Input) (Result, error) {
	if in.TaskClass == "" {
		in.TaskClass = "implement"
	}
	if in.MaxTurns <= 0 {
		in.MaxTurns = DefaultTurns
	}
	if in.HistoryBudget <= 0 {
		in.HistoryBudget = defaultHistory
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
	requirements, err := requirementBlocks(ctx, in.DB, card.ProjectID, card.RequirementIDs)
	if err != nil {
		return Result{}, err
	}
	decisions, err := decisionLines(ctx, in.DB, card.ProjectID, card.RequirementIDs)
	if err != nil {
		return Result{}, err
	}
	paths, err := pathMap(wt.Path)
	if err != nil {
		return Result{}, err
	}
	repoFiles, repoRefs, err := repoMemory(wt.Path, append([]string{card.Title}, requirements...))
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
		Requirements: requirements,
		Decisions:    decisions,
		PathMap:      paths,
		RepoFiles:    repoFiles,
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
		evicted: map[[2]int]bool{}, reads: map[string]readMark{},
	}
	if err := session.event(ctx, "tool_call", map[string]any{
		"name": "conventions_context", "ok": true, "rejected": false,
		"summary": fmt.Sprintf("%d rules and skills", len(convItems)), "items": convItems,
	}); err != nil {
		return Result{}, err
	}
	if len(repoRefs) > 0 {
		lines := make([]string, 0, len(repoRefs))
		for _, ref := range repoRefs {
			lines = append(lines, fmt.Sprintf("%s %d bytes sha256 %s", ref["path"], ref["bytes"], ref["sha256"]))
		}
		if err := session.event(ctx, "tool_call", map[string]any{
			"name": "repo_context", "ok": true, "rejected": false,
			"summary": fmt.Sprintf("%d repository files\n%s", len(repoRefs), strings.Join(lines, "\n")), "files": repoRefs,
		}); err != nil {
			return Result{}, err
		}
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
	if session.stop == "" {
		if err := session.autoCommit(ctx); err != nil {
			_ = board.FinishAttempt(ctx, in.DB, attemptID, "error", "")
			return Result{}, err
		}
	}
	head, err := in.Git.WorktreeHead(ctx, wt)
	if err != nil {
		return Result{}, err
	}
	stop := session.stop
	if stop == "" && head == baseSHA {
		clean, err := in.Git.Clean(ctx, wt)
		if err != nil {
			return Result{}, err
		}
		ahead, err := in.Git.Ahead(ctx, wt, base)
		if err != nil {
			return Result{}, err
		}
		if clean && ahead > 0 {
			if err := board.FinishAttempt(ctx, in.DB, attemptID, "no_change", head); err != nil {
				return Result{}, err
			}
			return Result{AttemptID: attemptID, Outcome: "no_change", Status: card.Status}, nil
		}
		if session.outOfTurns {
			stop = "turn_cap"
		}
	}
	if stop != "" {
		if err := session.event(ctx, "budget_stop", map[string]any{"cap": stop, "turns": session.used, "maxTurns": in.MaxTurns}); err != nil {
			return Result{}, err
		}
		if err := board.AddAttemptUsed(ctx, in.DB, card.ID); err != nil {
			return Result{}, err
		}
		if err := board.FinishAttempt(ctx, in.DB, attemptID, "budget", ""); err != nil {
			return Result{}, err
		}
		return Result{AttemptID: attemptID, Outcome: "budget", Reason: stop, Status: card.Status}, nil
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

// modelCallPayload is one model_call event. text is the reply the model wrote
// beside its tool calls, never the prompt. taskClass is the request's class,
// which the token usage page groups by (RFC-089).
func modelCallPayload(role, taskClass, modelID string, resp providers.Response, secrets []string) map[string]any {
	payload := map[string]any{
		"role": role, "taskClass": taskClass, "modelId": modelID, "stopReason": resp.StopReason,
		"inputTokens": resp.InputTokens, "outputTokens": resp.OutputTokens, "cachedInputTokens": resp.CachedInputTokens,
	}
	if text := trace.Clip(resp.Text, secrets, trace.TextCap); strings.TrimSpace(text) != "" {
		payload["text"] = text
	}
	return payload
}

// toolArgs are the arguments a person needs to follow a tool call. File text
// is never kept: edit_file records its size only.
func toolArgs(call providers.ToolCall, localCheck string, secrets []string) map[string]any {
	clip := func(s string) string { return trace.Clip(s, secrets, trace.ArgCap) }
	args := map[string]any{}
	setInt := func(key string) {
		if n := intArg(call.Args, key); n != 0 {
			args[key] = n
		}
	}
	setStr := func(key string) {
		if v := strArg(call.Args, key); v != "" {
			args[key] = clip(v)
		}
	}
	switch call.Name {
	case "list_files":
		args["path"] = clip(strArg(call.Args, "path"))
		setStr("glob")
		setInt("depth")
	case "read_file":
		args["path"] = clip(strArg(call.Args, "path"))
		setInt("offset")
		setInt("limit")
	case "read_linked_file":
		args["project"] = clip(strArg(call.Args, "project"))
		args["path"] = clip(strArg(call.Args, "path"))
		setInt("offset")
		setInt("limit")
	case "search_files":
		args["text"] = clip(strArg(call.Args, "text"))
		setStr("path")
		setStr("glob")
		setInt("context")
		if boolArg(call.Args, "regex") {
			args["regex"] = true
		}
		if boolArg(call.Args, "ignoreCase") {
			args["ignoreCase"] = true
		}
	case "replace_in_file":
		args["path"] = clip(strArg(call.Args, "path"))
		args["oldBytes"] = len(strArg(call.Args, "old"))
		args["newBytes"] = len(strArg(call.Args, "new"))
		if boolArg(call.Args, "replaceAll") {
			args["replaceAll"] = true
		}
	case "delete_file":
		args["path"] = clip(strArg(call.Args, "path"))
	case "move_file":
		args["from"] = clip(strArg(call.Args, "from"))
		args["to"] = clip(strArg(call.Args, "to"))
	case "edit_file":
		text := strArg(call.Args, "text")
		args["path"] = clip(strArg(call.Args, "path"))
		args["bytes"] = len(text)
		args["lines"] = countLines(text)
	case "commit":
		args["message"] = clip(strArg(call.Args, "message"))
	case "todo_write":
		items, _ := call.Args["items"].([]any)
		args["items"] = len(items)
	case "run_local_check":
		args["command"] = clip(localCheck)
	case "run_command":
		argv, _ := call.Args["argv"].([]any)
		var shown []string
		for _, arg := range argv {
			text, _ := arg.(string)
			shown = append(shown, clip(text))
		}
		args["argv"] = shown
	case "comment_pr", "verdict":
		args["body"] = clip(strArg(call.Args, "body"))
	}
	return args
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	n := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		n++
	}
	return n
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

// intArg reads a JSON number, or a string of digits, as an int.
func intArg(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return 0
}

func boolArg(args map[string]any, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
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
