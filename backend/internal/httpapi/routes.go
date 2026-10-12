package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/conventions"
	"orchestrator/backend/internal/intake"
	"orchestrator/backend/internal/providers/llm"
	reviewpkg "orchestrator/backend/internal/review"
	"orchestrator/backend/internal/settings"
	"orchestrator/backend/internal/trace"
	"orchestrator/backend/internal/worker"
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
	writeJSON(w, 200, s.boardBody(r, "", cards, ready))
}

// readyToRun is true for a ready card whose dependencies are done and that
// has no open run request.
func readyToRun(body map[string]any) bool {
	waitingOn, _ := body["waitingOn"].([]string)
	queued, _ := body["queued"].(bool)
	return body["status"] == "ready" && !queued && len(waitingOn) == 0
}

// boardBody reports running while the project has an open run request that
// is not scoped to one card, so the board polls until the worker is idle.
func (s *Server) boardBody(r *http.Request, projectID string, cards []any, ready int) map[string]any {
	user, _ := app.CurrentUser(r.Context())
	running := false
	if projectID != "" {
		running, _ = worker.Pending(r.Context(), s.DB, projectID)
	}
	return map[string]any{
		"canRun": ready > 0 && actionAllowed(user.Role, "run"), "readyCount": ready,
		"running": running, "cards": cards,
	}
}

