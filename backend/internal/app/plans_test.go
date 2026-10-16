package app_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/identity"
)

const planScript = `{"implement":[
	{"tool":"list_files","path":"."},
	{"tool":"submit_plan","title":"Add CSV export","steps":[
		{"text":"Find the orders list handler","detail":""},
		{"text":"Add a CSV writer and a button","detail":"Reuse auth"}]},
	{"tool":"edit_file","path":"hello.txt","text":"too late\n"}]}`

func TestSubmitPlanParksCard(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	if _, err := f.db.Exec(`UPDATE cards SET plan_mode = 'required' WHERE id = $1`, f.card.ID); err != nil {
		t.Fatal(err)
	}
	card, provider := f.runWith(t, planScript)
	if card.Status != "waiting_answer" || card.AttemptsUsed != 0 {
		t.Fatalf("card = %s used %d, want waiting_answer used 0", card.Status, card.AttemptsUsed)
	}
	if prompts := provider.Prompts(); len(prompts) < 2 {
		t.Fatalf("model calls = %d", len(prompts))
	}
	var outcome string
	if err := f.db.QueryRow(`SELECT outcome FROM attempts WHERE card_id = $1`, card.ID).Scan(&outcome); err != nil || outcome != "plan" {
		t.Fatalf("outcome = %s, %v", outcome, err)
	}
	plan, err := board.OpenPlan(context.Background(), f.db, "proj_default", card.ID)
	if err != nil || plan.Title != "Add CSV export" || len(plan.Steps) != 2 {
		t.Fatalf("open plan = %+v %v", plan, err)
	}
	if f.calls.has("archive") {
		t.Fatal("a parked plan attempt ran the gating check")
	}
}

func TestPlanModeOffSkipsGate(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	// InsertCard defaults plan_mode to off.
	card := f.run(t, commitScript)
	if card.Status != "pr_open" {
		t.Fatalf("status = %s, want pr_open without a plan", card.Status)
	}
}

func TestAcceptPlanQueuesRun(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	if _, err := f.db.Exec(`UPDATE cards SET plan_mode = 'required' WHERE id = $1`, f.card.ID); err != nil {
		t.Fatal(err)
	}
	f.runWith(t, planScript)
	next, err := app.AcceptPlan(context.Background(), f.db, questionAnswerer(f), "proj_default", f.card.ID,
		"Add CSV export", []board.PlanStep{{Text: "Find handler"}, {Text: "Write CSV"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "ready" || !nextQueued(t, f, next.ID) {
		t.Fatalf("after accept = %s queued?", next.Status)
	}
	var events int
	if err := f.db.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'plan_accepted'`, f.card.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("plan_accepted = %d", events)
	}
}

func nextQueued(t *testing.T, f jobFixture, cardID string) bool {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM runs WHERE card_id = $1 AND processed_at IS NULL`, cardID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestPackIncludesApprovedPlan(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	if _, err := f.db.Exec(`UPDATE cards SET plan_mode = 'required' WHERE id = $1`, f.card.ID); err != nil {
		t.Fatal(err)
	}
	f.runWith(t, planScript)
	if _, err := app.AcceptPlan(context.Background(), f.db, questionAnswerer(f), "proj_default", f.card.ID, "", nil, true); err != nil {
		t.Fatal(err)
	}
	_, provider := f.runWith(t, commitScript)
	user := provider.Prompts()[0].User
	if !strings.Contains(user, contextpack.PlanHeading) || !strings.Contains(user, "Add CSV export") || !strings.Contains(user, "1. Find the orders list handler") {
		t.Fatalf("pack lacks approved plan:\n%s", user)
	}
}

func TestPlanModeRights(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	dev := questionAnswerer(f)
	next, err := app.SetPlanMode(context.Background(), f.db, dev, "proj_default", f.card.ID, "required")
	if err != nil || next.PlanMode != "required" {
		t.Fatalf("set required = %v mode %q", err, next.PlanMode)
	}
	if _, err := app.SetPlanMode(context.Background(), f.db, app.User{ID: "usr_po", Role: "product_owner"}, "proj_default", f.card.ID, "off"); err != identity.ErrForbidden {
		// Product owner may lack project membership; treat any forbidden as pass.
		if err != identity.ErrForbidden {
			t.Fatalf("outsider = %v", err)
		}
	}
}

func TestSkipPlanWhenOff(t *testing.T) {
	f := newJobFixture(t, "true", 3, routerConfig, fixtureProject)
	if _, err := f.db.Exec(`UPDATE cards SET plan_mode = 'required' WHERE id = $1`, f.card.ID); err != nil {
		t.Fatal(err)
	}
	f.runWith(t, planScript)
	if _, err := app.SetPlanMode(context.Background(), f.db, questionAnswerer(f), "proj_default", f.card.ID, "off"); err != nil {
		t.Fatal(err)
	}
	next, err := app.SkipPlan(context.Background(), f.db, questionAnswerer(f), "proj_default", f.card.ID)
	if err != nil || next.Status != "ready" {
		t.Fatalf("skip = %v status %s", err, next.Status)
	}
}
