package app_test

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/providers/fake"
)

// liveRecorder keeps every published frame and, at each turn_end, how many
// model_call events the card had stored.
type liveRecorder struct {
	mu     sync.Mutex
	db     *sql.DB
	frames []live.Frame
	stored []int
}

func (r *liveRecorder) Publish(cardID string, f live.Frame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, f)
	if f.Kind == live.TurnEnd {
		var n int
		_ = r.db.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'model_call'`, cardID).Scan(&n)
		r.stored = append(r.stored, n)
	}
}

// turns joins each turn's delta text, in order, and checks that every turn
// opens with a turn frame for role and closes with turn_end.
func (r *liveRecorder) turns(t *testing.T, role string) []string {
	t.Helper()
	var out []string
	open := false
	for _, f := range r.frames {
		switch f.Kind {
		case live.Turn:
			if open || f.Role != role || f.ModelID == "" || f.AttemptID == "" {
				t.Fatalf("turn frame %+v", f)
			}
			open = true
			out = append(out, "")
		case live.Delta:
			if !open {
				t.Fatalf("delta outside a turn: %+v", f)
			}
			out[len(out)-1] += f.Text
		case live.TurnEnd:
			open = false
		}
	}
	if open {
		t.Fatal("a turn never ended")
	}
	return out
}

func TestLoopStreamsTurnText(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	rec := &liveRecorder{db: f.db}
	provider := loadFake(t, `{"implement":[
		{"tool":"edit_file","path":"hello.txt","text":"job\n","say":"Writing hello.txt with token `+ownerToken+` now."},
		{"tool":"commit","message":"Job commit","say":"Committing."}]}`)
	card, err := app.ImplementCard(context.Background(), f.db, app.JobDeps{
		Data: f.data, Key: tokenKey, Provider: provider, Live: rec,
		Tools: app.Tools{Git: lookGit(t), GH: "/usr/bin/gh", Exec: f.calls.run},
	}, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" {
		t.Fatalf("card = %s", card.Status)
	}
	got := rec.turns(t, "implementer")
	if len(got) != 3 || got[0] != "Writing hello.txt with token [redacted] now." || got[1] != "Committing." || got[2] != "" {
		t.Fatalf("streamed turns = %q, want two replies and the empty one that ends the attempt", got)
	}
	if len(rec.stored) != 3 || rec.stored[0] != 1 || rec.stored[1] != 2 || rec.stored[2] != 3 {
		t.Fatalf("model_call events at each turn_end = %v, want the event stored first", rec.stored)
	}
	for _, fr := range rec.frames {
		if strings.Contains(fr.Text, ownerToken) {
			t.Fatalf("frame leaks the token: %+v", fr)
		}
	}
}

func TestReviewStreamsText(t *testing.T) {
	f := newReviewFixture(t)
	rec := &liveRecorder{db: f.db}
	deps, _ := f.deps(t, `{"review":[
		{"tool":"read_diff","say":"Reading the diff."},
		{"tool":"verdict","verdict":"recommend_approve","body":"Meets REQ-001.","say":"It meets REQ-001."}]}`)
	deps.Live = rec
	before := modelCalls(t, f.db, f.card.ID)
	card, err := app.ReviewCard(context.Background(), f.db, deps, "proj_default", f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "human_review" {
		t.Fatalf("card = %s", card.Status)
	}
	got := rec.turns(t, "reviewer")
	if len(got) != 2 || got[0] != "Reading the diff." || got[1] != "It meets REQ-001." {
		t.Fatalf("streamed turns = %q", got)
	}
	if len(rec.stored) != 2 || rec.stored[0] != before+1 || rec.stored[1] != before+2 {
		t.Fatalf("model_call events at each turn_end = %v after %d", rec.stored, before)
	}
}

func modelCalls(t *testing.T, db *sql.DB, cardID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'model_call'`, cardID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func loadFake(t *testing.T, script string) *fake.Fake {
	t.Helper()
	provider, err := fake.Load([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
