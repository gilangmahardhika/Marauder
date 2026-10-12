package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/settings"
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

func (s *Server) setTribeAdmin(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	var body struct {
		TribeAdmin bool `json:"tribeAdmin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	if err := app.SetTribeAdmin(r.Context(), s.DB, actor, r.PathValue("id"), r.PathValue("userId"), body.TribeAdmin); err != nil {
		writeAccountErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) tribeCandidates(w http.ResponseWriter, r *http.Request) {
	actor, _ := app.CurrentUser(r.Context())
	list, err := app.TribeCandidates(r.Context(), s.DB, actor, r.PathValue("id"))
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"users": list})
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
			TribeID    string          `json:"tribeId"`
			Name       string          `json:"name"`
			Repo       string          `json:"repo"`
			BaseBranch string          `json:"baseBranch"`
			Links      []app.LinkInput `json:"links"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		project, err := app.CreateProject(r.Context(), s.DB, actor, app.NewProject{
			TribeID: body.TribeID, Name: body.Name, Repo: body.Repo, BaseBranch: body.BaseBranch, Links: body.Links,
		}, s.Data, app.GitHubRepoCheck(s.DB, s.TokenKey, s.gh()))
		if err != nil {
			writeRepoErr(w, err)
			return
		}
		links, err := app.ListLinks(r.Context(), s.DB, project.ID)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		if len(links) > 0 {
			s.configChanged(r, project.ID, "links", []string{"links"})
		}
		writeJSON(w, 201, map[string]any{
			"id": project.ID, "tribeId": project.TribeID, "name": project.Name,
			"repo": project.Repo, "baseBranch": project.BaseBranch, "links": links,
		})
	default:
		writeErr(w, 404, "not_found", "not found")
	}
}

func (s *Server) projectCards(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	reviewers, err := app.CardReviewerUsers(r.Context(), s.DB, projectID, "")
	if err != nil {
		writeErr(w, 500, "internal", "cards")
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id FROM cards WHERE project_id = $1 ORDER BY updated_at`, projectID)
	if err != nil {
		writeErr(w, 500, "internal", "cards")
		return
	}
	defer rows.Close()
	cards := []any{}
	ready := 0
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
		body := s.cardJSON(r, card, reviewers[card.ID])
		body["projectId"] = projectID
		if readyToRun(body) {
			ready++
		}
		cards = append(cards, body)
	}
	writeJSON(w, 200, s.boardBody(r, projectID, cards, ready))
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
		writeJSON(w, 200, s.cardDetail(r, next))
		return
	}
	if action == "run" {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "method_not_allowed", "method not allowed")
			return
		}
		next, err := app.QueueCardRun(r.Context(), s.DB, projectID, id)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardDetail(r, next))
		return
	}
	s.cardRoute(w, r, id, action)
}

func (s *Server) projectIntake(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	if s.DB == nil {
		writeJSON(w, 200, map[string]any{"draftPending": false, "planError": nil, "canAccept": false, "requirements": []any{}, "decisions": []any{}})
		return
	}
	s.writeIntake(w, r, projectID)
}

// projectIngest fetches the base branch when the company token is set, then
// parses the PRD and RFC paths from the project clone.
func (s *Server) projectIngest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	project, err := app.GetProject(r.Context(), s.DB, projectID)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	if s.Data == "" || project.RepoPath == "" {
		writeFields(w, 422, "validation", "the project has no clone", map[string]string{"repo": "set the repository and sync it first"})
		return
	}
	if token, err := app.CompanyToken(r.Context(), s.DB, s.TokenKey); err == nil && token != "" && project.Repo != "" {
		if _, err := app.Sync(r.Context(), s.DB, s.TokenKey, s.Data, projectID, s.Tools); err != nil {
			writeSyncErr(w, err)
			return
		}
	}
	if !app.ProjectGit(s.Data, project, runtime.Author{}, s.Tools).Cloned() {
		writeFields(w, 422, "validation", "the project has no clone", map[string]string{"repo": "sync the repository first"})
		return
	}
	user, _ := app.CurrentUser(r.Context())
	result, err := app.Ingest(r.Context(), s.DB, projectID, user.ID, project.RepoPath, settings.ProjectDir(s.Data, projectID))
	if err != nil {
		writeErr(w, 422, "validation", err.Error())
		return
	}
	payload, err := intakeBody(r, s.DB, projectID)
	if err != nil {
		writeErr(w, 500, "internal", "intake")
		return
	}
	if len(result.Skipped) > 0 {
		payload["skipped"] = result.Skipped
	}
	writeJSON(w, 200, payload)
}

func (s *Server) projectAccept(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if !canAccept(user.Role) {
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
	if err := intake.Accept(r.Context(), s.DB, projectID, user.ID, body.IDs); err != nil {
		if errors.Is(err, intake.ErrUnknownID) {
			writeFields(w, 422, "validation", err.Error(), map[string]string{"requirementIds": err.Error()})
			return
		}
		writeAppErr(w, err)
		return
	}
	s.writeIntake(w, r, projectID)
}

type requirementBody struct {
	ID           string   `json:"id"`
	Statement    string   `json:"statement"`
	Acceptance   []string `json:"acceptance"`
	SelfReview   bool     `json:"selfReview"`
	AIReviewMode string   `json:"aiReviewMode"`
}

// projectRequirements adds a manual requirement. Any member may call it
// (RFC-053); only a Lead Developer or an admin may turn Self review on
// (RFC-070) or choose the manual AI review mode (RFC-072).
func (s *Server) projectRequirements(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	var body requirementBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if body.SelfReview && !intake.CanSetSelfReview(user.Role, user.Admin) {
		writeIntakeErr(w, intake.ErrSelfReviewForbidden)
		return
	}
	if body.AIReviewMode == intake.ReviewManual && !intake.CanSetSelfReview(user.Role, user.Admin) {
		writeIntakeErr(w, intake.ErrAIReviewModeForbidden)
		return
	}
	draft := intake.Draft{
		Statement: body.Statement, Acceptance: body.Acceptance, SelfReview: body.SelfReview, AIReviewMode: body.AIReviewMode,
	}
	if _, err := intake.Add(r.Context(), s.DB, projectID, user.ID, body.ID, draft); err != nil {
		writeIntakeErr(w, err)
		return
	}
	s.writeIntake(w, r, projectID)
}

// projectRequirement edits or deletes one manual requirement.
func (s *Server) projectRequirement(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var err error
	if r.Method == http.MethodDelete {
		err = intake.Delete(r.Context(), s.DB, projectID, id)
	} else {
		var body requirementBody
		if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		err = intake.Update(r.Context(), s.DB, projectID, id, intake.Draft{Statement: body.Statement, Acceptance: body.Acceptance})
	}
	if err != nil {
		writeIntakeErr(w, err)
		return
	}
	s.writeIntake(w, r, projectID)
}

// projectReviewers replaces the reviewers of one requirement (RFC-054).
func (s *Server) projectReviewers(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	var body struct {
		UserIDs []string `json:"userIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if err := intake.SetReviewers(r.Context(), s.DB, projectID, r.PathValue("id"), user.ID, user.Role, body.UserIDs); err != nil {
		writeIntakeErr(w, err)
		return
	}
	s.writeIntake(w, r, projectID)
}

