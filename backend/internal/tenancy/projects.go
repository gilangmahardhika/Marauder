package tenancy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/settings"
)

const maxSharedPaths = 20

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

const defaultProjectConfig = "paths:\n  prd: docs/prd\n  rfc: docs/rfc\n"

// RepoCheck confirms a repository and base branch with GitHub and returns
// the repository name GitHub reports. Nil skips the check.
type RepoCheck func(ctx context.Context, repo, base string) (string, error)

type NewProject struct {
	TribeID    string
	Name       string
	Repo       string
	BaseBranch string
	Links      []LinkInput
}

// CreateProject stores a project and its links in one transaction. A company
// admin or a tribe admin of the project's tribe may call it (RFC-057).
func CreateProject(ctx context.Context, db *sql.DB, actor identity.User, in NewProject, dataRoot string, check RepoCheck) (Project, error) {
	ok, err := CanManageTribe(ctx, db, actor, in.TribeID)
	if err != nil {
		return Project{}, err
	}
	if !ok {
		return Project{}, identity.ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Repo = strings.TrimSpace(in.Repo)
	in.BaseBranch = strings.TrimSpace(in.BaseBranch)
	if in.BaseBranch == "" {
		in.BaseBranch = "main"
	}
	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "name is required"
	}
	if msg := checkRepo(in.Repo); msg != "" {
		fields["repo"] = msg
	}
	if !runtime.ValidBranch(in.BaseBranch) {
		fields["baseBranch"] = "base branch is not a valid branch name"
	}
	if len(fields) > 0 {
		return Project{}, &identity.Validation{Fields: fields}
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tribes WHERE id = $1`, in.TribeID).Scan(&n); err != nil {
		return Project{}, err
	}
	if n == 0 {
		return Project{}, &identity.Validation{Fields: map[string]string{"tribeId": "unknown tribe"}}
	}
	if check != nil {
		if in.Repo, err = check(ctx, in.Repo, in.BaseBranch); err != nil {
			return Project{}, err
		}
	}
	if err := repoFree(ctx, db, in.Repo, ""); err != nil {
		return Project{}, err
	}
	links, err := cleanLinks(ctx, db, in.TribeID, "", in.Links)
	if err != nil {
		return Project{}, err
	}
	id, err := board.NewID("proj_")
	if err != nil {
		return Project{}, err
	}
	repoPath := ""
	if dataRoot != "" {
		repoPath = filepath.Join(dataRoot, "projects", id, "repo")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at, github_repo, github_base)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, in.TribeID, in.Name, repoPath, now, in.Repo, in.BaseBranch); err != nil {
		if uniqueRepo(err) {
			return Project{}, repoTaken()
		}
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
	if err := insertLinks(ctx, tx, id, links, now); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	if dataRoot != "" {
		if err := PrepareProject(dataRoot, id); err != nil {
			return Project{}, err
		}
	}
	return Project{ID: id, TribeID: in.TribeID, Name: in.Name, Repo: in.Repo, BaseBranch: in.BaseBranch, RepoPath: repoPath}, nil
}

// PrepareProject creates <data>/projects/<id>/ with its subdirectories and
// starter files. Existing files are kept.
func PrepareProject(dataRoot, projectID string) error {
	dir := settings.ProjectDir(dataRoot, projectID)
	if err := runtime.PrepareProjectDir(dir); err != nil {
		return err
	}
	starters := map[string]string{
		"config.yaml":  defaultProjectConfig,
		"secrets.json": "{}\n",
		"worker.lock":  "",
	}
	for name, body := range starters {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// AdoptProjects points every project at its clone under dataRoot and creates
// the project directories. Rows from before RFC-050 held a user-entered path.
func AdoptProjects(ctx context.Context, db *sql.DB, dataRoot string) error {
	rows, err := db.QueryContext(ctx, `SELECT id, repo_path FROM projects`)
	if err != nil {
		return err
	}
	type row struct{ id, path string }
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.path); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range all {
		want := filepath.Join(dataRoot, "projects", r.id, "repo")
		if r.path != want {
			if _, err := db.ExecContext(ctx, `UPDATE projects SET repo_path = $2 WHERE id = $1`, r.id, want); err != nil {
				return err
			}
		}
		if err := PrepareProject(dataRoot, r.id); err != nil {
			return err
		}
	}
	return nil
}

func ListProjects(ctx context.Context, db *sql.DB, actor identity.User) ([]Project, error) {
	var query string
	var args []any
	if actor.Admin {
		query = `SELECT id, tribe_id, name, github_repo, github_base, repo_path FROM projects ORDER BY name`
	} else {
		query = `
			SELECT p.id, p.tribe_id, p.name, p.github_repo, p.github_base, p.repo_path FROM projects p
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
		if err := rows.Scan(&project.ID, &project.TribeID, &project.Name, &project.Repo, &project.BaseBranch, &project.RepoPath); err != nil {
			return nil, err
		}
		out = append(out, project)
	}
	return out, rows.Err()
}

