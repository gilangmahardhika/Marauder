package tenancy

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
)

const DefaultProject = "proj_default"

type Tribe struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Project struct {
	ID       string `json:"id"`
	TribeID  string `json:"tribeId"`
	Name     string `json:"name"`
	RepoPath string `json:"repoPath"`
}

type Member struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Admin    bool   `json:"admin"`
	Disabled bool   `json:"disabled"`
}

func CreateTribe(ctx context.Context, db *sql.DB, actor identity.User, name string) (Tribe, error) {
	if !actor.Admin {
		return Tribe{}, identity.ErrForbidden
	}
	if name == "" {
		return Tribe{}, &identity.Validation{Fields: map[string]string{"name": "name is required"}}
	}
	id, err := board.NewID("tribe_")
	if err != nil {
		return Tribe{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO tribes (id, name, created_at) VALUES ($1, $2, $3)`, id, name, now); err != nil {
		return Tribe{}, err
	}
	return Tribe{ID: id, Name: name}, nil
}

func ListTribes(ctx context.Context, db *sql.DB, actor identity.User) ([]Tribe, error) {
	var query string
	var args []any
	if actor.Admin {
		query = `SELECT id, name FROM tribes ORDER BY name`
	} else {
		query = `
			SELECT t.id, t.name FROM tribes t
			JOIN tribe_members m ON m.tribe_id = t.id
			WHERE m.user_id = $1 ORDER BY t.name`
		args = []any{actor.ID}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tribe{}
	for rows.Next() {
		var tribe Tribe
		if err := rows.Scan(&tribe.ID, &tribe.Name); err != nil {
			return nil, err
		}
		out = append(out, tribe)
	}
	return out, rows.Err()
}

func ListMembers(ctx context.Context, db *sql.DB, actor identity.User, tribeID string) ([]Member, error) {
	if !actor.Admin {
		return nil, identity.ErrForbidden
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribes WHERE id = $1`, tribeID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, identity.ErrNotFound
	}
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.email, u.role, u.admin, u.disabled
		FROM tribe_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.tribe_id = $1
		ORDER BY u.email`, tribeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.ID, &member.Email, &member.Role, &member.Admin, &member.Disabled); err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, rows.Err()
}

func AddMember(ctx context.Context, db *sql.DB, actor identity.User, tribeID, userID string) error {
	if !actor.Admin {
		return identity.ErrForbidden
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribes WHERE id = $1`, tribeID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrNotFound
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE id = $1`, userID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrNotFound
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO tribe_members (tribe_id, user_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, tribeID, userID)
	return err
}

func RemoveMember(ctx context.Context, db *sql.DB, actor identity.User, tribeID, userID string) error {
	if !actor.Admin {
		return identity.ErrForbidden
	}
	res, err := db.ExecContext(ctx, `DELETE FROM tribe_members WHERE tribe_id = $1 AND user_id = $2`, tribeID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrNotFound
	}
	return nil
}

func CreateProject(ctx context.Context, db *sql.DB, actor identity.User, tribeID, name, repoPath, dataRoot string) (Project, error) {
	if !actor.Admin {
		return Project{}, identity.ErrForbidden
	}
	if name == "" || repoPath == "" {
		return Project{}, &identity.Validation{Fields: map[string]string{"name": "name and repoPath are required"}}
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribes WHERE id = $1`, tribeID).Scan(&n); err != nil {
		return Project{}, err
	}
	if n == 0 {
		return Project{}, identity.ErrNotFound
	}
	id, err := board.NewID("proj_")
	if err != nil {
		return Project{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at) VALUES ($1, $2, $3, $4, $5)`,
		id, tribeID, name, repoPath, now); err != nil {
		return Project{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO intake_state (id, draft_pending, project_id) VALUES (1, false, $1)`, id); err != nil {
		return Project{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deploy_lock (id, project_id) VALUES (1, $1)`, id); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	if dataRoot != "" {
		dir := filepath.Join(dataRoot, "projects", id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Project{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("github:\n  host: github.com\n"), 0o600); err != nil {
			return Project{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte("{}\n"), 0o600); err != nil {
			return Project{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, "worker.lock"), nil, 0o600); err != nil {
			return Project{}, err
		}
	}
	return Project{ID: id, TribeID: tribeID, Name: name, RepoPath: repoPath}, nil
}

func ListProjects(ctx context.Context, db *sql.DB, actor identity.User) ([]Project, error) {
	var query string
	var args []any
	if actor.Admin {
		query = `SELECT id, tribe_id, name, repo_path FROM projects ORDER BY name`
	} else {
		query = `
			SELECT p.id, p.tribe_id, p.name, p.repo_path FROM projects p
			JOIN tribe_members m ON m.tribe_id = p.tribe_id
			WHERE m.user_id = $1 ORDER BY p.name`
		args = []any{actor.ID}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.TribeID, &project.Name, &project.RepoPath); err != nil {
			return nil, err
		}
		out = append(out, project)
	}
	return out, rows.Err()
}

type ProjectGitHub struct {
	Host     string
	Owner    string
	Repo     string
	Base     string
	BotLogin string
	TokenSet bool
}

func LoadProjectGitHub(ctx context.Context, db *sql.DB, projectID string) (ProjectGitHub, error) {
	var gh ProjectGitHub
	var cipher string
	err := db.QueryRowContext(ctx, `
		SELECT github_host, github_owner, github_repo, github_base, bot_login, bot_token_ciphertext
		FROM projects WHERE id = $1`, projectID).Scan(&gh.Host, &gh.Owner, &gh.Repo, &gh.Base, &gh.BotLogin, &cipher)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectGitHub{}, identity.ErrNotFound
	}
	if err != nil {
		return ProjectGitHub{}, err
	}
	gh.TokenSet = cipher != ""
	return gh, nil
}

func SaveProjectGitHub(ctx context.Context, db *sql.DB, actor identity.User, projectID string, gh ProjectGitHub, token *string, clear bool, key []byte) error {
	if actor.Role != "lead_developer" {
		return identity.ErrForbidden
	}
	if err := Visible(ctx, db, projectID, actor.ID); err != nil {
		return err
	}
	current, err := LoadProjectGitHub(ctx, db, projectID)
	if err != nil {
		return err
	}
	var cipher string
	if err := db.QueryRowContext(ctx, `SELECT bot_token_ciphertext FROM projects WHERE id = $1`, projectID).Scan(&cipher); err != nil {
		return err
	}
	if clear {
		cipher = ""
	} else if token != nil {
		if len(*token) < 8 {
			return &identity.Validation{Fields: map[string]string{"token": "token is too short"}}
		}
		sealed, err := identity.Seal(key, []byte(*token))
		if err != nil {
			return err
		}
		cipher = sealed
	}
	if gh.Host == "" {
		gh.Host = current.Host
	}
	_, err = db.ExecContext(ctx, `
		UPDATE projects
		SET github_host = $2, github_owner = $3, github_repo = $4, github_base = $5, bot_login = $6, bot_token_ciphertext = $7
		WHERE id = $1`, projectID, gh.Host, gh.Owner, gh.Repo, gh.Base, gh.BotLogin, cipher)
	return err
}

func ProjectBotToken(ctx context.Context, db *sql.DB, projectID string, key []byte) (string, error) {
	var cipher string
	err := db.QueryRowContext(ctx, `SELECT bot_token_ciphertext FROM projects WHERE id = $1`, projectID).Scan(&cipher)
	if errors.Is(err, sql.ErrNoRows) {
		return "", identity.ErrNotFound
	}
	if err != nil || cipher == "" {
		return "", err
	}
	return identity.OpenToken(key, cipher)
}

func Visible(ctx context.Context, db *sql.DB, projectID, userID string) error {
	var one int
	err := db.QueryRowContext(ctx, `
		SELECT 1 FROM projects p
		JOIN tribe_members m ON m.tribe_id = p.tribe_id
		WHERE p.id = $1 AND m.user_id = $2`, projectID, userID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.ErrNotFound
	}
	return err
}
