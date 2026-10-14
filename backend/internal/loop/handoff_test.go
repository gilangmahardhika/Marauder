package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/trace"
)

// scripted answers each request with the next reply, and records them.
type scripted struct {
	mu      sync.Mutex
	replies []func(providers.Request) (providers.Response, error)
	reqs    []providers.Request
}

func (p *scripted) Complete(_ context.Context, req providers.Request) (providers.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reqs = append(p.reqs, req)
	if len(p.reqs) > len(p.replies) {
		return providers.Response{}, fmt.Errorf("unexpected request %d", len(p.reqs))
	}
	return p.replies[len(p.reqs)-1](req)
}

func reply(resp providers.Response) func(providers.Request) (providers.Response, error) {
	return func(providers.Request) (providers.Response, error) { return resp, nil }
}

func fail(err error) func(providers.Request) (providers.Response, error) {
	return func(providers.Request) (providers.Response, error) { return providers.Response{}, err }
}

// handoffBoard records events, tokens, and handoffs. Any other call panics.
type handoffBoard struct {
	Board
	events  []trace.Event
	saved   []board.Handoff
	pending *board.Handoff
	marked  []string
	markErr error
}

func (b *handoffBoard) AppendEvent(_ context.Context, ev trace.Event, _ []string) error {
	b.events = append(b.events, ev)
	return nil
}

func (b *handoffBoard) AddAttemptTokens(context.Context, string, int, int, int) error { return nil }

func (b *handoffBoard) SaveHandoff(_ context.Context, h board.Handoff) error {
	b.saved = append(b.saved, h)
	return nil
}

func (b *handoffBoard) PendingHandoff(context.Context, string) (board.Handoff, bool, error) {
	if b.pending == nil {
		return board.Handoff{}, false, nil
	}
	return *b.pending, true, nil
}

func (b *handoffBoard) MarkHandoffApplied(_ context.Context, _, id string, ok bool, reason string) error {
	if b.markErr != nil {
		return b.markErr
	}
	b.marked = append(b.marked, fmt.Sprintf("%s %v %s", id, ok, reason))
	return nil
}

func (b *handoffBoard) payloads(kind string) []map[string]any {
	var out []map[string]any
	for _, ev := range b.events {
		if ev.Kind == kind {
			payload, _ := ev.Payload.(map[string]any)
			out = append(out, payload)
		}
	}
	return out
}

func compactSession(t *testing.T, p providers.Provider) (*session, *handoffBoard) {
	t.Helper()
	s := commandSession(t, nil, refuseExec(t))
	b := &handoffBoard{}
	s.in.Board, s.in.Provider, s.in.ContextTokens = b, p, 8000
	s.in.MaxTurns, s.in.Model, s.in.TaskClass = 3, "m", "implement"
	s.card = board.Card{ID: "card_compact"}
	s.user, s.pack = "CARD\nid: card_compact\n", "CARD\nid: card_compact\n"
	return s, b
}

func readReply(input int) providers.Response {
	return providers.Response{
		InputTokens: input,
		ToolCalls:   []providers.ToolCall{{ID: "r1", Name: "read_file", Args: map[string]any{"path": "main.go"}}},
	}
}

func TestCompactOnContextLimit(t *testing.T) {
	p := &scripted{replies: []func(providers.Request) (providers.Response, error){
		reply(readReply(5000)),
		reply(readReply(6500)),
		func(req providers.Request) (providers.Response, error) {
			last := req.Turns[len(req.Turns)-1].Note
			if req.ToolChoice != "none" || req.MaxTokens != compactMaxTokens || !strings.Contains(last, "Next steps") {
				return providers.Response{}, fmt.Errorf("summary request = %q %d %q", req.ToolChoice, req.MaxTokens, last)
			}
			return providers.Response{Text: "Goal: fix main\nDone: read main.go"}, nil
		},
		reply(providers.Response{Text: "done"}),
	}}
	s, b := compactSession(t, p)
	s.committed = true
	if err := s.loop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.reqs) != 4 {
		t.Fatalf("requests = %d, want 4", len(p.reqs))
	}
	after := p.reqs[3]
	if len(after.Turns) != 0 || !strings.Contains(after.User, "WORK SO FAR\nGoal: fix main") || !strings.HasPrefix(after.User, s.pack) {
		t.Fatalf("after compaction: %d turns, user %q", len(after.Turns), after.User)
	}
	got := b.payloads("context_compacted")
	if len(got) != 1 || got[0]["turnsDropped"] != 2 || got[0]["inputTokens"] != 6500 || got[0]["n"] != 1 {
		t.Fatalf("context_compacted = %v", got)
	}
	if calls := b.payloads("model_call"); len(calls) != 4 {
		t.Fatalf("model_call events = %d, want 4 with the summary", len(calls))
	}
}

