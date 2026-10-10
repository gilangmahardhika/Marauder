package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"orchestrator/backend/internal/app"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	session, err := app.Login(r.Context(), s.DB, body.Email, body.Password)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"bearer":    session.Bearer,
		"expiresAt": session.ExpiresAt.UTC().Format(time.RFC3339),
		"user":      userJSON(session.User),
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := app.Logout(r.Context(), s.DB, strings.TrimSpace(bearer)); err != nil {
		writeAccountErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	body := userJSON(user)
	body["githubLogin"] = user.GitHubLogin
	body["githubTokenSet"] = user.GitHubTokenSet
	writeJSON(w, 200, body)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	if err := app.ChangePassword(r.Context(), s.DB, user, body.CurrentPassword, body.NewPassword); err != nil {
		var ve *app.Validation
		if errors.As(err, &ve) {
			fields := ve.Fields
			if _, ok := fields["password"]; ok {
				fields = map[string]string{"newPassword": fields["password"]}
			}
			writeFields(w, 422, "validation", ve.Error(), fields)
			return
		}
		writeAccountErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) myGitHub(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{
			"host": user.GitHubHost, "login": user.GitHubLogin, "tokenSet": user.GitHubTokenSet,
		})
	case http.MethodPut:
		var body struct {
			Host  string `json:"host"`
			Login string `json:"login"`
			Token string `json:"token"`
			Clear bool   `json:"clear"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		var token *string
		if body.Token != "" {
			token = &body.Token
		}
		if err := app.SaveUserGitHub(r.Context(), s.DB, user.ID, body.Host, body.Login, token, body.Clear, s.TokenKey); err != nil {
			writeAccountErr(w, err)
			return
		}
		fresh, err := app.UserByBearer(r.Context(), s.DB, bearerOf(r))
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{
			"host": fresh.GitHubHost, "login": fresh.GitHubLogin, "tokenSet": fresh.GitHubTokenSet,
		})
	default:
		writeErr(w, 404, "not_found", "not found")
	}
}

func (s *Server) testMyGitHub(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	token, err := app.UserGitHubToken(r.Context(), s.DB, user.ID, s.TokenKey)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	if token == "" {
		writeErr(w, 422, "validation", "no stored token")
		return
	}
	if user.GitHubLogin == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "message": "authentication failed"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "login": user.GitHubLogin})
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	switch r.Method {
	case http.MethodGet:
		list, err := app.ListUsers(r.Context(), s.DB, actor)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		out := make([]map[string]any, 0, len(list))
		for _, user := range list {
			out = append(out, userJSON(user))
		}
		writeJSON(w, 200, map[string]any{"users": out})
	case http.MethodPost:
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
			Admin    bool   `json:"admin"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		user, err := app.CreateUser(r.Context(), s.DB, actor, body.Email, body.Password, body.Role, body.Admin)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 201, userJSON(user))
	default:
		writeErr(w, 404, "not_found", "not found")
	}
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	id := r.PathValue("id")
	var body struct {
		Role     *string `json:"role"`
		Admin    *bool   `json:"admin"`
		Disabled *bool   `json:"disabled"`
		Password string  `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	user, err := app.UpdateUser(r.Context(), s.DB, actor, id, body.Role, body.Admin, body.Disabled, body.Password)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, userJSON(user))
}

func userJSON(user app.User) map[string]any {
	if user.GitHubHost == "" {
		user.GitHubHost = "github.com"
	}
	return map[string]any{
		"id": user.ID, "email": user.Email, "role": user.Role, "admin": user.Admin, "disabled": user.Disabled,
	}
}

func bearerOf(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

func writeAccountErr(w http.ResponseWriter, err error) {
	var ve *app.Validation
	switch {
	case errors.As(err, &ve):
		writeFields(w, 422, "validation", ve.Error(), ve.Fields)
	case errors.Is(err, app.ErrUnauthenticated):
		writeErr(w, 401, "unauthenticated", "email or password")
	case errors.Is(err, app.ErrLocked):
		writeErr(w, 429, "rate_limited", "locked")
	case errors.Is(err, app.ErrForbidden):
		writeErr(w, 403, "forbidden", "forbidden")
	case errors.Is(err, app.ErrLastAdmin):
		writeErr(w, 422, "validation", "last admin")
	case errors.Is(err, app.ErrNotFoundUser):
		writeErr(w, 404, "not_found", "not found")
	default:
		writeErr(w, 500, "internal", "internal")
	}
}

func writeFields(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "fields": fields}})
}
