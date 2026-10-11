// Package company holds the one GitHub account the service uses for every
// project: clone, fetch, and the organization repo list. Pushes and pull
// requests use the card owner's token (RFC-054).
package company

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"orchestrator/backend/internal/ghcli"
	"orchestrator/backend/internal/identity"
)

var (
	ErrNoToken = errors.New("no company GitHub token is saved")
	ErrGitHub  = ghcli.ErrGitHub

	NamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	hostPattern = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
)

type Account struct {
	Host     string `json:"host"`
	Org      string `json:"org"`
	BotLogin string `json:"botLogin"`
	TokenSet bool   `json:"tokenSet"`
}

type Repo struct {
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
	Archived      bool   `json:"archived"`
	Private       bool   `json:"private"`
}

// CloneURL is the only remote the service talks to for a project.
func (a Account) CloneURL(repo string) string {
	return "https://" + a.Host + "/" + a.Org + "/" + repo + ".git"
}

func Load(ctx context.Context, db *sql.DB) (Account, error) {
	var acct Account
	var cipher string
	err := db.QueryRowContext(ctx, `
		SELECT host, org, bot_login, token_ciphertext FROM workspace_github WHERE id = 1`).
		Scan(&acct.Host, &acct.Org, &acct.BotLogin, &cipher)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{Host: "github.com"}, nil
	}
	if err != nil {
		return Account{}, err
	}
	if acct.Host == "" {
		acct.Host = "github.com"
	}
	acct.TokenSet = cipher != ""
	return acct, nil
}

// Save is admin only. A new host, org, or token clears the bot login until the next test.
func Save(ctx context.Context, db *sql.DB, actor identity.User, host, org string, token *string, clear bool, key []byte) (Account, error) {
	if !actor.Admin {
		return Account{}, identity.ErrForbidden
	}
	host = strings.TrimSpace(host)
	org = strings.TrimSpace(org)
	if host == "" {
		host = "github.com"
	}
	fields := map[string]string{}
	if !hostPattern.MatchString(host) {
		fields["host"] = "host must be a hostname, with no scheme or path"
	}
	if org != "" && !NamePattern.MatchString(org) {
		fields["org"] = "organization may use letters, digits, '.', '_', and '-'"
	}
	if token != nil && !clear && len(*token) < 8 {
		fields["token"] = "token is too short"
	}
	if len(fields) > 0 {
		return Account{}, &identity.Validation{Fields: fields}
	}
	current, err := Load(ctx, db)
	if err != nil {
		return Account{}, err
	}
	var cipher string
	if err := db.QueryRowContext(ctx, `SELECT token_ciphertext FROM workspace_github WHERE id = 1`).Scan(&cipher); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Account{}, err
	}
	botLogin := current.BotLogin
	if host != current.Host || org != current.Org {
		botLogin = ""
	}
	if clear {
		cipher = ""
		botLogin = ""
	} else if token != nil {
		sealed, err := identity.Seal(key, []byte(*token))
		if err != nil {
			return Account{}, err
		}
		cipher = sealed
		botLogin = ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspace_github (id, host, org, bot_login, token_ciphertext, updated_at)
		VALUES (1, $1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			host = EXCLUDED.host, org = EXCLUDED.org, bot_login = EXCLUDED.bot_login,
			token_ciphertext = EXCLUDED.token_ciphertext, updated_at = EXCLUDED.updated_at`,
		host, org, botLogin, cipher, now); err != nil {
		return Account{}, err
	}
	return Load(ctx, db)
}

func Token(ctx context.Context, db *sql.DB, key []byte) (string, error) {
	var cipher string
	err := db.QueryRowContext(ctx, `SELECT token_ciphertext FROM workspace_github WHERE id = 1`).Scan(&cipher)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && cipher == "") {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return identity.OpenToken(key, cipher)
}

// GH runs the gh CLI with the company token in the child environment only.
type GH = ghcli.Runner

func api(ctx context.Context, g GH, acct Account, token string, args ...string) ([]byte, error) {
	return g.API(ctx, acct.Host, token, args...)
}

// Access says what the company token can see. Hint names the most likely
// reason when it sees too little, and is empty otherwise.
type Access struct {
	Login        string   `json:"login"`
	TokenKind    string   `json:"tokenKind"`
	Scopes       []string `json:"scopes"`
	RepoCount    int      `json:"repoCount"`
	PrivateCount int      `json:"privateCount"`
	Hint         string   `json:"hint"`
}

// Test checks the token and the organization, stores the token's login as
// the bot login, and reports which repositories of the organization it sees.
func Test(ctx context.Context, db *sql.DB, key []byte, gh GH) (Access, error) {
	acct, err := Load(ctx, db)
	if err != nil {
		return Access{}, err
	}
	token, err := Token(ctx, db, key)
	if err != nil {
		return Access{}, err
	}
	if token == "" {
		return Access{}, ErrNoToken
	}
	out, err := api(ctx, gh, acct, token, "--include", "user")
	if err != nil {
		return Access{}, err
	}
	access := Access{TokenKind: tokenKind(token), Scopes: []string{}}
	headers, body := splitResponse(string(out))
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal([]byte(body), &user); err != nil || user.Login == "" {
		return Access{}, fmt.Errorf("%w: no login returned", ErrGitHub)
	}
	access.Login = user.Login
	if access.TokenKind != "fine-grained" {
		access.Scopes = oauthScopes(headers)
	}
	if acct.Org != "" {
		if _, err := api(ctx, gh, acct, token, "orgs/"+acct.Org, "--jq", ".login"); err != nil {
			return Access{}, err
		}
		repos, err := listRepos(ctx, gh, acct, token)
		if err != nil {
			return Access{}, err
		}
		access.RepoCount = len(repos)
		for _, repo := range repos {
			if repo.Private {
				access.PrivateCount++
			}
		}
		access.Hint = accessHint(access, acct.Org)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_github SET bot_login = $1 WHERE id = 1`, access.Login); err != nil {
		return Access{}, err
	}
	return access, nil
}

