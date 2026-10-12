package worker_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/worker"
)

type recordedJobs struct {
	mu   sync.Mutex
	runs []string
}

func (r *recordedJobs) record(kind string) func(context.Context, string, string) (string, error) {
	return func(_ context.Context, _, cardID string) (string, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.runs = append(r.runs, kind+":"+cardID)
		return "", nil
	}
}

func (r *recordedJobs) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.runs
	r.runs = nil
	slices.Sort(out)
	return out
}

func TestAdvancerSchedulesReviewAndRefresh(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	ids := map[string]string{}
	for _, status := range []string{"pr_open", "agent_review", "human_review", "ready"} {
		card, err := board.InsertCard(ctx, db, board.Card{Title: status, Status: status})
		if err != nil {
			t.Fatal(err)
		}
		ids[status] = card.ID
	}
	rec := &recordedJobs{}
	jobs := worker.Jobs{Review: rec.record("review"), Refresh: rec.record("refresh")}
	adv := worker.NewAdvancer()
	tick := func(at time.Time) []string {
		t.Helper()
		if err := adv.Tick(ctx, db, testProject, jobs, at, t.Logf); err != nil {
			t.Fatal(err)
		}
		adv.Wait()
		return rec.take()
	}
	start := time.Now()
	want := []string{"refresh:" + ids["agent_review"], "refresh:" + ids["human_review"], "review:" + ids["pr_open"]}
	slices.Sort(want)
	if got := tick(start); !slices.Equal(got, want) {
		t.Fatalf("first tick = %v, want %v", got, want)
	}
	if got := tick(start.Add(2 * time.Second)); len(got) != 0 {
		t.Fatalf("tick 2 s later = %v, want nothing", got)
	}
	refreshes := []string{"refresh:" + ids["agent_review"], "refresh:" + ids["human_review"]}
	slices.Sort(refreshes)
	if got := tick(start.Add(31 * time.Second)); !slices.Equal(got, refreshes) {
		t.Fatalf("tick 31 s later = %v, want only refreshes %v", got, refreshes)
	}
	if got := tick(start.Add(61 * time.Second)); !slices.Contains(got, "review:"+ids["pr_open"]) {
		t.Fatalf("tick 61 s later = %v, want the review retried", got)
	}
}
