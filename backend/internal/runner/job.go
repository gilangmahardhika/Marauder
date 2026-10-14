package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/runnerapi"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/sandbox"
	"orchestrator/backend/internal/trace"
)

const (
	checkTimeout = 10 * time.Minute
	checkMaxOut  = 1 << 20
)

// Tools are the programs a runner starts. Exec replaces execx.Command in
// tests.
type Tools struct {
	Git    string
	Podman string
	Docker string
	Exec   func(context.Context, execx.Child) (execx.Result, error)
}

func (t Tools) engine(name string) string {
	if name == "docker" {
		return t.Docker
	}
	return t.Podman
}

func (t Tools) exec() func(context.Context, execx.Child) (execx.Result, error) {
	if t.Exec != nil {
		return t.Exec
	}
	return execx.Command
}

// job is one leased implement job on this machine.
type job struct {
	r       *Runner
	leaseID string
	ctx     runnerapi.Job
	secrets []string
	git     runtime.Git
	engine  string
}

func (j *job) redact(s string) string {
	return trace.Redact(s, j.secrets)
}

func (j *job) report(ctx context.Context, rep runnerapi.Report) (runnerapi.ReportReply, error) {
	rep.Message = j.redact(rep.Message)
	rep.Tail = j.redact(rep.Tail)
	return j.r.Client.Report(ctx, j.leaseID, rep)
}

// fail reports an attempt that stopped with an error. cause is wall_cap,
// provider_error, or other.
func (j *job) fail(ctx context.Context, cause string, err error) error {
	_, rerr := j.report(ctx, runnerapi.Report{Fact: runnerapi.FactFinished, Outcome: "error", Cause: cause, Message: err.Error()})
	return rerr
}

// RunJob runs one lease to its end: start, checkout, the attempt or a resume,
// then the bundle and the gating check when the control plane asks for it.
// It ends the lease when the job is over. A cancelled ctx returns without
// ending it, so the caller can release it.
func (r *Runner) RunJob(ctx context.Context, leaseID string) error {
	start, err := r.Client.Start(ctx, leaseID)
	if err != nil {
		return err
	}
	if start.Done {
		return r.Client.End(ctx, leaseID)
	}
	j := &job{r: r, leaseID: leaseID, ctx: start, secrets: []string{start.CloneToken}}
	if err := j.run(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New(j.redact(err.Error()))
	}
	return r.Client.End(ctx, leaseID)
}

func (j *job) base() string {
	if j.ctx.BaseBranch == "" {
		return "main"
	}
	return j.ctx.BaseBranch
}

func (j *job) run(ctx context.Context) error {
	if !validID(j.ctx.ProjectID) || !validID(j.ctx.CardID) {
		return fmt.Errorf("job names an invalid project or card")
	}
	dir := filepath.Join(j.r.Data, "projects", j.ctx.ProjectID)
	j.git = runtime.Git{
		Bin: j.r.Tools.Git, Repo: filepath.Join(dir, "repo"), Data: dir,
		Author: runtime.Author{Name: j.ctx.Author.Name, Email: j.ctx.Author.Email}, Exec: j.r.Tools.Exec,
	}
	j.engine = j.r.Tools.engine(j.ctx.SandboxEngine)
	if err := runtime.PrepareProjectDir(dir); err != nil {
		return j.fail(ctx, "other", err)
	}
	remote := runtime.Remote{URL: j.ctx.CloneURL, Token: j.ctx.CloneToken}
	if _, err := j.git.EnsureCheckout(ctx, j.base(), remote); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return j.fail(ctx, "other", err)
	}
	if !j.ctx.Fixture {
		if err := sandbox.ProbeEngine(ctx, j.engine, j.r.Tools.exec()); err != nil {
			_, err := j.report(ctx, runnerapi.Report{
				Fact: runnerapi.FactBlocked, Reason: "sandbox_config",
				Message: err.Error() + "; start rootless podman on the runner " + j.r.Name + ", then unblock the card",
			})
			return err
		}
	}
	if j.ctx.PublishedBranch {
		if err := j.git.FetchCard(ctx, j.ctx.CardID, remote); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return j.fail(ctx, "other", err)
		}
		if err := j.git.SeedCardBranch(ctx, j.ctx.CardID); err != nil {
			return j.fail(ctx, "other", err)
		}
	} else if j.ctx.CardBundle {
		if err := j.seedFromControlPlane(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return j.fail(ctx, "other", err)
		}
	}
	reply, resumed, err := j.resume(ctx)
	if err != nil {
		return err
	}
	if !resumed {
		if reply, err = j.attempt(ctx); err != nil {
			return err
		}
	}
	if reply.Next == "check" {
		return j.check(ctx, reply)
	}
	return nil
}

