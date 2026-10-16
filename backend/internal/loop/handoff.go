package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

const (
	// MaxHandoffSummary is the most of a summary a handoff or a compaction
	// keeps (RFC-117).
	MaxHandoffSummary = 8 << 10
	compactMaxTokens  = 2048
	maxCompactions    = 3
	handoffTimeout    = 30 * time.Second
	maxWorkCommits    = 40
	checkTailCap      = 2 << 10
)

const compactNote = "Stop working now and write a handoff for whoever continues this card, in at most 400 words, with no tool call. " +
	"Use these headings: Goal, Done, In progress, Next steps, Files touched, Decisions and assumptions, Open problems. " +
	"Under Open problems give the key error line of a failing check. Say what is finished and what is not; do not repeat the requirements."

// contextLimit is the input size at which the attempt compacts: 75% of the
// routed model's context window.
func (s *session) contextLimit() int {
	return settings.ModelOptions{ContextTokens: s.in.ContextTokens}.ContextLimit()
}

// summarize asks the model, without tools, for a handoff of the attempt so
// far. When that request does not fit, it is sent once more with only the
// newer half of the turns. Its tokens count toward the attempt, and it is
// logged as a model call.
func (s *session) summarize(ctx context.Context) (string, error) {
	turns := withTodos(s.history(), s.todos)
	user := s.user
	if len(turns) == 0 {
		user = joinNote(user, compactNote)
	} else {
		last := &turns[len(turns)-1]
		last.Note = joinNote(last.Note, compactNote)
	}
	req := providers.Request{
		Model: s.in.Model, System: s.system, User: user, Turns: turns, Tools: s.profile(),
		ToolChoice: "none", TaskClass: s.in.TaskClass, MaxTokens: compactMaxTokens,
	}
	resp, err := s.in.Provider.Complete(ctx, req)
	if providers.IsContextFull(err) && len(turns) > 1 {
		req.Turns = turns[len(turns)/2:]
		resp, err = s.in.Provider.Complete(ctx, req)
	}
	if err != nil {
		return "", err
	}
	if err := s.in.board().AddAttemptTokens(ctx, s.attemptID, resp.InputTokens, resp.OutputTokens, resp.CachedInputTokens); err != nil {
		return "", err
	}
	modelID := resp.Model
	if modelID == "" {
		modelID = s.in.Model
	}
	if err := s.event(ctx, "model_call", modelCallPayload("implementer", s.in.TaskClass, modelID, resp, s.in.Secrets)); err != nil {
		return "", err
	}
	summary := clipSummary(trace.Redact(strings.TrimSpace(resp.Text), s.in.Secrets))
	if summary == "" {
		return "", errors.New("the model wrote no handoff summary")
	}
	return summary, nil
}

func clipSummary(text string) string {
	if len(text) <= MaxHandoffSummary {
		return text
	}
	const note = "\n(summary cut at 8 KiB)"
	return strings.ToValidUTF8(text[:MaxHandoffSummary-len(note)], "") + note
}

// compact replaces the attempt's turns with a model-written summary in the
// user message (RFC-117). A fourth compaction, a compaction with no turns to
// drop, or a summary request that does not fit ends the attempt with
// stop context_full.
func (s *session) compact(ctx context.Context, inputTokens int) error {
	s.compactNext = false
	if s.compactions >= maxCompactions || len(s.turns) == 0 {
		s.stop = "context_full"
		return nil
	}
	summary, err := s.summarize(ctx)
	if providers.IsContextFull(err) {
		s.stop = "context_full"
		return nil
	}
	if err != nil {
		return err
	}
	dropped := len(s.turns)
	s.compactions++
	s.lastSummary = summary
	s.turns, s.evicted, s.reads = nil, map[[2]int]bool{}, map[string]readMark{}
	s.user = contextpack.WithWorkSoFar(s.pack, contextpack.WorkSoFar{Summary: summary, Todos: todoLines(s.todos)}, s.in.Secrets)
	return s.event(ctx, "context_compacted", map[string]any{
		"inputTokens": inputTokens, "turnsDropped": dropped, "summaryBytes": len(summary), "n": s.compactions,
	})
}

func todoLines(items []todoItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, "["+item.Status+"] "+item.Text)
	}
	return out
}

func boardTodos(items []todoItem) []board.Todo {
	out := make([]board.Todo, 0, len(items))
	for _, item := range items {
		out = append(out, board.Todo{Text: item.Text, Status: item.Status})
	}
	return out
}

// modelSummaryStops are the endings after which the model can still write
// its own handoff summary; after the others the harness writes it (RFC-118).
var modelSummaryStops = map[string]bool{"turn_cap": true, "no_commit": true, "question": true, "plan": true}

