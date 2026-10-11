package app

import (
	"context"
	"database/sql"

	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/tenancy"
)

type Tribe = tenancy.Tribe
type Project = tenancy.Project
type Member = tenancy.Member
type Candidate = tenancy.Candidate
type NewProject = tenancy.NewProject
type Link = tenancy.Link
type LinkInput = tenancy.LinkInput
type CompanyAccount = company.Account

func CreateTribe(ctx context.Context, db *sql.DB, actor User, name string) (Tribe, error) {
	return tenancy.CreateTribe(ctx, db, actor, name)
}

func ListTribes(ctx context.Context, db *sql.DB, actor User) ([]Tribe, error) {
	return tenancy.ListTribes(ctx, db, actor)
}

func ListMembers(ctx context.Context, db *sql.DB, actor User, tribeID string) ([]Member, error) {
	return tenancy.ListMembers(ctx, db, actor, tribeID)
}

func AddMember(ctx context.Context, db *sql.DB, actor User, tribeID, userID string) error {
	return tenancy.AddMember(ctx, db, actor, tribeID, userID)
}

func RemoveMember(ctx context.Context, db *sql.DB, actor User, tribeID, userID string) error {
	return tenancy.RemoveMember(ctx, db, actor, tribeID, userID)
}

func SetTribeAdmin(ctx context.Context, db *sql.DB, actor User, tribeID, userID string, on bool) error {
	return tenancy.SetTribeAdmin(ctx, db, actor, tribeID, userID, on)
}

func TribeCandidates(ctx context.Context, db *sql.DB, actor User, tribeID string) ([]Candidate, error) {
	return tenancy.Candidates(ctx, db, actor, tribeID)
}

func TribeAdminOf(ctx context.Context, db *sql.DB, userID string) ([]string, error) {
	return tenancy.TribeAdminOf(ctx, db, userID)
}

type RepoCheck = tenancy.RepoCheck

// GitHubRepoCheck confirms a repository and branch with the company token.
func GitHubRepoCheck(db *sql.DB, key []byte, gh company.GH) RepoCheck {
	return func(ctx context.Context, repo, base string) (string, error) {
		return company.CheckRepo(ctx, db, key, gh, repo, base)
	}
}

func CreateProject(ctx context.Context, db *sql.DB, actor User, in NewProject, dataRoot string, check RepoCheck) (Project, error) {
	return tenancy.CreateProject(ctx, db, actor, in, dataRoot, check)
}

func ListProjects(ctx context.Context, db *sql.DB, actor User) ([]Project, error) {
	return tenancy.ListProjects(ctx, db, actor)
}

func GetProject(ctx context.Context, db *sql.DB, projectID string) (Project, error) {
	return tenancy.GetProject(ctx, db, projectID)
}

func SeeProject(ctx context.Context, db *sql.DB, projectID, userID string) error {
	return tenancy.Visible(ctx, db, projectID, userID)
}

func SettingsVisible(ctx context.Context, db *sql.DB, projectID string, actor User) error {
	return tenancy.SettingsVisible(ctx, db, projectID, actor)
}

func CanEditSettings(ctx context.Context, db *sql.DB, actor User, projectID string) (bool, error) {
	return tenancy.CanEditSettings(ctx, db, actor, projectID)
}

func SaveRepository(ctx context.Context, db *sql.DB, actor User, projectID, repo, base, dataRoot string, check RepoCheck) (Project, bool, error) {
	return tenancy.SaveRepository(ctx, db, actor, projectID, repo, base, dataRoot, check)
}

func ListLinks(ctx context.Context, db *sql.DB, projectID string) ([]Link, error) {
	return tenancy.ListLinks(ctx, db, projectID)
}

func SaveLinks(ctx context.Context, db *sql.DB, actor User, projectID string, links []LinkInput) ([]Link, error) {
	return tenancy.SaveLinks(ctx, db, actor, projectID, links)
}

func LoadCompany(ctx context.Context, db *sql.DB) (CompanyAccount, error) {
	return company.Load(ctx, db)
}

func SaveCompany(ctx context.Context, db *sql.DB, actor User, host, org string, token *string, clear bool, key []byte) (CompanyAccount, error) {
	return company.Save(ctx, db, actor, host, org, token, clear, key)
}

func TestCompany(ctx context.Context, db *sql.DB, key []byte, gh company.GH) (string, error) {
	return company.Test(ctx, db, key, gh)
}

func CompanyRepos(ctx context.Context, db *sql.DB, key []byte, gh company.GH) ([]company.Repo, error) {
	return company.Repos(ctx, db, key, gh)
}

func CompanyToken(ctx context.Context, db *sql.DB, key []byte) (string, error) {
	return company.Token(ctx, db, key)
}
