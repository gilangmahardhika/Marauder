package app

import (
	"context"
	"database/sql"

	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/tenancy"
)

// SaveProjectConventions saves a project's hand-added stacks and enabled
// skills for whoever may edit its settings (RFC-059).
func SaveProjectConventions(ctx context.Context, db *sql.DB, actor User, projectID string, manualStackIDs, skillIDs []string) (conventions.Project, error) {
	if err := tenancy.SettingsVisible(ctx, db, projectID, actor); err != nil {
		return conventions.Project{}, err
	}
	ok, err := tenancy.CanEditSettings(ctx, db, actor, projectID)
	if err != nil {
		return conventions.Project{}, err
	}
	if !ok {
		return conventions.Project{}, identity.ErrForbidden
	}
	return conventions.SaveProject(ctx, db, actor, projectID, manualStackIDs, skillIDs)
}