func TestCompactOnOverflowError(t *testing.T) {
	full := fmt.Errorf("provider_error: %w: prompt is too long", providers.ErrContextFull)
	p := &scripted{replies: []func(providers.Request) (providers.Response, error){
		reply(readReply(100)),
		fail(full),
		reply(providers.Response{Text: "Done: read main.go\nNext steps: edit it"}),
		func(req providers.Request) (providers.Response, error) {
			if len(req.Turns) != 0 || !strings.Contains(req.User, "Next steps: edit it") {
				return providers.Response{}, errors.New("the turn was not sent again after compacting")
			}
			return providers.Response{Text: "ok"}, nil
		},
	}}
	s, b := compactSession(t, p)
	s.committed = true
	if err := s.loop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.reqs) != 4 || s.stop != "" || s.compactions != 1 {
		t.Fatalf("requests %d, stop %q, compactions %d", len(p.reqs), s.stop, s.compactions)
	}
	if got := b.payloads("context_compacted"); len(got) != 1 {
		t.Fatalf("context_compacted = %v", got)
	}
}

func TestCompactCap(t *testing.T) {
	p := &scripted{replies: []func(providers.Request) (providers.Response, error){
		reply(readReply(7000)),
	}}
	s, b := compactSession(t, p)
	s.compactions = maxCompactions
	if err := s.loop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.stop != "context_full" || len(p.reqs) != 1 || len(b.payloads("context_compacted")) != 0 {
		t.Fatalf("stop %q after %d requests", s.stop, len(p.reqs))
	}

	full := fmt.Errorf("%w: prompt is too long", providers.ErrContextFull)
	p = &scripted{replies: []func(providers.Request) (providers.Response, error){
		reply(readReply(100)), fail(full), fail(full),
	}}
	s, _ = compactSession(t, p)
	if err := s.loop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.stop != "context_full" {
		t.Fatalf("a summary request that does not fit: stop %q", s.stop)
	}
}

