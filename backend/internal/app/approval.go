package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/ghcli"
	"orchestrator/backend/internal/settings"
)

// ApprovalContext is the commit status the harness sets on an approved SHA
// (RFC-068). The review job does not count it as a pull request check.
const ApprovalContext = "orchestrator/approval"

const announceTimeout = 30 * time.Second

// statusAPI returns the REST path of the commit statuses of sha in the pull
// request's repository.
func statusAPI(url, sha string) (string, error) {
	m := pullParts.FindStringSubmatch(url)
	if m == nil {
		return "", fmt.Errorf("%w: %q is not a pull request url", ghcli.ErrGitHub, url)
	}
	return "repos/" + m[1] + "/" + m[2] + "/statuses/" + sha, nil
}

// ApprovalComment is the pull request comment posted when a person approves
// the card in the orchestrator.
func ApprovalComment(approver, sha string, independent bool) string {
	text := fmt.Sprintf("**Approved in the orchestrator** by @%s at `%s`.", approver, shortSHA(sha))
	if !independent {
		text += " This approval is not independent (solo approval or self review): the approver also owns the card or is the bot account."
	}
	return text + " Merging stays a separate step on GitHub.\n"
}

// AnnounceApproval posts the approval on the card's pull request with the
// company token: a comment, and a success status on the approved SHA. A
// failure writes a job_error and never changes the card.
func AnnounceApproval(ctx context.Context, db *sql.DB, deps JobDeps, card board.Card, approver string, independent bool) {
	sha := card.ApprovedSHA
	if sha == "" {
		sha = card.HeadSHA
	}
	announce(ctx, db, deps, card, func(post poster) {
		post("approval comment: ", "comments", map[string]string{"body": ApprovalComment(approver, sha, independent)})
		post("approval status: ", "status", map[string]string{
			"state": "success", "context": ApprovalContext,
			"description": "Approved by @" + approver + " in the orchestrator",
		})
	}, sha)
}

// WithdrawApproval marks the approval status of sha as failed after a person
// requests changes on, or rejects, an approved card.
func WithdrawApproval(ctx context.Context, db *sql.DB, deps JobDeps, card board.Card, sha string) {
	announce(ctx, db, deps, card, func(post poster) {
		post("approval status: ", "status", map[string]string{
			"state": "failure", "context": ApprovalContext, "description": "Approval withdrawn",
		})
	}, sha)
}

type poster func(prefix, target string, payload map[string]string)

func announce(ctx context.Context, db *sql.DB, deps JobDeps, card board.Card, run func(poster), sha string) {
	if deps.Data == "" || card.PRURL == "" || sha == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), announceTimeout)
	defer cancel()
	gh, err := newGitHubReader(ctx, db, deps, card.ProjectID)
	if err != nil {
		_ = jobError(ctx, db, card, "", "approval comment: "+err.Error(), nil)
		return
	}
	redact := []string{gh.token}
	if _, secrets, err := settings.Load(deps.Data); err == nil {
		redact = append(redact, modelKeys(secrets)...)
	}
	dir := settings.ProjectDir(deps.Data, card.ProjectID)
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		_ = jobError(ctx, db, card, "", "approval comment: "+err.Error(), redact)
		return
	}
	_, comments, err := pullAPI(card.PRURL)
	if err != nil {
		_ = jobError(ctx, db, card, "", "approval comment: "+err.Error(), redact)
		return
	}
	status, err := statusAPI(card.PRURL, sha)
	if err != nil {
		_ = jobError(ctx, db, card, "", "approval status: "+err.Error(), redact)
		return
	}
	run(func(prefix, target string, payload map[string]string) {
		path := comments
		if target == "status" {
			path = status
		}
		if err := ghWrite(ctx, gh.gh, gh.acct.Host, gh.token, dir, "POST", path, payload); err != nil {
			_ = jobError(ctx, db, card, "", prefix+err.Error(), redact)
		}
	})
}
