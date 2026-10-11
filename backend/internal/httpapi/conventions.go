package httpapi

import (
	"encoding/json"
	"net/http"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/conventions"
)

// conventionKinds maps the route segment to the catalog kind.
var conventionKinds = map[string]string{"rules": "rule", "stacks": "stack", "skills": "skill"}

func (s *Server) conventionsCatalog(w http.ResponseWriter, r *http.Request) {
	cat, err := conventions.Load(r.Context(), s.DB)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, cat)
}

// conventionsSave handles POST /api/conventions/{kind} and
// PUT /api/conventions/{kind}/{id}. Only an admin may write (RFC-055).
func (s *Server) conventionsSave(w http.ResponseWriter, r *http.Request) {
	kind, ok := conventionKinds[r.PathValue("kind")]
	if !ok {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if !user.Admin {
		writeErr(w, 403, "forbidden", "forbidden")
		return
	}
	id := r.PathValue("id")
	var item any
	var changed []string
	var err error
	switch kind {
	case "rule":
		var in conventions.RuleInput
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		item, changed, err = conventions.SaveRule(r.Context(), s.DB, user, id, in)
	case "stack":
		var in conventions.StackInput
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		item, changed, err = conventions.SaveStack(r.Context(), s.DB, user, id, in)
	case "skill":
		var in conventions.SkillInput
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		item, changed, err = conventions.SaveSkill(r.Context(), s.DB, user, id, in)
	}
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	if len(changed) > 0 {
		s.configChanged(r, "", "conventions", changed)
	}
	status := 200
	if id == "" {
		status = 201
	}
	writeJSON(w, status, item)
}

func (s *Server) conventionsDelete(w http.ResponseWriter, r *http.Request) {
	kind, ok := conventionKinds[r.PathValue("kind")]
	if !ok {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	user, _ := app.CurrentUser(r.Context())
	if !user.Admin {
		writeErr(w, 403, "forbidden", "forbidden")
		return
	}
	deleted, err := conventions.Delete(r.Context(), s.DB, user, kind, r.PathValue("id"))
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	field := "disabled"
	if deleted {
		field = "deleted"
	}
	s.configChanged(r, "", "conventions", []string{field})
	w.WriteHeader(204)
}

func (s *Server) projectConventions(w http.ResponseWriter, r *http.Request) {
	projectID, canEdit, ok := s.settingsProject(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPut {
		user, _ := app.CurrentUser(r.Context())
		var body struct {
			ManualStackIDs []string `json:"manualStackIds"`
			SkillIDs       []string `json:"skillIds"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		view, err := app.SaveProjectConventions(r.Context(), s.DB, user, projectID, body.ManualStackIDs, body.SkillIDs)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		s.configChanged(r, projectID, "conventions", []string{"manualStackIds", "skillIds"})
		view.CanEdit = canEdit
		writeJSON(w, 200, view)
		return
	}
	view, err := conventions.LoadProject(r.Context(), s.DB, projectID)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	view.CanEdit = canEdit
	writeJSON(w, 200, view)
}
