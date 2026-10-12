package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/trace"
)

func usageServer(t *testing.T) (*httpapi.Server, string, string) {
	t.Helper()
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	seedUser(t, srv, "admin@example.com", "password-long-1", "lead_developer", true)
	seedUser(t, srv, "dev@example.com", "password-long-2", "lead_developer", false)
	return srv, loginToken(t, srv, "admin@example.com", "password-long-1"), loginToken(t, srv, "dev@example.com", "password-long-2")
}

func TestUsageAdminOnly(t *testing.T) {
	srv, admin, dev := usageServer(t)
	card, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: "human_review", Title: "card"})
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format(time.RFC3339)
	if _, err := trace.Append(context.Background(), srv.DB, trace.Event{
		TS: today, CardID: card.ID, ProjectID: "proj_default", Kind: "model_call",
		Payload: map[string]any{"role": "implementer", "taskClass": "implement", "modelId": "model-a", "inputTokens": 12, "outputTokens": 3, "text": "REPLY-TEXT"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if rec := getAuth(t, srv, "/api/usage", ""); rec.Code != 401 {
		t.Fatalf("no session = %d", rec.Code)
	}
	if rec := getAuth(t, srv, "/api/usage", dev); rec.Code != 403 {
		t.Fatalf("member = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, srv, "/api/usage?from=bad", dev); rec.Code != 403 {
		t.Fatalf("member with a bad range = %d, want 403 before the range is read", rec.Code)
	}
	rec := getAuth(t, srv, "/api/usage", admin)
	if rec.Code != 200 {
		t.Fatalf("admin = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "REPLY-TEXT") {
		t.Fatalf("usage carries reply text: %s", rec.Body.String())
	}
	var body struct {
		Rows []struct {
			ModelID     string `json:"modelId"`
			Task        string `json:"task"`
			Calls       int64  `json:"calls"`
			InputTokens int64  `json:"inputTokens"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Rows) != 1 || body.Rows[0].ModelID != "model-a" || body.Rows[0].Task != "implement" || body.Rows[0].InputTokens != 12 {
		t.Fatalf("rows = %+v", body.Rows)
	}
}

func TestUsageRange(t *testing.T) {
	srv, admin, _ := usageServer(t)
	rec := getAuth(t, srv, "/api/usage", admin)
	var report struct {
		From string `json:"from"`
		To   string `json:"to"`
		Rows []any  `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil || rec.Code != 200 {
		t.Fatalf("default = %d %s", rec.Code, rec.Body.String())
	}
	to := time.Now().UTC().Format(time.DateOnly)
	from := time.Now().UTC().AddDate(0, 0, -29).Format(time.DateOnly)
	if report.To != to || report.From != from || report.Rows == nil {
		t.Fatalf("default range = %+v, want %s to %s with empty rows", report, from, to)
	}
	for query, field := range map[string]string{
		"from=2026-10-05&to=2026-10-01": "from",
		"from=2025-01-01&to=2026-01-02": "from",
		"from=10/01/2026":               "from",
		"to=tomorrow":                   "to",
	} {
		rec := getAuth(t, srv, "/api/usage?"+query, admin)
		var body struct {
			Error struct {
				Code   string            `json:"code"`
				Fields map[string]string `json:"fields"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != 422 || body.Error.Fields[field] == "" {
			t.Fatalf("%s = %d %s, want 422 on %s", query, rec.Code, rec.Body.String(), field)
		}
	}
	if rec := getAuth(t, srv, "/api/usage?from=2025-10-01&to=2026-10-01", admin); rec.Code != 200 {
		t.Fatalf("366 days = %d %s", rec.Code, rec.Body.String())
	}
}

type usageBody struct {
	Rows []struct {
		ProjectID string `json:"projectId"`
		UserID    string `json:"userId"`
		Task      string `json:"task"`
		Calls     int64  `json:"calls"`
	} `json:"rows"`
}

func decodeUsage(t *testing.T, raw []byte) usageBody {
	t.Helper()
	var body usageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

// scopedUsageBoard has Gadget in its own tribe, a call on a card in each
// tribe, and the users who may or may not read each tribe.
func scopedUsageBoard(t *testing.T) (*httpapi.Server, map[string]string, map[string]app.User) {
	t.Helper()
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	ctx := context.Background()
	users := map[string]app.User{}
	tokens := map[string]string{}
	for i, spec := range []struct {
		name  string
		admin bool
	}{{"admin", true}, {"lead", false}, {"member", false}, {"other", false}} {
		email := spec.name + "@example.com"
		password := fmt.Sprintf("password-long-%d", i)
		users[spec.name] = seedUser(t, srv, email, password, "lead_developer", spec.admin)
		tokens[spec.name] = loginToken(t, srv, email, password)
	}
	for _, stmt := range []string{
		`INSERT INTO tribes (id, name, created_at) VALUES ('tribe_gadget', 'Gadget tribe', '2026-10-01T00:00:00Z')`,
		`INSERT INTO projects (id, tribe_id, name, repo_path, created_at) VALUES ('proj_gadget', 'tribe_gadget', 'Gadget', '/tmp/gadget', '2026-10-01T00:00:00Z')`,
	} {
		if _, err := srv.DB.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []struct {
		tribe, user string
		admin       bool
	}{{"tribe_gadget", "lead", true}, {"tribe_gadget", "member", false}, {"tribe_default", "other", true}} {
		if _, err := srv.DB.Exec(`INSERT INTO tribe_members (tribe_id, user_id, admin) VALUES ($1, $2, $3)`, m.tribe, users[m.user].ID, m.admin); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range []struct{ project, owner string }{{"proj_gadget", users["member"].ID}, {"proj_default", users["other"].ID}} {
		card, err := board.InsertCard(ctx, srv.DB, board.Card{Status: "human_review", Title: "card", ProjectID: c.project, OwnerID: c.owner})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := trace.Append(ctx, srv.DB, trace.Event{
			TS: now, CardID: card.ID, ProjectID: c.project, Kind: "model_call",
			Payload: map[string]any{"role": "implementer", "taskClass": "implement", "modelId": "model-a", "inputTokens": 10, "outputTokens": 1, "text": "REPLY-TEXT"},
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := trace.Append(ctx, srv.DB, trace.Event{
		TS: now, ActorID: users["member"].ID, ProjectID: "proj_default", Kind: "requirement_refined",
		Payload: map[string]any{"requirementId": "new", "modelId": "model-a", "inputTokens": 4, "outputTokens": 1},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return srv, tokens, users
}

func TestTribeUsageAccess(t *testing.T) {
	srv, tokens, _ := scopedUsageBoard(t)
	const path = "/api/tribes/tribe_gadget/usage"
	if rec := getAuth(t, srv, path, ""); rec.Code != 401 {
		t.Fatalf("no session = %d", rec.Code)
	}
	for _, who := range []string{"member", "other"} {
		if rec := getAuth(t, srv, path, tokens[who]); rec.Code != 403 {
			t.Fatalf("%s = %d %s", who, rec.Code, rec.Body.String())
		}
	}
	if rec := getAuth(t, srv, "/api/tribes/tribe_missing/usage", tokens["admin"]); rec.Code != 404 {
		t.Fatalf("missing tribe = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, srv, path+"?from=bad", tokens["lead"]); rec.Code != 422 {
		t.Fatalf("tribe admin with a bad range = %d", rec.Code)
	}
	for _, who := range []string{"lead", "admin"} {
		rec := getAuth(t, srv, path, tokens[who])
		if rec.Code != 200 {
			t.Fatalf("%s = %d %s", who, rec.Code, rec.Body.String())
		}
		body := decodeUsage(t, rec.Body.Bytes())
		if len(body.Rows) != 1 || body.Rows[0].ProjectID != "proj_gadget" || strings.Contains(rec.Body.String(), "REPLY-TEXT") {
			t.Fatalf("%s rows = %s, want only Gadget", who, rec.Body.String())
		}
	}
	rec := getAuth(t, srv, "/api/tribes/tribe_default/usage", tokens["other"])
	if rec.Code != 200 {
		t.Fatalf("default tribe admin = %d", rec.Code)
	}
	for _, r := range decodeUsage(t, rec.Body.Bytes()).Rows {
		if r.ProjectID != "proj_default" {
			t.Fatalf("default tribe has a %s row", r.ProjectID)
		}
	}
}

func TestMyUsage(t *testing.T) {
	srv, tokens, users := scopedUsageBoard(t)
	if rec := getAuth(t, srv, "/api/me/usage", ""); rec.Code != 401 {
		t.Fatalf("no session = %d", rec.Code)
	}
	rec := getAuth(t, srv, "/api/me/usage", tokens["member"])
	if rec.Code != 200 {
		t.Fatalf("member = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "other@example.com") || strings.Contains(rec.Body.String(), "REPLY-TEXT") {
		t.Fatalf("my usage leaks another user or reply text: %s", rec.Body.String())
	}
	body := decodeUsage(t, rec.Body.Bytes())
	projects := map[string]int64{}
	for _, r := range body.Rows {
		if r.UserID != users["member"].ID {
			t.Fatalf("row for %s in my usage", r.UserID)
		}
		projects[r.ProjectID] += r.Calls
	}
	if projects["proj_gadget"] != 1 || projects["proj_default"] != 1 || len(projects) != 2 {
		t.Fatalf("my calls by project = %v, want the owned card and the refine", projects)
	}
	if rec := getAuth(t, srv, "/api/me/usage?to=2026-13-01", tokens["member"]); rec.Code != 422 {
		t.Fatalf("bad range = %d", rec.Code)
	}
	if rows := decodeUsage(t, getAuth(t, srv, "/api/me/usage", tokens["admin"]).Body.Bytes()).Rows; len(rows) != 0 {
		t.Fatalf("an admin with no calls of their own sees %d rows", len(rows))
	}
}
