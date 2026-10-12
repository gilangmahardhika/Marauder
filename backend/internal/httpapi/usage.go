package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"orchestrator/backend/internal/app"
)

type usageRead func(ctx context.Context, actor app.User, from, to string, now time.Time) (app.UsageReport, error)

// usage serves GET /api/usage to an admin (RFC-089).
func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	s.serveUsage(w, r, func(ctx context.Context, actor app.User, from, to string, now time.Time) (app.UsageReport, error) {
		return app.Usage(ctx, s.DB, s.Data, actor, from, to, now)
	})
}

// tribeUsage serves GET /api/tribes/{id}/usage to an admin or a tribe admin
// of that tribe (RFC-090).
func (s *Server) tribeUsage(w http.ResponseWriter, r *http.Request) {
	tribeID := r.PathValue("id")
	s.serveUsage(w, r, func(ctx context.Context, actor app.User, from, to string, now time.Time) (app.UsageReport, error) {
		return app.TribeUsage(ctx, s.DB, s.Data, actor, tribeID, from, to, now)
	})
}

// myUsage serves GET /api/me/usage, the caller's own calls (RFC-090).
func (s *Server) myUsage(w http.ResponseWriter, r *http.Request) {
	s.serveUsage(w, r, func(ctx context.Context, actor app.User, from, to string, now time.Time) (app.UsageReport, error) {
		return app.MyUsage(ctx, s.DB, s.Data, actor, from, to, now)
	})
}

func (s *Server) serveUsage(w http.ResponseWriter, r *http.Request, read usageRead) {
	user, ok := app.CurrentUser(r.Context())
	if !ok {
		writeErr(w, 401, "unauthenticated", "bearer")
		return
	}
	query := r.URL.Query()
	report, err := read(r.Context(), user, query.Get("from"), query.Get("to"), time.Now())
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
