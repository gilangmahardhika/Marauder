package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/llm"
	"orchestrator/backend/internal/router"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/sandbox"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/trace"
)

// ErrNotStartable means the card is not in a status the implement job starts from,
// or another attempt of the card is still open.
var ErrNotStartable = errors.New("card cannot start an implement job")

// ErrProvider wraps a model failure after the router's retry and fallback.
var ErrProvider = errors.New("provider_error")

const (
	checkTimeout = 10 * time.Minute
	checkMaxOut  = 1 << 20
)

func RunImplementer(ctx context.Context, in loop.Input) (loop.Result, error) {
	return loop.Run(ctx, in)
}

func CreateCard(ctx context.Context, in loop.CreateInput) (board.Card, error) {
	return loop.CreateCard(ctx, in)
}

// JobDeps is what an implement job needs besides the database. Provider
// replaces the routed model clients in tests.
type JobDeps struct {
	Data     string
	Key      []byte
	Tools    Tools
	Provider providers.Provider
	// Secrets are service secrets every job redacts besides its own keys and
	// tokens: the databaseUrl and tokenKey text.
	Secrets []string
	// Now is the clock of the review job's check wait; nil means time.Now.
	Now func() time.Time
	// Live, when set, gets the reply text of each model call as it is written.
	Live live.Publisher
	// MCP opens the tribe's MCP servers (RFC-093).
	MCP MCPDialer
}

