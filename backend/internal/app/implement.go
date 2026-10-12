package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/execx"
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
}

// ImplementCard runs one implement job (RFC-063): preflight, one attempt of the
// tool loop, then the gating check on an export of the new commit, and the
// pull request when it exits 0. Every status change goes through
// machine.Transition. A preflight failure blocks the card with its reason and
// returns the card with a nil error.
func ImplementCard(ctx context.Context, db *sql.DB, deps JobDeps, projectID, cardID string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return card, err
	}
	if card.ProjectID != projectID {
		return card, board.ErrNotFound
	}
	open, err := board.OpenAttempt(ctx, db, card.ID)
	if err != nil {
		return card, err
	}
	if open || !startable(card.Status) {
		return card, ErrNotStartable
	}
	project, err := tenancy.GetProject(ctx, db, projectID)
	if err != nil {
		return card, err
	}
	cfg, secrets, err := settings.Load(deps.Data)
	if err != nil {
		return card, err
	}
	projectCfg, _, err := settings.LoadProject(deps.Data, projectID)
	if err != nil {
		return card, err
	}
	redact := modelKeys(secrets)

	if card.Status == "ready" || card.Status == "changes_requested" {
		if card, err = move(ctx, db, card, "in_progress", machine.Evidence{}, redact); err != nil {
			return card, err
		}
	}

	acct, err := company.Load(ctx, db)
	if err != nil {
		return card, err
	}
	companyToken, err := company.Token(ctx, db, deps.Key)
	if err != nil {
		return card, err
	}
	if acct.Org == "" || acct.BotLogin == "" || companyToken == "" || project.Repo == "" || project.RepoPath == "" {
		return BlockCard(ctx, db, card, "github_config", redact)
	}
	redact = append(redact, companyToken)

	owner, err := OwnerGitHub(ctx, db, deps.Key, card, acct.Host)
	if errors.Is(err, ErrOwnerGitHub) {
		return BlockCard(ctx, db, card, "owner_github", redact)
	}
	if err != nil {
		return card, err
	}
	redact = append(redact, owner.Token)

	fixture := runtime.FixtureMode(project.RepoPath)
	if card.LocalCheck == "" && projectCfg.Checks.Local != "" {
		if err := board.SetLocalCheck(ctx, db, card.ID, projectCfg.Checks.Local); err != nil {
			return card, err
		}
		card.LocalCheck = projectCfg.Checks.Local
	}
	engine := deps.Tools.Engine(projectCfg.Sandbox.Engine)
	if !sandboxReady(card.LocalCheck, projectCfg, engine, fixture) {
		return BlockCard(ctx, db, card, "sandbox_config", redact)
	}
	if !fixture {
		run := deps.Tools.Exec
		if run == nil {
			run = execx.Command
		}
		if err := sandbox.ProbeEngine(ctx, engine, run); err != nil {
			_ = jobError(ctx, db, card, "", err.Error()+"; start the podman machine or the docker daemon, then unblock the card", redact)
			return BlockCard(ctx, db, card, "sandbox_config", redact)
		}
	}

	class, err := router.TaskClass(card.Risk, card.AttemptsUsed)
	if err != nil {
		return card, err
	}
	route := cfg.Router.Routes[class]
	provider := deps.Provider
	if provider == nil {
		if _, _, _, err := llm.ForModel(cfg, secrets, route.Model); route.Model == "" || err != nil {
			return BlockCard(ctx, db, card, "router_config", redact)
		}
		provider = RoutedProvider(cfg, secrets, route.Model, route.Fallback)
	} else if route.Model == "" {
		return BlockCard(ctx, db, card, "router_config", redact)
	}

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
		result, err := loop.Run(attemptCtx, loop.Input{
			DB: db, Git: git, BaseBranch: project.BaseBranch, CardID: card.ID,
			Model: route.Model, TaskClass: class, MaxTurns: budgets.MaxModelTurnsPerAttempt,
			Provider: provider, Secrets: redact, Allow: projectCfg.Checks.AllowExecutables,
			Engine: engine, Image: projectCfg.Sandbox.Image,
		})
		if err != nil {
			return stopAttempt(ctx, db, card.ID, err, attemptCtx, redact)
		}
		if result.Outcome == "no_commit" {
			return AtCap(ctx, db, card.ID)
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
	head := attempt.CommitSHA
	exit, tail, err := gatingCheck(ctx, git, deps.Tools, card, projectCfg, engine, fixture, head, attempt.ID)
	if err != nil {
		reason := "check_error"
		if errors.Is(err, sandbox.ErrParse) || errors.Is(err, sandbox.ErrAllow) || errors.Is(err, sandbox.ErrEmpty) ||
			errors.Is(err, sandbox.ErrImage) || errors.Is(err, sandbox.ErrEngine) {
			reason = "sandbox_config"
		}
		_ = jobError(ctx, db, card, attempt.ID, err.Error(), redact)
		return BlockCard(ctx, db, card, reason, redact)
	}
	if attempt.Outcome != "checks_passed" {
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
		card, err = move(ctx, db, card, "in_progress", machine.Evidence{ExitCode: &code, AttemptID: attempt.ID}, redact)
		if err != nil {
			return card, err
		}
		return AtCap(ctx, db, card.ID)
	}
	if err := board.SetAttemptOutcome(ctx, db, attempt.ID, "checks_passed"); err != nil {
		return card, err
	}
	published, err := Publish(ctx, db, card.ID, PublishInput{
		Data: deps.Data, Key: deps.Key, Tools: deps.Tools, ExitCode: 0, AttemptID: attempt.ID,
		Summary:  prSummary(card),
		Metadata: fmt.Sprintf("card: %s\nattempt: %s\nmodel: %s\nrequirements: %s", card.ID, attempt.ID, model, strings.Join(card.RequirementIDs, ", ")),
		Secrets:  redact,
	})
	if errors.Is(err, ErrOwnerGitHub) || errors.Is(err, ErrGitHubSetup) {
		return published, nil
	}
	if err != nil {
		_ = jobError(ctx, db, card, attempt.ID, err.Error(), redact)
		return BlockCard(ctx, db, card, "publish_failed", redact)
	}
	return published, nil
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
// ready, changes_requested, or in_progress, and no attempt of it is open.
func CanStartImplement(ctx context.Context, db querier, card board.Card) (bool, error) {
	if !startable(card.Status) {
		return false, nil
	}
	open, err := board.OpenAttempt(ctx, db, card.ID)
	return !open, err
}

func startable(status string) bool {
	return status == "ready" || status == "changes_requested" || status == "in_progress"
}

func sandboxReady(check string, cfg settings.ProjectConfig, engine string, fixture bool) bool {
	if _, err := sandbox.ParseCheck(check, cfg.Checks.AllowExecutables); err != nil {
		return false
	}
	if fixture {
		return true
	}
	if sandbox.CheckEngine(engine) != nil {
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
		Payload: map[string]any{"message": message},
	}, redact)
	return err
}

func prSummary(card board.Card) string {
	if len(card.RequirementIDs) == 0 {
		return card.Title
	}
	return card.Title + "\n\nRequirements: " + strings.Join(card.RequirementIDs, ", ")
}

func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