func tokenKind(token string) string {
	switch {
	case strings.HasPrefix(token, "ghp_"):
		return "classic"
	case strings.HasPrefix(token, "github_pat_"):
		return "fine-grained"
	default:
		return "other"
	}
}

func splitResponse(out string) (headers, body string) {
	out = strings.ReplaceAll(out, "\r\n", "\n")
	if i := strings.Index(out, "\n\n"); i >= 0 {
		return out[:i], strings.TrimSpace(out[i+2:])
	}
	return "", strings.TrimSpace(out)
}

func oauthScopes(headers string) []string {
	scopes := []string{}
	for _, line := range strings.Split(headers, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "X-OAuth-Scopes") {
			continue
		}
		for _, scope := range strings.Split(value, ",") {
			if scope = strings.TrimSpace(scope); scope != "" {
				scopes = append(scopes, scope)
			}
		}
	}
	return scopes
}

func accessHint(a Access, org string) string {
	switch {
	case a.TokenKind == "classic" && !slices.Contains(a.Scopes, "repo"):
		return "The token has no repo scope, so private repositories are hidden."
	case a.RepoCount > 0:
		return ""
	case a.TokenKind == "fine-grained":
		return "Set the token's resource owner to " + org + " and give it access to the repositories, with Metadata read, Contents read and write, and Pull requests read and write."
	default:
		return "The token sees no repositories in " + org + ". If the organization uses SAML SSO, authorize the token for it."
	}
}

// Repos lists the organization's repositories the company token can read:
// the organization list, plus repositories of the organization the token's
// user reaches as a collaborator.
func Repos(ctx context.Context, db *sql.DB, key []byte, gh GH) ([]Repo, error) {
	acct, err := Load(ctx, db)
	if err != nil {
		return nil, err
	}
	if acct.Org == "" {
		return nil, &identity.Validation{Fields: map[string]string{"org": "set the company organization first"}}
	}
	token, err := Token(ctx, db, key)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrNoToken
	}
	return listRepos(ctx, gh, acct, token)
}

