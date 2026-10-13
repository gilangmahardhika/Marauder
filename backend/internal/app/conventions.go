package app

import (
	"context"
	"database/sql"

	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/tenancy"
)

// SaveProjectConventions saves a project's on and off choices over its tribe
// for whoever may edit its settings (RFC-059, RFC-104).
func SaveProjectConventions(ctx context.Context, db *sql.DB, actor User, projectID string, choices conventions.Choices) (conventions.Project, error) {
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
	return conventions.SaveProject(ctx, db, actor, projectID, choices)
}

// TribeConventions reads a tribe's layer for an admin or a member of the
// tribe, and reports whether the actor may save it (RFC-104).
func TribeConventions(ctx context.Context, db *sql.DB, actor User, tribeID string) (conventions.TribeLayer, bool, error) {
	if err := tenancy.TribeVisible(ctx, db, actor, tribeID); err != nil {
		return conventions.TribeLayer{}, false, err
	}
	canEdit, err := tenancy.CanManageTribe(ctx, db, actor, tribeID)
	if err != nil {
		return conventions.TribeLayer{}, false, err
	}
	layer, err := conventions.LoadTribe(ctx, db, tribeID)
	return layer, canEdit, err
}

// SaveTribeConventions saves a tribe's layer for a company admin or a tribe
// admin of the tribe (RFC-104).
func SaveTribeConventions(ctx context.Context, db *sql.DB, actor User, tribeID string, layer conventions.TribeLayer) (conventions.TribeLayer, error) {
	if err := tenancy.ManageTribe(ctx, db, actor, tribeID); err != nil {
		return conventions.TribeLayer{}, err
	}
	return conventions.SaveTribe(ctx, db, actor, tribeID, layer)
}
