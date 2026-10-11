package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/ghcli"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/machine"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/trace"
	"orchestrator/backend/internal/vcs"
)

var (
	// ErrOwnerGitHub means the card owner cannot push: no owner, a disabled
	// owner, an unverified login, or no token (RFC-054).
	ErrOwnerGitHub = errors.New("the card owner has no verified GitHub token")
	ErrGitHubSetup = errors.New("the company organization or the project repository is missing")
)

var pullURL = regexp.MustCompile(`^https://[^\s/]+/[^\s/]+/[^\s/]+/pull/([0-9]+)$`)

// Owner is the card owner as the API shows it.
type Owner struct {
	ID          string
	Name        string
	Email       string
	GitHubLogin string
}

// OwnerOf returns the owner for display. ok is false when the card has none.
func OwnerOf(ctx context.Context, db *sql.DB, userID string) (Owner, bool, error) {
	if userID == "" {
		return Owner{}, false, nil
	}
	var owner Owner
	err := db.QueryRowContext(ctx, `
		SELECT u.id, u.name, u.email, CASE WHEN g.verified_at IS NOT NULL THEN g.login ELSE '' END
		FROM users u LEFT JOIN user_github g ON g.user_id = u.id
		WHERE u.id = $1`, userID).Scan(&owner.ID, &owner.Name, &owner.Email, &owner.GitHubLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return Owner{}, false, nil
	}
	return owner, err == nil, err
}

// OwnerIdentity is what a job needs to commit and push as the card owner.
type OwnerIdentity struct {
	UserID string
	Login  string
	Author runtime.Author
	Token  string
}

// OwnerGitHub resolves the owner's verified login and token on host. The token
// is plaintext; the caller adds it to the job's redaction list.
func OwnerGitHub(ctx context.Context, db *sql.DB, key []byte, card board.Card, host string) (OwnerIdentity, error) {
	if card.OwnerID == "" {
		return OwnerIdentity{}, ErrOwnerGitHub
	}
	var disabled bool
	err := db.QueryRowContext(ctx, `SELECT disabled FROM users WHERE id = $1`, card.OwnerID).Scan(&disabled)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && disabled) {
		return OwnerIdentity{}, ErrOwnerGitHub
	}
	if err != nil {
		return OwnerIdentity{}, err
	}
	acct, err := identity.GitHubOf(ctx, db, card.OwnerID)
	if err != nil {
		return OwnerIdentity{}, err
	}
	if !acct.Verified || !acct.TokenSet || acct.Login == "" || (host != "" && !strings.EqualFold(acct.Host, host)) {
		return OwnerIdentity{}, ErrOwnerGitHub
	}
	token, err := identity.Token(ctx, db, card.OwnerID, key)
	if err != nil {
		return OwnerIdentity{}, err
	}
	if token == "" {
		return OwnerIdentity{}, ErrOwnerGitHub
	}
	return OwnerIdentity{
		UserID: card.OwnerID,
		Login:  acct.Login,
		Author: runtime.LoginAuthor(acct.Login, acct.ID),
		Token:  token,
	}, nil
}

// BlockCard moves the card to blocked with reason when its status has that edge.
func BlockCard(ctx context.Context, db *sql.DB, card board.Card, reason string, secrets []string) (board.Card, error) {
	if _, ok := machine.Edges()[[2]string{card.Status, "blocked"}]; !ok {
		return card, nil
	}
	return move(ctx, db, card, "blocked", machine.Evidence{Reason: reason}, secrets)
}

