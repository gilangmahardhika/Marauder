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

func ImplementerTools() []providers.Tool {
	return []providers.Tool{
		{Name: "read_file"},
		{Name: "read_linked_file"},
		{Name: "edit_file"},
		{Name: "run_local_check"},
		{Name: "commit"},
	}
}

type Settings struct {
	LocalCheck string
	DevCheck   string
	Allow      []string
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
	system, user, systemHash, err := contextpack.BuildImplementer(contextpack.Input{
		Card: contextpack.Card{
			ID: card.ID, Title: card.Title, Status: card.Status, LocalCheck: card.LocalCheck,
		},
		Requirements: statements,
		FailureNote:  note,
		Linked:       linked,
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
}

func (s *session) loop(ctx context.Context) error {
	for turn := 0; turn < s.in.MaxTurns; turn++ {
		resp, err := s.in.Provider.Complete(ctx, providers.Request{
			Model: s.in.Model, System: s.system, User: s.user,
			Tools: ImplementerTools(), TaskClass: s.in.TaskClass,
		})
		if err != nil {
			return err
		}
		if err := s.event(ctx, "model_call", map[string]any{
			"role": "implementer", "modelId": s.in.Model, "stopReason": resp.StopReason,
		}); err != nil {
			return err
		}
		if len(resp.ToolCalls) == 0 {
			return nil
		}
		var results []string
		for _, call := range resp.ToolCalls {
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
			results = append(results, call.Name+": "+summary)
		}
		s.user = s.user + "\n" + strings.Join(results, "\n")
	}
	return nil
}

func (s *session) dispatch(ctx context.Context, call providers.ToolCall) (string, bool, error) {
	if !allowedTool(call.Name) {
		return "not in profile", true, nil
	}
	switch call.Name {
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
		Dir:     dest,
		Timeout: 2 * time.Minute,
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
