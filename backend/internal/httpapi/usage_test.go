package httpapi_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