// CardReviewers are the verified GitHub logins named on the card's
// requirements, without the owner, the company bot, disabled users, and
// users who left the project's tribe.
func CardReviewers(ctx context.Context, db *sql.DB, card board.Card, host, botLogin string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, reqID := range card.RequirementIDs {
		rows, err := db.QueryContext(ctx, `
			SELECT g.login FROM requirement_reviewers rr
			JOIN users u ON u.id = rr.user_id AND u.disabled = false
			JOIN user_github g ON g.user_id = rr.user_id AND g.verified_at IS NOT NULL AND g.login <> ''
			JOIN projects p ON p.id = rr.project_id
			JOIN tribe_members m ON m.tribe_id = p.tribe_id AND m.user_id = rr.user_id
			WHERE rr.project_id = $1 AND rr.requirement_id = $2 AND rr.user_id <> $3 AND lower(g.host) = lower($4)`,
			card.ProjectID, reqID, card.OwnerID, host)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var login string
			if err := rows.Scan(&login); err != nil {
				rows.Close()
				return nil, err
			}
			lower := strings.ToLower(login)
			if seen[lower] || (botLogin != "" && lower == strings.ToLower(botLogin)) {
				continue
			}
			seen[lower] = true
			out = append(out, login)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// CardReviewerUsers lists the reviewers named on each card's requirements in
// projectID, keyed by card id, without the card owner, disabled users, and
// users who left the project's tribe. cardID "" lists every card in the
// project in one query.
func CardReviewerUsers(ctx context.Context, db *sql.DB, projectID, cardID string) (map[string][]intake.Reviewer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT c.id, u.id, u.name, u.email,
			CASE WHEN g.verified_at IS NOT NULL THEN g.login ELSE '' END
		FROM cards c
		CROSS JOIN LATERAL jsonb_array_elements_text(c.requirement_ids_json::jsonb) AS req(id)
		JOIN requirement_reviewers rr ON rr.project_id = c.project_id AND rr.requirement_id = req.id
		JOIN users u ON u.id = rr.user_id AND u.disabled = false
		JOIN projects p ON p.id = c.project_id
		JOIN tribe_members m ON m.tribe_id = p.tribe_id AND m.user_id = u.id
		LEFT JOIN user_github g ON g.user_id = u.id
		WHERE c.project_id = $1 AND ($2 = '' OR c.id = $2) AND u.id <> COALESCE(c.owner_id, '')
		ORDER BY c.id, u.email`, projectID, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]intake.Reviewer{}
	for rows.Next() {
		var id string
		var r intake.Reviewer
		if err := rows.Scan(&id, &r.ID, &r.Name, &r.Email, &r.GitHubLogin); err != nil {
			return nil, err
		}
		out[id] = append(out[id], r)
	}
	return out, rows.Err()
}

type PublishInput struct {
	Data      string
	Key       []byte
	Tools     Tools
	ExitCode  int
	AttemptID string
	Summary   string
	Metadata  string
	Secrets   []string
}

// Publish is the pull request step of the implement job (RFC-054). It pushes
// orch/<card> and opens or updates the pull request with the owner's token,
// moves self_check to pr_open, then requests the requirement reviewers. When
// the owner cannot push, the card is blocked with owner_github and nothing
// reaches GitHub.
func Publish(ctx context.Context, db *sql.DB, cardID string, in PublishInput) (board.Card, error) {
	card, err := board.GetCard(ctx, db, cardID)
	if err != nil {
		return board.Card{}, err
	}
	if card.Status != "self_check" {
		return card, machine.ErrIllegal
	}
	project, err := tenancy.GetProject(ctx, db, card.ProjectID)
	if err != nil {
		return card, err
	}
	acct, err := company.Load(ctx, db)
	if err != nil {
		return card, err
	}
	if acct.Org == "" || project.Repo == "" {
		blocked, err := BlockCard(ctx, db, card, "github_config", in.Secrets)
		if err != nil {
			return blocked, err
		}
		return blocked, ErrGitHubSetup
	}
	owner, err := OwnerGitHub(ctx, db, in.Key, card, acct.Host)
	if errors.Is(err, ErrOwnerGitHub) {
		blocked, blockErr := BlockCard(ctx, db, card, "owner_github", in.Secrets)
		if blockErr != nil {
			return blocked, blockErr
		}
		return blocked, ErrOwnerGitHub
	}
	if err != nil {
		return card, err
	}
	secrets := append(append([]string{}, in.Secrets...), owner.Token)

	git := ProjectGit(in.Data, project, owner.Author, in.Tools)
	if err := git.Push(ctx, card.ID, runtime.Remote{URL: acct.CloneURL(project.Repo), Token: owner.Token}); err != nil {
		return card, errors.New(trace.Redact(err.Error(), secrets))
	}
	gh := ghcli.Runner{Bin: in.Tools.GH, Exec: in.Tools.Exec, ConfigDir: filepath.Join(git.Data, "empty-gh")}
	prURL, err := ensurePR(ctx, gh, acct, project, card, owner.Token, in, git.Data)
	if err != nil {
		return card, errors.New(trace.Redact(err.Error(), secrets))
	}
	exit := in.ExitCode
	next, err := move(ctx, db, card, "pr_open", machine.Evidence{ExitCode: &exit, PRURL: prURL, AttemptID: in.AttemptID}, secrets)
	if err != nil {
		return card, err
	}
	if err := requestReviewers(ctx, db, gh, acct, project, next, owner.Token, prURL, secrets); err != nil {
		return next, err
	}
	return next, nil
}

func ensurePR(ctx context.Context, gh ghcli.Runner, acct company.Account, project tenancy.Project, card board.Card, token string, in PublishInput, dir string) (string, error) {
	previous := ""
	if card.PRURL != "" {
		out, err := gh.Run(ctx, acct.Host, token, "pr", "view", card.PRURL, "--json", "body", "--jq", ".body")
		if err != nil {
			return "", err
		}
		previous = strings.TrimRight(string(out), "\n")
	}
	body := vcs.RewriteBody(previous, in.Summary, in.Metadata)
	file, err := os.CreateTemp(filepath.Join(dir, "logs"), "pr-body-*.md")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(body); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if card.PRURL != "" {
		if _, err := gh.Run(ctx, acct.Host, token, "pr", "edit", card.PRURL, "--body-file", file.Name()); err != nil {
			return "", err
		}
		return card.PRURL, nil
	}
	base := project.BaseBranch
	if base == "" {
		base = "main"
	}
	out, err := gh.Run(ctx, acct.Host, token, "pr", "create",
		"--repo", acct.Org+"/"+project.Repo, "--base", base, "--head", "orch/"+card.ID,
		"--title", card.Title, "--body-file", file.Name())
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	url := strings.TrimSpace(lines[len(lines)-1])
	if !pullURL.MatchString(url) {
		return "", fmt.Errorf("%w: no pull request url", ghcli.ErrGitHub)
	}
	return url, nil
}

// requestReviewers asks GitHub for the card's reviewers. A failure is an
// event, never a status change.
func requestReviewers(ctx context.Context, db *sql.DB, gh ghcli.Runner, acct company.Account, project tenancy.Project, card board.Card, token, prURL string, secrets []string) error {
	logins, err := CardReviewers(ctx, db, card, acct.Host, acct.BotLogin)
	if err != nil {
		return err
	}
	if len(logins) == 0 {
		return nil
	}
	payload := map[string]any{"reviewers": logins, "ok": true}
	match := pullURL.FindStringSubmatch(prURL)
	if match == nil {
		payload["ok"] = false
		payload["message"] = "pull request url has no number"
	} else {
		args := []string{"-X", "POST", "repos/" + acct.Org + "/" + project.Repo + "/pulls/" + match[1] + "/requested_reviewers"}
		for _, login := range logins {
			args = append(args, "-f", "reviewers[]="+login)
		}
		if _, err := gh.API(ctx, acct.Host, token, args...); err != nil {
			payload["ok"] = false
			payload["message"] = err.Error()
		}
	}
	_, err = trace.Append(ctx, db, trace.Event{
		CardID: card.ID, ProjectID: card.ProjectID, Kind: "reviewers_requested", Payload: payload,
	}, secrets)
	return err
}

func move(ctx context.Context, db *sql.DB, card board.Card, to string, ev machine.Evidence, secrets []string) (board.Card, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return card, err
	}
	defer tx.Rollback()
	next, err := machine.Transition(ctx, tx, card, to, ev, secrets)
	if err != nil {
		if errors.Is(err, machine.ErrIllegal) || errors.Is(err, machine.ErrEvidence) {
			_ = tx.Commit()
		}
		return card, err
	}
	if err := tx.Commit(); err != nil {
		return card, err
	}
	return next, nil
}

// OwnerGit is the project's git runner with the card owner as commit author.
// When the owner cannot push, the card is blocked with owner_github where its
// status allows it.
func OwnerGit(ctx context.Context, db *sql.DB, key []byte, data string, card board.Card, tools Tools) (runtime.Git, OwnerIdentity, error) {
	project, err := tenancy.GetProject(ctx, db, card.ProjectID)
	if err != nil {
		return runtime.Git{}, OwnerIdentity{}, err
	}
	acct, err := company.Load(ctx, db)
	if err != nil {
		return runtime.Git{}, OwnerIdentity{}, err
	}
	owner, err := OwnerGitHub(ctx, db, key, card, acct.Host)
	if errors.Is(err, ErrOwnerGitHub) {
		if _, blockErr := BlockCard(ctx, db, card, "owner_github", nil); blockErr != nil {
			return runtime.Git{}, OwnerIdentity{}, blockErr
		}
		return runtime.Git{}, OwnerIdentity{}, ErrOwnerGitHub
	}
	if err != nil {
		return runtime.Git{}, OwnerIdentity{}, err
	}
	return ProjectGit(data, project, owner.Author, tools), owner, nil
}