// gitSession is a session on a real card worktree of a one-commit repo.
func gitSession(t *testing.T, p providers.Provider) (*session, *handoffBoard) {
	t.Helper()
	bin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repo, data := filepath.Join(root, "repo"), filepath.Join(root, "data")
	for _, dir := range []string{repo, filepath.Join(data, "empty-hooks"), filepath.Join(data, "empty-gh"), filepath.Join(data, "exports")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeTree(t, repo, map[string]string{"main.go": "package main\n\nfunc main() {}\n"})
	for _, args := range [][]string{
		{"init", "-b", "main"}, {"add", "main.go"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-m", "init"},
	} {
		if out, err := exec.Command(bin, append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git := runtime.Git{Bin: bin, Repo: repo, Data: data, Author: runtime.LoginAuthor("bot", 0)}
	wt, err := git.EnsureWorktree(context.Background(), "card_0123456789ab", "main")
	if err != nil {
		t.Fatal(err)
	}
	b := &handoffBoard{}
	s := &session{
		in:   Input{Git: git, Board: b, Provider: p, Model: "m", TaskClass: "implement", MaxTurns: 5},
		card: board.Card{ID: "card_0123456789ab", ProjectID: "proj_default"}, wt: wt, attemptID: "att_handoff",
		user: "CARD\n", pack: "CARD\n", evicted: map[[2]int]bool{}, reads: map[string]readMark{},
	}
	return s, b
}

func editMain(t *testing.T, s *session, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.wt.Path, "main.go"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readMain(t *testing.T, s *session) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(s.wt.Path, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestHandoffSavedOnBudget(t *testing.T) {
	p := &scripted{replies: []func(providers.Request) (providers.Response, error){
		func(req providers.Request) (providers.Response, error) {
			if req.ToolChoice != "none" {
				return providers.Response{}, errors.New("the summary request offered tools")
			}
			return providers.Response{Text: "Goal: greet\nIn progress: main.go prints hello"}, nil
		},
	}}
	s, b := gitSession(t, p)
	editMain(t, s, "package main\n\nfunc main() { println(\"hello\") }\n")
	writeTree(t, s.wt.Path, map[string]string{"greet.go": "package main\n"})
	s.turns = []providers.Turn{readTurn(0, 10)}
	s.todos = []todoItem{{Text: "print hello", Status: "in_progress"}}
	s.used = 5
	s.saveHandoff(context.Background(), "turn_cap", "main")
	if len(b.saved) != 1 {
		t.Fatalf("handoffs = %d", len(b.saved))
	}
	h := b.saved[0]
	head, _ := s.in.Git.WorktreeHead(context.Background(), s.wt)
	if h.SummarySource != "model" || !strings.Contains(h.Summary, "main.go prints hello") || h.PatchFiles != 2 ||
		h.PatchBaseSHA != head || !strings.Contains(h.Patch, "greet.go") || len(h.Todos) != 1 || h.Todos[0].Text != "print hello" {
		t.Fatalf("handoff = %+v", h)
	}
	if clean, _ := s.in.Git.Clean(context.Background(), s.wt); clean {
		t.Fatal("saving the handoff changed the worktree")
	}
	saved := b.payloads("handoff_saved")
	if len(saved) != 1 || saved[0]["files"] != 2 || saved[0]["source"] != "model" {
		t.Fatalf("handoff_saved = %v", saved)
	}
}

func TestHandoffHarnessFallback(t *testing.T) {
	refuse := &scripted{}
	s, b := gitSession(t, refuse)
	editMain(t, s, "package main\n\nfunc main() { panic(1) }\n")
	s.turns = []providers.Turn{readTurn(0, 10)}
	s.checked, s.checkExit, s.checkTail = true, 1, "--- FAIL: TestMain\nmain_test.go:9: boom"
	s.lastSummary = "Goal: make main fail loudly"
	s.saveHandoff(context.Background(), "token_cap", "main")
	if len(refuse.reqs) != 0 {
		t.Fatal("a token cap asked the model for a summary")
	}
	h := b.saved[0]
	for _, want := range []string{"token_cap", "main.go", "exited 1", "main_test.go:9: boom", "make main fail loudly"} {
		if h.SummarySource != "harness" || !strings.Contains(h.Summary, want) {
			t.Fatalf("summary (%s) lacks %q:\n%s", h.SummarySource, want, h.Summary)
		}
	}

	failing := &scripted{replies: []func(providers.Request) (providers.Response, error){fail(errors.New("provider down"))}}
	s, b = gitSession(t, failing)
	editMain(t, s, "package main\n")
	s.turns = []providers.Turn{readTurn(0, 10)}
	s.saveHandoff(context.Background(), "turn_cap", "main")
	if len(b.saved) != 1 || b.saved[0].SummarySource != "harness" || len(failing.reqs) != 1 {
		t.Fatalf("a failed summary call: %+v after %d requests", b.saved, len(failing.reqs))
	}
}

// pendingFrom saves the session's worktree as a handoff, then resets it as
// a new attempt does.
func pendingFrom(t *testing.T, s *session) *board.Handoff {
	t.Helper()
	work, err := s.in.Git.WorkPatch(context.Background(), s.wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.in.Git.DiscardChanges(context.Background(), s.wt); err != nil {
		t.Fatal(err)
	}
	return &board.Handoff{
		ID: "hof_000000000001", AttemptID: "att_before", Summary: "Done: main prints hello", SummarySource: "model",
		Todos: []board.Todo{{Text: "print hello", Status: "done"}, {Text: "add a test", Status: "pending"}},
		Patch: work.Patch, PatchBaseSHA: work.Base, PatchFiles: len(work.Files),
	}
}

func TestHandoffAppliedNextAttempt(t *testing.T) {
	s, b := gitSession(t, &scripted{})
	edited := "package main\n\nfunc main() { println(\"hello\") }\n"
	editMain(t, s, edited)
	b.pending = pendingFrom(t, s)
	if readMain(t, s) == edited {
		t.Fatal("the reset kept the edit")
	}
	work, _, err := applyHandoff(context.Background(), s.in, b, s.card, s.wt, "main")
	if err != nil {
		t.Fatal(err)
	}
	if readMain(t, s) != edited {
		t.Fatalf("main.go = %q, want the last attempt's edit", readMain(t, s))
	}
	if work == nil || work.PatchFiles != 1 || work.PatchNote != "" || work.Summary != "Done: main prints hello" {
		t.Fatalf("work so far = %+v", work)
	}
	if len(b.marked) != 1 || b.marked[0] != "hof_000000000001 true " {
		t.Fatalf("marked = %v", b.marked)
	}
	applied := b.payloads("handoff_applied")
	if len(applied) != 1 || applied[0]["ok"] != true || applied[0]["files"] != 1 || applied[0]["fromAttemptId"] != "att_before" {
		t.Fatalf("handoff_applied = %v", applied)
	}
}

func TestHandoffConflictSkipped(t *testing.T) {
	s, b := gitSession(t, &scripted{})
	editMain(t, s, "package main\n\nfunc main() { println(\"hello\") }\n")
	b.pending = pendingFrom(t, s)
	editMain(t, s, "package main\n\nfunc main() { println(\"goodbye\") }\n")
	if err := s.in.Git.Commit(context.Background(), s.wt, "say goodbye"); err != nil {
		t.Fatal(err)
	}
	work, _, err := applyHandoff(context.Background(), s.in, b, s.card, s.wt, "main")
	if err != nil {
		t.Fatal(err)
	}
	if clean, _ := s.in.Git.Clean(context.Background(), s.wt); !clean {
		t.Fatal("a patch that did not apply left changes in the worktree")
	}
	if work.PatchFiles != 0 || work.PatchNote == "" || len(work.Commits) != 1 || !strings.Contains(work.Commits[0], "say goodbye") {
		t.Fatalf("work so far = %+v", work)
	}
	if len(b.marked) != 1 || !strings.HasPrefix(b.marked[0], "hof_000000000001 false") {
		t.Fatalf("marked = %v", b.marked)
	}
	if applied := b.payloads("handoff_applied"); len(applied) != 1 || applied[0]["ok"] != false || applied[0]["reason"] == "" {
		t.Fatalf("handoff_applied = %v", applied)
	}
}

func TestHandoffTodosSeeded(t *testing.T) {
	s, b := gitSession(t, &scripted{})
	editMain(t, s, "package main\n\n// hello\n")
	b.pending = pendingFrom(t, s)
	work, todos, err := applyHandoff(context.Background(), s.in, b, s.card, s.wt, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 2 || todos[0] != (todoItem{Text: "print hello", Status: "done"}) || todos[1].Status != "pending" {
		t.Fatalf("todos = %+v", todos)
	}
	if len(work.Todos) != 2 || work.Todos[1] != "[pending] add a test" {
		t.Fatalf("work so far todos = %v", work.Todos)
	}
	turns := withTodos([]providers.Turn{{}}, todos)
	if !strings.Contains(turns[0].Note, "[pending] add a test") {
		t.Fatalf("note = %q", turns[0].Note)
	}

	used, ub := gitSession(t, &scripted{})
	editMain(t, used, "package main\n\n// used\n")
	ub.pending = pendingFrom(t, used)
	ub.markErr = board.ErrConflict
	work, todos, err = applyHandoff(context.Background(), used.in, ub, used.card, used.wt, "main")
	if err != nil || work != nil || todos != nil {
		t.Fatalf("a handoff another attempt used: %+v %v %v", work, todos, err)
	}
	if clean, _ := used.in.Git.Clean(context.Background(), used.wt); !clean {
		t.Fatal("a used handoff's patch stayed in the worktree")
	}
}
