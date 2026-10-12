package worker

import (
	"context"
	"database/sql"
	"time"
)

const (
	reviewCap    = 2
	refreshCap   = 2
	refreshEvery = 30 * time.Second
	reviewRetry  = time.Minute
)

// Advancer moves cards that have a pull request (RFC-065). Each tick it starts
// a review job for every pr_open card, and refreshes the pull request head and
// checks of cards in agent_review, human_review, and approved at most once
// every 30 seconds per card (0003 pass steps 1, 2, 6, and 7). It runs on every
// tick, with or without an open run request.
type Advancer struct {
	reviews   *Slots
	refreshes *Slots
	started   map[string]time.Time
}

func NewAdvancer() *Advancer {
	return &Advancer{reviews: NewSlots(), refreshes: NewSlots(), started: map[string]time.Time{}}
}

// Wait returns when every review and refresh this Advancer started has returned.
func (a *Advancer) Wait() {
	a.reviews.Wait()
	a.refreshes.Wait()
}

// Tick starts the review and refresh jobs that are due. It must be called
// from one goroutine.
func (a *Advancer) Tick(ctx context.Context, db *sql.DB, projectID string, jobs Jobs, now time.Time, logf func(string, ...any)) error {
	if jobs.Review == nil && jobs.Refresh == nil {
		return nil
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, status FROM cards
		WHERE project_id = $1 AND status IN ('pr_open', 'agent_review', 'human_review', 'approved')
		ORDER BY updated_at, id`, projectID)
	if err != nil {
		return err
	}
	type due struct{ id, status string }
	var cards []due
	for rows.Next() {
		var c due
		if err := rows.Scan(&c.id, &c.status); err != nil {
			rows.Close()
			return err
		}
		cards = append(cards, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range cards {
		if c.status == "pr_open" {
			a.start(ctx, "review", c.id, jobs.Review, a.reviews, reviewCap, reviewRetry, projectID, now, logf)
			continue
		}
		a.start(ctx, "refresh", c.id, jobs.Refresh, a.refreshes, refreshCap, refreshEvery, projectID, now, logf)
	}
	return nil
}

func (a *Advancer) start(ctx context.Context, kind, cardID string, job func(context.Context, string, string) (string, error),
	slots *Slots, limit int, every time.Duration, projectID string, now time.Time, logf func(string, ...any)) {
	if job == nil {
		return
	}
	key := kind + ":" + cardID
	if last, ok := a.started[key]; ok && now.Sub(last) < every {
		return
	}
	if !slots.take(cardID, limit) {
		return
	}
	a.started[key] = now
	go func() {
		defer slots.release(cardID)
		status, err := job(ctx, projectID, cardID)
		if err != nil && ctx.Err() == nil {
			logf("worker: card %s %s: %v", cardID, kind, err)
			return
		}
		if kind == "review" && status != "" {
			logf("worker: card %s review -> %s", cardID, status)
		}
	}()
}