const repoFields = "{name: .name, defaultBranch: .default_branch, archived: .archived, private: .private}"

func listRepos(ctx context.Context, gh GH, acct Account, token string) ([]Repo, error) {
	out, err := api(ctx, gh, acct, token, "--paginate", "orgs/"+acct.Org+"/repos?type=all&per_page=100",
		"--jq", ".[] | "+repoFields)
	if err != nil {
		return nil, err
	}
	repos, err := parseRepos(out)
	if err != nil {
		return nil, err
	}
	owned := `.[] | select(.owner.login | ascii_downcase == "` + strings.ToLower(acct.Org) + `") | ` + repoFields
	more, err := api(ctx, gh, acct, token, "--paginate", "user/repos?affiliation=owner,collaborator,organization_member&per_page=100",
		"--jq", owned)
	if err != nil {
		return nil, err
	}
	extra, err := parseRepos(more)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, repo := range repos {
		seen[strings.ToLower(repo.Name)] = true
	}
	for _, repo := range extra {
		if !seen[strings.ToLower(repo.Name)] {
			seen[strings.ToLower(repo.Name)] = true
			repos = append(repos, repo)
		}
	}
	sort.Slice(repos, func(i, j int) bool { return strings.ToLower(repos[i].Name) < strings.ToLower(repos[j].Name) })
	return repos, nil
}

func parseRepos(out []byte) ([]Repo, error) {
	repos := []Repo{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 64<<10), ghcli.MaxOut)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var repo Repo
		if err := json.Unmarshal([]byte(line), &repo); err != nil {
			return nil, fmt.Errorf("%w: unexpected repo list", ErrGitHub)
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// CheckRepo asks GitHub, with the company token, whether repo is an
// unarchived repository of the company organization with branch base. It
// returns the name GitHub reports. Answers GitHub gives are validation errors
// on repo or baseBranch; a failed call is ErrGitHub.
func CheckRepo(ctx context.Context, db *sql.DB, key []byte, gh GH, repo, base string) (string, error) {
	acct, err := Load(ctx, db)
	if err != nil {
		return "", err
	}
	if acct.Org == "" {
		return "", repoField("an admin must set the company GitHub organization first")
	}
	token, err := Token(ctx, db, key)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", repoField("an admin must save the company GitHub token first")
	}
	out, err := api(ctx, gh, acct, token, "repos/"+acct.Org+"/"+repo,
		"--jq", "{name: .name, owner: .owner.login, archived: .archived}")
	if errors.Is(err, ghcli.ErrNotFound) {
		return "", repoField("no repository " + repo + " in " + acct.Org + " on " + acct.Host)
	}
	if err != nil {
		return "", err
	}
	var found struct {
		Name     string `json:"name"`
		Owner    string `json:"owner"`
		Archived bool   `json:"archived"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &found); err != nil || found.Name == "" {
		return "", fmt.Errorf("%w: unexpected repository reply", ErrGitHub)
	}
	if !strings.EqualFold(found.Owner, acct.Org) {
		return "", repoField(repo + " has moved out of " + acct.Org)
	}
	if found.Archived {
		return "", repoField(found.Name + " is archived")
	}
	_, err = api(ctx, gh, acct, token, "repos/"+acct.Org+"/"+found.Name+"/branches/"+branchPath(base), "--jq", ".name")
	if errors.Is(err, ghcli.ErrNotFound) {
		return "", &identity.Validation{Fields: map[string]string{"baseBranch": "no branch " + base + " in " + found.Name}}
	}
	if err != nil {
		return "", err
	}
	return found.Name, nil
}

func repoField(msg string) error {
	return &identity.Validation{Fields: map[string]string{"repo": msg}}
}

func branchPath(branch string) string {
	parts := strings.Split(branch, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
