package app

import (
	"context"
	"database/sql"

	"orchestrator/backend/internal/tenancy"
)

type Tribe = tenancy.Tribe
type Project = tenancy.Project
type Member = tenancy.Member

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

func CreateProject(ctx context.Context, db *sql.DB, actor User, tribeID, name, repoPath, dataRoot string) (Project, error) {
	return tenancy.CreateProject(ctx, db, actor, tribeID, name, repoPath, dataRoot)
}

func ListProjects(ctx context.Context, db *sql.DB, actor User) ([]Project, error) {
	return tenancy.ListProjects(ctx, db, actor)
}

func SeeProject(ctx context.Context, db *sql.DB, projectID, userID string) error {
	return tenancy.Visible(ctx, db, projectID, userID)
}

func LoadProjectGitHub(ctx context.Context, db *sql.DB, projectID string) (tenancy.ProjectGitHub, error) {
	return tenancy.LoadProjectGitHub(ctx, db, projectID)
}

func SaveProjectGitHub(ctx context.Context, db *sql.DB, actor User, projectID string, gh tenancy.ProjectGitHub, token *string, clear bool, key []byte) error {
	return tenancy.SaveProjectGitHub(ctx, db, actor, projectID, gh, token, clear, key)
}

func ProjectBotToken(ctx context.Context, db *sql.DB, projectID string, key []byte) (string, error) {
	return tenancy.ProjectBotToken(ctx, db, projectID, key)
}
