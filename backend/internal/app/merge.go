package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/ghcli"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

var (
	// ErrDeployOff refuses a deploy on a project whose deploy switch is off (RFC-111).
	ErrDeployOff = errors.New("deploy is off for this project")
	// ErrDeployOn refuses a merge on a project that deploys approved cards.
	ErrDeployOn = errors.New("deploy is on for this project, so approved cards deploy instead of merging")
	// ErrMergeToken is a merge by a person with no verified GitHub token on the company host.
	ErrMergeToken = errors.New("connect your GitHub account in Account to merge")
	// ErrMergeRefused wraps the reason GitHub gave for not merging.
	ErrMergeRefused = errors.New("GitHub did not merge the pull request")
)

// DeployEnabled reports the project's deploy switch. A missing or unreadable
// config reads as on, the default.
func DeployEnabled(data, projectID string) bool {
	if data == "" || projectID == "" {
		return true
	}
	cfg, _, err := settings.LoadProject(data, projectID)
	return err != nil || settings.DeployOn(cfg)
}

// CanMerge reports whether a person may squash-merge an approved card: a Lead
// Developer, or the card owner when the project allows solo approval (RFC-111).
func CanMerge(role, userID string, card board.Card, solo bool) bool {
	if role == "lead_developer" {
		return true
	}
	return solo && userID != "" && card.OwnerID == userID
}

// SquashMerge squash-merges the approved card's pull request with the actor's
// own GitHub token, pinned to the approved SHA, then moves the card to done
// with the merge commit as evidence. GitHub refuses when the head moved.
func SquashMerge(ctx context.Context, db *sql.DB, deps JobDeps, cardID, actorID string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	if DeployEnabled(deps.Data, card.ProjectID) {
		return card, ErrDeployOn
	}
	if card.Status != "approved" || card.ApprovedSHA == "" {
		return card, board.ErrConflict
	}
	pull, _, err := pullAPI(card.PRURL)
	if err != nil {
		return card, fmt.Errorf("%w: %w", ErrMergeRefused, err)
	}
	acct, err := company.Load(ctx, db)
	if err != nil {
		return card, err
	}
	login, token, err := mergeToken(ctx, db, deps.Key, actorID, acct.Host)
	if err != nil {
		return card, err
	}
	dir := settings.ProjectDir(deps.Data, card.ProjectID)
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		return card, err
	}
	gh := ghcli.Runner{Bin: deps.Tools.GH, Git: deps.Tools.Git, Exec: deps.Tools.Exec, ConfigDir: filepath.Join(dir, "empty-gh")}
	out, err := ghWriteOut(ctx, gh, acct.Host, token, dir, "PUT", pull+"/merge",
		map[string]string{"merge_method": "squash", "sha": card.ApprovedSHA}, "--jq", ".sha")
	if err != nil {
		return card, fmt.Errorf("%w: %w", ErrMergeRefused, err)
	}
	mergeSHA := strings.TrimSpace(string(out))
	next, err := transition(ctx, db, card, "done", machine.Evidence{SHA: mergeSHA, ActorID: actorID})
	if err != nil {
		return card, err
	}
	_, _ = trace.Append(ctx, db, trace.Event{
		CardID: card.ID, ProjectID: card.ProjectID, ActorID: actorID, Kind: "merge",
		Payload: map[string]any{"prUrl": card.PRURL, "approvedSha": card.ApprovedSHA, "mergeSha": mergeSHA, "login": login, "method": "squash"},
	}, []string{token})
	return next, nil
}

func mergeToken(ctx context.Context, db *sql.DB, key []byte, userID, host string) (string, string, error) {
	acct, err := identity.GitHubOf(ctx, db, userID)
	if err != nil {
		return "", "", err
	}
	if !acct.Verified || !acct.TokenSet || acct.Login == "" || (host != "" && !strings.EqualFold(acct.Host, host)) {
		return "", "", ErrMergeToken
	}
	token, err := identity.Token(ctx, db, userID, key)
	if err != nil {
		return "", "", err
	}
	if token == "" {
		return "", "", ErrMergeToken
	}
	return acct.Login, token, nil
}
