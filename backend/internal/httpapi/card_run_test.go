package httpapi_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
)

type runCard struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Actions   []string `json:"actions"`
	Queued    bool     `json:"queued"`
	WaitingOn []string `json:"waitingOn"`
}

type runBoard struct {
	CanRun     bool      `json:"canRun"`
	Running    bool      `json:"running"`
	ReadyCount int       `json:"readyCount"`
	Cards      []runCard `json:"cards"`
}

func runMember(t *testing.T, srv *httpapi.Server, role string) string {
	t.Helper()
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	user := seedUser(t, srv, role+"@example.com", "password-long-2", role, false)
	if err := app.AddMember(context.Background(), srv.DB, admin, "tribe_default", user.ID); err != nil {
		t.Fatal(err)
	}
	return loginToken(t, srv, role+"@example.com", "password-long-2")
}

func statusCard(t *testing.T, srv *httpapi.Server, status string, deps ...string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: status, DependsOn: deps})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET project_id = 'proj_default' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	return card
}

func decodeRunCard(t *testing.T, body []byte) runCard {
	t.Helper()
	var card runCard
	if err := json.Unmarshal(body, &card); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return card
}

func openRuns(t *testing.T, srv *httpapi.Server, cardID string) int {
	t.Helper()
	var n int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM runs WHERE card_id = $1 AND processed_at IS NULL`, cardID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func boardOf(t *testing.T, srv *httpapi.Server, token string) runBoard {
	t.Helper()
	rec := getAuth(t, srv, "/api/projects/proj_default/cards", token)
	if rec.Code != 200 {
		t.Fatalf("cards = %d %s", rec.Code, rec.Body.String())
	}
	var out runBoard
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCardRunQueues(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "developer")
	card := statusCard(t, srv, "ready")
	path := "/api/projects/proj_default/cards/" + card.ID

	before := decodeRunCard(t, getAuth(t, srv, path, token).Body.Bytes())
	if !slices.Contains(before.Actions, "run") || before.Queued {
		t.Fatalf("before = %+v", before)
	}
	for i := 0; i < 2; i++ {
		rec := postJSON(t, srv, path+"/run", token, map[string]any{})
		if rec.Code != 200 {
			t.Fatalf("run %d = %d %s", i, rec.Code, rec.Body.String())
		}
		after := decodeRunCard(t, rec.Body.Bytes())
		if !after.Queued || slices.Contains(after.Actions, "run") || after.Status != "ready" {
			t.Fatalf("after %d = %+v", i, after)
		}
	}
	if n := openRuns(t, srv, card.ID); n != 1 {
		t.Fatalf("open runs = %d, want 1", n)
	}
}

func TestCardRunNeedsDependencies(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "developer")
	dep := statusCard(t, srv, "in_progress")
	card := statusCard(t, srv, "ready", dep.ID)
	path := "/api/projects/proj_default/cards/" + card.ID

	got := decodeRunCard(t, getAuth(t, srv, path, token).Body.Bytes())
	if slices.Contains(got.Actions, "run") || !slices.Equal(got.WaitingOn, []string{dep.ID}) {
		t.Fatalf("card = %+v", got)
	}
	if rec := postJSON(t, srv, path+"/run", token, map[string]any{}); rec.Code != 409 {
		t.Fatalf("run = %d %s, want 409", rec.Code, rec.Body.String())
	}
	if n := openRuns(t, srv, card.ID); n != 0 {
		t.Fatalf("open runs = %d, want 0", n)
	}
}

func TestCardRunNotReady(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "lead_developer")
	card := statusCard(t, srv, "in_progress")
	if rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/run", token, map[string]any{}); rec.Code != 409 {
		t.Fatalf("run = %d %s, want 409", rec.Code, rec.Body.String())
	}
}

func TestCardRunRole(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "product_owner")
	card := statusCard(t, srv, "ready")
	path := "/api/projects/proj_default/cards/" + card.ID

	got := decodeRunCard(t, getAuth(t, srv, path, token).Body.Bytes())
	if slices.Contains(got.Actions, "run") {
		t.Fatalf("product owner sees run: %+v", got)
	}
	if rec := postJSON(t, srv, path+"/run", token, map[string]any{}); rec.Code != 403 {
		t.Fatalf("run = %d %s, want 403", rec.Code, rec.Body.String())
	}
	if b := boardOf(t, srv, token); b.CanRun || b.ReadyCount != 1 {
		t.Fatalf("board = canRun %t readyCount %d", b.CanRun, b.ReadyCount)
	}
}

func TestBoardRunningWhileProjectRunOpen(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "developer")
	if b := boardOf(t, srv, token); b.Running {
		t.Fatal("running with no run request")
	}
	card := statusCard(t, srv, "ready")
	if rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/run", token, map[string]any{}); rec.Code != 200 {
		t.Fatalf("run = %d %s", rec.Code, rec.Body.String())
	}
	if b := boardOf(t, srv, token); b.Running {
		t.Fatal("a card-scoped request set running")
	}
	if _, err := srv.DB.Exec(`
		INSERT INTO runs (id, trigger, created_at, project_id) VALUES ('run_accept000001', 'accept', '2026-10-11T00:00:00Z', 'proj_default')`); err != nil {
		t.Fatal(err)
	}
	if b := boardOf(t, srv, token); !b.Running {
		t.Fatal("open project request did not set running")
	}
	if _, err := srv.DB.Exec(`UPDATE runs SET processed_at = '2026-10-11T00:00:01Z' WHERE id = 'run_accept000001'`); err != nil {
		t.Fatal(err)
	}
	if b := boardOf(t, srv, token); b.Running {
		t.Fatal("processed request still running")
	}
}

func TestRunReadyCards(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "qa")
	done := statusCard(t, srv, "done")
	blocker := statusCard(t, srv, "in_progress")
	first := statusCard(t, srv, "ready")
	second := statusCard(t, srv, "ready", done.ID)
	waiting := statusCard(t, srv, "ready", blocker.ID)
	statusCard(t, srv, "changes_requested")

	if b := boardOf(t, srv, token); !b.CanRun || b.ReadyCount != 2 {
		t.Fatalf("board = canRun %t readyCount %d", b.CanRun, b.ReadyCount)
	}
	var res struct {
		Queued     int `json:"queued"`
		ReadyCount int `json:"readyCount"`
	}
	rec := postJSON(t, srv, "/api/projects/proj_default/runs", token, map[string]any{})
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != 200 || res.Queued != 2 || res.ReadyCount != 0 {
		t.Fatalf("runs = %d %s", rec.Code, rec.Body.String())
	}
	for _, id := range []string{first.ID, second.ID} {
		if n := openRuns(t, srv, id); n != 1 {
			t.Fatalf("%s open runs = %d, want 1", id, n)
		}
	}
	if n := openRuns(t, srv, waiting.ID); n != 0 {
		t.Fatalf("waiting card queued %d runs", n)
	}
	rec = postJSON(t, srv, "/api/projects/proj_default/runs", token, map[string]any{})
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.Queued != 0 {
		t.Fatalf("repeat runs = %d %s", rec.Code, rec.Body.String())
	}
	b := boardOf(t, srv, token)
	if b.CanRun || b.ReadyCount != 0 {
		t.Fatalf("board after = canRun %t readyCount %d", b.CanRun, b.ReadyCount)
	}
	for _, card := range b.Cards {
		if card.ID == first.ID && !card.Queued {
			t.Fatalf("first card not queued: %+v", card)
		}
	}
}
