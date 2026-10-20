package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/mcp"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/llm"
	"orchestrator/backend/internal/runnerapi"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/trace"
)

var (
	// ErrRunnerOp is a runner call the lease does not allow: an unknown board
	// op, an event kind a runner may not write, or a call on another attempt.
	ErrRunnerOp = errors.New("runner call is not allowed")
	// ErrBundle is a bundle the control plane refused.
	ErrBundle = errors.New("bundle refused")
)

// remoteEventKinds are the event kinds a runner may append (RFC-108).
var remoteEventKinds = map[string]bool{
	"model_call": true, "tool_call": true, "budget_stop": true, "job_error": true, "assertion_failed": true,
	"context_compacted": true, "handoff_saved": true, "handoff_applied": true, "agent_run": true,
}

// RemoteJobs holds what the control plane resolved for each open lease: the
// redaction list, the routed model, and the MCP toolset. A lease missing
// here, after a restart, is resolved again from its row.
type RemoteJobs struct {
	mu   sync.Mutex
	jobs map[string]*remoteJob
}

type remoteJob struct {
	mu      sync.Mutex
	p       preparedJob
	toolset *MCPToolset
	opened  bool
}

func NewRemoteJobs() *RemoteJobs {
	return &RemoteJobs{jobs: map[string]*remoteJob{}}
}

func (r *RemoteJobs) put(leaseID string, job *remoteJob) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.jobs[leaseID]; old != nil {
		old.close()
	}
	r.jobs[leaseID] = job
}

// Drop forgets a lease and closes its MCP servers.
func (r *RemoteJobs) Drop(leaseID string) {
	r.mu.Lock()
	job := r.jobs[leaseID]
	delete(r.jobs, leaseID)
	r.mu.Unlock()
	if job != nil {
		job.close()
	}
}

func (j *remoteJob) close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.toolset.Close()
	j.toolset = nil
}

func (r *RemoteJobs) get(ctx context.Context, db *sql.DB, deps JobDeps, lease board.Lease) (*remoteJob, error) {
	r.mu.Lock()
	job := r.jobs[lease.ID]
	r.mu.Unlock()
	if job != nil {
		return job, nil
	}
	p, err := resolveRemote(ctx, db, deps, lease)
	if err != nil {
		return nil, err
	}
	job = &remoteJob{p: p}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.jobs[lease.ID]; existing != nil {
		return existing, nil
	}
	r.jobs[lease.ID] = job
	return job, nil
}

// resolveRemote rebuilds a lease's job state without moving the card.
func resolveRemote(ctx context.Context, db *sql.DB, deps JobDeps, lease board.Lease) (preparedJob, error) {
	var p preparedJob
	card, err := board.GetCard(ctx, db, lease.CardID)
	if err != nil {
		return p, err
	}
	project, err := tenancy.GetProject(ctx, db, lease.ProjectID)
	if err != nil {
		return p, err
	}
	cfg, secrets, err := settings.Load(deps.Data)
	if err != nil {
		return p, err
	}
	projectCfg, _, err := settings.LoadProject(deps.Data, lease.ProjectID)
	if err != nil {
		return p, err
	}
	redact := append(modelKeys(secrets), deps.Secrets...)
	acct, err := company.Load(ctx, db)
	if err != nil {
		return p, err
	}
	companyToken, err := company.Token(ctx, db, deps.Key)
	if err != nil {
		return p, err
	}
	redact = append(redact, companyToken)
	owner, err := OwnerGitHub(ctx, db, deps.Key, card, acct.Host)
	if err != nil && !errors.Is(err, ErrOwnerGitHub) {
		return p, err
	}
	redact = append(redact, owner.Token)
	conns, err := LoadProjectMCP(ctx, db, deps.Key, lease.ProjectID)
	if err != nil {
		return p, err
	}
	redact = append(redact, MCPSecrets(conns)...)
	conns, scope, err := MCPScopeFor(ctx, db, lease.ProjectID, card.ID, card.RequirementIDs, conns)
	if err != nil {
		return p, err
	}
	provider := deps.Provider
	if provider == nil {
		provider = RoutedProvider(cfg, secrets, lease.ModelID, lease.FallbackModelID)
	}
	return preparedJob{
		card: card, project: project, cfg: cfg, projectCfg: projectCfg, redact: redact,
		acct: acct, companyToken: companyToken, owner: owner, mcpConns: conns, mcpScope: scope,
		fixture: runtime.FixtureMode(project.RepoPath), engine: deps.Tools.Engine(projectCfg.Sandbox.Engine),
		class: lease.TaskClass, model: lease.ModelID, fallback: lease.FallbackModelID, provider: provider,
	}, nil
}

// RemoteLease finds an open lease the runner holds; any other is
// board.ErrNotFound.
func RemoteLease(ctx context.Context, db *sql.DB, r board.Runner, leaseID string) (board.Lease, error) {
	return board.OpenLease(ctx, db, r.ID, leaseID)
}

