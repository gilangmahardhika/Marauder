package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/settings"
)

func (s *Server) executor() string {
	cfg, _, err := s.loadSettings()
	if err != nil {
		return settings.ExecutorLocal
	}
	return cfg.Executor()
}

func (s *Server) jobsSettings(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var body struct {
		Executor string `json:"executor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "validation", "body")
		return
	}
	cfg, err := settings.SaveJobs(s.Data, body.Executor)
	if err != nil {
		writeAppErr(w, err)
		return
	}
	s.configChanged(r, "", "jobs", []string{"executor"})
	writeJSON(w, 200, map[string]any{"executor": cfg.Executor()})
}

func (s *Server) listRunners(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	runners, err := app.ListRunners(r.Context(), s.DB, user)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	out := make([]any, 0, len(runners))
	for _, view := range runners {
		out = append(out, runnerJSON(view))
	}
	writeJSON(w, 200, map[string]any{"executor": s.executor(), "protocol": app.RunnerProtocol, "runners": out})
}

func (s *Server) mintRunnerToken(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	token, expires, err := app.MintRunnerToken(r.Context(), s.DB, user)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "expiresAt": expires.Format(time.RFC3339)})
}

func (s *Server) runnerAction(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	action := r.PathValue("action")
	view, err := app.RunnerAction(r.Context(), s.DB, user, r.PathValue("id"), action)
	if err != nil {
		writeRunnerErr(w, err)
		return
	}
	if action == "remove" {
		w.WriteHeader(204)
		return
	}
	writeJSON(w, 200, runnerJSON(view))
}

func runnerJSON(view app.RunnerView) map[string]any {
	leases := make([]any, 0, len(view.Leases))
	for _, l := range view.Leases {
		leases = append(leases, map[string]any{"id": l.ID, "cardId": l.CardID, "projectId": l.ProjectID, "since": l.CreatedAt})
	}
	return map[string]any{
		"id": view.ID, "name": view.Name, "labels": view.Labels, "slots": view.Slots, "busy": view.Busy,
		"status": view.Status, "version": view.Version, "protocol": view.Protocol,
		"updateRequired": view.UpdateRequired, "heartbeatAt": nullString(view.HeartbeatAt),
		"leases": leases, "actions": view.Actions,
	}
}

func writeRunnerErr(w http.ResponseWriter, err error) {
	var invalid *settings.Invalid
	switch {
	case errors.As(err, &invalid):
		writeFields(w, 422, "validation", invalid.Error(), invalid.Fields)
	case errors.Is(err, app.ErrRunnerAuth):
		writeErr(w, 401, "unauthenticated", "runner")
	case errors.Is(err, identity.ErrForbidden):
		writeErr(w, 403, "forbidden", "forbidden")
	case errors.Is(err, board.ErrNotFound):
		writeErr(w, 404, "not_found", "not found")
	case errors.Is(err, board.ErrConflict):
		writeErr(w, 409, "conflict", "already done")
	case errors.Is(err, app.ErrProtocol):
		writeErr(w, 409, "protocol", "runner protocol does not match; update the runner")
	case errors.Is(err, app.ErrRunnerAction):
		writeErr(w, 422, "transition_rejected", err.Error())
	case errors.Is(err, app.ErrRunnerOp):
		writeErr(w, 422, "validation", err.Error())
	case errors.Is(err, app.ErrBundle):
		writeErr(w, 422, "bundle", err.Error())
	case errors.Is(err, providers.ErrContextFull):
		writeErr(w, 413, "context_full", "the request does not fit the model's context window")
	case errors.Is(err, app.ErrProvider):
		writeErr(w, 502, "provider_error", "model call failed")
	default:
		writeErr(w, 500, "internal", "internal")
	}
}
