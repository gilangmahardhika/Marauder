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

func CreateProject(ctx context.Context, db *sql.DB, actor User, in NewProject, dataRoot string) (Project, error) {
	return tenancy.CreateProject(ctx, db, actor, in, dataRoot)
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

func SaveRepository(ctx context.Context, db *sql.DB, actor User, projectID, repo, base, dataRoot string) (Project, bool, error) {
	return tenancy.SaveRepository(ctx, db, actor, projectID, repo, base, dataRoot)
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
