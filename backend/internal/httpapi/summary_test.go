package httpapi_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/trace"
)

func summaryFixture(t *testing.T) (tribeFixture, string, string) {
	t.Helper()
	f := newTribeFixture(t)
	f.srv.Data = dataRoot(t)
	project := newProject(t, f.srv, f.adminToken, f.tribe, "Widget", "widget")
	memberToken := loginToken(t, f.srv, "member@example.com", "password-long-3")
	return f, project, memberToken
}

func TestProjectSummaryMember(t *testing.T) {
	f, project, memberToken := summaryFixture(t)
	ctx := context.Background()
	for _, status := range []string{"ready", "ready", "waiting_answer", "human_review", "blocked", "done"} {
		card, err := board.InsertCard(ctx, f.srv.DB, board.Card{Status: status, Title: status, ProjectID: project})
		if err != nil {
			t.Fatal(err)
		}
		if status == "blocked" {
			if _, err := f.srv.DB.Exec(`UPDATE cards SET blocked_reason = 'token_cap' WHERE id = $1`, card.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	rec := getAuth(t, f.srv, "/api/projects/"+project+"/summary", memberToken)
	if rec.Code != 200 {
		t.Fatalf("member = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Groups struct {
			Waiting  int `json:"waiting"`
			Question int `json:"question"`
			Review   int `json:"review"`
			Finished int `json:"finished"`
			Stopped  int `json:"stopped"`
		} `json:"groups"`
		Attention struct {
			ReadyCount    int `json:"readyCount"`
			WaitingAnswer int `json:"waitingAnswer"`
			HumanReview   int `json:"humanReview"`
			Blocked       []struct {
				Reason string `json:"reason"`
				Count  int    `json:"count"`
			} `json:"blocked"`
			ImplementationCap int `json:"implementationCap"`
		} `json:"attention"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Groups.Waiting != 2 || body.Groups.Question != 1 || body.Groups.Review != 1 || body.Groups.Finished != 1 || body.Groups.Stopped != 1 {
		t.Fatalf("groups = %+v", body.Groups)
	}
	if body.Attention.ReadyCount != 2 || body.Attention.WaitingAnswer != 1 || body.Attention.HumanReview != 1 {
		t.Fatalf("attention = %+v", body.Attention)
	}
	if len(body.Attention.Blocked) != 1 || body.Attention.Blocked[0].Reason != "token_cap" || body.Attention.Blocked[0].Count != 1 {
		t.Fatalf("blocked = %+v", body.Attention.Blocked)
	}
	if body.Attention.ImplementationCap != 2 {
		t.Fatalf("cap = %d", body.Attention.ImplementationCap)
	}
}

func TestProjectSummaryOutsider404(t *testing.T) {
	f, project, _ := summaryFixture(t)
	outsider := loginToken(t, f.srv, "out@example.com", "password-long-4")
	if rec := getAuth(t, f.srv, "/api/projects/"+project+"/summary", outsider); rec.Code != 404 {
		t.Fatalf("outsider = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, f.srv, "/api/projects/proj_missing/summary", outsider); rec.Code != 404 {
		t.Fatalf("missing = %d %s", rec.Code, rec.Body.String())
	}
	chief := getAuth(t, f.srv, "/api/projects/"+project+"/summary", f.chiefToken)
	if chief.Code != 200 {
		t.Fatalf("tribe admin member = %d %s", chief.Code, chief.Body.String())
	}
}

func TestTribeSummaryManageOnly(t *testing.T) {
	f, project, memberToken := summaryFixture(t)
	ctx := context.Background()
	if _, err := board.InsertCard(ctx, f.srv.DB, board.Card{Status: "ready", Title: "A", ProjectID: project}); err != nil {
		t.Fatal(err)
	}
	path := "/api/tribes/" + f.tribe + "/summary"
	if rec := getAuth(t, f.srv, path, f.chiefToken); rec.Code != 200 {
		t.Fatalf("tribe admin = %d %s", rec.Code, rec.Body.String())
	} else {
		var body struct {
			Groups struct {
				Waiting int `json:"waiting"`
			} `json:"groups"`
			Projects []struct {
				ProjectID string `json:"projectId"`
			} `json:"projects"`
			MCP struct {
				Changed int `json:"changed"`
			} `json:"mcp"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Groups.Waiting < 1 || len(body.Projects) != 1 || body.Projects[0].ProjectID != project {
			t.Fatalf("tribe summary = %+v", body)
		}
	}
	if rec := getAuth(t, f.srv, path, memberToken); rec.Code != 403 {
		t.Fatalf("member = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, f.srv, "/api/tribes/tribe_missing/summary", f.adminToken); rec.Code != 404 {
		t.Fatalf("missing tribe = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, f.srv, path+"?from=bad", memberToken); rec.Code != 403 {
		t.Fatalf("member with bad range = %d, want 403 before range", rec.Code)
	}
}

func TestSummaryRange(t *testing.T) {
	f, project, memberToken := summaryFixture(t)
	ctx := context.Background()
	card, err := board.InsertCard(ctx, f.srv.DB, board.Card{Status: "done", Title: "Shipped", ProjectID: project})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339)
	if _, err := trace.Append(ctx, f.srv.DB, trace.Event{
		TS: ts, CardID: card.ID, ProjectID: project, Kind: "transition",
		Payload: map[string]any{"from": "verifying", "to": "done"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.Append(ctx, f.srv.DB, trace.Event{
		TS: ts, CardID: card.ID, ProjectID: project, Kind: "transition",
		Payload: map[string]any{"from": "agent_review", "to": "changes_requested"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.Append(ctx, f.srv.DB, trace.Event{
		TS: ts, CardID: card.ID, ProjectID: project, Kind: "budget_stop",
		Payload: map[string]any{"cap": "token_cap"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.Append(ctx, f.srv.DB, trace.Event{
		TS: ts, CardID: card.ID, ProjectID: project, Kind: "assertion_failed",
		Payload: map[string]any{"name": "mcp_changed", "server": "tickets", "tool": "list"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	rec := getAuth(t, f.srv, "/api/projects/"+project+"/summary", memberToken)
	if rec.Code != 200 {
		t.Fatalf("default = %d %s", rec.Code, rec.Body.String())
	}
	var report struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Range struct {
			Done   int `json:"done"`
			Rework int `json:"rework"`
			BudgetStops []struct {
				Cap   string `json:"cap"`
				Count int    `json:"count"`
			} `json:"budgetStops"`
		} `json:"range"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	to := now.Format(time.DateOnly)
	from := now.AddDate(0, 0, -29).Format(time.DateOnly)
	if report.To != to || report.From != from {
		t.Fatalf("range days = %s..%s, want %s..%s", report.From, report.To, from, to)
	}
	if report.Range.Done != 1 || report.Range.Rework != 1 {
		t.Fatalf("range totals = %+v", report.Range)
	}
	if len(report.Range.BudgetStops) != 1 || report.Range.BudgetStops[0].Cap != "token_cap" {
		t.Fatalf("budgetStops = %+v", report.Range.BudgetStops)
	}

	bad := getAuth(t, f.srv, "/api/projects/"+project+"/summary?from=not-a-day", memberToken)
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), `"from"`) {
		t.Fatalf("bad from = %d %s", bad.Code, bad.Body.String())
	}

	tribe := getAuth(t, f.srv, "/api/tribes/"+f.tribe+"/summary", f.chiefToken)
	if tribe.Code != 200 {
		t.Fatalf("tribe = %d %s", tribe.Code, tribe.Body.String())
	}
	if !strings.Contains(tribe.Body.String(), `"changed":1`) {
		t.Fatalf("mcp health missing: %s", tribe.Body.String())
	}
}