func GetProject(ctx context.Context, db *sql.DB, projectID string) (Project, error) {
	var project Project
	err := db.QueryRowContext(ctx, `
		SELECT id, tribe_id, name, github_repo, github_base, repo_path FROM projects WHERE id = $1`, projectID).
		Scan(&project.ID, &project.TribeID, &project.Name, &project.Repo, &project.BaseBranch, &project.RepoPath)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, identity.ErrNotFound
	}
	return project, err
}

// SaveRepository sets the project's repository and base branch for whoever
// may edit the project's settings. check runs when either one changes.
// changed is true when the repository name moved.
func SaveRepository(ctx context.Context, db *sql.DB, actor identity.User, projectID, repo, base, dataRoot string, check RepoCheck) (project Project, changed bool, err error) {
	if err := requireSettingsEditor(ctx, db, actor, projectID); err != nil {
		return Project{}, false, err
	}
	repo = strings.TrimSpace(repo)
	base = strings.TrimSpace(base)
	if base == "" {
		base = "main"
	}
	fields := map[string]string{}
	if msg := checkRepo(repo); msg != "" {
		fields["repo"] = msg
	}
	if !runtime.ValidBranch(base) {
		fields["baseBranch"] = "base branch is not a valid branch name"
	}
	if len(fields) > 0 {
		return Project{}, false, &identity.Validation{Fields: fields}
	}
	current, err := GetProject(ctx, db, projectID)
	if err != nil {
		return Project{}, false, err
	}
	if check != nil && (repo != current.Repo || base != current.BaseBranch) {
		if repo, err = check(ctx, repo, base); err != nil {
			return Project{}, false, err
		}
	}
	if err := repoFree(ctx, db, repo, projectID); err != nil {
		return Project{}, false, err
	}
	repoPath := current.RepoPath
	if repoPath == "" && dataRoot != "" {
		repoPath = filepath.Join(dataRoot, "projects", projectID, "repo")
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE projects SET github_repo = $2, github_base = $3, repo_path = $4 WHERE id = $1`,
		projectID, repo, base, repoPath); err != nil {
		if uniqueRepo(err) {
			return Project{}, false, repoTaken()
		}
		return Project{}, false, err
	}
	next, err := GetProject(ctx, db, projectID)
	return next, current.Repo != repo, err
}

func checkRepo(repo string) string {
	if repo == "" {
		return "repository is required"
	}
	if !repoPattern.MatchString(repo) || repo == "." || repo == ".." {
		return "repository may use letters, digits, '.', '_', and '-'"
	}
	return ""
}

func repoFree(ctx context.Context, db *sql.DB, repo, self string) error {
	var n int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM projects WHERE github_repo = $1 AND id <> $2`, repo, self).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return repoTaken()
	}
	return nil
}

func repoTaken() error {
	return &identity.Validation{Fields: map[string]string{"repo": "another project already uses this repository"}}
}

func uniqueRepo(err error) bool {
	return err != nil && strings.Contains(err.Error(), "projects_repo")
}

// Link is one project this project reads from.
type Link struct {
	ProjectID   string   `json:"projectId"`
	Name        string   `json:"name"`
	Repo        string   `json:"repo"`
	BaseBranch  string   `json:"-"`
	RepoPath    string   `json:"-"`
	SharedPaths []string `json:"sharedPaths"`
}

type LinkInput struct {
	ProjectID   string   `json:"projectId"`
	SharedPaths []string `json:"sharedPaths"`
}

