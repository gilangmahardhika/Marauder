package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
)

type Server struct {
	DB          *sql.DB
	Data        string
	TokenKey    []byte
	TestBearer  string
	AllowOrigin string
	AllowHosts  map[string]bool
	Cloud       bool
}

func New() *Server {
	return &Server{AllowOrigin: "http://127.0.0.1:4173"}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/session", s.login)
	mux.HandleFunc("DELETE /api/session", s.logout)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("PUT /api/me/password", s.changePassword)
	mux.HandleFunc("POST /api/me/github/test", s.testMyGitHub)
	mux.HandleFunc("/api/me/github", s.myGitHub)
	mux.HandleFunc("PUT /api/users/{id}", s.updateUser)
	mux.HandleFunc("/api/users", s.users)
	mux.HandleFunc("GET /api/tribes", s.tribes)
	mux.HandleFunc("POST /api/tribes", s.tribes)
	mux.HandleFunc("GET /api/tribes/{id}/members", s.listMembers)
	mux.HandleFunc("POST /api/tribes/{id}/members", s.addMember)
	mux.HandleFunc("DELETE /api/tribes/{id}/members/{userId}", s.removeMember)
	mux.HandleFunc("GET /api/projects", s.projectCollection)
	mux.HandleFunc("POST /api/projects", s.projectCollection)
	mux.HandleFunc("/api/projects/{projectId}/cards", s.projectCards)
	mux.HandleFunc("/api/projects/{projectId}/cards/", s.projectCard)
	mux.HandleFunc("/api/projects/{projectId}/intake", s.projectIntake)
	mux.HandleFunc("POST /api/projects/{projectId}/ingest", s.projectIngest)
	mux.HandleFunc("POST /api/projects/{projectId}/accept", s.projectAccept)
	mux.HandleFunc("POST /api/projects/{projectId}/runs", s.projectRuns)
	mux.HandleFunc("/api/projects/{projectId}/settings/github", s.projectGitHub)
	mux.HandleFunc("/api/cards", s.cards)
	mux.HandleFunc("/api/cards/", s.card)
	mux.HandleFunc("/api/intake", s.intake)
	mux.HandleFunc("/api/ingest", s.ingest)
	mux.HandleFunc("/api/accept", s.accept)
	mux.HandleFunc("/api/runs", s.runs)
	mux.HandleFunc("/api/settings/github", s.github)
	mux.HandleFunc("/api/settings/github/test", s.githubTest)
	mux.HandleFunc("/api/settings/llm", s.llm)
	mux.HandleFunc("/api/settings/llm/", s.llmTest)
	mux.HandleFunc("/api/settings/router", s.routerSettings)
	mux.HandleFunc("/api/settings/deploy", s.deploySettings)
	return s.wrap(mux)
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r.Host) {
			writeErr(w, 403, "forbidden", "host")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && !s.originAllowed(origin) {
			writeErr(w, 403, "forbidden", "origin")
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == "/api/session" && r.Method == http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		if s.DB == nil {
			if s.TestBearer != "" && bearerOf(r) == s.TestBearer {
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, 401, "unauthenticated", "bearer")
			return
		}
		user, err := app.UserByBearer(r.Context(), s.DB, bearerOf(r))
		if err != nil {
			writeErr(w, 401, "unauthenticated", "bearer")
			return
		}
		next.ServeHTTP(w, r.WithContext(app.WithUser(r.Context(), user)))
	})
}

func (s *Server) cards(w http.ResponseWriter, r *http.Request) {
	if s.DB != nil {
		s.listCards(w, r)
		return
	}
	writeJSON(w, 200, map[string]any{"canRun": false, "running": false, "cards": []any{}})
}

func (s *Server) card(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/cards/"), "/")
	id, action, _ := strings.Cut(rest, "/")
	if s.DB != nil {
		s.cardRoute(w, r, id, action)
		return
	}
	if action != "" {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "in_progress"
	}
	role := r.URL.Query().Get("role")
	if role == "" {
		role = "lead_developer"
	}
	group, actions, unblock := Actions(status, r.URL.Query().Get("reason"), r.URL.Query().Get("open") != "0", role)
	writeJSON(w, 200, map[string]any{
		"id": id, "title": "card", "status": status, "group": group,
		"actions": actions, "unblockTo": unblock,
	})
}

func Actions(status, reason string, open bool, role string) (string, []string, any) {
	var group string
	var actions []string
	var unblock any
	switch status {
	case "backlog", "ready":
		group, actions, unblock = "waiting", []string{}, nil
	case "in_progress", "self_check":
		group, actions, unblock = "running", []string{}, nil
	case "human_review":
		group, actions, unblock = "review", []string{"approve", "request_changes", "reject"}, nil
	case "approved":
		group, actions, unblock = "shipping", []string{"request_changes", "reject", "deploy"}, nil
	case "pr_open", "agent_review", "changes_requested":
		group, actions, unblock = "review", []string{}, nil
	case "deploying_dev", "verifying":
		group, actions, unblock = "shipping", []string{}, nil
	case "done":
		group, actions, unblock = "finished", []string{}, nil
	case "blocked":
		to, ok := unblockTo(reason, open)
		if !ok {
			return "stopped", []string{}, nil
		}
		group, actions, unblock = "stopped", []string{"unblock"}, to
	default:
		group, actions, unblock = "waiting", []string{}, nil
	}
	return group, filterActions(actions, role), unblock
}

func filterActions(actions []string, role string) []string {
	out := make([]string, 0, len(actions))
	for _, name := range actions {
		if actionAllowed(role, strings.ReplaceAll(name, "_", "-")) {
			out = append(out, name)
		}
	}
	return out
}

func actionAllowed(role, action string) bool {
	switch action {
	case "", "trace":
		return true
	case "approve", "reject":
		return role == "lead_developer" || role == "qa_lead" || role == "product_owner"
	case "request-changes":
		return role == "lead_developer" || role == "qa_lead" || role == "product_owner" || role == "senior_developer"
	case "unblock":
		return role == "lead_developer" || role == "product_owner"
	case "deploy":
		return role == "lead_developer"
	default:
		return false
	}
}

func unblockTo(reason string, open bool) (string, bool) {
	if reason == "source_changed" && !open {
		return "", false
	}
	switch reason {
	case "health", "deploy_failed", "deploy_config":
		return "human_review", true
	default:
		return "ready", true
	}
}

func (s *Server) originAllowed(origin string) bool {
	allowed := s.AllowOrigin
	if allowed == "" {
		allowed = "http://127.0.0.1:4173"
	}
	if origin == allowed {
		return true
	}
	if s.Cloud {
		return false
	}
	return origin == "http://127.0.0.1:4173" || origin == "http://localhost:4173"
}

func (s *Server) hostAllowed(host string) bool {
	if s.Cloud && len(s.AllowHosts) > 0 {
		h := host
		if i := strings.LastIndex(host, ":"); i >= 0 {
			h = host[:i]
		}
		h = strings.Trim(h, "[]")
		return s.AllowHosts[h] || s.AllowHosts[host]
	}
	return loopbackHost(host)
}

func loopbackHost(host string) bool {
	h := host
	if i := strings.LastIndex(host, ":"); i >= 0 {
		h = host[:i]
	}
	h = strings.Trim(h, "[]")
	return h == "127.0.0.1" || h == "::1" || h == "localhost"
}

func writeErr(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func CardActions(card board.Card, open bool) []string {
	_, actions, _ := Actions(card.Status, card.BlockedReason, open, "lead_developer")
	return actions
}
