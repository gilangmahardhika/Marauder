package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/company"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
)

func (s *Server) gh() company.GH {
	gh := company.GH{Bin: s.Tools.GH, Exec: s.Tools.Exec}
	if s.Data != "" {
		gh.ConfigDir = filepath.Join(s.Data, "empty-gh")
	}
	return gh
}

func (s *Server) configChanged(r *http.Request, projectID, section string, fields []string) {
	user, _ := app.CurrentUser(r.Context())
	_, _ = trace.Append(r.Context(), s.DB, trace.Event{
		Kind: "config_changed", ActorID: user.ID, ProjectID: projectID,
		Payload: map[string]any{"section": section, "fields": fields},
	}, nil)
}

func (s *Server) companyGitHub(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	if !user.Admin {
		writeErr(w, 403, "forbidden", "forbidden")
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Host  string `json:"host"`
			Org   string `json:"org"`
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
		acct, err := app.SaveCompany(r.Context(), s.DB, user, body.Host, body.Org, token, body.Clear, s.TokenKey)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		fields := []string{"host", "org"}
		if token != nil || body.Clear {
			fields = append(fields, "token")
		}
		s.configChanged(r, "", "github", fields)
		writeJSON(w, 200, acct)
		return
	}
	acct, err := app.LoadCompany(r.Context(), s.DB)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, acct)
}

func (s *Server) companyGitHubTest(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	if !user.Admin {
		writeErr(w, 403, "forbidden", "forbidden")
		return
	}
	access, err := app.TestCompany(r.Context(), s.DB, s.TokenKey, s.gh())
	switch {
	case errors.Is(err, company.ErrNoToken):
		writeFields(w, 422, "validation", err.Error(), map[string]string{"token": err.Error()})
	case errors.Is(err, company.ErrGitHub):
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
	case err != nil:
		writeAccountErr(w, err)
	default:
		writeJSON(w, 200, map[string]any{
			"ok": true, "login": access.Login, "tokenKind": access.TokenKind, "scopes": access.Scopes,
			"repoCount": access.RepoCount, "privateCount": access.PrivateCount, "hint": access.Hint,
		})
	}
}

func (s *Server) companyRepos(w http.ResponseWriter, r *http.Request) {
	user, _ := app.CurrentUser(r.Context())
	if !user.Admin && user.Role != "lead_developer" {
		admins, err := app.TribeAdminOf(r.Context(), s.DB, user.ID)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		if len(admins) == 0 {
			writeErr(w, 403, "forbidden", "forbidden")
			return
		}
	}
	repos, err := app.CompanyRepos(r.Context(), s.DB, s.TokenKey, s.gh())
	switch {
	case errors.Is(err, company.ErrNoToken):
		writeFields(w, 422, "validation", err.Error(), map[string]string{"token": err.Error()})
	case errors.Is(err, company.ErrGitHub):
		writeErr(w, 502, "github", err.Error())
	case err != nil:
		writeAccountErr(w, err)
	default:
		writeJSON(w, 200, map[string]any{"repos": repos})
	}
}