func ListLinks(ctx context.Context, db *sql.DB, projectID string) ([]Link, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT p.id, p.name, p.github_repo, p.github_base, p.repo_path, l.shared_paths_json
		FROM project_links l
		JOIN projects p ON p.id = l.linked_project_id
		WHERE l.project_id = $1
		ORDER BY p.name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Link{}
	for rows.Next() {
		var link Link
		var raw string
		if err := rows.Scan(&link.ProjectID, &link.Name, &link.Repo, &link.BaseBranch, &link.RepoPath, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &link.SharedPaths); err != nil || link.SharedPaths == nil {
			link.SharedPaths = []string{}
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

// SaveLinks replaces the project's links. Whoever may edit the project's
// settings may call it (RFC-059).
func SaveLinks(ctx context.Context, db *sql.DB, actor identity.User, projectID string, links []LinkInput) ([]Link, error) {
	if err := requireSettingsEditor(ctx, db, actor, projectID); err != nil {
		return nil, err
	}
	self, err := GetProject(ctx, db, projectID)
	if err != nil {
		return nil, err
	}
	clean, err := cleanLinks(ctx, db, self.TribeID, projectID, links)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_links WHERE project_id = $1`, projectID); err != nil {
		return nil, err
	}
	if err := insertLinks(ctx, tx, projectID, clean, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ListLinks(ctx, db, projectID)
}

// cleanLinks checks each link: an existing project in tribeID, not selfID,
// listed once, with at most 20 shared paths that stay inside its repo.
func cleanLinks(ctx context.Context, db *sql.DB, tribeID, selfID string, links []LinkInput) ([]LinkInput, error) {
	seen := map[string]bool{}
	clean := make([]LinkInput, 0, len(links))
	for _, link := range links {
		id := strings.TrimSpace(link.ProjectID)
		if selfID != "" && id == selfID {
			return nil, &identity.Validation{Fields: map[string]string{"links": "a project cannot link to itself"}}
		}
		if seen[id] {
			return nil, &identity.Validation{Fields: map[string]string{"links": "a project is linked twice"}}
		}
		seen[id] = true
		other, err := GetProject(ctx, db, id)
		if errors.Is(err, identity.ErrNotFound) {
			return nil, &identity.Validation{Fields: map[string]string{"links": "unknown project"}}
		}
		if err != nil {
			return nil, err
		}
		if other.TribeID != tribeID {
			return nil, &identity.Validation{Fields: map[string]string{"links": "linked projects must be in the same tribe"}}
		}
		if len(link.SharedPaths) > maxSharedPaths {
			return nil, &identity.Validation{Fields: map[string]string{"sharedPaths": "at most 20 shared paths per link"}}
		}
		paths := []string{}
		dup := map[string]bool{}
		for _, raw := range link.SharedPaths {
			p := strings.TrimSpace(raw)
			if p == "" {
				continue
			}
			if !settings.RelativeInside(p) {
				return nil, &identity.Validation{Fields: map[string]string{"sharedPaths": "shared paths are relative, with no '..', and not under .git or .orchestrator"}}
			}
			p = filepath.ToSlash(filepath.Clean(p))
			if !dup[p] {
				dup[p] = true
				paths = append(paths, p)
			}
		}
		clean = append(clean, LinkInput{ProjectID: id, SharedPaths: paths})
	}
	return clean, nil
}

func insertLinks(ctx context.Context, tx *sql.Tx, projectID string, links []LinkInput, now string) error {
	for _, link := range links {
		raw, err := json.Marshal(link.SharedPaths)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_links (project_id, linked_project_id, shared_paths_json, created_at)
			VALUES ($1, $2, $3, $4)`, projectID, link.ProjectID, string(raw), now); err != nil {
			return err
		}
	}
	return nil
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

// SettingsVisible lets a company admin read any project's settings. Everyone
// else must be a member of the project's tribe (RFC-059). It does not grant
// card access.
func SettingsVisible(ctx context.Context, db *sql.DB, projectID string, actor identity.User) error {
	if actor.Admin {
		_, err := GetProject(ctx, db, projectID)
		return err
	}
	return Visible(ctx, db, projectID, actor.ID)
}

// CanEditSettings is true for a company admin, a Lead Developer member, or a
// tribe admin of the project's tribe (RFC-059). Call SettingsVisible first.
func CanEditSettings(ctx context.Context, db *sql.DB, actor identity.User, projectID string) (bool, error) {
	if actor.Admin {
		return true, nil
	}
	if err := Visible(ctx, db, projectID, actor.ID); err != nil {
		return false, err
	}
	if actor.Role == "lead_developer" {
		return true, nil
	}
	project, err := GetProject(ctx, db, projectID)
	if err != nil {
		return false, err
	}
	return IsTribeAdmin(ctx, db, actor.ID, project.TribeID)
}

func requireSettingsEditor(ctx context.Context, db *sql.DB, actor identity.User, projectID string) error {
	if err := SettingsVisible(ctx, db, projectID, actor); err != nil {
		return err
	}
	ok, err := CanEditSettings(ctx, db, actor, projectID)
	if err != nil {
		return err
	}
	if !ok {
		return identity.ErrForbidden
	}
	return nil
}
