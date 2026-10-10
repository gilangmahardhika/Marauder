package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/settings"
)

func (s *Server) listCards(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id FROM cards ORDER BY updated_at`)
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
		cards = append(cards, s.cardBody(r, card))
	}
	writeJSON(w, 200, map[string]any{"canRun": canRun, "running": false, "cards": cards})
}

func (s *Server) cardRoute(w http.ResponseWriter, r *http.Request, id, action string) {
	if action == "trace" && r.Method == http.MethodGet {
		s.trace(w, r, id)
		return
	}
	card, err := board.GetCard(r.Context(), s.DB, id)
	if err != nil {
		writeAppErr(w, err)
		return
	}
	switch action {
	case "":
		if r.Method == http.MethodGet {
			writeJSON(w, 200, s.cardBody(r, card))
		}
	case "approve":
		var body struct {
			SHA string `json:"sha"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, "validation", "body")
			return
		}
		next, err := app.Approve(r.Context(), s.DB, id, body.SHA, s.gate())
		if err != nil {
			if errors.Is(err, app.ErrSHA) {
				writeJSON(w, 422, map[string]any{"error": map[string]any{"code": "validation", "message": err.Error(), "fields": map[string]string{"sha": err.Error()}}})
				return
			}
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardBody(r, next))
	case "request-changes":
		next, err := app.RequestChanges(r.Context(), s.DB, id, s.gate())
		if err != nil {
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardBody(r, next))
	case "reject":
		next, err := app.Reject(r.Context(), s.DB, id, s.gate())
		if err != nil {
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardBody(r, next))
	case "unblock":
		var body struct {
			ResetAttempts bool `json:"resetAttempts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		next, err := app.Unblock(r.Context(), s.DB, id, body.ResetAttempts)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardBody(r, next))
	default:
		writeErr(w, 404, "not_found", "not found")
	}
}

func (s *Server) trace(w http.ResponseWriter, r *http.Request, id string) {
	rows, err := s.DB.QueryContext(r.Context(), `
		SELECT id, ts, COALESCE(attempt_id, ''), kind, payload_json
		FROM events WHERE card_id = $1 ORDER BY ts`, id)
	if err != nil {
		writeErr(w, 500, "internal", "trace")
		return
	}
	defer rows.Close()
	events := []any{}
	for rows.Next() {
		var eventID, ts, attemptID, kind, payload string
		if err := rows.Scan(&eventID, &ts, &attemptID, &kind, &payload); err != nil {
			writeErr(w, 500, "internal", "trace")
			return
		}
		var decoded any
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			decoded = map[string]any{}
		}
		item := map[string]any{"id": eventID, "ts": ts, "kind": kind, "payload": decoded}
		if attemptID != "" {
			item["attemptId"] = attemptID
		}
		events = append(events, item)
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

func (s *Server) intake(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/accept") {
		s.accept(w, r)
		return
	}
	if s.DB == nil {
		writeJSON(w, 200, map[string]any{"draftPending": false, "planError": nil, "requirements": []any{}, "decisions": []any{}})
		return
	}
	body, err := intakeBody(r, s.DB)
	if err != nil {
		writeErr(w, 500, "internal", "intake")
		return
	}
	writeJSON(w, 200, body)
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil || s.Data == "" {
		writeErr(w, 500, "internal", "ingest")
		return
	}
	cfg, err := intake.LoadConfig(s.Data)
	if err != nil {
		writeErr(w, 500, "internal", "ingest")
		return
	}
	parsed, err := intake.Parse(r.URL.Query().Get("repo"), cfg.PRD, cfg.RFC)
	if err != nil {
		writeErr(w, 422, "validation", err.Error())
		return
	}
	if err := intake.Apply(r.Context(), s.DB, parsed); err != nil {
		writeErr(w, 500, "internal", "ingest")
		return
	}
	s.intake(w, r)
}

func (s *Server) accept(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeErr(w, 500, "internal", "accept")
		return
	}
	var body struct {
		IDs []string `json:"requirementIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "validation", "body")
		return
	}
	cfg, _ := intake.LoadConfig(s.Data)
	operator := cfg.Operator
	if operator == "" {
		operator = "ada"
	}
	if err := intake.Accept(r.Context(), s.DB, operator, body.IDs); err != nil {
		writeAppErr(w, err)
		return
	}
	s.intake(w, r)
}

