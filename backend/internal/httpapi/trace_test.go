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

func TestTraceOrderAndAttempts(t *testing.T) {
	srv, _, card, token := approvalServer(t)
	ctx := context.Background()
	if _, err := srv.DB.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_other', 'tribe_default', 'Other', '/tmp/other', $1)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	attempt := board.Attempt{ID: "att_0123456789ab", CardID: card.ID, N: 1, TaskClass: "implement", ModelID: "model-a", MaxTurns: 8}
	if err := board.InsertAttempt(ctx, srv.DB, attempt); err != nil {
		t.Fatal(err)
	}
	if err := board.AddAttemptTokens(ctx, srv.DB, attempt.ID, 1200, 80); err != nil {
		t.Fatal(err)
	}
	if err := board.FinishAttempt(ctx, srv.DB, attempt.ID, "checks_passed", headSHA); err != nil {
		t.Fatal(err)
	}

	// Ids sort the other way round, so only the insert order puts them right.
	second := "2026-10-11T12:29:25Z"
	appends := []trace.Event{
		{ID: "evt_ffffffffffff", TS: second, CardID: card.ID, Kind: "transition", Payload: map[string]any{"from": "ready", "to": "in_progress"}},
		{ID: "evt_000000000000", TS: second, CardID: card.ID, Kind: "transition", Payload: map[string]any{"from": "in_progress", "to": "blocked"}},
		{ID: "evt_888888888888", TS: second, CardID: card.ID, AttemptID: attempt.ID, Kind: "model_call", Payload: map[string]any{"modelId": "model-a"}},
	}
	for _, ev := range appends {
		if _, err := trace.Append(ctx, srv.DB, ev, nil); err != nil {
			t.Fatal(err)
		}
	}

	rec := getAuth(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/trace", token)
	if rec.Code != 200 {
		t.Fatalf("trace = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Attempts []struct {
			ID                string
			N                 int
			ModelID           string
			InputTokens       int
			OutputTokens      int
			CommitSHA         string `json:"commitSha"`
			Outcome           string
			EndedAt           string
			ReviewIndependent *bool
		}
		Events []struct {
			ID        string
			AttemptID *string
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, ev := range body.Events {
		ids = append(ids, ev.ID)
	}
	if len(ids) != 3 || ids[0] != "evt_ffffffffffff" || ids[1] != "evt_000000000000" || ids[2] != "evt_888888888888" {
		t.Fatalf("event order = %v", ids)
	}
	if body.Events[0].AttemptID != nil || body.Events[2].AttemptID == nil || *body.Events[2].AttemptID != attempt.ID {
		t.Fatalf("attempt ids = %v %v", body.Events[0].AttemptID, body.Events[2].AttemptID)
	}
	if len(body.Attempts) != 1 {
		t.Fatalf("attempts = %+v", body.Attempts)
	}
	got := body.Attempts[0]
	if got.ID != attempt.ID || got.N != 1 || got.ModelID != "model-a" || got.InputTokens != 1200 || got.OutputTokens != 80 ||
		got.Outcome != "checks_passed" || got.CommitSHA != headSHA || got.EndedAt == "" || got.ReviewIndependent != nil {
		t.Fatalf("attempt = %+v", got)
	}

	if other := getAuth(t, srv, "/api/projects/proj_other/cards/"+card.ID+"/trace", token); other.Code != 404 {
		t.Fatalf("trace in another project = %d %s", other.Code, other.Body.String())
	}
}

type traceBody struct {
	Live     bool
	Attempts []struct {
		ID       string
		MaxTurns int `json:"maxTurns"`
	}
	Events []struct {
		ID  string
		Seq int64
		TS  string
	}
}

func readTrace(t *testing.T, raw []byte) traceBody {
	t.Helper()
	var body traceBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestTraceSince(t *testing.T) {
	srv, _, card, token := approvalServer(t)
	ctx := context.Background()
	if _, err := srv.DB.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_other', 'tribe_default', 'Other', '/tmp/other', $1)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	appends := []trace.Event{
		{ID: "evt_old000000000", TS: "2026-10-11T12:00:00Z", CardID: card.ID, Kind: "transition"},
		{ID: "evt_edge00000000", TS: "2026-10-11T12:00:30Z", CardID: card.ID, Kind: "transition"},
		{ID: "evt_same_b000000", TS: "2026-10-11T12:00:40Z", CardID: card.ID, Kind: "transition"},
		{ID: "evt_same_a000000", TS: "2026-10-11T12:00:40Z", CardID: card.ID, Kind: "transition"},
	}
	for _, ev := range appends {
		if _, err := trace.Append(ctx, srv.DB, ev, nil); err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/projects/proj_default/cards/" + card.ID + "/trace"

	rec := getAuth(t, srv, path+"?since=2026-10-11T12:00:40Z", token)
	if rec.Code != 200 {
		t.Fatalf("since = %d %s", rec.Code, rec.Body.String())
	}
	body := readTrace(t, rec.Body.Bytes())
	var ids []string
	for i, ev := range body.Events {
		ids = append(ids, ev.ID)
		if ev.Seq == 0 || (i > 0 && ev.Seq <= body.Events[i-1].Seq) {
			t.Fatalf("seq = %+v", body.Events)
		}
	}
	if len(ids) != 3 || ids[0] != "evt_edge00000000" || ids[1] != "evt_same_b000000" || ids[2] != "evt_same_a000000" {
		t.Fatalf("since events = %v", ids)
	}

	if all := readTrace(t, getAuth(t, srv, path, token).Body.Bytes()); len(all.Events) != 4 {
		t.Fatalf("all events = %d", len(all.Events))
	}
	bad := getAuth(t, srv, path+"?since=yesterday", token)
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), `"since"`) {
		t.Fatalf("bad since = %d %s", bad.Code, bad.Body.String())
	}
	if other := getAuth(t, srv, "/api/projects/proj_other/cards/"+card.ID+"/trace?since=2026-10-11T12:00:40Z", token); other.Code != 404 {
		t.Fatalf("since in another project = %d %s", other.Code, other.Body.String())
	}
}

func TestTraceLive(t *testing.T) {
	srv, _, card, token := approvalServer(t)
	ctx := context.Background()
	path := "/api/projects/proj_default/cards/" + card.ID + "/trace"
	live := func(path string) traceBody {
		t.Helper()
		rec := getAuth(t, srv, path, token)
		if rec.Code != 200 {
			t.Fatalf("trace = %d %s", rec.Code, rec.Body.String())
		}
		return readTrace(t, rec.Body.Bytes())
	}
	if live(path).Live {
		t.Fatal("a card in human_review with no open attempt is live")
	}

	attempt := board.Attempt{ID: "att_live00000000", CardID: card.ID, N: 1, TaskClass: "implement", ModelID: "model-a", MaxTurns: 12}
	if err := board.InsertAttempt(ctx, srv.DB, attempt); err != nil {
		t.Fatal(err)
	}
	if got := live(path); !got.Live || len(got.Attempts) != 1 || got.Attempts[0].MaxTurns != 12 {
		t.Fatalf("open attempt: %+v", got)
	}
	if err := board.FinishAttempt(ctx, srv.DB, attempt.ID, "checks_passed", headSHA); err != nil {
		t.Fatal(err)
	}
	if live(path).Live {
		t.Fatal("live after the attempt ended")
	}

	if _, err := srv.DB.Exec(`
		INSERT INTO runs (id, trigger, created_at, project_id, card_id) VALUES ('run_live00000000', 'api', $1, 'proj_default', $2)`,
		time.Now().UTC().Format(time.RFC3339), card.ID); err != nil {
		t.Fatal(err)
	}
	if !live(path).Live {
		t.Fatal("a queued card is not live")
	}

	working, err := board.InsertCard(ctx, srv.DB, board.Card{ProjectID: "proj_default", Status: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	if !live("/api/projects/proj_default/cards/" + working.ID + "/trace").Live {
		t.Fatal("an in_progress card is not live")
	}
}
