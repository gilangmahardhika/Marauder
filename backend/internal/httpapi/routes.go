package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/conventions"
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
		body := s.cardBody(r, card)
		if readyToRun(body) {
			ready++
		}
		cards = append(cards, body)
	}
	writeJSON(w, 200, s.boardBody(r, cards, ready))
}

// readyToRun is true for a ready card whose dependencies are done and that
// has no open run request.
func readyToRun(body map[string]any) bool {
	waitingOn, _ := body["waitingOn"].([]string)
	queued, _ := body["queued"].(bool)
	return body["status"] == "ready" && !queued && len(waitingOn) == 0
}

func (s *Server) boardBody(r *http.Request, cards []any, ready int) map[string]any {
	user, _ := app.CurrentUser(r.Context())
	return map[string]any{
		"canRun": ready > 0 && actionAllowed(user.Role, "run"), "readyCount": ready,
		"running": false, "cards": cards,
	}
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
			writeJSON(w, 200, s.cardDetail(r, card))
		}
	case "approve":
		var body struct {
			SHA string `json:"sha"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, "validation", "body")
			return
		}
		next, err := app.Approve(r.Context(), s.DB, id, body.SHA, s.gate(r))
		if err != nil {
			if errors.Is(err, app.ErrSHA) {
				writeJSON(w, 422, map[string]any{"error": map[string]any{"code": "validation", "message": err.Error(), "fields": map[string]string{"sha": err.Error()}}})
				return
			}
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardDetail(r, next))
	case "request-changes":
		next, err := app.RequestChanges(r.Context(), s.DB, id, s.gate(r))
		if err != nil {
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardDetail(r, next))
	case "reject":
		next, err := app.Reject(r.Context(), s.DB, id, s.gate(r))
		if err != nil {
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, s.cardDetail(r, next))
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
		writeJSON(w, 200, s.cardDetail(r, next))
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

func (s *Server) llm(w http.ResponseWriter, r *http.Request) {
	cfg, secrets, err := s.loadSettings()
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	if r.Method == http.MethodPut {
		if !requireAdmin(w, r) {
			return
		}
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
		fields := []string{"families"}
		if len(keys) > 0 {
			fields = append(fields, "apiKey")
		}
		s.configChanged(r, "", "llm", fields)
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
	if !requireAdmin(w, r) {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) routerSettings(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := s.loadSettings()
	if err != nil {
		writeErr(w, 500, "internal", "settings")
		return
	}
	if r.Method == http.MethodPut {
		if !requireAdmin(w, r) {
			return
		}
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
		s.configChanged(r, "", "router", []string{"routes", "budgets"})
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

// requireAdmin writes 403 unless the caller is an admin. LLM APIs and the
// router are company settings (RFC-052).
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	user, ok := app.CurrentUser(r.Context())
	if !ok || !user.Admin {
		writeErr(w, 403, "forbidden", "forbidden")
		return false
	}
	return true
}

func (s *Server) loadSettings() (settings.Config, settings.Secrets, error) {
	if s.Data == "" {
		return settings.Config{}, settings.Secrets{LLMKeys: map[string]string{}}, nil
	}
	return settings.Load(s.Data)
}

func (s *Server) gate(r *http.Request) app.GateConfig {
	user, _ := app.CurrentUser(r.Context())
	acct, _ := app.LoadCompany(r.Context(), s.DB)
	return app.GateConfig{
		Operator:  user.GitHubLogin,
		BotLogin:  acct.BotLogin,
		Approvers: []string{user.GitHubLogin},
		ActorID:   user.ID,
	}
}

func (s *Server) cardBody(r *http.Request, card board.Card) map[string]any {
	reviewers, err := app.CardReviewerUsers(r.Context(), s.DB, card.ProjectID, card.ID)
	if err != nil {
		reviewers = nil
	}
	return s.cardJSON(r, card, reviewers[card.ID])
}

func (s *Server) cardJSON(r *http.Request, card board.Card, reviewers []intake.Reviewer) map[string]any {
	open := true
	if card.BlockedReason == "source_changed" {
		_, open = app.UnblockTo(r.Context(), s.DB, card)
	}
	role, userID := "", ""
	if user, ok := app.CurrentUser(r.Context()); ok {
		role, userID = user.Role, user.ID
	}
	group, actions, unblock := Actions(card.Status, card.BlockedReason, open, role)
	if card.OwnerID != "" && card.OwnerID == userID {
		actions = withoutAction(actions, "approve")
	}
	queued, waitingOn, err := app.CardRunState(r.Context(), s.DB, card)
	if err != nil {
		queued, waitingOn = false, []string{}
	}
	if card.Status == "ready" && !queued && len(waitingOn) == 0 && actionAllowed(role, "run") {
		actions = append(actions, "run")
	}
	var owner any
	if found, ok, err := app.OwnerOf(r.Context(), s.DB, card.OwnerID); err == nil && ok {
		owner = map[string]any{"id": found.ID, "name": found.Name, "email": found.Email, "githubLogin": nullString(found.GitHubLogin)}
	}
	skills, err := conventions.CardSkills(r.Context(), s.DB, card.ProjectID, card.ID)
	if err != nil {
		skills = []conventions.SkillRef{}
	}
	if reviewers == nil {
		reviewers = []intake.Reviewer{}
	}
	return map[string]any{
		"id": card.ID, "title": card.Title, "status": card.Status, "group": group,
		"actions": actions, "unblockTo": unblock, "owner": owner, "reviewers": reviewers, "skills": skills,
		"requirementIds": nonNil(card.RequirementIDs), "decisionIds": []string{},
		"dependsOn": nonNil(card.DependsOn), "queued": queued, "waitingOn": waitingOn,
		"risk": card.Risk, "service": "default",
		"manual": card.Manual, "localCheck": nullString(card.LocalCheck), "devCheck": nullString(card.DevCheck),
		"attemptsUsed": card.AttemptsUsed, "maxAttempts": card.MaxAttempts,
		"branch": nullString(card.Branch), "prUrl": nullString(card.PRURL),
		"headSha": nullString(card.HeadSHA), "reviewedSha": nil, "reviewIndependent": nil,
		"approvedSha": nullString(card.ApprovedSHA), "deployedSha": nullString(card.DeployedSHA),
		"environment": "dev", "failureReason": nullString(card.FailureReason),
		"blockedReason": nullString(card.BlockedReason), "updatedAt": card.UpdatedAt,
	}
}

func intakeBody(r *http.Request, db *sql.DB, projectID string) (map[string]any, error) {
	var draft bool
	var planError sql.NullString
	err := db.QueryRowContext(r.Context(), `SELECT draft_pending, plan_error FROM intake_state WHERE project_id = $1`, projectID).Scan(&draft, &planError)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	user, _ := app.CurrentUser(r.Context())
	reviewers, err := intake.Reviewers(r.Context(), db, projectID)
	if err != nil {
		return nil, err
	}
	options, err := intake.ReviewerOptions(r.Context(), db, projectID)
	if err != nil {
		return nil, err
	}
	linked, err := linkedCards(r.Context(), db, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(r.Context(), `
		SELECT q.id, q.source_kind, q.source_path, q.heading, q.statement, q.acceptance_json,
			q.accepted_at IS NOT NULL, u.email, COALESCE(q.created_by, '')
		FROM requirements q LEFT JOIN users u ON u.id = q.created_by
		WHERE q.project_id = $1 ORDER BY q.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reqs := []any{}
	for rows.Next() {
		var id, kind, path, heading, statement, acceptance, creatorID string
		var accepted bool
		var createdBy sql.NullString
		if err := rows.Scan(&id, &kind, &path, &heading, &statement, &acceptance, &accepted, &createdBy, &creatorID); err != nil {
			return nil, err
		}
		var lines []string
		_ = json.Unmarshal([]byte(acceptance), &lines)
		if lines == nil {
			lines = []string{}
		}
		named := reviewers[id]
		if named == nil {
			named = []intake.Reviewer{}
		}
		reqs = append(reqs, map[string]any{
			"id": id, "sourceKind": kind, "sourcePath": path, "heading": heading,
			"statement": statement, "acceptance": lines, "accepted": accepted,
			"createdBy": nullString(createdBy.String), "linkedCards": nonNil(linked[id]),
			"reviewers":        named,
			"canEditReviewers": intake.CanEditReviewers(user.ID, user.Role, creatorID),
		})
	}
	decisions := []any{}
	drows, err := db.QueryContext(r.Context(), `SELECT id, source_path, statement FROM decisions WHERE project_id = $1 ORDER BY id`, projectID)
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
	return map[string]any{
		"draftPending": draft, "planError": plan, "canAccept": canAccept(user.Role),
		"requirements": reqs, "decisions": decisions, "reviewerOptions": options,
	}, nil
}

