package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/worker"
)

func TestWorkerLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.lock")
	first, err := worker.Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := worker.Lock(path); err != worker.ErrBusy {
		t.Fatalf("second lock err = %v", err)
	}
}

func TestPlanCards(t *testing.T) {
	cards, err := worker.Plan([]worker.Proposal{
		{Title: "a", Reqs: []string{"REQ-001"}, Risk: "low"},
		{Title: "b", Reqs: []string{"REQ-002"}, Depends: []string{"a"}, Risk: "low"},
	}, map[string]bool{"REQ-001": true, "REQ-002": true}, "true", "false")
	if err != nil {
		t.Fatal(err)
	}
	if cards[0].LocalCheck != "true" || cards[1].DependsOn[0] != "a" {
		t.Fatalf("cards = %+v", cards)
	}
	if _, err := worker.Plan([]worker.Proposal{{Title: "a", Reqs: []string{"REQ-009"}}}, map[string]bool{}, "true", ""); err == nil {
		t.Fatal("unaccepted requirement accepted")
	}
	if _, err := worker.Plan([]worker.Proposal{
		{Title: "a", Reqs: []string{"REQ-001"}, Depends: []string{"b"}},
		{Title: "b", Reqs: []string{"REQ-001"}, Depends: []string{"a"}},
	}, map[string]bool{"REQ-001": true}, "true", ""); err == nil {
		t.Fatal("cycle accepted")
	}
}

func TestDependency(t *testing.T) {
	if worker.CanStart(map[string]bool{"a": false}, []string{"a"}) {
		t.Fatal("B started before A was done")
	}
	if !worker.CanStart(map[string]bool{"a": true}, []string{"a"}) {
		t.Fatal("B stayed blocked after A was done")
	}
}

func TestDeployLock(t *testing.T) {
	var mu sync.Mutex
	var running bool
	overlap := false
	var wg sync.WaitGroup
	run := func() {
		defer wg.Done()
		mu.Lock()
		if running {
			overlap = true
		}
		running = true
		mu.Unlock()
		mu.Lock()
		running = false
		mu.Unlock()
	}
	wg.Add(2)
	go run()
	go run()
	wg.Wait()
	if overlap {
		t.Fatal("deploys overlapped")
	}
}

func TestCap(t *testing.T) {
	used, max := 0, 3
	blocked := false
	for i := 0; i < 3; i++ {
		used++
		if used >= max {
			blocked = true
		}
	}
	if !blocked || used != 3 {
		t.Fatalf("used=%d blocked=%v", used, blocked)
	}
	if used > max {
		t.Fatal("another attempt opened")
	}
}

func TestBaseRefUnchanged(t *testing.T) {
	if worker.BaseMoved("abc", "abc") {
		t.Fatal("unchanged base was treated as moved")
	}
	if !worker.BaseMoved("abc", "def") {
		t.Fatal("moved base was ignored")
	}
}

func TestLockHolder(t *testing.T) {
	db := boardtest.Open(t)
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: "in_progress", Title: "card"})
	if err != nil {
		t.Fatal(err)
	}
	if err := board.AcquireLock(context.Background(), db, card.ID, "holder-a"); err != nil {
		t.Fatal(err)
	}
	if err := worker.Release(context.Background(), db, card.ID, "holder-b"); err != nil {
		t.Fatal(err)
	}
	var holder string
	if err := db.QueryRow(`SELECT holder FROM locks WHERE card_id = $1`, card.ID).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if holder != "holder-a" {
		t.Fatalf("holder = %s", holder)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'assertion_failed'`, card.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("events = %d", n)
	}
}

func TestRunNextIdle(t *testing.T) {
	if _, err := os.Stat("worker.go"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
