package board_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
)

const offline = time.Minute

func capOf(n int) func(string) int {
	return func(string) int { return n }
}

func addRunner(t *testing.T, db *sql.DB, id string, slots int) {
	t.Helper()
	r := board.Runner{ID: id, Name: id, Slots: slots, Version: "test", Protocol: 1}
	if err := board.InsertRunner(context.Background(), db, r, "hash_"+id); err != nil {
		t.Fatal(err)
	}
}

func leaseCard(t *testing.T, db *sql.DB, projectID, status string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), db, board.Card{ProjectID: projectID, Status: status})
	if err != nil {
		t.Fatal(err)
	}
	return card
}

var runSeq int

func queueRun(t *testing.T, db *sql.DB, card board.Card) string {
	t.Helper()
	runSeq++
	id := fmt.Sprintf("run_lease%07d", runSeq)
	created := time.Now().UTC().Add(time.Duration(runSeq) * time.Millisecond).Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO runs (id, trigger, created_at, project_id, card_id) VALUES ($1, 'api', $2, $3, $4)`,
		id, created, card.ProjectID, card.ID); err != nil {
		t.Fatal(err)
	}
	return id
}

func stale(t *testing.T, db *sql.DB, runnerID string) {
	t.Helper()
	past := time.Now().Add(-2 * offline).UTC().Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE runners SET heartbeat_at = $2 WHERE id = $1`, runnerID, past); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE runner_leases SET leased_until = $2 WHERE runner_id = $1 AND ended_at IS NULL`, runnerID, past); err != nil {
		t.Fatal(err)
	}
}

func TestClaimSkipLocked(t *testing.T) {
	db := boardtest.Open(t)
	runs := map[string]bool{}
	for range 3 {
		runs[queueRun(t, db, leaseCard(t, db, "proj_default", "ready"))] = true
	}
	for i := range 8 {
		addRunner(t, db, fmt.Sprintf("rnr_claim%d", i), 1)
	}
	var mu sync.Mutex
	var leases []board.Lease
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l, err := board.ClaimRun(context.Background(), db, fmt.Sprintf("rnr_claim%d", i), capOf(10), offline)
			if errors.Is(err, board.ErrNotFound) {
				return
			}
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			leases = append(leases, l)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if len(leases) != 3 {
		t.Fatalf("leases = %d, want 3", len(leases))
	}
	seen := map[string]bool{}
	for _, l := range leases {
		if !runs[l.RunID] || seen[l.RunID] {
			t.Fatalf("lease run %s is unknown or claimed twice", l.RunID)
		}
		seen[l.RunID] = true
	}
	var open int
	if err := db.QueryRow(`SELECT count(*) FROM runs WHERE processed_at IS NULL`).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("%d run requests still open", open)
	}
}

func TestClaimRespectsCap(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	if _, err := db.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_other', 'tribe_default', 'Other', '/tmp/other', '2026-10-12T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	queueRun(t, db, leaseCard(t, db, "proj_default", "ready"))
	second := queueRun(t, db, leaseCard(t, db, "proj_default", "ready"))
	other := queueRun(t, db, leaseCard(t, db, "proj_other", "ready"))
	addRunner(t, db, "rnr_one", 2)
	addRunner(t, db, "rnr_two", 2)
	capFor := func(projectID string) int {
		if projectID == "proj_default" {
			return 1
		}
		return 2
	}
	first, err := board.ClaimRun(ctx, db, "rnr_one", capFor, offline)
	if err != nil {
		t.Fatal(err)
	}
	got, err := board.ClaimRun(ctx, db, "rnr_two", capFor, offline)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != other {
		t.Fatalf("claim at cap took %s, want the other project's %s", got.RunID, other)
	}
	if _, err := board.ClaimRun(ctx, db, "rnr_two", capFor, offline); !errors.Is(err, board.ErrNotFound) {
		t.Fatalf("claim over cap = %v, want ErrNotFound", err)
	}
	if err := board.EndLease(ctx, db, first.ID); err != nil {
		t.Fatal(err)
	}
	got, err = board.ClaimRun(ctx, db, "rnr_two", capFor, offline)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != second {
		t.Fatalf("claim after end took %s, want %s", got.RunID, second)
	}
}

func TestCardAffinity(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	card := leaseCard(t, db, "proj_default", "ready")
	addRunner(t, db, "rnr_home", 1)
	addRunner(t, db, "rnr_away", 1)
	queueRun(t, db, card)
	first, err := board.ClaimRun(ctx, db, "rnr_home", capOf(2), offline)
	if err != nil {
		t.Fatal(err)
	}
	if err := board.EndLease(ctx, db, first.ID); err != nil {
		t.Fatal(err)
	}
	queueRun(t, db, card)
	if _, err := board.ClaimRun(ctx, db, "rnr_away", capOf(2), offline); !errors.Is(err, board.ErrNotFound) {
		t.Fatalf("other runner claim = %v, want ErrNotFound", err)
	}
	again, err := board.ClaimRun(ctx, db, "rnr_home", capOf(2), offline)
	if err != nil {
		t.Fatal(err)
	}
	if again.CardID != card.ID {
		t.Fatalf("home runner took %s, want %s", again.CardID, card.ID)
	}
	if err := board.EndLease(ctx, db, again.ID); err != nil {
		t.Fatal(err)
	}
	queueRun(t, db, card)
	stale(t, db, "rnr_home")
	moved, err := board.ClaimRun(ctx, db, "rnr_away", capOf(2), offline)
	if err != nil {
		t.Fatalf("claim after home runner went offline: %v", err)
	}
	if moved.CardID != card.ID {
		t.Fatalf("away runner took %s, want %s", moved.CardID, card.ID)
	}
	var affinity string
	if err := db.QueryRow(`SELECT runner_affinity FROM cards WHERE id = $1`, card.ID).Scan(&affinity); err != nil {
		t.Fatal(err)
	}
	if affinity != "rnr_away" {
		t.Fatalf("affinity = %s, want rnr_away", affinity)
	}
}

func TestReaperRequeues(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	card := leaseCard(t, db, "proj_default", "in_progress")
	live := leaseCard(t, db, "proj_default", "in_progress")
	addRunner(t, db, "rnr_gone", 1)
	addRunner(t, db, "rnr_live", 1)
	run := queueRun(t, db, card)
	lease, err := board.ClaimRun(ctx, db, "rnr_gone", capOf(5), offline)
	if err != nil {
		t.Fatal(err)
	}
	queueRun(t, db, live)
	kept, err := board.ClaimRun(ctx, db, "rnr_live", capOf(5), offline)
	if err != nil {
		t.Fatal(err)
	}
	attempt := board.Attempt{ID: "att_reap0000001", CardID: card.ID, N: 1, TaskClass: "implement", ModelID: "m", MaxTurns: 1}
	if err := board.InsertAttempt(ctx, db, attempt); err != nil {
		t.Fatal(err)
	}
	if err := board.AcquireLock(ctx, db, card.ID, lease.ID); err != nil {
		t.Fatal(err)
	}
	stale(t, db, "rnr_gone")
	ended, err := board.ReapLeases(ctx, db, board.ReapFilter{Expired: true}, "lost")
	if err != nil {
		t.Fatal(err)
	}
	if len(ended) != 1 || ended[0] != lease.ID {
		t.Fatalf("ended = %v, want [%s]", ended, lease.ID)
	}
	var outcome string
	if err := db.QueryRow(`SELECT outcome FROM runner_leases WHERE id = $1`, lease.ID).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome != "lost" {
		t.Fatalf("lease outcome = %s, want lost", outcome)
	}
	if err := db.QueryRow(`SELECT outcome FROM attempts WHERE id = $1`, attempt.ID).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome != "cancelled" {
		t.Fatalf("attempt outcome = %s, want cancelled", outcome)
	}
	var locks int
	if err := db.QueryRow(`SELECT count(*) FROM locks WHERE card_id = $1`, card.ID).Scan(&locks); err != nil {
		t.Fatal(err)
	}
	if locks != 0 {
		t.Fatalf("card still has %d locks", locks)
	}
	var affinity sql.NullString
	var status string
	if err := db.QueryRow(`SELECT runner_affinity, status FROM cards WHERE id = $1`, card.ID).Scan(&affinity, &status); err != nil {
		t.Fatal(err)
	}
	if affinity.Valid {
		t.Fatalf("affinity = %s, want NULL", affinity.String)
	}
	if status != "in_progress" {
		t.Fatalf("card status = %s, want in_progress unchanged", status)
	}
	var processed sql.NullString
	if err := db.QueryRow(`SELECT processed_at FROM runs WHERE id = $1`, run).Scan(&processed); err != nil {
		t.Fatal(err)
	}
	if processed.Valid {
		t.Fatal("run request is still processed")
	}
	if _, err := board.OpenLease(ctx, db, "rnr_live", kept.ID); err != nil {
		t.Fatalf("live runner's lease: %v", err)
	}
}

func TestReconcileSkipsLiveLease(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	leased := leaseCard(t, db, "proj_default", "in_progress")
	local := leaseCard(t, db, "proj_default", "in_progress")
	addRunner(t, db, "rnr_busy", 1)
	queueRun(t, db, leased)
	lease, err := board.ClaimRun(ctx, db, "rnr_busy", capOf(5), offline)
	if err != nil {
		t.Fatal(err)
	}
	remote := board.Attempt{ID: "att_remote00001", CardID: leased.ID, N: 1, TaskClass: "implement", ModelID: "m", MaxTurns: 1}
	orphan := board.Attempt{ID: "att_orphan00001", CardID: local.ID, N: 1, TaskClass: "implement", ModelID: "m", MaxTurns: 1}
	for _, a := range []board.Attempt{remote, orphan} {
		if err := board.InsertAttempt(ctx, db, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := board.AcquireLock(ctx, db, leased.ID, lease.ID); err != nil {
		t.Fatal(err)
	}
	if err := board.AcquireLock(ctx, db, local.ID, "worker"); err != nil {
		t.Fatal(err)
	}
	n, err := board.ReconcileProject(ctx, db, "proj_default")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reconcile cancelled %d attempts, want 1", n)
	}
	var ended sql.NullString
	if err := db.QueryRow(`SELECT ended_at FROM attempts WHERE id = $1`, remote.ID).Scan(&ended); err != nil {
		t.Fatal(err)
	}
	if ended.Valid {
		t.Fatal("leased card's attempt was cancelled")
	}
	var outcome string
	if err := db.QueryRow(`SELECT outcome FROM attempts WHERE id = $1`, orphan.ID).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome != "cancelled" {
		t.Fatalf("unleased attempt outcome = %s, want cancelled", outcome)
	}
	for card, want := range map[string]int{leased.ID: 1, local.ID: 0} {
		var locks int
		if err := db.QueryRow(`SELECT count(*) FROM locks WHERE card_id = $1`, card).Scan(&locks); err != nil {
			t.Fatal(err)
		}
		if locks != want {
			t.Fatalf("card %s locks = %d, want %d", card, locks, want)
		}
	}
}