// saveHandoff stores what this attempt leaves the card's next one: a summary,
// the todo list, and a patch of the uncommitted work. It runs on its own
// 30-second limit after the attempt's context may be cancelled, and a
// failure here does not change how the attempt ended.
func (s *session) saveHandoff(ctx context.Context, ending, base string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), handoffTimeout)
	defer cancel()
	work, err := s.in.Git.WorkPatch(ctx, s.wt)
	if err != nil {
		work = runtime.WorkPatch{}
	}
	commits, _ := s.in.Git.Commits(ctx, s.wt, base, maxWorkCommits)
	if len(work.Files) == 0 && len(work.Skipped) == 0 && len(s.todos) == 0 && len(s.turns) == 0 && s.lastSummary == "" && s.finalText == "" {
		return
	}
	source, summary := "harness", ""
	if s.finalText != "" {
		source, summary = "model", clipSummary(s.finalText)
	} else if modelSummaryStops[ending] && len(s.turns) > 0 {
		if text, err := s.summarize(ctx); err == nil {
			source, summary = "model", text
		}
	}
	if summary == "" {
		summary = s.harnessSummary(ending, work, commits)
	}
	if notes := patchNotes(work); notes != "" {
		summary = clipSummary(summary + "\n\n" + notes)
	}
	h := board.Handoff{
		CardID: s.card.ID, AttemptID: s.attemptID, Summary: summary, SummarySource: source,
		Todos: boardTodos(s.todos), Patch: work.Patch, PatchBaseSHA: work.Base, PatchFiles: len(work.Files),
	}
	if work.Patch == "" {
		h.PatchFiles = 0
	}
	if err := s.in.board().SaveHandoff(ctx, h); err != nil {
		return
	}
	_ = s.event(ctx, "handoff_saved", map[string]any{
		"files": h.PatchFiles, "patchBytes": len(h.Patch), "source": source, "left": len(work.Skipped),
	})
}

// patchNotes says what the patch left out, for the summary.
func patchNotes(work runtime.WorkPatch) string {
	var notes []string
	if len(work.Skipped) > 0 {
		notes = append(notes, "Left out of the saved changes as over 1 MiB: "+strings.Join(work.Skipped, ", ")+".")
	}
	if work.Dropped {
		notes = append(notes, fmt.Sprintf("The uncommitted changes to %d files were over 2 MiB and were not saved: %s.",
			len(work.Files), strings.Join(work.Files, ", ")))
	}
	return strings.Join(notes, "\n")
}

// harnessSummary is the handoff summary when the model cannot write one: how
// the attempt ended, its todo list, the changed files, the branch's commits,
// the last local check, and the last compaction summary.
func (s *session) harnessSummary(ending string, work runtime.WorkPatch, commits []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The last attempt stopped (%s) after %d model turns, before it committed. This summary was written by the harness.\n", ending, s.used)
	if len(s.todos) > 0 {
		b.WriteString("\nIts todo list:\n")
		for _, line := range todoLines(s.todos) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if len(work.Files) > 0 {
		b.WriteString("\nFiles it changed and did not commit:\n")
		for _, file := range work.Files {
			b.WriteString(file)
			b.WriteString("\n")
		}
	}
	if len(commits) > 0 {
		fmt.Fprintf(&b, "\n%d commits on the branch since the base.\n", len(commits))
	}
	if s.checked {
		fmt.Fprintf(&b, "\nThe last run_local_check exited %d. Its last lines:\n%s\n", s.checkExit, tailBytes(s.checkTail, checkTailCap))
	}
	if s.lastSummary != "" {
		b.WriteString("\nThe model's last summary:\n")
		b.WriteString(s.lastSummary)
		b.WriteString("\n")
	}
	return clipSummary(trace.Redact(strings.TrimSpace(b.String()), s.in.Secrets))
}

func tailBytes(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return strings.ToValidUTF8(text[len(text)-n:], "")
}

// applyHandoff puts the last attempt's uncommitted work back in the worktree
// and returns what the pack and the session take from its handoff: the WORK
// SO FAR block and the todo list. The handoff is marked used whether the
// patch applied or not, so it is applied once (RFC-118).
func applyHandoff(ctx context.Context, in Input, b Board, card board.Card, wt runtime.Worktree, base string) (*contextpack.WorkSoFar, []todoItem, error) {
	h, ok, err := b.PendingHandoff(ctx, card.ID)
	if err != nil || !ok {
		return nil, nil, err
	}
	work := &contextpack.WorkSoFar{Summary: h.Summary}
	applied, reason := true, ""
	if h.Patch != "" {
		if _, err := in.Git.ApplyPatch(ctx, wt, h.Patch, h.PatchBaseSHA); err != nil {
			if !errors.Is(err, runtime.ErrPatch) {
				return nil, nil, err
			}
			applied, reason = false, "it conflicts with the branch as it is now"
			work.PatchNote = reason
		} else {
			work.PatchFiles = h.PatchFiles
		}
	}
	if err := b.MarkHandoffApplied(ctx, card.ID, h.ID, applied, reason); err != nil {
		if errors.Is(err, board.ErrConflict) {
			return nil, nil, in.Git.DiscardChanges(ctx, wt)
		}
		return nil, nil, err
	}
	todos := make([]todoItem, 0, len(h.Todos))
	for _, todo := range h.Todos {
		todos = append(todos, todoItem{Text: todo.Text, Status: todo.Status})
	}
	work.Todos = todoLines(todos)
	if commits, err := in.Git.Commits(ctx, wt, base, maxWorkCommits); err == nil {
		work.Commits = commits
	}
	payload := map[string]any{
		"fromAttemptId": h.AttemptID, "ok": applied, "files": work.PatchFiles, "todos": len(todos),
	}
	if reason != "" {
		payload["reason"] = reason
	}
	if err := b.AppendEvent(ctx, trace.Event{
		CardID: card.ID, ProjectID: card.ProjectID, Kind: "handoff_applied", Payload: payload,
	}, in.Secrets); err != nil {
		return nil, nil, err
	}
	return work, todos, nil
}
