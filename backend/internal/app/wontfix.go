package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/ghcli"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

var (
	// ErrCloseToken is a won't fix on a card with an open pull request by a
	// person with no verified GitHub token on the company host (RFC-112).
	ErrCloseToken = errors.New("connect your GitHub account in Account to close the pull request")
	// ErrCloseRefused wraps the reason GitHub gave for not closing the pull request.
	ErrCloseRefused = errors.New("GitHub did not close the pull request")
)

const wontFixReasonMax = 500

// CanCloseCard reports whether a person may set a blocked card to won't fix
// or reopen it: a Lead Developer, a Product Owner, or the card owner (RFC-112).
func CanCloseCard(role, userID string, card board.Card) bool {
	if role == "lead_developer" || role == "product_owner" {
		return true
	}
	return userID != "" && card.OwnerID == userID
}

// WontFix closes a blocked card. An open pull request gets the reason as a
// comment and is closed with the actor's own GitHub token first, so a refusal
// leaves the card blocked.
func WontFix(ctx context.Context, db *sql.DB, deps JobDeps, cardID, actorID, reason string) (board.Card, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > wontFixReasonMax {
		return board.Card{}, &settings.Invalid{Fields: map[string]string{"reason": "Say why, in 1 to 500 characters."}}
	}
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	if card.Status != "blocked" {
		return card, board.ErrConflict
	}
	login, closed, secrets, err := closePR(ctx, db, deps, card, actorID, reason)
	if err != nil {
		return card, err
	}
	next, err := transition(ctx, db, card, "wont_fix", machine.Evidence{Reason: reason, ActorID: actorID})
	if err != nil {
		return card, err
	}
	_, _ = trace.Append(ctx, db, trace.Event{
		CardID: card.ID, ProjectID: card.ProjectID, ActorID: actorID, Kind: "wont_fix",
		Payload: map[string]any{"reason": reason, "prUrl": card.PRURL, "prClosed": closed, "login": login},
	}, secrets)
	return next, nil
}

// closePR comments on and closes the card's pull request when GitHub reports
// it open. A card with no pull request needs no token.
func closePR(ctx context.Context, db *sql.DB, deps JobDeps, card board.Card, actorID, reason string) (string, bool, []string, error) {
	if card.PRURL == "" {
		return "", false, nil, nil
	}
	pull, comments, err := pullAPI(card.PRURL)
	if err != nil {
		return "", false, nil, fmt.Errorf("%w: %w", ErrCloseRefused, err)
	}
	acct, err := company.Load(ctx, db)
	if err != nil {
		return "", false, nil, err
	}
	login, token, err := mergeToken(ctx, db, deps.Key, actorID, acct.Host)
	if errors.Is(err, ErrMergeToken) {
		return "", false, nil, ErrCloseToken
	}
	if err != nil {
		return "", false, nil, err
	}
	secrets := []string{token}
	dir := settings.ProjectDir(deps.Data, card.ProjectID)
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o700); err != nil {
		return login, false, secrets, err
	}
	gh := ghcli.Runner{Bin: deps.Tools.GH, Git: deps.Tools.Git, Exec: deps.Tools.Exec, ConfigDir: filepath.Join(dir, "empty-gh")}
	out, err := gh.Run(ctx, acct.Host, token, "api", pull, "--jq", ".state")
	if err != nil {
		return login, false, secrets, fmt.Errorf("%w: %w", ErrCloseRefused, err)
	}
	if strings.TrimSpace(string(out)) != "open" {
		return login, false, secrets, nil
	}
	body := "Closed as won't fix by @" + login + ": " + reason
	if err := ghWrite(ctx, gh, acct.Host, token, dir, "POST", comments, map[string]string{"body": body}); err != nil {
		return login, false, secrets, fmt.Errorf("%w: %w", ErrCloseRefused, err)
	}
	if err := ghWrite(ctx, gh, acct.Host, token, dir, "PATCH", pull, map[string]string{"state": "closed"}); err != nil {
		return login, false, secrets, fmt.Errorf("%w: %w", ErrCloseRefused, err)
	}
	return login, true, secrets, nil
}

// Reopen returns a won't fix card to blocked with the reason it was blocked
// for, so the usual unblock rules apply again. GitHub is not touched; the next
// publish reopens the pull request.
func Reopen(ctx context.Context, db *sql.DB, cardID, actorID string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	if card.Status != "wont_fix" {
		return card, board.ErrConflict
	}
	reason := card.BlockedReason
	if reason == "" {
		reason = "reopened"
	}
	return transition(ctx, db, card, "blocked", machine.Evidence{Reason: reason, ActorID: actorID})
}