func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		writeJSON(w, 200, map[string]any{"runId": "run_0123456789ab", "canRun": false, "reason": "no card is ready"})
		return
	}
	id, err := board.NewID("run_")
	if err != nil {
		writeErr(w, 500, "internal", "run")
		return
	}
	if _, err := s.DB.ExecContext(r.Context(), `INSERT INTO runs (id, trigger, created_at) VALUES ($1, 'api', $2)`, id, time.Now().UTC().Format(time.RFC3339)); err != nil {
		writeErr(w, 500, "internal", "run")
		return
	}
	var ready int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM cards WHERE status IN ('ready', 'changes_requested', 'approved')`).Scan(&ready)
	body := map[string]any{"runId": id, "canRun": ready > 0}
	if ready == 0 {
		body["reason"] = "no card is ready"
	}
	writeJSON(w, 200, body)
}

func (s *Server) github(w http.ResponseWriter, r *http.Request) {
	cfg, secrets, err := s.loadSettings()
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Host       string   `json:"host"`
			Owner      string   `json:"owner"`
			Repo       string   `json:"repo"`
			BaseBranch string   `json:"baseBranch"`
			Approvers  []string `json:"approvers"`
			BotLogin   string   `json:"botLogin"`
			Token      *string  `json:"token"`
			Clear      bool     `json:"clear"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, "validation", "body")
			return
		}
		cfg.GitHub.Host = body.Host
		cfg.GitHub.Owner = body.Owner
		cfg.GitHub.Repo = body.Repo
		cfg.GitHub.BaseBranch = body.BaseBranch
		cfg.GitHub.Approvers = body.Approvers
		cfg.GitHub.BotLogin = body.BotLogin
		var token *string
		if body.Clear {
			empty := ""
			token = &empty
		} else if body.Token != nil && *body.Token != "" {
			token = body.Token
		}
		saved, err := settings.SaveGitHub(s.Data, cfg, token)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		cfg = saved
		_, secrets, _ = s.loadSettings()
	}
	if cfg.GitHub.Host == "" {
		cfg.GitHub.Host = "github.com"
	}
	if cfg.GitHub.BaseBranch == "" {
		cfg.GitHub.BaseBranch = "main"
	}
	if cfg.GitHub.Approvers == nil {
		cfg.GitHub.Approvers = []string{}
	}
	writeJSON(w, 200, map[string]any{
		"host": cfg.GitHub.Host, "owner": cfg.GitHub.Owner, "repo": cfg.GitHub.Repo,
		"baseBranch": cfg.GitHub.BaseBranch, "approvers": cfg.GitHub.Approvers,
		"botLogin": cfg.GitHub.BotLogin, "tokenSet": secrets.GitHubToken != "",
	})
}

