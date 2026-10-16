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
			Name: "ask_user",
			Description: "Ask the team up to 5 questions when the requirements leave a product choice open that the code and the earlier answers cannot settle. " +
				"Ask before you edit files: on a card where a person answers, the attempt ends after this call, and the next attempt starts with the answers and your uncommitted edits. " +
				"On a card where the agent decides, your recommended answers are recorded and returned, and you go on. " +
				"Give each question your recommended answer and why. Not available after commit.",
			Parameters: object(map[string]any{
				"questions": map[string]any{
					"type": "array", "minItems": 1, "maxItems": maxQuestions, "description": "The questions, most important first",
					"items": object(map[string]any{
						"question": str("The question, at most 500 characters"),
						"kind": map[string]any{"type": "string", "enum": questionKinds,
							"description": "single: exactly one option; multiple: one or more options; text: a written answer with no options"},
						"options": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": maxOptions,
							"description": "2 to 8 distinct options of at most 120 characters for single and multiple; none for text"},
						"allowOther": boolean("Let the person write their own answer beside or instead of the options"),
						"recommended": object(map[string]any{
							"selected": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "The options you recommend"},
							"text":     str("Your recommended written answer, for text questions or an other answer"),
						}),
						"why": str("Why you recommend that answer, in one or two sentences"),
					}, "question", "kind", "recommended"),
				},
			}, "questions"),
		},
		{
			Name: "submit_plan",
			Description: "Submit a structured execution plan for the person to accept before you edit files. " +
				"Use it when this attempt is a planning phase: explore with read tools, then submit a title and an ordered checklist of steps. " +
				"The attempt ends after this call; the next attempt starts with the approved plan. Not available after commit.",
			Parameters: object(map[string]any{
				"title": str("Short plan title, at most 200 characters"),
				"steps": map[string]any{
					"type": "array", "minItems": 1, "maxItems": maxPlanSteps, "description": "Ordered steps, most important first",
					"items": object(map[string]any{
						"text":   str("What to do in this step, at most 500 characters"),
						"detail": str("Optional detail, at most 1000 characters"),
					}, "text"),
				},
			}, "title", "steps"),
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
	// ContextTokens is the routed model's context window; zero means
	// 128,000. The attempt compacts at 75% of it (RFC-117).
	ContextTokens int
	// Live, when set, gets each turn's reply text as the model writes it.
	Live live.Publisher
	// Extra is the tribe's MCP tools for this attempt; nil offers none (RFC-094).
	Extra ExtraTools
	// MCP is how the job scoped the tribe's servers (RFC-102); nil records
	// no scope.
	MCP *MCPContext
	// Board takes the attempt's board reads and writes; nil means
	// DBBoard{DB} (RFC-108).
	Board Board
	// Fixture runs checks as fixture commands, as runtime.FixtureMode does
	// for the fixture repo path.
	Fixture bool
	// QuestionMode is the card's answer mode when the attempt starts: auto,
	// or manual when empty (RFC-115). QuestionRounds counts the card's
	// earlier attempts that ended with a question (RFC-114).
	QuestionMode   string
	QuestionRounds int
	// Planning is true when planMode is required and the card has no
	// accepted plan: write tools are refused and submit_plan parks the
	// card (RFC-128). Claude Code is not used for a planning attempt.
	Planning bool
	// ClaudeCode, when set, runs the Claude Code CLI in place of the tool
	// loop, with Model and MaxTurns (RFC-121).
	ClaudeCode *ClaudeCode
	// Logf, when set, receives thinking text as the model streams it (and
	// Claude Code assistant lines), for the runner or process log.
	Logf func(string, ...any)
}

// ClaudeCode is how an attempt runs the Claude Code CLI. Binary is an
// absolute path; Auth is apiKey or bearer; APIKey must be in Secrets.
type ClaudeCode struct {
	Binary       string
	Version      string
	SmallModel   string
	BaseURL      string
	APIKey       string
	Auth         string
	AllowWeb     bool
	Instructions string
}

func (in Input) board() Board {
	if in.Board != nil {
		return in.Board
	}
	return DBBoard{DB: in.DB}
}

func (in Input) fixture() bool {
	return in.Fixture || runtime.FixtureMode(in.Git.Repo)
}

// MCPContext is the scope of an attempt's MCP servers (RFC-102). Scope is
// all, mentioned, or none. Offered are the mentioned slugs the job kept,
// Missing those that are not an enabled server, and Total the tribe's
// enabled servers.
type MCPContext struct {
	Scope    string
	Offered  []string
	Missing  []string
	Attached []string
	Detached []string
	Reason   string
	Total    int
}

