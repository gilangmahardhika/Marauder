package httpapi

import (
	"errors"
	"net/http"
	"time"

	"orchestrator/backend/internal/app"
)

// projectSummary serves GET /api/projects/{projectId}/summary (RFC-127).
func (s *Server) projectSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := app.CurrentUser(r.Context())
	if !ok {
		writeErr(w, 401, "unauthenticated", "bearer")
		return
	}
	projectID := r.PathValue("projectId")
	query := r.URL.Query()
	report, err := app.ProjectSummary(r.Context(), s.DB, s.Data, user, projectID, query.Get("from"), query.Get("to"), time.Now())
	var invalid app.UsageRangeError
	switch {
	case errors.As(err, &invalid):
		writeFields(w, 422, "validation", invalid.Error(), map[string]string{invalid.Field: invalid.Message})
	case err != nil:
		writeAccountErr(w, err)
	default:
		writeJSON(w, 200, report)
	}
}

// tribeSummary serves GET /api/tribes/{id}/summary (RFC-127).
func (s *Server) tribeSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := app.CurrentUser(r.Context())
	if !ok {
		writeErr(w, 401, "unauthenticated", "bearer")
		return
	}
	tribeID := r.PathValue("id")
	query := r.URL.Query()
	report, err := app.TribeSummary(r.Context(), s.DB, s.Data, user, tribeID, query.Get("from"), query.Get("to"), time.Now())
	var invalid app.UsageRangeError
	switch {
	case errors.As(err, &invalid):
		writeFields(w, 422, "validation", invalid.Error(), map[string]string{invalid.Field: invalid.Message})
	case err != nil:
		writeAccountErr(w, err)
	default:
		writeJSON(w, 200, report)
	}
}
