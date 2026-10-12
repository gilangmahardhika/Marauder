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
	file, err := os.CreateTemp(filepath.Join(dir, "logs"), "review-comment-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = r.gh.Run(ctx, r.acct.Host, r.token, "pr", "comment", url, "--body-file", file.Name())
	return err
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
		return BlockCard(ctx, db, card, "router_config", redact)
	}

	dir := settings.ProjectDir(deps.Data, projectID)
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
		return BlockCard(ctx, db, card, "review_parse", redact)
	case errors.Is(err, ErrProvider):
		_ = jobError(ctx, db, card, attempt.AttemptID, err.Error(), redact)
		return BlockCard(ctx, db, card, "provider_error", redact)
	case err != nil:
		_ = jobError(ctx, db, card, attempt.AttemptID, err.Error(), redact)
		return card, errors.New(trace.Redact(err.Error(), redact))
	}

	if err := board.RecordReview(ctx, db, attempt.AttemptID, model, result.PromptHash, independent, result.Verdict, pr.Head); err != nil {
		return card, err
	}
	if err := board.SetHeadSHA(ctx, db, card.ID, pr.Head); err != nil {
		return card, err
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
	if err := board.SetCIRequired(ctx, db, attempt.AttemptID, len(pr.Checks) > 0); err != nil {
		return card, err
	}
	return review.ApplyCI(ctx, db, card, attempt.AttemptID, pr.Checks, redact)
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
// agent_review card whose review recommended approval.
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
		if attempt, err = board.LatestReview(ctx, db, card.ID); err != nil || attempt.Verdict != "recommend_approve" {
			return card, err
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
	if err := board.SetCIRequired(ctx, db, attempt.AttemptID, len(pr.Checks) > 0); err != nil {
		return card, err
	}
	return review.ApplyCI(ctx, db, card, attempt.AttemptID, pr.Checks, redact)
}
