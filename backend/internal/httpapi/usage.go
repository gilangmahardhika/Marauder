package httpapi

import (
	"errors"
	"net/http"
	"time"

	"orchestrator/backend/internal/app"
)

// usage serves GET /api/usage to an admin (RFC-089).
func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	user, ok := app.CurrentUser(r.Context())
	if !ok {
		writeErr(w, 401, "unauthenticated", "bearer")
		return
	}
	query := r.URL.Query()
	report, err := app.Usage(r.Context(), s.DB, s.Data, user, query.Get("from"), query.Get("to"), time.Now())
	var invalid app.UsageRangeError
	switch {
	case errors.As(err, &invalid):
		writeFields(w, 422, "validation", invalid.Error(), map[string]string{invalid.Field: invalid.Message})
	case errors.Is(err, app.ErrForbidden):
		writeErr(w, 403, "forbidden", "forbidden")
	case err != nil:
		writeErr(w, 500, "internal", "usage")
	default:
		writeJSON(w, 200, report)
	}
}
