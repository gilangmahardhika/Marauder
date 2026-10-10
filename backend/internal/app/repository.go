package app

import (
	"context"
	"database/sql"
	"errors"

	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/tenancy"
)

var ErrSync = errors.New("sync failed")

type Repository struct {
	Host       string `json:"host"`
	Org        string `json:"org"`
	Repo       string `json:"repo"`
	BaseBranch string `json:"baseBranch"`
	Cloned     bool   `json:"cloned"`
	HeadSHA    any    `json:"headSha"`
}

// Tools are the binaries and the runner used for git and gh calls.
type Tools struct {
	Git  string
	GH   string
	Exec func(context.Context, execx.Child) (execx.Result, error)
}

// ProjectGit is the project's git runner. author is the card owner for a job,
// and empty for clone, fetch, and reads.
func ProjectGit(data string, project tenancy.Project, author runtime.Author, tools Tools) runtime.Git {
	return runtime.Git{
		Bin:    tools.Git,
		Repo:   project.RepoPath,
		Data:   settings.ProjectDir(data, project.ID),
		Author: author,
		Exec:   tools.Exec,
	}
}

func RepositoryView(ctx context.Context, db *sql.DB, data, projectID string, tools Tools) (Repository, error) {
	project, err := tenancy.GetProject(ctx, db, projectID)
	if err != nil {
		return Repository{}, err
	}
	acct, err := company.Load(ctx, db)
	if err != nil {
		return Repository{}, err
	}
	view := Repository{Host: acct.Host, Org: acct.Org, Repo: project.Repo, BaseBranch: project.BaseBranch}
	if project.RepoPath == "" || tools.Git == "" {
		return view, nil
	}
	git := ProjectGit(data, project, runtime.Author{}, tools)
	if git.Cloned() {
		view.Cloned = true
		if head, err := git.Head(ctx); err == nil {
			view.HeadSHA = head
		}
	}
	return view, nil
}

// Sync clones the project repository with the company token, or fetches its base branch.
func Sync(ctx context.Context, db *sql.DB, key []byte, data, projectID string, tools Tools) (Repository, error) {
	project, err := tenancy.GetProject(ctx, db, projectID)
	if err != nil {
		return Repository{}, err
	}
	if project.Repo == "" {
		return Repository{}, &identity.Validation{Fields: map[string]string{"repo": "set the repository first"}}
	}
	if project.RepoPath == "" || data == "" {
		return Repository{}, &identity.Validation{Fields: map[string]string{"repo": "the service has no data directory"}}
	}
	acct, err := company.Load(ctx, db)
	if err != nil {
		return Repository{}, err
	}
	if acct.Org == "" {
		return Repository{}, &identity.Validation{Fields: map[string]string{"org": "an admin must set the company organization"}}
	}
	token, err := company.Token(ctx, db, key)
	if err != nil {
		return Repository{}, err
	}
	if token == "" {
		return Repository{}, company.ErrNoToken
	}
	if err := tenancy.PrepareProject(data, projectID); err != nil {
		return Repository{}, err
	}
	git := ProjectGit(data, project, runtime.Author{}, tools)
	if _, err := git.EnsureCheckout(ctx, project.BaseBranch, runtime.Remote{URL: acct.CloneURL(project.Repo), Token: token}); err != nil {
		return Repository{}, errors.Join(ErrSync, err)
	}
	return RepositoryView(ctx, db, data, projectID, tools)
}