func (s *Server) cardRoute(w http.ResponseWriter, r *http.Request, id, action string) {
	card, err := board.GetCard(r.Context(), s.DB, id)
	if err != nil {
		writeAppErr(w, err)
		return
	}
	if action == "trace" && r.Method == http.MethodGet {
		s.trace(w, r, card)
		return
	}
	if action == "trace/stream" && r.Method == http.MethodGet {
		s.traceStream(w, r, card)
		return
	}
	if action == "diff" && r.Method == http.MethodGet {
		diff, err := app.ReadCardDiff(r.Context(), s.DB, s.jobDeps(), card.ProjectID, id)
		if err != nil {
			writeAppErr(w, err)
			return
		}
		writeJSON(w, 200, diff)
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
		gate := s.gate(r, card)
		result, err := app.ApproveFresh(r.Context(), s.DB, id, body.SHA, gate)
		if err != nil {
			if errors.Is(err, app.ErrSHA) {
				writeJSON(w, 422, map[string]any{"error": map[string]any{"code": "validation", "message": err.Error(), "fields": map[string]string{"sha": err.Error()}}})
				return
			}
			writeAppErr(w, err)
			return
		}
		if result.Fresh {
			app.AnnounceApproval(r.Context(), s.DB, s.jobDeps(), result.Card, gate.Operator, result.Independent)
		}
		writeJSON(w, 200, s.cardDetail(r, result.Card))
	case "request-changes":
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		next, err := app.RequestChanges(r.Context(), s.DB, id, body.Reason, s.gate(r, card))
		if err != nil {
			writeAppErr(w, err)
			return
		}
		s.withdraw(r, card)
		writeJSON(w, 200, s.cardDetail(r, next))
	case "reject":
		next, err := app.Reject(r.Context(), s.DB, id, s.gate(r, card))
		if err != nil {
			writeAppErr(w, err)
			return
		}
		s.withdraw(r, card)
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

// traceOverlap is how far before since a trace poll reads again, so an event
// that committed late or in the same second is not missed.
const traceOverlap = 10 * time.Second

// liveStatuses are the statuses in which a job may still write trace events.
var liveStatuses = map[string]bool{
	"in_progress": true, "self_check": true, "agent_review": true, "deploying_dev": true, "verifying": true,
}

func (s *Server) trace(w http.ResponseWriter, r *http.Request, card board.Card) {
	since, ok := traceSince(w, r)
	if !ok {
		return
	}
	listed, attempts, live, err := s.traceRead(r.Context(), card, since)
	if err != nil {
		writeErr(w, 500, "internal", "trace")
		return
	}
	writeJSON(w, 200, map[string]any{"attempts": attempts, "events": traceEvents(listed), "live": live})
}

// traceSince reads the since query as the time a trace read starts from,
// traceOverlap earlier, or writes a 422.
func traceSince(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := r.URL.Query().Get("since")
	if raw == "" {
		return "", true
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeFields(w, 422, "validation", "since must be an RFC3339 time", map[string]string{"since": "must be an RFC3339 time"})
		return "", false
	}
	return at.Add(-traceOverlap).UTC().Format(time.RFC3339), true
}

// traceRead returns the card's events since since, its attempts, and whether
// a job may still write to its trace.
func (s *Server) traceRead(ctx context.Context, card board.Card, since string) ([]trace.Listed, []any, bool, error) {
	listed, err := trace.ListSince(ctx, s.DB, card.ID, since)
	if err != nil {
		return nil, nil, false, err
	}
	stored, err := board.CardAttempts(ctx, s.DB, card.ProjectID, card.ID)
	if err != nil {
		return nil, nil, false, err
	}
	live := liveStatuses[card.Status]
	if !live {
		if live, _, err = app.CardRunState(ctx, s.DB, card); err != nil {
			return nil, nil, false, err
		}
	}
	attempts := make([]any, 0, len(stored))
	for _, a := range stored {
		var independent any
		if a.ReviewIndependent.Valid {
			independent = a.ReviewIndependent.Bool
		}
		if a.EndedAt == "" {
			live = true
		}
		attempts = append(attempts, map[string]any{
			"id": a.ID, "n": a.N, "taskClass": a.TaskClass, "modelId": a.ModelID,
			"fallbackModelId": a.FallbackModelID, "inputTokens": a.InputTokens, "outputTokens": a.OutputTokens,
			"maxTurns": a.MaxTurns, "commitSha": a.CommitSHA, "reviewModelId": a.ReviewModelID, "reviewVerdict": a.ReviewVerdict,
			"reviewIndependent": independent, "startedAt": a.StartedAt, "endedAt": a.EndedAt, "outcome": a.Outcome,
		})
	}
	return listed, attempts, live, nil
}

func traceEvents(listed []trace.Listed) []any {
	events := make([]any, 0, len(listed))
	for _, ev := range listed {
		var decoded any
		if err := json.Unmarshal([]byte(ev.Payload), &decoded); err != nil {
			decoded = map[string]any{}
		}
		item := map[string]any{"id": ev.ID, "seq": ev.Seq, "ts": ev.TS, "kind": ev.Kind, "payload": decoded}
		if ev.AttemptID != "" {
			item["attemptId"] = ev.AttemptID
		}
		events = append(events, item)
	}
	return events
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
				ID           string                           `json:"id"`
				Protocol     string                           `json:"protocol"`
				BaseURL      string                           `json:"baseUrl"`
				Models       []string                         `json:"models"`
				ModelOptions map[string]settings.ModelOptions `json:"modelOptions"`
				APIKey       *string                          `json:"apiKey"`
				Clear        bool                             `json:"clear"`
			} `json:"families"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, "validation", "body")
			return
		}
		var families []settings.Family
		keys := map[string]*string{}
		for _, family := range body.Families {
			models, err := settings.NewModels(family.Models, family.ModelOptions)
			if err != nil {
				writeAppErr(w, err)
				return
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
			"models": models, "modelOptions": family.Options(), "keySet": secrets.LLMKeys[family.ID] != "",
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
	if r.Method != http.MethodPost {
		writeErr(w, 405, "method_not_allowed", "method not allowed")
		return
	}
	family, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/api/settings/llm/"), "/test")
	if !ok || family == "" || strings.Contains(family, "/") {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	err := app.TestLLM(r.Context(), s.Data, family)
	if errors.Is(err, app.ErrUnknownFamily) {
		writeErr(w, 404, "not_found", "family")
		return
	}
	if errors.Is(err, llm.ErrNoKey) {
		writeFields(w, 422, "validation", "the family has no API key", map[string]string{"apiKey": "set the API key first"})
		return
	}
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
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
		budgets.MaxModelTurnsPerAttempt = settings.DefaultTurns
	}
	if budgets.MaxTokensPerCard == 0 {
		budgets.MaxTokensPerCard = settings.DefaultTokensPerCard
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

func (s *Server) gate(r *http.Request, card board.Card) app.GateConfig {
	user, _ := app.CurrentUser(r.Context())
	acct, _ := app.LoadCompany(r.Context(), s.DB)
	return app.GateConfig{
		Operator:  user.GitHubLogin,
		BotLogin:  acct.BotLogin,
		Approvers: []string{user.GitHubLogin},
		ActorID:   user.ID,
		Solo:      s.soloFor(r.Context(), card),
	}
}

// soloFor reports whether the card's owner and the bot login may pass the
// approval gate: the project has Solo approval on (RFC-067), or every
// requirement of the card has Self review on (RFC-070). A read error keeps
// the default, off.
func (s *Server) soloFor(ctx context.Context, card board.Card) bool {
	if s.soloApproval(card.ProjectID) {
		return true
	}
	self, err := app.CardSelfReview(ctx, s.DB, card)
	return err == nil && self
}

func (s *Server) jobDeps() app.JobDeps {
	return app.JobDeps{Data: s.Data, Key: s.TokenKey, Tools: s.Tools}
}

// withdraw fails the approval status on GitHub when before was an approved
// card that a person sent back (RFC-068).
func (s *Server) withdraw(r *http.Request, before board.Card) {
	if before.Status == "approved" && before.ApprovedSHA != "" {
		app.WithdrawApproval(r.Context(), s.DB, s.jobDeps(), before, before.ApprovedSHA)
	}
}

// soloApproval reports the project's approval.solo setting (RFC-067). A
// missing or unreadable config keeps the default, off.
func (s *Server) soloApproval(projectID string) bool {
	if s.Data == "" || projectID == "" {
		return false
	}
	cfg, _, err := settings.LoadProject(s.Data, projectID)
	return err == nil && cfg.Approval.Solo
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
	selfReview, err := app.CardSelfReview(r.Context(), s.DB, card)
	if err != nil {
		selfReview = false
	}
	if card.OwnerID != "" && card.OwnerID == userID && !selfReview && !s.soloApproval(card.ProjectID) {
		actions = withoutAction(actions, "approve")
	}
	reviewMode, err := app.CardAIReviewMode(r.Context(), s.DB, card)
	if err != nil {
		reviewMode = app.ReviewContinue
	}
	queued, waitingOn, err := app.CardRunState(r.Context(), s.DB, card)
	if err != nil {
		queued, waitingOn = false, []string{}
	}
	if !queued && len(waitingOn) == 0 && actionAllowed(role, "run") {
		if ok, err := app.CanStartImplement(r.Context(), s.DB, card); err == nil && ok {
			actions = append(actions, "run")
		}
	}
	if card.Status != "done" && actionAllowed(role, "instructions") {
		actions = append(actions, "instructions")
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
	var reviewedSHA, reviewIndependent, reviewVerdict any
	if card.PRURL != "" {
		if review, err := board.LatestReview(r.Context(), s.DB, card.ID); err == nil && review.Verdict != "" {
			reviewedSHA = nullString(review.ReviewedSHA)
			reviewVerdict = review.Verdict
			if review.Independent.Valid && review.Verdict != reviewpkg.VerdictFailed {
				reviewIndependent = review.Independent.Bool
			}
			if card.Status == "agent_review" && review.Verdict == reviewpkg.VerdictFailed && actionAllowed(role, "retry-review") {
				actions = append(actions, "retry_review")
			}
		}
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
		"headSha": nullString(card.HeadSHA), "reviewedSha": reviewedSHA, "reviewIndependent": reviewIndependent,
		"reviewVerdict": reviewVerdict, "selfReview": selfReview, "aiReviewMode": reviewMode,
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
			q.accepted_at IS NOT NULL, u.email, COALESCE(q.created_by, ''), q.self_review, q.ai_review_mode
		FROM requirements q LEFT JOIN users u ON u.id = q.created_by
		WHERE q.project_id = $1 ORDER BY q.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reqs := []any{}
	canSetSelfReview := intake.CanSetSelfReview(user.Role, user.Admin)
	for rows.Next() {
		var id, kind, path, heading, statement, acceptance, creatorID, reviewMode string
		var accepted, selfReview bool
		var createdBy sql.NullString
		if err := rows.Scan(&id, &kind, &path, &heading, &statement, &acceptance, &accepted, &createdBy, &creatorID,
			&selfReview, &reviewMode); err != nil {
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
			"selfReview":       selfReview,
			"canSetSelfReview": canSetSelfReview,
			"aiReviewMode":     reviewMode,
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
	case errors.Is(err, board.ErrConflict), errors.Is(err, board.ErrLocked), errors.Is(err, app.ErrNotRunnable), errors.Is(err, app.ErrCardDone),
		errors.Is(err, app.ErrReviewNotFailed):
		writeErr(w, 409, "conflict", err.Error())
	case errors.Is(err, app.ErrOwnerApprove):
		writeErr(w, 403, "forbidden", err.Error())
	default:
		writeErr(w, 500, "internal", err.Error())
	}
}