func (d JobDeps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// ImplementCard runs one implement job (RFC-063): preflight, one attempt of the
// tool loop, then the gating check on an export of the new commit, and the
// pull request when it exits 0. Every status change goes through
// machine.Transition. A preflight failure blocks the card with its reason and
// returns the card with a nil error.
// preparedJob is what the implement preflight resolved for one card.
type preparedJob struct {
	card         board.Card
	project      tenancy.Project
	cfg          settings.Config
	projectCfg   settings.ProjectConfig
	redact       []string
	acct         company.Account
	companyToken string
	owner        OwnerIdentity
	mcpConns     []MCPConn
	mcpScope     loop.MCPContext
	fixture      bool
	engine       string
	class        string
	model        string
	fallback     string
	provider     providers.Provider
}

// implementPreflight checks and resolves everything an implement attempt
// needs, and moves a ready card to in_progress. done is true when it blocked
// the card or the card is at its attempt cap; the card is then final for this
// job. A remote job skips the engine probe, which its runner makes.
func implementPreflight(ctx context.Context, db *sql.DB, deps JobDeps, projectID, cardID string, remote bool) (preparedJob, bool, error) {
	var p preparedJob
	card, err := board.GetCard(ctx, db, cardID)
	p.card = card
	if err != nil {
		return p, false, err
	}
	if card.ProjectID != projectID {
		return p, false, board.ErrNotFound
	}
	open, err := board.OpenAttempt(ctx, db, card.ID)
	if err != nil {
		return p, false, err
	}
	if open || !startable(card.Status) {
		return p, false, ErrNotStartable
	}
	done := func(card board.Card, err error) (preparedJob, bool, error) {
		p.card = card
		return p, true, err
	}
	if card.AttemptsUsed >= card.MaxAttempts {
		return done(AtCap(ctx, db, card.ID))
	}
	project, err := tenancy.GetProject(ctx, db, projectID)
	if err != nil {
		return p, false, err
	}
	cfg, secrets, err := settings.Load(deps.Data)
	if err != nil {
		return p, false, err
	}
	projectCfg, _, err := settings.LoadProject(deps.Data, projectID)
	if err != nil {
		return p, false, err
	}
	redact := append(modelKeys(secrets), deps.Secrets...)

	if card.Status == "ready" || card.Status == "changes_requested" {
		if card, err = move(ctx, db, card, "in_progress", machine.Evidence{}, redact); err != nil {
			p.card = card
			return p, false, err
		}
		p.card = card
	}

	acct, err := company.Load(ctx, db)
	if err != nil {
		return p, false, err
	}
	companyToken, err := company.Token(ctx, db, deps.Key)
	if err != nil {
		return p, false, err
	}
	if acct.Org == "" || acct.BotLogin == "" || companyToken == "" || project.Repo == "" || project.RepoPath == "" {
		return done(BlockCard(ctx, db, card, "github_config", redact))
	}
	redact = append(redact, companyToken)

	owner, err := OwnerGitHub(ctx, db, deps.Key, card, acct.Host)
	if errors.Is(err, ErrOwnerGitHub) {
		return done(BlockCard(ctx, db, card, "owner_github", redact))
	}
	if err != nil {
		return p, false, err
	}
	redact = append(redact, owner.Token)

	mcpConns, err := LoadProjectMCP(ctx, db, deps.Key, projectID)
	if err != nil {
		return p, false, err
	}
	redact = append(redact, MCPSecrets(mcpConns)...)
	mcpConns, mcpScope, err := MCPScopeFor(ctx, db, projectID, card.ID, card.RequirementIDs, mcpConns)
	if err != nil {
		return p, false, err
	}

	fixture := runtime.FixtureMode(project.RepoPath)
	if card.LocalCheck == "" && projectCfg.Checks.Local != "" {
		if err := board.SetLocalCheck(ctx, db, card.ID, projectCfg.Checks.Local); err != nil {
			return p, false, err
		}
		card.LocalCheck = projectCfg.Checks.Local
		p.card = card
	}
	engine := deps.Tools.Engine(projectCfg.Sandbox.Engine)
	if !sandboxReady(card.LocalCheck, projectCfg, engine, fixture, remote) {
		return done(BlockCard(ctx, db, card, "sandbox_config", redact))
	}
	if !fixture && !remote {
		run := deps.Tools.Exec
		if run == nil {
			run = execx.Command
		}
		if err := sandbox.ProbeEngine(ctx, engine, run); err != nil {
			_ = jobError(ctx, db, card, "", err.Error()+"; start the podman machine or the docker daemon, then unblock the card", redact)
			return done(BlockCard(ctx, db, card, "sandbox_config", redact))
		}
	}

	class, err := router.TaskClass(card.Risk, card.AttemptsUsed)
	if err != nil {
		return p, false, err
	}
	route := cfg.Router.Routes[class]
	provider := deps.Provider
	if provider == nil {
		if _, _, _, err := llm.ForModel(cfg, secrets, route.Model); route.Model == "" || err != nil {
			return done(BlockCard(ctx, db, card, "router_config", redact))
		}
		provider = RoutedProvider(cfg, secrets, route.Model, route.Fallback)
	} else if route.Model == "" {
		return done(BlockCard(ctx, db, card, "router_config", redact))
	}
	return preparedJob{
		card: card, project: project, cfg: cfg, projectCfg: projectCfg, redact: redact,
		acct: acct, companyToken: companyToken, owner: owner, mcpConns: mcpConns, mcpScope: mcpScope,
		fixture: fixture, engine: engine, class: class, model: route.Model, fallback: route.Fallback, provider: provider,
	}, false, nil
}

func ImplementCard(ctx context.Context, db *sql.DB, deps JobDeps, projectID, cardID string) (board.Card, error) {
	p, done, err := implementPreflight(ctx, db, deps, projectID, cardID, false)
	if done || err != nil {
		return p.card, err
	}
	card, project, cfg, projectCfg, redact := p.card, p.project, p.cfg, p.projectCfg, p.redact
	acct, companyToken, owner, mcpConns, mcpScope := p.acct, p.companyToken, p.owner, p.mcpConns, p.mcpScope
	fixture, engine, class, provider := p.fixture, p.engine, p.class, p.provider
	route := struct{ Model string }{Model: p.model}

	git := ProjectGit(deps.Data, project, owner.Author, deps.Tools)
	if _, err := git.EnsureCheckout(ctx, project.BaseBranch, runtime.Remote{URL: acct.CloneURL(project.Repo), Token: companyToken}); err != nil {
		return card, errors.New(trace.Redact(err.Error(), redact))
	}

	attempt, err := unpublishedCommit(ctx, db, git, card.ID, project.BaseBranch)
	if err != nil {
		return card, errors.New(trace.Redact(err.Error(), redact))
	}
	model := attempt.ModelID
	if attempt.ID == "" {
		budgets := cfg.Router.Budgets
		attemptCtx := ctx
		if budgets.MaxWallMinutesPerAttempt > 0 {
			var cancel context.CancelFunc
			attemptCtx, cancel = context.WithTimeout(ctx, time.Duration(budgets.MaxWallMinutesPerAttempt)*time.Minute)
			defer cancel()
		}
		tokenCap := budgets.MaxTokensPerCard
		if tokenCap == 0 {
			tokenCap = settings.DefaultTokensPerCard
		}
		dialer := deps.MCP
		if dialer.Stdio == nil {
			dialer.Stdio = StdioOpener(engine, fixture, card.ID, deps.Tools)
		}
		if !fixture && slices.ContainsFunc(mcpConns, func(c MCPConn) bool { return c.Transport == "stdio" }) {
			run := deps.Tools.Exec
			if run == nil {
				run = execx.Command
			}
			_ = sandbox.SweepMCP(ctx, engine, card.ID, run)
		}
		toolset, err := OpenMCPToolset(attemptCtx, db, dialer, mcpConns, MCPScope{ProjectID: projectID, CardID: card.ID}, redact)
		if err != nil {
			return card, err
		}
		defer toolset.Close()
		var extra loop.ExtraTools
		if toolset != nil {
			extra = toolset
		}
		result, err := loop.Run(attemptCtx, loop.Input{
			DB: db, Git: git, BaseBranch: project.BaseBranch, CardID: card.ID,
			Model: route.Model, TaskClass: class, MaxTurns: budgets.MaxModelTurnsPerAttempt,
			Provider: provider, Secrets: redact, Live: deps.Live, Allow: projectCfg.Checks.AllowExecutables,
			Commands: projectCfg.Checks.CommandExecutables,
			Engine:   engine, Image: projectCfg.Sandbox.Image,
			HistoryBudget: llm.Options(cfg, route.Model).HistoryBytes(), MaxTokensPerCard: tokenCap,
			Extra: extra, MCP: &mcpScope,
		})
		if err != nil {
			return stopAttempt(ctx, db, card.ID, err, attemptCtx, redact)
		}
		switch result.Outcome {
		case "no_commit":
			return retryOrCap(ctx, db, card.ID)
		case "budget":
			card, err = board.GetCard(ctx, db, card.ID)
			if err != nil {
				return card, err
			}
			return BlockCard(ctx, db, card, result.Reason, redact)
		}
		attempt.ID = result.AttemptID
		model = route.Model
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(commit_sha, '') FROM attempts WHERE id = $1`, attempt.ID).Scan(&attempt.CommitSHA); err != nil {
			return card, err
		}
	}

	card, err = board.GetCard(ctx, db, card.ID)
	if err != nil {
		return card, err
	}
	card, err = move(ctx, db, card, "self_check", machine.Evidence{AttemptID: attempt.ID}, redact)
	if err != nil {
		return card, err
	}
	exit, tail, err := gatingCheck(ctx, git, deps.Tools, card, projectCfg, engine, fixture, attempt.CommitSHA, attempt.ID)
	reason := ""
	if err != nil {
		reason = CheckErrorReason(err)
	}
	return settleCheck(ctx, db, deps, git, project, card, attempt, model, redact, exit, tail, reason, err)
}

// CheckErrorReason is the block reason of a gating check that could not run:
// sandbox_config when the command or the engine is misconfigured, else
// check_error.
func CheckErrorReason(err error) string {
	if errors.Is(err, sandbox.ErrParse) || errors.Is(err, sandbox.ErrAllow) || errors.Is(err, sandbox.ErrEmpty) ||
		errors.Is(err, sandbox.ErrImage) || errors.Is(err, sandbox.ErrEngine) {
		return "sandbox_config"
	}
	return "check_error"
}

// settleCheck applies the gating check of attempt to a card in self_check:
// a check that could not run blocks the card with reason, an engine exit of
// 125 or 127 blocks it with sandbox_config, another non-zero exit returns it
// to in_progress, and exit 0 opens the pull request from git's orch/<card>.
func settleCheck(ctx context.Context, db *sql.DB, deps JobDeps, git runtime.Git, project tenancy.Project, card board.Card, attempt board.Attempt, model string, redact []string, exit int, tail, reason string, checkErr error) (board.Card, error) {
	head := attempt.CommitSHA
	if reason != "" {
		message := reason
		if checkErr != nil {
			message = checkErr.Error()
		}
		_ = jobError(ctx, db, card, attempt.ID, message, redact)
		return BlockCard(ctx, db, card, reason, redact)
	}
	if exit == 125 || exit == 127 {
		// The engine, not the check, failed: the attempt is kept (RFC-085).
		_ = jobError(ctx, db, card, attempt.ID, fmt.Sprintf("sandbox engine exited %d before the check ran\n%s", exit, tail), redact)
		return BlockCard(ctx, db, card, "sandbox_config", redact)
	}
	if exit != 0 && attempt.Outcome != "checks_passed" {
		if err := board.AddAttemptUsed(ctx, db, card.ID); err != nil {
			return card, err
		}
	}
	if exit != 0 {
		if err := board.SetAttemptOutcome(ctx, db, attempt.ID, "checks_failed"); err != nil {
			return card, err
		}
		note := fmt.Sprintf("local check %q exited %d\n%s", card.LocalCheck, exit, tail)
		if err := board.InsertFailureNote(ctx, db, card.ID, attempt.ID, "local_check", trace.Redact(note, redact)); err != nil {
			return card, err
		}
		code := exit
		card, err := move(ctx, db, card, "in_progress", machine.Evidence{ExitCode: &code, AttemptID: attempt.ID}, redact)
		if err != nil {
			return card, err
		}
		return retryOrCap(ctx, db, card.ID)
	}
	if err := board.SetAttemptOutcome(ctx, db, attempt.ID, "checks_passed"); err != nil {
		return card, err
	}
	text, err := BuildPRText(ctx, db, git, card, project.BaseBranch, head, attempt.ID, model, redact)
	if err != nil {
		return card, err
	}
	published, err := Publish(ctx, db, card.ID, PublishInput{
		Data: deps.Data, Key: deps.Key, Tools: deps.Tools, ExitCode: 0, AttemptID: attempt.ID,
		Text: text, Secrets: redact,
	})
	if errors.Is(err, ErrOwnerGitHub) || errors.Is(err, ErrGitHubSetup) {
		return published, nil
	}
	if err != nil {
		_ = jobError(ctx, db, card, attempt.ID, err.Error(), redact)
		return BlockCard(ctx, db, card, "publish_failed", redact)
	}
	// A passing attempt counts once its pull request is out, so a failed
	// publish does not use up an attempt (RFC-085).
	if err := board.AddAttemptUsed(ctx, db, card.ID); err != nil {
		return published, err
	}
	return board.GetCard(ctx, db, card.ID)
}

// unpublishedCommit returns the card's latest attempt when its commit has no
// pull request yet and is still the worktree head, so a rerun checks and
// publishes that commit without a model call (RFC-065). Otherwise it returns
// a zero Attempt.
func unpublishedCommit(ctx context.Context, db *sql.DB, git runtime.Git, cardID, baseBranch string) (board.Attempt, error) {
	attempt, err := board.UnpublishedCommit(ctx, db, cardID)
	if err != nil || attempt.ID == "" {
		return board.Attempt{}, err
	}
	wt, err := git.EnsureWorktree(ctx, cardID, baseBranch)
	if err != nil {
		return board.Attempt{}, err
	}
	head, err := git.WorktreeHead(ctx, wt)
	if err != nil {
		return board.Attempt{}, err
	}
	if head != attempt.CommitSHA {
		return board.Attempt{}, nil
	}
	return attempt, nil
}

// stopAttempt handles a tool loop that ended with an error. The loop already
// ended the attempt with outcome error.
func stopAttempt(ctx context.Context, db *sql.DB, cardID string, cause error, attemptCtx context.Context, redact []string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return card, err
	}
	if ctx.Err() != nil {
		return card, ctx.Err()
	}
	_ = jobError(ctx, db, card, "", cause.Error(), redact)
	switch {
	case errors.Is(attemptCtx.Err(), context.DeadlineExceeded):
		if err := board.AddAttemptUsed(ctx, db, card.ID); err != nil {
			return card, err
		}
		return BlockCard(ctx, db, card, "wall_cap", redact)
	case errors.Is(cause, ErrProvider):
		return BlockCard(ctx, db, card, "provider_error", redact)
	default:
		return card, errors.New(trace.Redact(cause.Error(), redact))
	}
}

// CanStartImplement reports whether a Run request may start the card: it is
// ready, changes_requested, or in_progress, it has attempts left, and no
// attempt of it is open.
func CanStartImplement(ctx context.Context, db querier, card board.Card) (bool, error) {
	if !startable(card.Status) || card.AttemptsUsed >= card.MaxAttempts {
		return false, nil
	}
	open, err := board.OpenAttempt(ctx, db, card.ID)
	return !open, err
}

func startable(status string) bool {
	return status == "ready" || status == "changes_requested" || status == "in_progress"
}

// sandboxReady reports whether the card's check can run. A remote job's
// engine is on its runner, which probes it.
func sandboxReady(check string, cfg settings.ProjectConfig, engine string, fixture, remote bool) bool {
	if _, err := sandbox.ParseCheck(check, cfg.Checks.AllowExecutables); err != nil {
		return false
	}
	if fixture {
		return true
	}
	if !remote && sandbox.CheckEngine(engine) != nil {
		return false
	}
	return sandbox.PinnedImage(cfg.Sandbox.Image)
}

// gatingCheck runs the card's local check on an export of sha. A process that
// ran returns its exit code; an error means the check could not start.
func gatingCheck(ctx context.Context, git runtime.Git, tools Tools, card board.Card, cfg settings.ProjectConfig, engine string, fixture bool, sha, attemptID string) (int, string, error) {
	if sha == "" {
		return 0, "", errors.New("attempt has no commit")
	}
	dest := filepath.Join(git.Data, "exports", attemptID+"-gate")
	defer os.RemoveAll(dest)
	if err := git.Export(ctx, sha, dest); err != nil {
		return 0, "", err
	}
	run := tools.Exec
	if run == nil {
		run = execx.Command
	}
	res, executed, err := sandbox.ExecuteCheck(ctx, sandbox.ExecuteOpts{
		Command: card.LocalCheck, Allow: cfg.Checks.AllowExecutables, Fixture: fixture,
		Engine: engine, Image: cfg.Sandbox.Image, Dir: dest, Timeout: checkTimeout, MaxOut: checkMaxOut,
	}, run)
	if err != nil {
		return 0, "", err
	}
	if !executed {
		return 0, "", errors.New("check was not executed")
	}
	return res.Exit, tailLines(string(res.Out), 200), nil
}

// jobError appends a job_error event with the redacted message.
func jobError(ctx context.Context, db *sql.DB, card board.Card, attemptID, message string, redact []string) error {
	_, err := trace.Append(ctx, db, trace.Event{
		CardID: card.ID, AttemptID: attemptID, ProjectID: card.ProjectID, Kind: "job_error",
		Payload: map[string]any{"message": trace.Clip(message, redact, JobErrorCap)},
	}, redact)
	return err
}

// JobErrorCap is the most bytes of a job_error message that are stored. A
// longer message keeps its head and its tail, where git and providers put the
// cause.
const JobErrorCap = 8 << 10

func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
