package httpapi_test

import (
	"context"
	"encoding/json"
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