// StartRemote runs the implement preflight for a lease and returns the job
// context (RFC-108). When the preflight blocked the card, the card cannot
// start, or a passed commit only needed its pull request, the job is done
// here and the context says so.
func StartRemote(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease) (runnerapi.Job, error) {
	doneJob := func(card board.Card) runnerapi.Job {
		return runnerapi.Job{Done: true, CardID: lease.CardID, ProjectID: lease.ProjectID, Status: card.Status}
	}
	p, done, err := implementPreflight(ctx, db, deps, lease.ProjectID, lease.CardID, true)
	if errors.Is(err, ErrNotStartable) {
		card, err := board.GetCard(ctx, db, lease.CardID)
		return doneJob(card), err
	}
	if err != nil {
		return runnerapi.Job{}, err
	}
	if done {
		return doneJob(p.card), nil
	}
	card := p.card
	var resume *runnerapi.Resume
	attempt, err := board.UnpublishedCommit(ctx, db, card.ID)
	if err != nil {
		return runnerapi.Job{}, err
	}
	if attempt.ID != "" {
		git := ProjectGit(deps.Data, p.project, p.owner.Author, deps.Tools)
		if attempt.Outcome == "checks_passed" {
			if head, err := git.RevParse(ctx, "refs/heads/orch/"+card.ID); err == nil && head == attempt.CommitSHA {
				card, err = move(ctx, db, card, "self_check", machine.Evidence{AttemptID: attempt.ID}, p.redact)
				if err != nil {
					return runnerapi.Job{}, err
				}
				card, err = settleCheck(ctx, db, deps, git, p.project, card, attempt, attempt.ModelID, p.redact, 0, "", "", nil)
				return doneJob(card), err
			}
		}
		resume = &runnerapi.Resume{AttemptID: attempt.ID, SHA: attempt.CommitSHA}
	}
	planning, err := PlanningPhase(ctx, db, card)
	if err != nil {
		return runnerapi.Job{}, err
	}
	// Planning uses the harness tool loop and the plan route (RFC-128, RFC-133).
	if planning {
		cfg, secrets, err := settings.Load(deps.Data)
		if err != nil {
			return runnerapi.Job{}, err
		}
		model, fallback, provider, ok := planRouteProvider(cfg, secrets, deps.Provider)
		if !ok {
			_ = jobError(ctx, db, card, "", "planning needs a plan route model; fix it on the Router page, then unblock the card", p.redact)
			card, err = BlockCard(ctx, db, card, "router_config", p.redact)
			return doneJob(card), err
		}
		p.class, p.model, p.fallback, p.provider = "plan", model, fallback, provider
	}
	if err := board.SetLeaseRoute(ctx, db, lease.ID, p.model, p.fallback, p.class); err != nil {
		return runnerapi.Job{}, err
	}
	jobs.put(lease.ID, &remoteJob{p: p})
	budgets := p.cfg.Router.Budgets
	tokenCap := budgets.MaxTokensPerCard
	if tokenCap == 0 {
		tokenCap = settings.DefaultTokensPerCard
	}
	base := p.project.BaseBranch
	if base == "" {
		base = "main"
	}
	scope := p.mcpScope
	engine := p.projectCfg.Sandbox.Engine
	if engine == "" {
		engine = "podman"
	}
	rounds, err := board.QuestionRounds(ctx, db, card.ID)
	if err != nil {
		return runnerapi.Job{}, err
	}
	// Planning uses the built-in tool loop (RFC-128). Omit Claude Code settings
	// and agent so the runner does not require a Claude Code binary for plan.
	var claude *runnerapi.ClaudeCodeJob
	agent := p.cfg.AgentKind()
	if planning {
		agent = settings.AgentBuiltin
	} else if cc := p.claude; cc != nil {
		claude = &runnerapi.ClaudeCodeJob{
			BaseURL: cc.BaseURL, APIKey: cc.APIKey, Auth: cc.Auth, Model: p.model, SmallModel: cc.SmallModel,
			MaxTurns: p.maxTurns, AllowWeb: cc.AllowWeb, Instructions: cc.Instructions,
		}
	}
	cardBundle := false
	if card.PRURL == "" {
		_, cardBundle = controlBranchAhead(ctx, ProjectGit(deps.Data, p.project, p.owner.Author, deps.Tools), card.ID, base)
	}
	return runnerapi.Job{
		CardBundle:   cardBundle,
		QuestionMode: card.QuestionMode, QuestionRounds: rounds, Planning: planning,
		CardID: card.ID, ProjectID: card.ProjectID, Title: card.Title, Status: card.Status,
		BaseBranch: base, CloneURL: p.acct.CloneURL(p.project.Repo), CloneToken: p.companyToken,
		Author:          runnerapi.Author{Name: p.owner.Author.Name, Email: p.owner.Author.Email},
		PublishedBranch: card.PRURL != "", Fixture: p.fixture,
		Model: p.model, TaskClass: p.class, MaxTurns: p.maxTurns,
		Agent: agent, ClaudeCode: claude,
		HistoryBudget:    llmHistory(p.cfg, p.model),
		ContextTokens:    llm.Options(p.cfg, p.model).ContextWindow(),
		MaxTokensPerCard: tokenCap, WallMinutes: budgets.MaxWallMinutesPerAttempt,
		LocalCheck: card.LocalCheck, AllowExecutables: p.projectCfg.Checks.AllowExecutables,
		CommandExecutables: p.projectCfg.Checks.CommandExecutables,
		SandboxEngine:      engine, SandboxImage: p.projectCfg.Sandbox.Image,
		MCP: &runnerapi.MCP{
			Scope: scope.Scope, Offered: nonNil(scope.Offered), Missing: nonNil(scope.Missing),
			Attached: nonNil(scope.Attached), Detached: nonNil(scope.Detached), Reason: scope.Reason, Total: scope.Total,
		},
		Resume: resume,
	}, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func wireCard(c board.Card) *runnerapi.Card {
	return &runnerapi.Card{
		ID: c.ID, ProjectID: c.ProjectID, Title: c.Title, Status: c.Status, LocalCheck: c.LocalCheck,
		RequirementIDs: nonNil(c.RequirementIDs), AttemptsUsed: c.AttemptsUsed, MaxAttempts: c.MaxAttempts,
	}
}

// RemoteBoardOp runs one board call of a lease's attempt. Every op works on
// the lease card and its project whatever the body names; no op moves a card.
// Text a reply carries is redacted with the job's list, as a local job's
// prompt is.
func RemoteBoardOp(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease, op runnerapi.BoardOp) (runnerapi.BoardReply, error) {
	reply, err := remoteBoardOp(ctx, db, deps, jobs, lease, op)
	if err != nil {
		return reply, err
	}
	job, err := jobs.get(ctx, db, deps, lease)
	if err != nil {
		return runnerapi.BoardReply{}, err
	}
	redact := func(s string) string { return trace.Redact(s, job.p.redact) }
	if reply.Card != nil {
		reply.Card.Title = redact(reply.Card.Title)
	}
	for i := range reply.Lines {
		reply.Lines[i] = redact(reply.Lines[i])
	}
	reply.Text = redact(reply.Text)
	if reply.Handoff != nil {
		reply.Handoff.Summary = redact(reply.Handoff.Summary)
	}
	for i := range reply.Questions {
		q := &reply.Questions[i]
		q.Question, q.Why = redact(q.Question), redact(q.Why)
		if q.Answer != nil {
			q.Answer.Text = redact(q.Answer.Text)
		}
	}
	for i := range reply.Conventions {
		reply.Conventions[i].Body = redact(reply.Conventions[i].Body)
		reply.Conventions[i].Title = redact(reply.Conventions[i].Title)
	}
	return reply, nil
}

func remoteBoardOp(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease, op runnerapi.BoardOp) (runnerapi.BoardReply, error) {
	var reply runnerapi.BoardReply
	card, err := board.GetCard(ctx, db, lease.CardID)
	if err != nil {
		return reply, err
	}
	dbBoard := loop.DBBoard{DB: db}
	needAttempt := func() error {
		if lease.AttemptID == "" {
			return fmt.Errorf("%w: the lease has no attempt", ErrRunnerOp)
		}
		return nil
	}
	switch op.Op {
	case "card":
		reply.Card = wireCard(card)
	case "lock":
		err = board.AcquireLock(ctx, db, card.ID, lease.ID)
	case "unlock":
		err = board.ReleaseLock(ctx, db, card.ID, lease.ID)
	case "next_attempt":
		reply.N, reply.ParentID, err = board.NextAttempt(ctx, db, card.ID)
	case "insert_attempt":
		a := op.Attempt
		if a == nil || !strings.HasPrefix(a.ID, "att_") || lease.AttemptID != "" {
			return reply, fmt.Errorf("%w: insert_attempt needs a new att_ id once per lease", ErrRunnerOp)
		}
		if err = board.InsertAttempt(ctx, db, board.Attempt{
			ID: a.ID, CardID: card.ID, N: a.N, ParentID: a.ParentID, TaskClass: lease.TaskClass, ModelID: lease.ModelID,
			PromptHash: a.PromptHash, SystemHash: a.SystemHash, BaseSHA: a.BaseSHA, MaxTurns: a.MaxTurns,
		}); err == nil {
			err = board.SetLeaseAttempt(ctx, db, lease.ID, a.ID)
		}
	case "finish_attempt":
		if err := needAttempt(); err != nil {
			return reply, err
		}
		if !slices.Contains([]string{"", "error", "no_change", "budget", "no_commit", "question", "plan"}, op.Outcome) {
			return reply, fmt.Errorf("%w: outcome %q", ErrRunnerOp, op.Outcome)
		}
		err = board.FinishAttempt(ctx, db, lease.AttemptID, op.Outcome, op.CommitSHA)
	case "event":
		if !remoteEventKinds[op.Kind] {
			return reply, fmt.Errorf("%w: event kind %q", ErrRunnerOp, op.Kind)
		}
		job, err := jobs.get(ctx, db, deps, lease)
		if err != nil {
			return reply, err
		}
		_, err = trace.Append(ctx, db, trace.Event{
			CardID: card.ID, AttemptID: lease.AttemptID, ProjectID: card.ProjectID, Kind: op.Kind, Payload: op.Payload,
		}, job.p.redact)
		return reply, err
	case "add_tokens":
		if err := needAttempt(); err != nil {
			return reply, err
		}
		if op.Input < 0 || op.Output < 0 || op.Cached < 0 {
			return reply, fmt.Errorf("%w: negative tokens", ErrRunnerOp)
		}
		err = board.AddAttemptTokens(ctx, db, lease.AttemptID, op.Input, op.Output, op.Cached)
	case "card_tokens":
		reply.Tokens, err = board.CardTokens(ctx, db, card.ID)
	case "failure_note":
		if err := needAttempt(); err != nil {
			return reply, err
		}
		if op.Source != "no_commit" {
			return reply, fmt.Errorf("%w: failure note source %q", ErrRunnerOp, op.Source)
		}
		job, err := jobs.get(ctx, db, deps, lease)
		if err != nil {
			return reply, err
		}
		err = board.InsertFailureNote(ctx, db, card.ID, lease.AttemptID, op.Source, trace.Redact(op.Note, job.p.redact))
		return reply, err
	case "add_attempt_used":
		err = board.AddAttemptUsed(ctx, db, card.ID)
	case "requirements":
		reply.Lines, err = dbBoard.Requirements(ctx, card.ProjectID, card.RequirementIDs)
	case "decisions":
		reply.Lines, err = dbBoard.Decisions(ctx, card.ProjectID, card.RequirementIDs)
	case "latest_note":
		reply.Text, err = dbBoard.LatestNote(ctx, card.ID)
	case "latest_review_note":
		reply.Text, err = dbBoard.LatestReviewNote(ctx, card.ID)
	case "instructions":
		reply.Text, err = dbBoard.Instructions(ctx, card.ID)
	case "answers":
		reply.Questions, err = dbBoard.Answers(ctx, card.ID)
	case "ask_questions":
		if err := needAttempt(); err != nil {
			return reply, err
		}
		questions, err := remoteQuestions(op.Questions)
		if err != nil || op.First < 0 {
			return reply, fmt.Errorf("%w: ask_questions: %v", ErrRunnerOp, err)
		}
		job, err := jobs.get(ctx, db, deps, lease)
		if err != nil {
			return reply, err
		}
		redact := func(s string) string { return trace.Redact(s, job.p.redact) }
		for i := range questions {
			q := &questions[i]
			q.Question, q.Why, q.Recommended.Text = redact(q.Question), redact(q.Why), redact(q.Recommended.Text)
			for j := range q.Options {
				q.Options[j] = redact(q.Options[j])
			}
			for j := range q.Recommended.Selected {
				q.Recommended.Selected[j] = redact(q.Recommended.Selected[j])
			}
		}
		err = board.InsertQuestions(ctx, db, card.ID, lease.AttemptID, op.First, questions, op.Auto)
		return reply, err
	case "submit_plan":
		if err := needAttempt(); err != nil {
			return reply, err
		}
		title, steps, err := loop.NormalizePlan(op.PlanTitle, op.PlanSteps)
		if err != nil {
			return reply, fmt.Errorf("%w: submit_plan: %v", ErrRunnerOp, err)
		}
		job, err := jobs.get(ctx, db, deps, lease)
		if err != nil {
			return reply, err
		}
		title = trace.Redact(title, job.p.redact)
		for i := range steps {
			steps[i].Text = trace.Redact(steps[i].Text, job.p.redact)
			steps[i].Detail = trace.Redact(steps[i].Detail, job.p.redact)
		}
		_, err = board.InsertOpenPlan(ctx, db, card.ID, lease.AttemptID, title, steps)
		return reply, err
	case "approved_plan":
		plan, err := board.AcceptedPlan(ctx, db, card.ID)
		if errors.Is(err, board.ErrNotFound) {
			return reply, nil
		}
		if err != nil {
			return reply, err
		}
		reply.Plan = &plan
		return reply, nil
	case "save_handoff":
		if err := needAttempt(); err != nil {
			return reply, err
		}
		h, err := remoteHandoff(op.Handoff)
		if err != nil {
			return reply, fmt.Errorf("%w: save_handoff: %v", ErrRunnerOp, err)
		}
		job, err := jobs.get(ctx, db, deps, lease)
		if err != nil {
			return reply, err
		}
		h.CardID, h.AttemptID = card.ID, lease.AttemptID
		h.Summary = trace.Redact(h.Summary, job.p.redact)
		for i := range h.Todos {
			h.Todos[i].Text = trace.Redact(h.Todos[i].Text, job.p.redact)
		}
		_, err = board.SaveHandoff(ctx, db, h)
		return reply, err
	case "pending_handoff":
		h, ok, perr := board.PendingHandoff(ctx, db, card.ID)
		if ok {
			reply.Handoff = &h
		}
		err = perr
	case "handoff_applied":
		job, err := jobs.get(ctx, db, deps, lease)
		if err != nil {
			return reply, err
		}
		err = board.MarkHandoffApplied(ctx, db, card.ID, op.HandoffID, op.OK, trace.Clip(op.Reason, job.p.redact, trace.ArgCap))
		return reply, err
	case "conventions":
		var convs []runnerapi.Convention
		items, refs, cerr := dbBoard.Conventions(ctx, card.ProjectID, card.ID)
		for _, c := range items {
			convs = append(convs, runnerapi.Convention{
				Kind: c.Kind, ID: c.ID, Version: c.Version, Hash: c.Hash, Title: c.Title, Label: c.Label,
				Category: c.Category, Slug: c.Slug, UseWhen: c.UseWhen, Body: c.Body,
			})
		}
		reply.Conventions, reply.Items, err = convs, refs, cerr
	default:
		return reply, fmt.Errorf("%w: op %q", ErrRunnerOp, op.Op)
	}
	return reply, err
}

// remoteHandoff checks a runner's handoff with the limits a local attempt
// keeps: a known source, the summary cap, the patch cap, and the todo rules.
func remoteHandoff(h *board.Handoff) (board.Handoff, error) {
	if h == nil {
		return board.Handoff{}, errors.New("no handoff")
	}
	out := *h
	switch {
	case out.SummarySource != "model" && out.SummarySource != "harness":
		return out, fmt.Errorf("summary source %q", out.SummarySource)
	case len(out.Summary) > loop.MaxHandoffSummary:
		return out, errors.New("the summary is over 8 KiB")
	case len(out.Patch) > runtime.MaxPatchBytes:
		return out, errors.New("the patch is over 2 MiB")
	case out.PatchBaseSHA != "" && (len(out.PatchBaseSHA) < 40 || strings.Trim(out.PatchBaseSHA, "0123456789abcdef") != ""):
		return out, errors.New("the patch base is not a commit id")
	case out.PatchFiles < 0 || len(out.Todos) > loop.MaxTodos:
		return out, errors.New("bad file or todo count")
	}
	for _, todo := range out.Todos {
		if len(todo.Text) > loop.MaxTodoText || !slices.Contains([]string{"pending", "in_progress", "done"}, todo.Status) {
			return out, errors.New("a todo item is not valid")
		}
	}
	out.Todos = slices.Clone(out.Todos)
	return out, nil
}

// remoteQuestions checks a runner's questions with the rules ask_user
// applies, so a runner cannot store what a local attempt could not.
func remoteQuestions(questions []board.Question) ([]board.Question, error) {
	raw, err := json.Marshal(questions)
	if err != nil {
		return nil, err
	}
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return loop.ParseQuestions(map[string]any{"questions": items})
}

// RemoteModel runs one model call of a lease's attempt with the lease's
// routed model, whatever model the request names, and publishes its turn to
// the card's live stream. A card over its token cap gets no call.
func RemoteModel(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease, call runnerapi.ModelCall) (providers.Response, error) {
	if lease.AttemptID == "" || call.AttemptID != lease.AttemptID {
		return providers.Response{}, fmt.Errorf("%w: model call for another attempt", ErrRunnerOp)
	}
	job, err := jobs.get(ctx, db, deps, lease)
	if err != nil {
		return providers.Response{}, err
	}
	tokenCap := job.p.cfg.Router.Budgets.MaxTokensPerCard
	if tokenCap == 0 {
		tokenCap = settings.DefaultTokensPerCard
	}
	used, err := board.CardTokens(ctx, db, lease.CardID)
	if err != nil {
		return providers.Response{}, err
	}
	if used >= tokenCap {
		return providers.Response{}, fmt.Errorf("%w: the card has used its token cap", ErrRunnerOp)
	}
	if job.p.provider == nil {
		return providers.Response{}, fmt.Errorf("%w: the lease has no model provider", ErrRunnerOp)
	}
	req := call.Request
	req.Model, req.TaskClass = lease.ModelID, lease.TaskClass
	req.System, req.User = trace.Redact(req.System, job.p.redact), trace.Redact(req.User, job.p.redact)
	resp, err := loop.CompleteLive(ctx, job.p.provider, req, deps.Live, lease.CardID,
		live.Frame{AttemptID: lease.AttemptID, Turn: call.Turn, Role: "implementer"}, job.p.redact)
	if providers.IsContextFull(err) {
		return providers.Response{}, fmt.Errorf("%w: %v", providers.ErrContextFull, err)
	}
	if err != nil {
		card, cerr := board.GetCard(ctx, db, lease.CardID)
		if cerr == nil {
			_ = jobError(ctx, db, card, lease.AttemptID, err.Error(), job.p.redact)
		}
		if !errors.Is(err, ErrProvider) {
			err = fmt.Errorf("%w: %v", ErrProvider, err)
		}
		return providers.Response{}, err
	}
	return resp, nil
}

func (j *remoteJob) tools(ctx context.Context, db *sql.DB, deps JobDeps, lease board.Lease) (*MCPToolset, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.opened {
		return j.toolset, nil
	}
	dialer := deps.MCP
	if dialer.Stdio == nil {
		dialer.Stdio = StdioOpener(j.p.engine, j.p.fixture, lease.CardID, deps.Tools)
	}
	set, err := OpenMCPToolset(ctx, db, dialer, j.p.mcpConns, MCPScope{ProjectID: lease.ProjectID, CardID: lease.CardID}, j.p.redact)
	if err != nil {
		return nil, err
	}
	j.toolset, j.opened = set, true
	return set, nil
}

// RemoteMCPTools opens the lease's MCP servers on the control plane, once per
// lease, and lists the tools they offer (RFC-108). The runner gets tool
// names and schemas, never a server URL or secret.
func RemoteMCPTools(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease) (runnerapi.MCPTools, error) {
	out := runnerapi.MCPTools{Tools: []providers.Tool{}, Servers: []map[string]any{}}
	job, err := jobs.get(ctx, db, deps, lease)
	if err != nil {
		return out, err
	}
	set, err := job.tools(ctx, db, deps, lease)
	if err != nil || set == nil {
		return out, err
	}
	out.Tools = set.Tools()
	for _, server := range set.Describe() {
		desc := map[string]any{}
		for _, key := range []string{"id", "slug", "transport", "tools", "error"} {
			if v, ok := server[key]; ok {
				desc[key] = v
			}
		}
		out.Servers = append(out.Servers, desc)
	}
	return out, nil
}

// RemoteMCPCall calls one MCP tool for a lease and redacts its result.
func RemoteMCPCall(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease, name string, args map[string]any) (mcp.Result, error) {
	job, err := jobs.get(ctx, db, deps, lease)
	if err != nil {
		return mcp.Result{}, err
	}
	set, err := job.tools(ctx, db, deps, lease)
	if err != nil {
		return mcp.Result{}, err
	}
	if set == nil {
		return mcp.Result{}, fmt.Errorf("%w: no MCP tools for this lease", ErrRunnerOp)
	}
	callCtx, cancel := context.WithTimeout(ctx, loop.MCPCallTimeout)
	defer cancel()
	res, err := set.Call(callCtx, name, args)
	if err != nil {
		return mcp.Result{Text: trace.Redact(MCPFailure(err), job.p.redact), IsError: true}, nil
	}
	res.Text = trace.Redact(res.Text, job.p.redact)
	return res, nil
}

// ImportBundle takes a runner's bundle of orch/<card> at sha into the
// project clone. sha must be the lease attempt's commit and descend from its
// base.
func ImportBundle(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease, sha string, body io.Reader) error {
	if lease.AttemptID == "" {
		return fmt.Errorf("%w: the lease has no attempt", ErrBundle)
	}
	var commit, base string
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(commit_sha, ''), COALESCE(base_sha, '') FROM attempts WHERE id = $1`, lease.AttemptID).Scan(&commit, &base); err != nil {
		return err
	}
	if commit == "" || sha != commit {
		return fmt.Errorf("%w: sha is not the attempt commit", ErrBundle)
	}
	job, err := jobs.get(ctx, db, deps, lease)
	if err != nil {
		return err
	}
	p := job.p
	git := ProjectGit(deps.Data, p.project, p.owner.Author, deps.Tools)
	if _, err := git.EnsureCheckout(ctx, p.project.BaseBranch, runtime.Remote{URL: p.acct.CloneURL(p.project.Repo), Token: p.companyToken}); err != nil {
		return errors.New(trace.Redact(err.Error(), p.redact))
	}
	dir := filepath.Join(git.Data, "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, "bundle-*.bundle")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := io.Copy(file, body); err != nil {
		file.Close()
		return fmt.Errorf("%w: %v", ErrBundle, err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := git.ImportCardBundle(ctx, lease.CardID, file.Name(), sha, base); err != nil {
		if errors.Is(err, runtime.ErrBundle) {
			return fmt.Errorf("%w: %s", ErrBundle, trace.Redact(err.Error(), p.redact))
		}
		return err
	}
	return nil
}

// controlBranchAhead returns the head of the control plane's orch/<card>
// when that branch has commits its base does not (RFC-119).
func controlBranchAhead(ctx context.Context, git runtime.Git, cardID, base string) (string, bool) {
	head, err := git.RevParse(ctx, "refs/heads/orch/"+cardID)
	if err != nil || git.IsAncestor(ctx, head, "refs/heads/"+base) {
		return "", false
	}
	return head, true
}

// CardBundle writes a git bundle of the control plane's orch/<card> for a
// runner that starts the card without a pull request (RFC-119), and returns
// its head and the bundle file, which the caller removes. A branch that is
// not ahead of its base is board.ErrNotFound.
func CardBundle(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease) (string, string, error) {
	job, err := jobs.get(ctx, db, deps, lease)
	if err != nil {
		return "", "", err
	}
	p := job.p
	base := p.project.BaseBranch
	if base == "" {
		base = "main"
	}
	git := ProjectGit(deps.Data, p.project, p.owner.Author, deps.Tools)
	head, ok := controlBranchAhead(ctx, git, lease.CardID, base)
	if !ok {
		return "", "", board.ErrNotFound
	}
	dir := filepath.Join(git.Data, "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	file, err := os.CreateTemp(dir, "card-*.bundle")
	if err != nil {
		return "", "", err
	}
	path := file.Name()
	file.Close()
	os.Remove(path)
	if err := git.BundleCard(ctx, lease.CardID, base, path); err != nil {
		os.Remove(path)
		return "", "", errors.New(trace.Redact(err.Error(), p.redact))
	}
	return head, path, nil
}

// ReportRemote applies one fact a runner reports, as the local implement job
// does at the same step; every status change goes through machine.Transition.
func ReportRemote(ctx context.Context, db *sql.DB, deps JobDeps, jobs *RemoteJobs, lease board.Lease, rep runnerapi.Report) (runnerapi.ReportReply, error) {
	job, err := jobs.get(ctx, db, deps, lease)
	if err != nil {
		return runnerapi.ReportReply{}, err
	}
	p := job.p
	card, err := board.GetCard(ctx, db, lease.CardID)
	if err != nil {
		return runnerapi.ReportReply{}, err
	}
	done := func(card board.Card, err error) (runnerapi.ReportReply, error) {
		return runnerapi.ReportReply{Status: card.Status, Next: "done"}, err
	}
	toCheck := func(attemptID string) (runnerapi.ReportReply, error) {
		var sha string
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(commit_sha, '') FROM attempts WHERE id = $1 AND card_id = $2`,
			attemptID, card.ID).Scan(&sha); err != nil {
			return runnerapi.ReportReply{}, err
		}
		if sha == "" {
			return runnerapi.ReportReply{}, fmt.Errorf("%w: the attempt has no commit", ErrRunnerOp)
		}
		next, err := move(ctx, db, card, "self_check", machine.Evidence{AttemptID: attemptID}, p.redact)
		if err != nil {
			return runnerapi.ReportReply{}, err
		}
		return runnerapi.ReportReply{Status: next.Status, Next: "check", AttemptID: attemptID, SHA: sha}, nil
	}
	switch rep.Fact {
	case runnerapi.FactBlocked:
		_ = jobError(ctx, db, card, lease.AttemptID, rep.Message, p.redact)
		reason := "sandbox_config"
		if rep.Reason == "agent_config" {
			reason = rep.Reason
		}
		return done(BlockCard(ctx, db, card, reason, p.redact))
	case runnerapi.FactFinished:
		switch rep.Outcome {
		case "error":
			switch rep.Cause {
			case "wall_cap":
				_ = jobError(ctx, db, card, lease.AttemptID, rep.Message, p.redact)
				if err := board.AddAttemptUsed(ctx, db, card.ID); err != nil {
					return runnerapi.ReportReply{}, err
				}
				return done(BlockCard(ctx, db, card, "wall_cap", p.redact))
			case "provider_error":
				return done(BlockCard(ctx, db, card, "provider_error", p.redact))
			default:
				_ = jobError(ctx, db, card, lease.AttemptID, rep.Message, p.redact)
				return done(card, nil)
			}
		case "no_commit":
			if err := needLeaseAttempt(lease); err != nil {
				return runnerapi.ReportReply{}, err
			}
			return done(retryOrCap(ctx, db, card.ID))
		case "budget":
			if err := needLeaseAttempt(lease); err != nil {
				return runnerapi.ReportReply{}, err
			}
			if rep.Reason != "turn_cap" && rep.Reason != "token_cap" && rep.Reason != "context_full" {
				return runnerapi.ReportReply{}, fmt.Errorf("%w: budget reason %q", ErrRunnerOp, rep.Reason)
			}
			return done(BlockCard(ctx, db, card, rep.Reason, p.redact))
		case "question", "plan":
			if err := needLeaseAttempt(lease); err != nil {
				return runnerapi.ReportReply{}, err
			}
			return done(move(ctx, db, card, "waiting_answer", machine.Evidence{AttemptID: lease.AttemptID}, p.redact))
		case "", "no_change":
			if err := needLeaseAttempt(lease); err != nil {
				return runnerapi.ReportReply{}, err
			}
			return toCheck(lease.AttemptID)
		}
		return runnerapi.ReportReply{}, fmt.Errorf("%w: outcome %q", ErrRunnerOp, rep.Outcome)
	case runnerapi.FactResume:
		attempt, err := board.UnpublishedCommit(ctx, db, card.ID)
		if err != nil {
			return runnerapi.ReportReply{}, err
		}
		if attempt.ID == "" || attempt.ID != rep.AttemptID || (lease.AttemptID != "" && lease.AttemptID != attempt.ID) {
			return runnerapi.ReportReply{}, fmt.Errorf("%w: attempt %q cannot resume", ErrRunnerOp, rep.AttemptID)
		}
		if err := board.SetLeaseAttempt(ctx, db, lease.ID, attempt.ID); err != nil {
			return runnerapi.ReportReply{}, err
		}
		return toCheck(attempt.ID)
	case runnerapi.FactCheck:
		if err := needLeaseAttempt(lease); err != nil {
			return runnerapi.ReportReply{}, err
		}
		if card.Status != "self_check" {
			return runnerapi.ReportReply{}, fmt.Errorf("%w: the card is %s, not self_check", ErrRunnerOp, card.Status)
		}
		var attempt board.Attempt
		var outcome sql.NullString
		if err := db.QueryRowContext(ctx, `
			SELECT id, COALESCE(commit_sha, ''), outcome, COALESCE(model_id, '') FROM attempts WHERE id = $1`,
			lease.AttemptID).Scan(&attempt.ID, &attempt.CommitSHA, &outcome, &attempt.ModelID); err != nil {
			return runnerapi.ReportReply{}, err
		}
		attempt.Outcome = outcome.String
		git := ProjectGit(deps.Data, p.project, p.owner.Author, deps.Tools)
		var checkErr error
		if rep.Error != "" {
			if rep.Error != "sandbox_config" && rep.Error != "check_error" {
				return runnerapi.ReportReply{}, fmt.Errorf("%w: check error %q", ErrRunnerOp, rep.Error)
			}
			checkErr = errors.New(rep.Message)
		} else if rep.Exit == 0 {
			head, err := git.RevParse(ctx, "refs/heads/orch/"+card.ID)
			if err != nil || head != attempt.CommitSHA {
				return runnerapi.ReportReply{}, fmt.Errorf("%w: upload the bundle of the attempt commit before a passing check", ErrBundle)
			}
		}
		model := attempt.ModelID
		if model == "" {
			model = lease.ModelID
		}
		return done(settleCheck(ctx, db, deps, git, p.project, card, attempt, model, p.redact, rep.Exit, rep.Tail, rep.Error, checkErr))
	}
	return runnerapi.ReportReply{}, fmt.Errorf("%w: fact %q", ErrRunnerOp, rep.Fact)
}

