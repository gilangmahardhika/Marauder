// Package company holds the one GitHub account the service uses for every
// project: clone, fetch, push, pull requests, and the organization repo list.
package company

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/trace"
)

var (
	ErrNoToken = errors.New("no company GitHub token is saved")
	ErrGitHub  = errors.New("github call failed")

	NamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	hostPattern = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
)

const (
	ghTimeout = 30 * time.Second
	ghMaxOut  = 4 << 20
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
type GH struct {
	Bin       string
	ConfigDir string
	Exec      func(context.Context, execx.Child) (execx.Result, error)
}

func (g GH) api(ctx context.Context, acct Account, token string, args ...string) ([]byte, error) {
	bin := g.Bin
	if bin == "" {
		found, err := exec.LookPath("gh")
		if err != nil {
			return nil, fmt.Errorf("%w: gh is not installed", ErrGitHub)
		}
		bin = found
	}
	run := g.Exec
	if run == nil {
		run = execx.Command
	}
	configDir := g.ConfigDir
	if configDir == "" {
		dir, err := os.MkdirTemp("", "orchestrator-gh-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		configDir = dir
	}
	argv := append([]string{bin, "api", "--hostname", acct.Host}, args...)
	for _, arg := range argv {
		if token != "" && strings.Contains(arg, token) {
			return nil, fmt.Errorf("%w: token in argv", ErrGitHub)
		}
	}
	env := []string{
		"GH_TOKEN=" + token,
		"GH_HOST=" + acct.Host,
		"GH_CONFIG_DIR=" + configDir,
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
		"NO_COLOR=1",
		"HOME=" + configDir,
	}
	if acct.Host != "github.com" {
		env = append(env, "GH_ENTERPRISE_TOKEN="+token)
	}
	res, err := run(ctx, execx.Child{Argv: argv, Env: env, Timeout: ghTimeout, MaxOut: ghMaxOut})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrGitHub, trace.Redact(err.Error(), []string{token}))
	}
	if res.Exit != 0 {
		return nil, fmt.Errorf("%w: %s", ErrGitHub, firstLine(trace.Redact(string(res.Out), []string{token})))
	}
	return res.Out, nil
}

// Test checks the token and the organization, then stores the token's login as the bot login.
func Test(ctx context.Context, db *sql.DB, key []byte, gh GH) (string, error) {
	acct, err := Load(ctx, db)
	if err != nil {
		return "", err
	}
	token, err := Token(ctx, db, key)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", ErrNoToken
	}
	out, err := gh.api(ctx, acct, token, "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	login := strings.TrimSpace(string(out))
	if login == "" {
		return "", fmt.Errorf("%w: no login returned", ErrGitHub)
	}
	if acct.Org != "" {
		if _, err := gh.api(ctx, acct, token, "orgs/"+acct.Org, "--jq", ".login"); err != nil {
			return "", err
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_github SET bot_login = $1 WHERE id = 1`, login); err != nil {
		return "", err
	}
	return login, nil
}

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
	out, err := gh.api(ctx, acct, token, "--paginate", "orgs/"+acct.Org+"/repos?per_page=100",
		"--jq", ".[] | {name: .name, defaultBranch: .default_branch, archived: .archived}")
	if err != nil {
		return nil, err
	}
	repos := []Repo{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 64<<10), ghMaxOut)
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
	sort.Slice(repos, func(i, j int) bool { return strings.ToLower(repos[i].Name) < strings.ToLower(repos[j].Name) })
	return repos, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	if s == "" {
		return "gh exited non-zero"
	}
	return s
}