// seedFromControlPlane starts orch/<card> from the control plane's copy of
// the branch when the card has no pull request to fetch (RFC-119).
func (j *job) seedFromControlPlane(ctx context.Context) error {
	dir := filepath.Join(j.git.Data, "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, "card-*.bundle")
	if err != nil {
		return err
	}
	path := file.Name()
	file.Close()
	os.Remove(path)
	defer os.Remove(path)
	sha, err := j.r.Client.CardBundle(ctx, j.leaseID, path)
	if err != nil {
		return err
	}
	return j.git.SeedCardBundle(ctx, j.ctx.CardID, path, sha)
}

// resume reports the control plane's unchecked commit when the worktree head
// is still that commit, so it is checked without a model call.
func (j *job) resume(ctx context.Context) (runnerapi.ReportReply, bool, error) {
	if j.ctx.Resume == nil {
		return runnerapi.ReportReply{}, false, nil
	}
	wt, err := j.git.EnsureWorktree(ctx, j.ctx.CardID, j.base())
	if err != nil {
		return runnerapi.ReportReply{}, false, err
	}
	head, err := j.git.WorktreeHead(ctx, wt)
	if err != nil || head != j.ctx.Resume.SHA {
		return runnerapi.ReportReply{}, false, err
	}
	reply, err := j.report(ctx, runnerapi.Report{Fact: runnerapi.FactResume, AttemptID: j.ctx.Resume.AttemptID})
	return reply, true, err
}

// attempt runs one tool loop with the board, model, and MCP of the lease and
// reports how it ended.
func (j *job) attempt(ctx context.Context) (runnerapi.ReportReply, error) {
	b := &Board{Client: j.r.Client, LeaseID: j.leaseID, Secrets: j.secrets}
	proxy, err := OpenMCP(ctx, j.r.Client, j.leaseID)
	if err != nil {
		return runnerapi.ReportReply{}, err
	}
	var extra loop.ExtraTools
	if proxy != nil {
		extra = proxy
	}
	var scope *loop.MCPContext
	if m := j.ctx.MCP; m != nil {
		scope = &loop.MCPContext{
			Scope: m.Scope, Offered: m.Offered, Missing: m.Missing, Attached: m.Attached,
			Detached: m.Detached, Reason: m.Reason, Total: m.Total,
		}
	}
	attemptCtx := ctx
	if j.ctx.WallMinutes > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, time.Duration(j.ctx.WallMinutes)*time.Minute)
		defer cancel()
	}
	result, err := loop.Run(attemptCtx, loop.Input{
		Git: j.git, BaseBranch: j.base(), CardID: j.ctx.CardID,
		Model: j.ctx.Model, TaskClass: j.ctx.TaskClass, MaxTurns: j.ctx.MaxTurns,
		Provider: &Provider{Client: j.r.Client, Board: b}, Secrets: j.secrets,
		Allow: j.ctx.AllowExecutables, Commands: j.ctx.CommandExecutables,
		Engine: j.engine, Image: j.ctx.SandboxImage, Exec: j.r.Tools.Exec,
		HistoryBudget: j.ctx.HistoryBudget, MaxTokensPerCard: j.ctx.MaxTokensPerCard, ContextTokens: j.ctx.ContextTokens,
		Extra: extra, MCP: scope, Board: b, Fixture: j.ctx.Fixture,
		QuestionMode: j.ctx.QuestionMode, QuestionRounds: j.ctx.QuestionRounds,
	})
	if err != nil {
		if ctx.Err() != nil {
			return runnerapi.ReportReply{}, ctx.Err()
		}
		cause := "other"
		switch {
		case errors.Is(attemptCtx.Err(), context.DeadlineExceeded):
			cause = "wall_cap"
		case errors.Is(err, ErrProvider):
			cause = "provider_error"
		}
		return runnerapi.ReportReply{}, j.fail(ctx, cause, err)
	}
	return j.report(ctx, runnerapi.Report{Fact: runnerapi.FactFinished, Outcome: result.Outcome, Reason: result.Reason})
}