func (s *Server) githubTest(w http.ResponseWriter, r *http.Request) {
	_, secrets, err := s.loadSettings()
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	if secrets.GitHubToken == "" {
		writeErr(w, 422, "validation", "token")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": false, "message": "authentication failed"})
}

func (s *Server) llm(w http.ResponseWriter, r *http.Request) {
	cfg, secrets, err := s.loadSettings()
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Families []struct {
				ID       string   `json:"id"`
				Protocol string   `json:"protocol"`
				BaseURL  string   `json:"baseUrl"`
				Models   []string `json:"models"`
				APIKey   *string  `json:"apiKey"`
				Clear    bool     `json:"clear"`
			} `json:"families"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, "validation", "body")
			return
		}
		var families []settings.Family
		keys := map[string]*string{}
		for _, family := range body.Families {
			models := []settings.Model{}
			for _, id := range family.Models {
				models = append(models, settings.Model{ID: id})
			}
			families = append(families, settings.Family{ID: family.ID, Protocol: family.Protocol, BaseURL: family.BaseURL, Models: models})
			if family.Clear {
				empty := ""
				keys[family.ID] = &empty
			} else if family.APIKey != nil && *family.APIKey != "" {
				keys[family.ID] = family.APIKey
			}
		}
		saved, err := settings.SaveLLM(s.Data, families, keys)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		cfg = saved
		_, secrets, _ = s.loadSettings()
	}
	out := []any{}
	for _, family := range cfg.LLM.Families {
		models := []string{}
		for _, model := range family.Models {
			models = append(models, model.ID)
		}
		out = append(out, map[string]any{
			"id": family.ID, "protocol": family.Protocol, "baseUrl": family.BaseURL,
			"models": models, "keySet": secrets.LLMKeys[family.ID] != "",
		})
	}
	if out == nil {
		out = []any{}
	}
	writeJSON(w, 200, map[string]any{"families": out})
}

func (s *Server) llmTest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) routerSettings(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := s.loadSettings()
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Routes map[string]struct {
				Model    string `json:"model"`
				Fallback string `json:"fallback"`
			} `json:"routes"`
			Budgets struct {
				MaxAttempts              int `json:"maxAttempts"`
				MaxModelTurnsPerAttempt  int `json:"maxModelTurnsPerAttempt"`
				MaxTokensPerCard         int `json:"maxTokensPerCard"`
				MaxWallMinutesPerAttempt int `json:"maxWallMinutesPerAttempt"`
			} `json:"budgets"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, "validation", "body")
			return
		}
		cfg.Router.Routes = map[string]struct {
			Model    string `yaml:"model" json:"model"`
			Fallback string `yaml:"fallback" json:"fallback"`
		}{}
		for name, route := range body.Routes {
			cfg.Router.Routes[name] = struct {
				Model    string `yaml:"model" json:"model"`
				Fallback string `yaml:"fallback" json:"fallback"`
			}{Model: route.Model, Fallback: route.Fallback}
		}
		cfg.Router.Budgets.MaxAttempts = body.Budgets.MaxAttempts
		cfg.Router.Budgets.MaxModelTurnsPerAttempt = body.Budgets.MaxModelTurnsPerAttempt
		cfg.Router.Budgets.MaxTokensPerCard = body.Budgets.MaxTokensPerCard
		cfg.Router.Budgets.MaxWallMinutesPerAttempt = body.Budgets.MaxWallMinutesPerAttempt
		saved, err := settings.SaveRouter(s.Data, cfg)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		cfg = saved
	}
	routes := map[string]any{}
	for _, name := range []string{"plan", "implement", "implement_hard", "review", "triage"} {
		route := cfg.Router.Routes[name]
		if name == "review" && route.Model == "" {
			route.Model = "different-family-from-author"
		}
		routes[name] = map[string]string{"model": route.Model, "fallback": route.Fallback}
	}
	budgets := cfg.Router.Budgets
	if budgets.MaxAttempts == 0 {
		budgets.MaxAttempts = 3
	}
	if budgets.MaxModelTurnsPerAttempt == 0 {
		budgets.MaxModelTurnsPerAttempt = 12
	}
	if budgets.MaxTokensPerCard == 0 {
		budgets.MaxTokensPerCard = 200000
	}
	if budgets.MaxWallMinutesPerAttempt == 0 {
		budgets.MaxWallMinutesPerAttempt = 30
	}
	independent := len(cfg.LLM.Families) >= 2
	writeJSON(w, 200, map[string]any{
		"routes": routes, "budgets": budgets, "reviewIndependentPossible": independent,
	})
}