func (s *Server) projectRepository(w http.ResponseWriter, r *http.Request) {
	projectID, canEdit, ok := s.settingsProject(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPut {
		user, _ := app.CurrentUser(r.Context())
		var body struct {
			Repo       string `json:"repo"`
			BaseBranch string `json:"baseBranch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		if _, _, err := app.SaveRepository(r.Context(), s.DB, user, projectID, body.Repo, body.BaseBranch, s.Data, app.GitHubRepoCheck(s.DB, s.TokenKey, s.gh())); err != nil {
			writeRepoErr(w, err)
			return
		}
		s.configChanged(r, projectID, "repository", []string{"repo", "baseBranch"})
	}
	view, err := app.RepositoryView(r.Context(), s.DB, s.Data, projectID, s.Tools)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	view.CanEdit = canEdit
	writeJSON(w, 200, view)
}

func (s *Server) projectSync(w http.ResponseWriter, r *http.Request) {
	projectID, canEdit, ok := s.settingsProject(w, r)
	if !ok {
		return
	}
	user, _ := app.CurrentUser(r.Context())
	switch user.Role {
	case "lead_developer", "senior_developer", "developer":
	default:
		if !canEdit {
			writeErr(w, 403, "forbidden", "forbidden")
			return
		}
	}
	view, err := app.Sync(r.Context(), s.DB, s.TokenKey, s.Data, projectID, s.Tools)
	if err != nil {
		writeSyncErr(w, err)
		return
	}
	view.CanEdit = canEdit
	writeJSON(w, 200, view)
}

func writeRepoErr(w http.ResponseWriter, err error) {
	if errors.Is(err, company.ErrGitHub) {
		writeErr(w, 502, "github", err.Error())
		return
	}
	writeAccountErr(w, err)
}

func writeSyncErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, company.ErrNoToken):
		writeFields(w, 422, "validation", err.Error(), map[string]string{"token": "an admin must save the company GitHub token"})
	case errors.Is(err, app.ErrSync):
		writeErr(w, 502, "github", err.Error())
	default:
		writeAccountErr(w, err)
	}
}

func (s *Server) projectLinks(w http.ResponseWriter, r *http.Request) {
	projectID, canEdit, ok := s.settingsProject(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPut {
		user, _ := app.CurrentUser(r.Context())
		var body struct {
			Links []app.LinkInput `json:"links"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		links, err := app.SaveLinks(r.Context(), s.DB, user, projectID, body.Links)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		s.configChanged(r, projectID, "links", []string{"links"})
		writeJSON(w, 200, map[string]any{"links": links, "canEdit": canEdit})
		return
	}
	links, err := app.ListLinks(r.Context(), s.DB, projectID)
	if err != nil {
		writeAccountErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"links": links, "canEdit": canEdit})
}

func (s *Server) projectDeploy(w http.ResponseWriter, r *http.Request) {
	projectID, canEdit, ok := s.settingsProject(w, r)
	if !ok {
		return
	}
	if s.Data == "" {
		writeErr(w, 500, "internal", "settings")
		return
	}
	if r.Method == http.MethodPut {
		if !canEdit {
			writeErr(w, 403, "forbidden", "forbidden")
			return
		}
		var body struct {
			settings.Deploy
			DeployToken string `json:"deployToken"`
			Clear       bool   `json:"clear"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 422, "validation", "body")
			return
		}
		project, err := app.GetProject(r.Context(), s.DB, projectID)
		if err != nil {
			writeAccountErr(w, err)
			return
		}
		var token *string
		if body.DeployToken != "" {
			token = &body.DeployToken
		}
		paths := settings.ProjectPaths{Data: s.Data, Repo: project.RepoPath}
		_, fields, err := settings.SaveProjectDeploy(s.Data, projectID, paths, body.Deploy, token, body.Clear)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		s.configChanged(r, projectID, "deploy", fields)
	}
	cfg, secrets, err := settings.LoadProject(s.Data, projectID)
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	body := deployBody(settings.DeployView(cfg), secrets.DeployToken != "")
	body["canEdit"] = canEdit
	writeJSON(w, 200, body)
}

func deployBody(d settings.Deploy, tokenSet bool) map[string]any {
	return map[string]any{
		"environment": d.Environment, "devUrl": d.DevURL, "devHosts": d.DevHosts,
		"devCommand": d.DevCommand, "rollbackCommand": d.RollbackCommand, "healthUrl": d.HealthURL,
		"deployTokenSet": tokenSet, "localCheck": d.LocalCheck, "devCheck": d.DevCheck,
		"allowExecutables": d.AllowExecutables, "commandExecutables": d.CommandExecutables, "sandboxEngine": d.SandboxEngine, "sandboxImage": d.SandboxImage,
		"implementationCap": d.ImplementationCap, "prdPath": d.PRDPath, "rfcPath": d.RFCPath,
		"soloApproval": d.SoloApproval, "deployEnabled": d.DeployEnabled == nil || *d.DeployEnabled,
	}
}