// Result is how the attempt ended. Outcome is empty for a new commit,
// no_commit, no_change for a branch already ahead of its base, budget with
// the cap in Reason (turn_cap, token_cap, or context_full), or question when
// a manual card waits for answers.
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
	b := in.board()
	card, err := b.GetCard(ctx, in.CardID)
	if err != nil {
		return Result{}, err
	}
	if card.Status == "ready" || card.Status == "changes_requested" {
		if in.DB == nil {
			return Result{}, fmt.Errorf("card is %s; the control plane starts it", card.Status)
		}
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
	if err := b.StoreWorktree(ctx, card.ID, wt.Branch, wt.Path, wt.GitDir); err != nil {
		return Result{}, err
	}
	holder, err := board.NewID("lok_")
	if err != nil {
		return Result{}, err
	}
	if err := b.AcquireLock(ctx, card.ID, holder); err != nil {
		return Result{}, err
	}
	defer b.ReleaseLock(context.WithoutCancel(ctx), card.ID, holder)

	if err := in.Git.DiscardChanges(ctx, wt); err != nil {
		return Result{}, err
	}
	baseSHA, err := in.Git.WorktreeHead(ctx, wt)
	if err != nil {
		return Result{}, err
	}
	requirements, err := b.Requirements(ctx, card.ProjectID, card.RequirementIDs)
	if err != nil {
		return Result{}, err
	}
	decisions, err := b.Decisions(ctx, card.ProjectID, card.RequirementIDs)
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
	note, err := b.LatestNote(ctx, card.ID)
	if err != nil {
		return Result{}, err
	}
	reviewNote, err := b.LatestReviewNote(ctx, card.ID)
	if err != nil {
		return Result{}, err
	}
	instructions, err := b.Instructions(ctx, card.ID)
	if err != nil {
		return Result{}, err
	}
	answered, err := b.Answers(ctx, card.ID)
	if err != nil {
		return Result{}, err
	}
	answers := make([]contextpack.Answer, 0, len(answered))
	for _, q := range answered {
		if q.Answer != nil {
			answers = append(answers, contextpack.Answer{Question: q.Question, Answer: AnswerText(*q.Answer), Source: q.AnswerSource})
		}
	}
	var approved *contextpack.Plan
	if plan, ok, err := b.ApprovedPlan(ctx, card.ID); err != nil {
		return Result{}, err
	} else if ok {
		steps := make([]contextpack.PlanStep, 0, len(plan.Steps))
		for _, step := range plan.Steps {
			steps = append(steps, contextpack.PlanStep{Text: step.Text, Detail: step.Detail})
		}
		approved = &contextpack.Plan{Title: plan.Title, Steps: steps}
	}
	convs, convItems, err := b.Conventions(ctx, card.ProjectID, card.ID)
	if err != nil {
		return Result{}, err
	}
	workSoFar, seeded, err := applyHandoff(ctx, in, b, card, wt, base)
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
		ReviewNote:   reviewNote,
		FailureNote:  note,
		Instructions: instructions,
		Answers:      answers,
		Plan:         approved,
		WorkSoFar:    workSoFar,
		Linked:       linked,
		Conventions:  convs,
		Secrets:      in.Secrets,
		MCPServers:   mentionedServers(in.MCP),
	})
	if err != nil {
		return Result{}, err
	}
	n, parent, err := b.NextAttempt(ctx, card.ID)
	if err != nil {
		return Result{}, err
	}
	attemptID, err := board.NewID("att_")
	if err != nil {
		return Result{}, err
	}
	if err := b.InsertAttempt(ctx, board.Attempt{
		ID: attemptID, CardID: card.ID, N: n, ParentID: parent,
		TaskClass: in.TaskClass, ModelID: in.Model, PromptHash: hashText(user),
		SystemHash: systemHash, BaseSHA: baseSHA, MaxTurns: in.MaxTurns,
	}); err != nil {
		return Result{}, err
	}

	session := &session{
		in: in, card: card, wt: wt, attemptID: attemptID,
		system: system, user: user, pack: user, linkedRoots: linkedRoots,
		evicted: map[[2]int]bool{}, reads: map[string]readMark{}, todos: seeded,
	}
	session.tools = ProfileTools(ImplementerTools(), in.Extra)
	session.allowed = map[string]providers.Tool{}
	for _, tool := range session.tools {
		session.allowed[tool.Name] = tool
	}
	if in.Extra != nil || (in.MCP != nil && in.MCP.Scope == "none") {
		if err := session.event(ctx, "tool_call", mcpContextEvent(in.Extra, in.MCP, len(session.tools)-len(ImplementerTools()))); err != nil {
			return Result{}, err
		}
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
	drive := session.loop
	if in.ClaudeCode != nil && !in.Planning {
		drive = session.claude
	}
	if err := drive(ctx); err != nil {
		session.saveHandoff(ctx, "error", base)
		_ = b.FinishAttempt(context.WithoutCancel(ctx), attemptID, "error", "")
		return Result{}, err
	}
	if session.stop == "question" {
		session.saveHandoff(ctx, "question", base)
		if err := b.FinishAttempt(ctx, attemptID, "question", ""); err != nil {
			return Result{}, err
		}
		return Result{AttemptID: attemptID, Outcome: "question", Status: card.Status}, nil
	}
	if session.stop == "plan" {
		session.saveHandoff(ctx, "plan", base)
		if err := b.FinishAttempt(ctx, attemptID, "plan", ""); err != nil {
			return Result{}, err
		}
		return Result{AttemptID: attemptID, Outcome: "plan", Status: card.Status}, nil
	}
	if session.stop == "" {
		if err := session.autoCommit(ctx); err != nil {
			session.saveHandoff(ctx, "error", base)
			_ = b.FinishAttempt(context.WithoutCancel(ctx), attemptID, "error", "")
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
			if err := b.FinishAttempt(ctx, attemptID, "no_change", head); err != nil {
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
		session.saveHandoff(ctx, stop, base)
		if err := b.AddAttemptUsed(ctx, card.ID); err != nil {
			return Result{}, err
		}
		if err := b.FinishAttempt(ctx, attemptID, "budget", ""); err != nil {
			return Result{}, err
		}
		return Result{AttemptID: attemptID, Outcome: "budget", Reason: stop, Status: card.Status}, nil
	}
	if head == baseSHA {
		session.saveHandoff(ctx, "no_commit", base)
		if err := b.InsertFailureNote(ctx, card.ID, attemptID, "no_commit", "no commit"); err != nil {
			return Result{}, err
		}
		if err := b.AddAttemptUsed(ctx, card.ID); err != nil {
			return Result{}, err
		}
		if err := b.FinishAttempt(ctx, attemptID, "no_commit", ""); err != nil {
			return Result{}, err
		}
		fresh, err := b.GetCard(ctx, card.ID)
		if err != nil {
			return Result{}, err
		}
		return Result{AttemptID: attemptID, Outcome: "no_commit", Status: fresh.Status}, nil
	}
	if err := b.FinishAttempt(ctx, attemptID, "", head); err != nil {
		return Result{}, err
	}
	fresh, err := b.GetCard(ctx, card.ID)
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

// toolArgs are the arguments a person needs to follow a tool call. Ordinary
// values use ArgCap. edit_file and replace_in_file also keep capped old/new
// snippets for the Trace UI (RFC-132). editOld is the file content before an
// edit_file write, or empty when the file is new or unknown.
func toolArgs(call providers.ToolCall, localCheck string, secrets []string, editOld string) map[string]any {
	clip := func(s string) string { return trace.Clip(s, secrets, trace.ArgCap) }
	diff := func(s string) string { return trace.Clip(s, secrets, trace.DiffCap) }
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
		args["old"] = diff(strArg(call.Args, "old"))
		args["new"] = diff(strArg(call.Args, "new"))
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
		args["new"] = diff(text)
		if editOld != "" {
			args["old"] = diff(editOld)
		}
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
	case "ask_user":
		questions, _ := call.Args["questions"].([]any)
		args["count"] = len(questions)
	case "submit_plan":
		args["title"] = clip(strArg(call.Args, "title"))
		steps, _ := call.Args["steps"].([]any)
		args["steps"] = len(steps)
	case "comment_pr", "verdict":
		args["body"] = clip(strArg(call.Args, "body"))
	default:
		if IsMCP(call.Name) {
			return MCPArgs(call.Name, call.Args)
		}
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
	return s.in.board().AppendEvent(ctx, trace.Event{
		CardID: s.card.ID, AttemptID: s.attemptID, ProjectID: s.card.ProjectID, Kind: kind, Payload: payload,
	}, s.in.Secrets)
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
