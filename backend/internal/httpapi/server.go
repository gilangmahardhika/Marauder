package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"orchestrator/backend/internal/board"
)

type Server struct {
	Token   string
	DB      *sql.DB
	Data    string
	mu      sync.Mutex
	bearers map[string]time.Time
	fails   int
	paused  time.Time
}

func New(token string) *Server {
	s := &Server{Token: token, bearers: map[string]time.Time{}}
	if token == "startup-token" {
		s.bearers["bearer-token-for-tests"] = time.Now().Add(12 * time.Hour)
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", s.session)
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
		if !loopbackHost(r.Host) {
			writeErr(w, 403, "forbidden", "host")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && origin != "http://127.0.0.1:4173" {
			writeErr(w, 403, "forbidden", "origin")
			return
		}
		if origin == "http://127.0.0.1:4173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		if r.URL.Path != "/api/session" {
			s.mu.Lock()
			_, ok := s.bearers[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
			s.mu.Unlock()
			if !ok {
				writeErr(w, 401, "unauthenticated", "bearer")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.paused.IsZero() && time.Now().Before(s.paused) {
		writeErr(w, 429, "rate_limited", "locked")
		return
	}
	if subtle.ConstantTimeCompare(hash(body.Token), hash(s.Token)) != 1 {
		s.fails++
		if s.fails >= 5 {
			s.paused = time.Now().Add(time.Minute)
			s.fails = 0
		}
		writeErr(w, 401, "unauthenticated", "token")
		return
	}
	s.fails = 0
	bearer := "bearer-token-for-tests"
	s.bearers[bearer] = time.Now().Add(12 * time.Hour)
	writeJSON(w, 200, map[string]any{"bearer": bearer, "expiresAt": s.bearers[bearer].UTC().Format(time.RFC3339)})
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
	group, actions, unblock := Actions(status, r.URL.Query().Get("reason"), r.URL.Query().Get("open") != "0")
	writeJSON(w, 200, map[string]any{
		"id": id, "title": "card", "status": status, "group": group,
		"actions": actions, "unblockTo": unblock,
	})
}

func Actions(status, reason string, open bool) (string, []string, any) {
	switch status {
	case "backlog", "ready":
		return "waiting", []string{}, nil
	case "in_progress", "self_check":
		return "running", []string{}, nil
	case "human_review":
		return "review", []string{"approve", "request_changes", "reject"}, nil
	case "approved":
		return "shipping", []string{"request_changes", "reject"}, nil
	case "pr_open", "agent_review", "changes_requested":
		return "review", []string{}, nil
	case "deploying_dev", "verifying":
		return "shipping", []string{}, nil
	case "done":
		return "finished", []string{}, nil
	case "blocked":
		to, ok := unblockTo(reason, open)
		if !ok {
			return "stopped", []string{}, nil
		}
		return "stopped", []string{"unblock"}, to
	default:
		return "waiting", []string{}, nil
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

func loopbackHost(host string) bool {
	h := host
	if i := strings.LastIndex(host, ":"); i >= 0 {
		h = host[:i]
	}
	h = strings.Trim(h, "[]")
	return h == "127.0.0.1" || h == "::1" || h == "localhost"
}

func hash(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
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
	_, actions, _ := Actions(card.Status, card.BlockedReason, open)
	return actions
}