// check uploads the card branch, then runs the gating check on an export of
// the commit and reports its exit code. The control plane publishes from the
// bundle when the check exits 0.
func (j *job) check(ctx context.Context, reply runnerapi.ReportReply) error {
	if err := j.bundle(ctx, reply.SHA); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := j.report(ctx, runnerapi.Report{Fact: runnerapi.FactCheck, Error: "check_error", Message: "bundle upload: " + err.Error()})
		return err
	}
	exit, tail, err := j.gatingCheck(ctx, reply.SHA, reply.AttemptID)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	rep := runnerapi.Report{Fact: runnerapi.FactCheck, Exit: exit, Tail: tail}
	if err != nil {
		rep = runnerapi.Report{Fact: runnerapi.FactCheck, Error: checkErrorReason(err), Message: err.Error()}
	}
	_, err = j.report(ctx, rep)
	return err
}

func (j *job) bundle(ctx context.Context, sha string) error {
	file, err := os.CreateTemp(filepath.Join(j.git.Data, "exports"), "card-*.bundle")
	if err != nil {
		return err
	}
	name := file.Name()
	file.Close()
	defer os.Remove(name)
	if err := j.git.BundleCard(ctx, j.ctx.CardID, j.base(), name); err != nil {
		return err
	}
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return j.r.Client.Bundle(ctx, j.leaseID, sha, f)
}

func (j *job) gatingCheck(ctx context.Context, sha, attemptID string) (int, string, error) {
	if sha == "" {
		return 0, "", errors.New("attempt has no commit")
	}
	dest := filepath.Join(j.git.Data, "exports", attemptID+"-gate")
	defer os.RemoveAll(dest)
	if err := j.git.Export(ctx, sha, dest); err != nil {
		return 0, "", err
	}
	res, executed, err := sandbox.ExecuteCheck(ctx, sandbox.ExecuteOpts{
		Command: j.ctx.LocalCheck, Allow: j.ctx.AllowExecutables, Fixture: j.ctx.Fixture,
		Engine: j.engine, Image: j.ctx.SandboxImage, Dir: dest, Timeout: checkTimeout, MaxOut: checkMaxOut,
	}, j.r.Tools.exec())
	if err != nil {
		return 0, "", err
	}
	if !executed {
		return 0, "", errors.New("check was not executed")
	}
	return res.Exit, tailLines(string(res.Out), 200), nil
}

// checkErrorReason matches app.CheckErrorReason: sandbox_config when the
// command or engine is misconfigured, else check_error.
func checkErrorReason(err error) string {
	if errors.Is(err, sandbox.ErrParse) || errors.Is(err, sandbox.ErrAllow) || errors.Is(err, sandbox.ErrEmpty) ||
		errors.Is(err, sandbox.ErrImage) || errors.Is(err, sandbox.ErrEngine) {
		return "sandbox_config"
	}
	return "check_error"
}

func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// validID keeps a control plane id usable as one path element.
func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
