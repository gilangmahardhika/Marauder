package review_test

import (
	"context"
	"database/sql"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/review"
)

func TestCIPending(t *testing.T) {
	db := boardtest.Open(t)
	card := mustCard(t, db, "agent_review")
	next, err := review.ApplyCI(context.Background(), db, card, "att_0123456789ab", []review.Check{{Name: "ci", State: "pending"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "agent_review" {
		t.Fatalf("status = %s", next.Status)
	}
	passed, err := review.ApplyCI(context.Background(), db, next, "att_0123456789ab", []review.Check{{Name: "ci", State: "success"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if passed.Status != "human_review" {
		t.Fatalf("status = %s", passed.Status)
	}
}

func TestCIRed(t *testing.T) {
	db := boardtest.Open(t)
	card := mustCard(t, db, "agent_review")
	next, err := review.ApplyCI(context.Background(), db, card, "att_0123456789ab", []review.Check{{Name: "ci", State: "failure"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "changes_requested" {
		t.Fatalf("status = %s", next.Status)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM failure_notes WHERE card_id = $1 AND source = 'ci'`, card.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ci notes = %d", n)
	}
}

func TestReviewNotCounted(t *testing.T) {
	db := boardtest.Open(t)
	card := mustCard(t, db, "agent_review")
	if _, err := db.Exec(`UPDATE cards SET attempts_used = 2 WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	card.AttemptsUsed = 2
	if _, err := review.ApplyCI(context.Background(), db, card, "att_0123456789ab", []review.Check{{Name: "ci", State: "success"}}, nil); err != nil {
		t.Fatal(err)
	}
	var used int
	if err := db.QueryRow(`SELECT attempts_used FROM cards WHERE id = $1`, card.ID).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != 2 {
		t.Fatalf("attempts_used = %d", used)
	}
}

func mustCard(t *testing.T, db *sql.DB, status string) board.Card {
	t.Helper()
	card, err := board.InsertCard(context.Background(), db, board.Card{Status: status, Title: "card"})
	if err != nil {
		t.Fatal(err)
	}
	return card
}