// projectSelfReview turns Self review on or off for one requirement (RFC-070).
func (s *Server) projectSelfReview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	var body struct {
		SelfReview bool `json:"selfReview"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if !intake.CanSetSelfReview(user.Role, user.Admin) {
		writeIntakeErr(w, intake.ErrSelfReviewForbidden)
		return
	}
	if err := intake.SetSelfReview(r.Context(), s.DB, projectID, r.PathValue("id"), body.SelfReview); err != nil {
		writeIntakeErr(w, err)
		return
	}
	s.writeIntake(w, r, projectID)
}

// projectAIReviewMode sets the AI review mode of one requirement (RFC-072).
func (s *Server) projectAIReviewMode(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	var body struct {
		AIReviewMode string `json:"aiReviewMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "body")
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if !intake.CanSetSelfReview(user.Role, user.Admin) {
		writeIntakeErr(w, intake.ErrAIReviewModeForbidden)
		return
	}
	if err := intake.SetAIReviewMode(r.Context(), s.DB, projectID, r.PathValue("id"), body.AIReviewMode); err != nil {
		writeIntakeErr(w, err)
		return
	}
	s.writeIntake(w, r, projectID)
}

func writeIntakeErr(w http.ResponseWriter, err error) {
	var invalid *intake.Invalid
	switch {
	case errors.As(err, &invalid):
		writeFields(w, 422, "validation", invalid.Error(), invalid.Fields)
	case errors.Is(err, intake.ErrForbidden), errors.Is(err, intake.ErrSelfReviewForbidden),
		errors.Is(err, intake.ErrAIReviewModeForbidden):
		writeErr(w, 403, "forbidden", err.Error())
	case errors.Is(err, intake.ErrNotFound):
		writeErr(w, 404, "not_found", err.Error())
	case errors.Is(err, intake.ErrDuplicate):
		writeFields(w, 409, "conflict", err.Error(), map[string]string{"id": "This id is already used in this project."})
	case errors.Is(err, intake.ErrNotManual), errors.Is(err, intake.ErrInUse):
		writeErr(w, 409, "conflict", err.Error())
	default:
		writeErr(w, 500, "internal", "requirement")
	}
}

func (s *Server) writeIntake(w http.ResponseWriter, r *http.Request, projectID string) {
	payload, err := intakeBody(r, s.DB, projectID)
	if err != nil {
		writeErr(w, 500, "internal", "intake")
		return
	}
	writeJSON(w, 200, payload)
}

func (s *Server) projectRuns(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.visibleProject(w, r)
	if !ok {
		return
	}
	if s.DB == nil {
		writeJSON(w, 200, map[string]any{"queued": 0, "readyCount": 0})
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if !actionAllowed(user.Role, "run") {
		writeErr(w, 403, "forbidden", "forbidden")
		return
	}
	queued, err := app.QueueReadyRuns(r.Context(), s.DB, projectID)
	if err != nil {
		writeErr(w, 500, "internal", "run")
		return
	}
	ready, err := app.ReadyCount(r.Context(), s.DB, projectID)
	if err != nil {
		writeErr(w, 500, "internal", "run")
		return
	}
	writeJSON(w, 200, map[string]any{"queued": queued, "readyCount": ready})
}

// settingsProject admits a company admin to any project's settings and
// everyone else to their tribes' projects (RFC-059). canEdit says whether the
// caller may save them.
func (s *Server) settingsProject(w http.ResponseWriter, r *http.Request) (projectID string, canEdit, ok bool) {
	projectID = r.PathValue("projectId")
	if s.DB == nil {
		return projectID, true, true
	}
	user, found := app.CurrentUser(r.Context())
	if !found {
		writeErr(w, 401, "unauthenticated", "bearer")
		return "", false, false
	}
	if err := app.SettingsVisible(r.Context(), s.DB, projectID, user); err != nil {
		writeAccountErr(w, err)
		return "", false, false
	}
	canEdit, err := app.CanEditSettings(r.Context(), s.DB, user, projectID)
	if err != nil {
		writeAccountErr(w, err)
		return "", false, false
	}
	return projectID, canEdit, true
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