func needLeaseAttempt(lease board.Lease) error {
	if lease.AttemptID == "" {
		return fmt.Errorf("%w: the lease has no attempt", ErrRunnerOp)
	}
	return nil
}

// EndRemote ends a lease with outcome done and forgets its job state.
func EndRemote(ctx context.Context, db *sql.DB, jobs *RemoteJobs, lease board.Lease) error {
	jobs.Drop(lease.ID)
	return board.EndLease(ctx, db, lease.ID)
}

// ReleaseRemote gives a lease back as the reaper would, for a runner that
// stops a job it cannot finish: the open attempt is cancelled and the run
// request opens again.
func ReleaseRemote(ctx context.Context, db *sql.DB, jobs *RemoteJobs, lease board.Lease) error {
	jobs.Drop(lease.ID)
	_, err := board.ReapLeases(ctx, db, board.ReapFilter{RunnerID: lease.RunnerID, LeaseID: lease.ID}, "lost")
	return err
}

// ReapRemote ends leases whose runner stopped heartbeating and forgets their
// job state (RFC-107). jobs may be nil.
func ReapRemote(ctx context.Context, db *sql.DB, jobs *RemoteJobs) (int, error) {
	ended, err := board.ReapLeases(ctx, db, board.ReapFilter{Expired: true}, "lost")
	if jobs != nil {
		for _, id := range ended {
			jobs.Drop(id)
		}
	}
	return len(ended), err
}

func llmHistory(cfg settings.Config, model string) int {
	return llm.Options(cfg, model).HistoryBytes()
}
