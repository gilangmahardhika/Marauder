package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/ghcli"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/providers/llm"
	"orchestrator/backend/internal/review"
	"orchestrator/backend/internal/router"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/trace"
)

const reviewSentinel = "different-family-from-author"

// pullRequest is what gh reports for a card's pull request.
type pullRequest struct {
	Head     string
	Body     string
	Comments string
	Checks   []review.Check
}

// githubReader reads pull requests with the company token.
type githubReader struct {
	gh    ghcli.Runner
	acct  company.Account
	token string
}

func newGitHubReader(ctx context.Context, db *sql.DB, deps JobDeps, projectID string) (githubReader, error) {
	acct, err := company.Load(ctx, db)
	if err != nil {
		return githubReader{}, err
	}
	token, err := company.Token(ctx, db, deps.Key)
	if err != nil {
		return githubReader{}, err
	}
	if acct.Org == "" || token == "" {
		return githubReader{}, ErrGitHubSetup
	}
	gh := ghcli.Runner{Bin: deps.Tools.GH, Git: deps.Tools.Git, Exec: deps.Tools.Exec, ConfigDir: filepath.Join(settings.ProjectDir(deps.Data, projectID), "empty-gh")}
	return githubReader{gh: gh, acct: acct, token: token}, nil
}

func (r githubReader) read(ctx context.Context, url string) (pullRequest, error) {
	out, err := r.gh.Run(ctx, r.acct.Host, r.token, "pr", "view", url, "--json", "headRefOid,body,comments,statusCheckRollup")
	if err != nil {
		return pullRequest{}, err
	}
	var raw struct {
		HeadRefOid string `json:"headRefOid"`
		Body       string `json:"body"`
		Comments   []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
		} `json:"comments"`
		StatusCheckRollup []struct {
			Name       string `json:"name"`
			Context    string `json:"context"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return pullRequest{}, fmt.Errorf("%w: pr view: %v", ghcli.ErrGitHub, err)
	}
	if len(raw.HeadRefOid) != 40 {
		return pullRequest{}, fmt.Errorf("%w: pr view has no head", ghcli.ErrGitHub)
	}
	pr := pullRequest{Head: raw.HeadRefOid, Body: raw.Body}
	var comments []string
	for _, c := range raw.Comments {
		comments = append(comments, c.Author.Login+": "+c.Body)
	}
	pr.Comments = strings.Join(comments, "\n\n")
	for _, c := range raw.StatusCheckRollup {
		name := c.Name
		if name == "" {
			name = c.Context
		}
		if name == ApprovalContext {
			continue
		}
		pr.Checks = append(pr.Checks, review.Check{Name: name, State: checkState(c.Status, c.Conclusion, c.State)})
	}
	return pr, nil
}

// checkState maps a GitHub check run (status and conclusion) or status
// context (state) to success, pending, or failure.
func checkState(status, conclusion, state string) string {
	if state != "" {
		switch state {
		case "SUCCESS":
			return "success"
		case "PENDING", "EXPECTED":
			return "pending"
		default:
			return "failure"
		}
	}
	if status != "" && status != "COMPLETED" {
		return "pending"
	}
	switch conclusion {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return "success"
	default:
		return "failure"
	}
}

func (r githubReader) comment(ctx context.Context, dir, url, body string) error {
	_, comments, err := pullAPI(url)
	if err != nil {
		return err
	}
	return ghWrite(ctx, r.gh, r.acct.Host, r.token, dir, "POST", comments, map[string]string{"body": body})
}

// reviewRoute picks the review model for an attempt written by author. The
// sentinel picks the first model of another family; with one family the
// review is not independent.
func reviewRoute(cfg settings.Config, secrets settings.Secrets, author string) (model, fallback string, independent bool) {
	route := cfg.Router.Routes["review"]
	authorFamily := ""
	if _, family, _, err := llm.ForModel(cfg, secrets, author); err == nil {
		authorFamily = family.ID
	}
	if route.Model == "" || route.Model == reviewSentinel {
		var families []router.Family
		for _, f := range cfg.LLM.Families {
			ids := make([]string, 0, len(f.Models))
			for _, m := range f.Models {
				ids = append(ids, m.ID)
			}
			families = append(families, router.Family{ID: f.ID, Models: ids})
		}
		model, independent = router.ReviewModel(authorFamily, families)
		return model, route.Fallback, independent
	}
	_, family, _, err := llm.ForModel(cfg, secrets, route.Model)
	return route.Model, route.Fallback, err == nil && family.ID != authorFamily
}

// ReviewCard runs the review job (RFC-065) for a pr_open card: it moves the
// card to agent_review, asks the review model for a verdict on the pull
// request diff, records it on the attempt, then moves the card to
// changes_requested, or applies the pull request checks. It never changes
// attempts_used. A GitHub or git read failure leaves the card in pr_open so
// the worker tries again.
func ReviewCard(ctx context.Context, db *sql.DB, deps JobDeps, projectID, cardID string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return card, err
	}
	if card.ProjectID != projectID {
		return card, board.ErrNotFound
	}
	if card.Status != "pr_open" {
		return card, ErrNotStartable
	}
	attempt, err := board.LatestReview(ctx, db, card.ID)
	if err != nil {
		return card, err
	}
	if attempt.AttemptID == "" || attempt.CommitSHA == "" {
		return card, fmt.Errorf("card %s has no checked commit to review", card.ID)
	}
	cfg, secrets, err := settings.Load(deps.Data)
	if err != nil {
		return card, err
	}
	redact := modelKeys(secrets)
	project, err := tenancy.GetProject(ctx, db, projectID)
	if err != nil {
		return card, err
	}
	gh, err := newGitHubReader(ctx, db, deps, projectID)
	if err != nil && !errors.Is(err, ErrGitHubSetup) {
		return card, err
	}
	setupErr := err
	if setupErr == nil {
		redact = append(redact, gh.token)
	}

	holder, err := board.NewID("lok_")
	if err != nil {
		return card, err
	}
	if err := board.AcquireLock(ctx, db, card.ID, holder); err != nil {
		return card, err
	}
	defer board.ReleaseLock(context.WithoutCancel(ctx), db, card.ID, holder)

	var pr pullRequest
	var diff string
	if setupErr == nil {
		if pr, err = gh.read(ctx, card.PRURL); err != nil {
			_ = jobError(ctx, db, card, attempt.AttemptID, err.Error(), redact)
			return card, errors.New(trace.Redact(err.Error(), redact))
		}
		base := project.BaseBranch
		if base == "" {
			base = "main"
		}
		git := ProjectGit(deps.Data, project, runtime.Author{}, deps.Tools)
		text, truncated, err := git.Diff(ctx, "refs/heads/"+base, attempt.CommitSHA, loop.ReviewDiffCap)
		if err != nil {
			_ = jobError(ctx, db, card, attempt.AttemptID, err.Error(), redact)
			return card, errors.New(trace.Redact(err.Error(), redact))
		}
		diff = text
		if truncated {
			diff += "\ntruncated: true"
		}
	}

	card, err = move(ctx, db, card, "agent_review", machine.Evidence{AttemptID: attempt.AttemptID}, redact)
	if err != nil {
		return card, err
	}
	if setupErr != nil {
		return BlockCard(ctx, db, card, "github_config", redact)
	}
	dir := settings.ProjectDir(deps.Data, projectID)
	model, fallback, independent := reviewRoute(cfg, secrets, attempt.ModelID)
	providerFor := func(model, fallback string) (providers.Provider, bool) {
		if deps.Provider != nil {
			return deps.Provider, model != ""
		}
		if _, _, _, err := llm.ForModel(cfg, secrets, model); model == "" || err != nil {
			return nil, false
		}
		return RoutedProvider(cfg, secrets, model, fallback), true
	}
	provider, ok := providerFor(model, fallback)
	if !ok {
		_ = jobError(ctx, db, card, attempt.AttemptID, "router_config: no review model is routable", redact)
		return reviewFailed(ctx, db, deps, gh, dir, card, attempt.AttemptID, model, pr, "No review model is configured", redact)
	}

	in := loop.ReviewInput{
		DB: db, Card: card, AttemptID: attempt.AttemptID, Model: model, Provider: provider,
		Diff:        diff,
		CheckOutput: fmt.Sprintf("local check %q exited 0 on %s", card.LocalCheck, attempt.CommitSHA),
		PRBody:      pr.Body, Comments: pr.Comments, Secrets: redact,
		Comment: func(ctx context.Context, body string) error {
			return gh.comment(ctx, dir, card.PRURL, body)
		},
	}
	writeReviewPack(ctx, dir, attempt.AttemptID, in)
	result, err := loop.Review(ctx, in)
	if errors.Is(err, loop.ErrReviewParse) && fallback != "" && fallback != model {
		if p, ok := providerFor(fallback, ""); ok {
			in.Model, in.Provider = fallback, p
			model = fallback
			result, err = loop.Review(ctx, in)
		}
	}
	switch {
	case errors.Is(err, loop.ErrReviewParse):
		_ = jobError(ctx, db, card, attempt.AttemptID, "the reviewer gave no valid verdict", redact)
		return reviewFailed(ctx, db, deps, gh, dir, card, attempt.AttemptID, model, pr, "The reviewer gave no valid verdict", redact)
	case err != nil && ctx.Err() != nil:
		// RefreshCard finishes a review that stopped without a verdict.
		return card, errors.New(trace.Redact(err.Error(), redact))
	case errors.Is(err, ErrProvider):
		_ = jobError(ctx, db, card, attempt.AttemptID, err.Error(), redact)
		return reviewFailed(ctx, db, deps, gh, dir, card, attempt.AttemptID, model, pr, "The review model could not be reached", redact)
	case err != nil:
		_ = jobError(ctx, db, card, attempt.AttemptID, err.Error(), redact)
		return reviewFailed(ctx, db, deps, gh, dir, card, attempt.AttemptID, model, pr, "The review job failed", redact)
	}

	if err := board.RecordReview(ctx, db, attempt.AttemptID, model, result.PromptHash, independent, result.Verdict, pr.Head); err != nil {
		return card, err
	}
	if err := board.SetHeadSHA(ctx, db, card.ID, pr.Head); err != nil {
		return card, err
	}
	comment := trace.Redact(ReviewComment(result.Verdict, result.Body, model, independent, pr.Head), redact)
	if err := gh.comment(ctx, dir, card.PRURL, comment); err != nil {
		_ = jobError(ctx, db, card, attempt.AttemptID, "review comment: "+err.Error(), redact)
	}
	if result.Verdict == "request_changes" {
		note := result.Body
		if note == "" {
			note = "the reviewer requested changes"
		}
		if err := board.InsertFailureNote(ctx, db, card.ID, attempt.AttemptID, "review", note); err != nil {
			return card, err
		}
		return move(ctx, db, card, "changes_requested", machine.Evidence{AttemptID: attempt.AttemptID}, redact)
	}
	return settleChecks(ctx, db, deps, card, attempt.AttemptID, freshChecks(ctx, gh, card.PRURL, pr), redact)
}

// reviewFailed records a review that gave no verdict, says so on the pull
// request, and lets the pull request checks decide the card (RFC-069).
func reviewFailed(ctx context.Context, db *sql.DB, deps JobDeps, gh githubReader, dir string, card board.Card,
	attemptID, model string, pr pullRequest, why string, redact []string) (board.Card, error) {
	if err := board.RecordReview(ctx, db, attemptID, model, "", false, review.VerdictFailed, pr.Head); err != nil {
		return card, err
	}
	if err := board.SetHeadSHA(ctx, db, card.ID, pr.Head); err != nil {
		return card, err
	}
	if err := gh.comment(ctx, dir, card.PRURL, ReviewFailedComment(why, pr.Head)); err != nil {
		_ = jobError(ctx, db, card, attemptID, "review comment: "+err.Error(), redact)
	}
	return settleChecks(ctx, db, deps, card, attemptID, freshChecks(ctx, gh, card.PRURL, pr), redact)
}

// freshChecks reads the pull request again after the review, so checks that
// registered while the model ran count. A failed read or a moved head keeps
// the checks of the first read.
func freshChecks(ctx context.Context, gh githubReader, url string, pr pullRequest) []review.Check {
	if fresh, err := gh.read(ctx, url); err == nil && fresh.Head == pr.Head {
		return fresh.Checks
	}
	return pr.Checks
}

// settleChecks applies the pull request checks of an agent_review card. An
// empty list within review.NoChecksWait of entering pr_open leaves the card
// waiting, since the checks may not have registered yet.
func settleChecks(ctx context.Context, db *sql.DB, deps JobDeps, card board.Card, attemptID string, checks []review.Check, redact []string) (board.Card, error) {
	if len(checks) == 0 {
		published, err := board.PublishedAt(ctx, db, card.ID)
		if err != nil {
			return card, err
		}
		if deps.now().Sub(published) < review.NoChecksWait {
			return card, nil
		}
	}
	if err := board.SetCIRequired(ctx, db, attemptID, len(checks) > 0); err != nil {
		return card, err
	}
	return review.ApplyCI(ctx, db, card, attemptID, checks, redact)
}

// ReviewFailedComment is the pull request comment the bot posts when the AI
// review gave no verdict (RFC-069).
func ReviewFailedComment(why, sha string) string {
	return fmt.Sprintf("**AI review: Not done**\n\n%s, so `%s` has no AI verdict. The card waits for the pull request checks, then a person reviews it in the orchestrator before merge.\n", why, shortSHA(sha))
}

// ReviewComment is the pull request comment the bot posts with the review
// verdict (RFC-066).
func ReviewComment(verdict, body, model string, independent bool, sha string) string {
	heading := "**AI review: Recommend approve**"
	if verdict == "request_changes" {
		heading = "**AI review: Request changes**"
	}
	family := "a model of another family than the author"
	if !independent {
		family = "a model of the same family as the author, so this review is not independent"
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "(the reviewer gave no reason)"
	}
	next := "A person approves the card in the orchestrator before merge."
	if verdict == "request_changes" {
		next = "The card went back to the implementer with this note."
	}
	return fmt.Sprintf("%s\n\n%s\n\nReviewed `%s` with `%s`, %s. %s\n", heading, body, shortSHA(sha), model, family, next)
}

// writeReviewPack keeps the reviewer prompt at logs/<attempt>-review.json. A
// failed write does not stop the review.
func writeReviewPack(ctx context.Context, dir, attemptID string, in loop.ReviewInput) {
	system, user, err := loop.ReviewPack(ctx, in)
	if err != nil {
		return
	}
	body, err := json.MarshalIndent(map[string]string{"model": in.Model, "system": system, "user": user}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "logs", attemptID+"-review.json"), body, 0o600)
}

// RefreshCard reads the pull request head and checks of a card past review
// (0003 pass steps 1, 2, and 7). It stores head_sha, sends an approved card
// whose head moved back to human_review, and finishes the CI wait of an
// agent_review card whose review recommended approval or gave no verdict.
// A review that stopped without recording anything, and no longer holds the
// card lock, is recorded as failed first (RFC-069).
func RefreshCard(ctx context.Context, db *sql.DB, deps JobDeps, projectID, cardID string) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return card, err
	}
	if card.ProjectID != projectID || card.PRURL == "" {
		return card, nil
	}
	var attempt board.Review
	switch card.Status {
	case "agent_review":
		if attempt, err = board.LatestReview(ctx, db, card.ID); err != nil || attempt.AttemptID == "" {
			return card, err
		}
		switch attempt.Verdict {
		case "recommend_approve", review.VerdictFailed:
		case "":
			return finishStoppedReview(ctx, db, deps, projectID, card, attempt.AttemptID)
		default:
			return card, nil
		}
	case "human_review", "approved":
	default:
		return card, nil
	}
	gh, err := newGitHubReader(ctx, db, deps, projectID)
	if err != nil {
		return card, err
	}
	redact := []string{gh.token}
	pr, err := gh.read(ctx, card.PRURL)
	if err != nil {
		return card, errors.New(trace.Redact(err.Error(), redact))
	}
	if pr.Head != card.HeadSHA {
		if card, err = HeadMoved(ctx, db, card.ID, pr.Head); err != nil {
			return card, err
		}
	}
	if card.Status != "agent_review" {
		return card, nil
	}
	return settleChecks(ctx, db, deps, card, attempt.AttemptID, pr.Checks, redact)
}

// finishStoppedReview records the review of an agent_review card as failed
// when no review job holds the card lock, then applies the checks.
func finishStoppedReview(ctx context.Context, db *sql.DB, deps JobDeps, projectID string, card board.Card, attemptID string) (board.Card, error) {
	holder, err := board.NewID("lok_")
	if err != nil {
		return card, err
	}
	if err := board.AcquireLock(ctx, db, card.ID, holder); err != nil {
		if errors.Is(err, board.ErrLocked) {
			return card, nil
		}
		return card, err
	}
	defer board.ReleaseLock(context.WithoutCancel(ctx), db, card.ID, holder)
	attempt, err := board.LatestReview(ctx, db, card.ID)
	if err != nil || attempt.AttemptID != attemptID || attempt.Verdict != "" {
		return card, err
	}
	if card, err = board.GetCard(ctx, db, card.ID); err != nil || card.Status != "agent_review" {
		return card, err
	}
	gh, err := newGitHubReader(ctx, db, deps, projectID)
	if err != nil {
		return card, err
	}
	redact := []string{gh.token}
	pr, err := gh.read(ctx, card.PRURL)
	if err != nil {
		return card, errors.New(trace.Redact(err.Error(), redact))
	}
	_ = jobError(ctx, db, card, attemptID, "the review job stopped before a verdict", redact)
	return reviewFailed(ctx, db, deps, gh, settings.ProjectDir(deps.Data, projectID), card, attemptID, "", pr,
		"The review job stopped before a verdict", redact)
}