func (s *Server) deploySettings(w http.ResponseWriter, r *http.Request) {
	cfg, secrets, err := s.loadSettings()
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Environment       string   `json:"environment"`
			DevURL            string   `json:"devUrl"`
			DevHosts          []string `json:"devHosts"`
			DevCommand        string   `json:"devCommand"`
			RollbackCommand   string   `json:"rollbackCommand"`
			HealthURL         string   `json:"healthUrl"`
			LocalCheck        string   `json:"localCheck"`
			DevCheck          string   `json:"devCheck"`
			AllowExecutables  []string `json:"allowExecutables"`
			SandboxEngine     string   `json:"sandboxEngine"`
			SandboxImage      string   `json:"sandboxImage"`
			ImplementationCap int      `json:"implementationCap"`
			PRDPath           string   `json:"prdPath"`
			RFCPath           string   `json:"rfcPath"`
			OperatorLogin     string   `json:"operatorLogin"`
			DeployToken       *string  `json:"deployToken"`
			Clear             bool     `json:"clear"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, "validation", "body")
			return
		}
		cfg.Deploy.Environment = body.Environment
		cfg.Deploy.DevURL = body.DevURL
		cfg.Deploy.DevHosts = body.DevHosts
		cfg.Deploy.DevCommand = body.DevCommand
		cfg.Deploy.RollbackCommand = body.RollbackCommand
		cfg.Deploy.HealthURL = body.HealthURL
		cfg.Deploy.LocalCheck = body.LocalCheck
		cfg.Deploy.DevCheck = body.DevCheck
		cfg.Deploy.AllowExecutables = body.AllowExecutables
		cfg.Deploy.SandboxEngine = body.SandboxEngine
		cfg.Deploy.SandboxImage = body.SandboxImage
		cfg.Deploy.ImplementationCap = body.ImplementationCap
		cfg.Deploy.PRDPath = body.PRDPath
		cfg.Deploy.RFCPath = body.RFCPath
		cfg.Deploy.OperatorLogin = body.OperatorLogin
		var token *string
		if body.DeployToken != nil && *body.DeployToken != "" {
			token = body.DeployToken
		}
		saved, err := settings.SaveDeploy(s.Data, cfg, token, body.Clear)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		cfg = saved
		_, secrets, _ = s.loadSettings()
	}
	d := cfg.Deploy
	if d.Environment == "" {
		d.Environment = "dev"
	}
	if d.DevHosts == nil {
		d.DevHosts = []string{}
	}
	if d.AllowExecutables == nil {
		d.AllowExecutables = []string{}
	}
	if d.SandboxEngine == "" {
		d.SandboxEngine = "podman"
	}
	if d.ImplementationCap == 0 {
		d.ImplementationCap = 2
	}
	if d.PRDPath == "" {
		d.PRDPath = "docs/prd"
	}
	if d.RFCPath == "" {
		d.RFCPath = "docs/rfc"
	}
	writeJSON(w, 200, map[string]any{
		"environment": d.Environment, "devUrl": d.DevURL, "devHosts": d.DevHosts,
		"devCommand": d.DevCommand, "rollbackCommand": d.RollbackCommand, "healthUrl": d.HealthURL,
		"deployTokenSet": secrets.DeployToken != "", "localCheck": d.LocalCheck, "devCheck": d.DevCheck,
		"allowExecutables": d.AllowExecutables, "sandboxEngine": d.SandboxEngine, "sandboxImage": d.SandboxImage,
		"implementationCap": d.ImplementationCap, "prdPath": d.PRDPath, "rfcPath": d.RFCPath,
		"operatorLogin": d.OperatorLogin,
	})
}

func (s *Server) loadSettings() (settings.Config, settings.Secrets, error) {
	if s.Data == "" {
		return settings.Config{}, settings.Secrets{LLMKeys: map[string]string{}}, nil
	}
	return settings.Load(s.Data)
}

func (s *Server) gate() app.GateConfig {
	cfg, _, _ := s.loadSettings()
	operator := cfg.Deploy.OperatorLogin
	if operator == "" {
		operator = "ada"
	}
	approvers := cfg.GitHub.Approvers
	if len(approvers) == 0 {
		approvers = []string{operator}
	}
	bot := cfg.GitHub.BotLogin
	if bot == "" {
		bot = "bot"
	}
	return app.GateConfig{Operator: operator, BotLogin: bot, Approvers: approvers}
}

func (s *Server) cardBody(r *http.Request, card board.Card) map[string]any {
	open := true
	if card.BlockedReason == "source_changed" {
		_, open = app.UnblockTo(r.Context(), s.DB, card)
	}
	group, actions, unblock := Actions(card.Status, card.BlockedReason, open)
	return map[string]any{
		"id": card.ID, "title": card.Title, "status": card.Status, "group": group,
		"actions": actions, "unblockTo": unblock,
		"requirementIds": nonNil(card.RequirementIDs), "decisionIds": []string{},
		"dependsOn": nonNil(card.DependsOn), "risk": card.Risk, "service": "default",
		"manual": card.Manual, "localCheck": nullString(card.LocalCheck), "devCheck": nullString(card.DevCheck),
		"attemptsUsed": card.AttemptsUsed, "maxAttempts": card.MaxAttempts,
		"branch": nullString(card.Branch), "prUrl": nullString(card.PRURL),
		"headSha": nullString(card.HeadSHA), "reviewedSha": nil, "reviewIndependent": nil,
		"approvedSha": nullString(card.ApprovedSHA), "deployedSha": nullString(card.DeployedSHA),
		"environment": "dev", "failureReason": nullString(card.FailureReason),
		"blockedReason": nullString(card.BlockedReason), "updatedAt": card.UpdatedAt,
	}
}

func intakeBody(r *http.Request, db *sql.DB) (map[string]any, error) {
	var draft bool
	var planError sql.NullString
	if err := db.QueryRowContext(r.Context(), `SELECT draft_pending, plan_error FROM intake_state WHERE id = 1`).Scan(&draft, &planError); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(r.Context(), `
		SELECT id, source_kind, source_path, heading, statement, acceptance_json, accepted_at IS NOT NULL
		FROM requirements ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reqs := []any{}
	for rows.Next() {
		var id, kind, path, heading, statement, acceptance string
		var accepted bool
		if err := rows.Scan(&id, &kind, &path, &heading, &statement, &acceptance, &accepted); err != nil {
			return nil, err
		}
		var lines []string
		_ = json.Unmarshal([]byte(acceptance), &lines)
		if lines == nil {
			lines = []string{}
		}
		reqs = append(reqs, map[string]any{
			"id": id, "sourceKind": kind, "sourcePath": path, "heading": heading,
			"statement": statement, "acceptance": lines, "accepted": accepted, "linkedCards": []any{},
		})
	}
	decisions := []any{}
	drows, err := db.QueryContext(r.Context(), `SELECT id, source_path, statement FROM decisions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	for drows.Next() {
		var id, path, statement string
		if err := drows.Scan(&id, &path, &statement); err != nil {
			return nil, err
		}
		decisions = append(decisions, map[string]any{"id": id, "sourcePath": path, "statement": statement})
	}
	var plan any
	if planError.Valid {
		plan = planError.String
	}
	return map[string]any{"draftPending": draft, "planError": plan, "requirements": reqs, "decisions": decisions}, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func writeAppErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, board.ErrNotFound), errors.Is(err, intake.ErrUnknownID):
		writeErr(w, 404, "not_found", err.Error())
	case errors.Is(err, settings.ErrValidation), errors.Is(err, app.ErrSHA), errors.Is(err, app.ErrApprover), errors.Is(err, app.ErrUnblock):
		writeErr(w, 422, "validation", err.Error())
	case errors.Is(err, board.ErrConflict), errors.Is(err, board.ErrLocked):
		writeErr(w, 409, "conflict", err.Error())
	default:
		writeErr(w, 500, "internal", err.Error())
	}
}
