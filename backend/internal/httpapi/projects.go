package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/tenancy"
)

func (s *Server) tribes(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	switch r.Method {
	case http.MethodGet:
		list, err := app.ListTribes(r.Context(), s.DB, actor)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"tribes": list})
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		tribe, err := app.CreateTribe(r.Context(), s.DB, actor, body.Name)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 201, tribe)
	default:
		writeErr(w, 404, "not_found", "not found")
	}
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	list, err := app.ListMembers(r.Context(), s.DB, actor, r.PathValue("id"))
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"members": list})
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	var body struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	if err := app.AddMember(r.Context(), s.DB, actor, r.PathValue("id"), body.UserID); err != nil {
		writeAccountErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	if err := app.RemoveMember(r.Context(), s.DB, actor, r.PathValue("id"), r.PathValue("userId")); err != nil {
		writeAccountErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) projectCollection(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	switch r.Method {
	case http.MethodGet:
		list, err := app.ListProjects(r.Context(), s.DB, actor)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"projects": list})
	case http.MethodPost:
		var body struct {
			TribeID  string `json:"tribeId"`
			Name     string `json:"name"`
			RepoPath string `json:"repoPath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		project, err := app.CreateProject(r.Context(), s.DB, actor, body.TribeID, body.Name, body.RepoPath, s.Data)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 201, project)
	default:
		writeErr(w, 404, "not_found", "not found")
	}
}

func (s *Server) projectCards(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id FROM cards WHERE project_id = $1 ORDER BY updated_at`, projectID)
	if err != nil {
		writeErr(w, 500, "internal", "cards")
		return
	}
	defer rows.Close()
	cards := []any{}
	canRun := false
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			writeErr(w, 500, "internal", "cards")
			return
		}
		card, err := board.GetCard(r.Context(), s.DB, id)
		if err != nil {
			writeErr(w, 500, "internal", "cards")
			return
		}
		if card.Status == "ready" || card.Status == "changes_requested" || card.Status == "approved" || card.Status == "backlog" {
			canRun = true
		}
		body := s.cardBody(r, card)
		body["projectId"] = projectID
		cards = append(cards, body)
	}
	writeJSON(w, 200, map[string]any{"canRun": canRun, "running": false, "cards": cards})
}

func (s *Server) projectCard(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/projects/"+projectID+"/cards/")
	id, action, _ := strings.Cut(strings.Trim(rest, "/"), "/")
	var n int
	if err := s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM cards WHERE id = $1 AND project_id = $2`, id, projectID).Scan(&n); err != nil {
		writeErr(w, 500, "internal", "cards")
		return
	}
	if n == 0 {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if !actionAllowed(user.Role, action) {
		writeErr(w, 403, "forbidden", "forbidden")
		return
	}
	if action == "deploy" {
		next, err := app.Deploy(r.Context(), s.DB, id, user.ID)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardBody(r, next))
		return
	}
	s.cardRoute(w, r, id, action)
}

func (s *Server) projectGitHub(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		gh, err := app.LoadProjectGitHub(r.Context(), s.DB, projectID)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{
			"host": gh.Host, "owner": gh.Owner, "repo": gh.Repo, "baseBranch": gh.Base,
			"botLogin": gh.BotLogin, "tokenSet": gh.TokenSet,
		})
	case http.MethodPut:
		user, _ := app.CurrentUser(r.Context())
		var body struct {
			Host       string `json:"host"`
			Owner      string `json:"owner"`
			Repo       string `json:"repo"`
			BaseBranch string `json:"baseBranch"`
			BotLogin   string `json:"botLogin"`
			Token      string `json:"token"`
			Clear      bool   `json:"clear"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		var token *string
		if body.Token != "" {
			token = &body.Token
		}
		err := app.SaveProjectGitHub(r.Context(), s.DB, user, projectID, tenancy.ProjectGitHub{
			Host: body.Host, Owner: body.Owner, Repo: body.Repo, Base: body.BaseBranch, BotLogin: body.BotLogin,
		}, token, body.Clear, s.TokenKey)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		gh, err := app.LoadProjectGitHub(r.Context(), s.DB, projectID)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{
			"host": gh.Host, "owner": gh.Owner, "repo": gh.Repo, "baseBranch": gh.Base,
			"botLogin": gh.BotLogin, "tokenSet": gh.TokenSet,
		})
	default:
		writeErr(w, 404, "not_found", "not found")
	}
}

func (s *Server) projectIntake(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.visibleProject(w, r); !ok {
		return
	}
	s.intake(w, r)
}

func (s *Server) projectIngest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.visibleProject(w, r); !ok {
		return
	}
	s.ingest(w, r)
}

func (s *Server) projectAccept(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.visibleProject(w, r); !ok {
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if user.Role != "product_owner" {
		writeErr(w, 403, "forbidden", "forbidden")
		return
	}
	var body struct {
		IDs []string `json:"requirementIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	if err := intake.Accept(r.Context(), s.DB, user.ID, body.IDs); err != nil {
		writeAppErr(w, err)
		return
	}
	payload, err := intakeBody(r, s.DB)
	if err != nil {
		writeErr(w, 500, "internal", "intake")
		return
	}
	writeJSON(w, 200, payload)
}

func (s *Server) projectRuns(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.visibleProject(w, r); !ok {
		return
	}
	user, _ := app.CurrentUser(r.Context())
	switch user.Role {
	case "lead_developer", "senior_developer", "developer", "qa":
	default:
		writeErr(w, 403, "forbidden", "forbidden")
		return
	}
	s.runs(w, r)
}

func (s *Server) visibleProject(w http.ResponseWriter, r *http.Request) (string, bool) {
	projectID := r.PathValue("projectId")
	if s.DB == nil {
		return projectID, true
	}
	user, ok := app.CurrentUser(r.Context())
	if !ok {
		writeErr(w, 401, "unauthenticated", "bearer")
		return "", false
	}
	if err := app.SeeProject(r.Context(), s.DB, projectID, user.ID); err != nil {
		writeAccountErr(w, err)
		return "", false
	}
	return projectID, true
}