// linkedCards maps each requirement id to the ids of the project's cards
// that name it, in card creation order.
func linkedCards(ctx context.Context, db *sql.DB, projectID string) (map[string][]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, requirement_ids_json FROM cards
		WHERE project_id = $1 ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var id, reqs string
		if err := rows.Scan(&id, &reqs); err != nil {
			return nil, err
		}
		var ids []string
		if err := json.Unmarshal([]byte(reqs), &ids); err != nil {
			return nil, fmt.Errorf("card %s requirement ids: %w", id, err)
		}
		for _, req := range ids {
			out[req] = append(out[req], id)
		}
	}
	return out, rows.Err()
}

// canAccept reports whether a job role may accept requirements (RFC-053).
func canAccept(role string) bool {
	switch role {
	case "lead_developer", "senior_developer", "qa_lead", "product_owner":
		return true
	}
	return false
}

func withoutAction(actions []string, name string) []string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if action != name {
			out = append(out, action)
		}
	}
	return out
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
	var invalid *settings.Invalid
	switch {
	case errors.As(err, &invalid):
		writeFields(w, 422, "validation", invalid.Error(), invalid.Fields)
	case errors.Is(err, board.ErrNotFound), errors.Is(err, intake.ErrUnknownID):
		writeErr(w, 404, "not_found", err.Error())
	case errors.Is(err, settings.ErrValidation), errors.Is(err, app.ErrSHA), errors.Is(err, app.ErrApprover), errors.Is(err, app.ErrUnblock):
		writeErr(w, 422, "validation", err.Error())
	case errors.Is(err, board.ErrConflict), errors.Is(err, board.ErrLocked), errors.Is(err, app.ErrNotRunnable):
		writeErr(w, 409, "conflict", err.Error())
	case errors.Is(err, app.ErrOwnerApprove):
		writeErr(w, 403, "forbidden", err.Error())
	default:
		writeErr(w, 500, "internal", err.Error())
	}
}
