package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/live"
	"orchestrator/backend/internal/trace"
)

// Trace stream timing (RFC-088); tests shorten them.
var (
	traceStreamPoll = time.Second
	traceStreamPing = 15 * time.Second
	traceStreamMax  = 10 * time.Minute
)

// traceStream sends the card's trace as server-sent events: stored events as
// they commit, read from the database, and the reply text the jobs of this
// process publish while the model writes it.
func (s *Server) traceStream(w http.ResponseWriter, r *http.Request, card board.Card) {
	since, ok := traceSince(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok || s.Live == nil {
		writeErr(w, 500, "internal", "trace stream")
		return
	}
	sub, snapshot, err := s.Live.Subscribe(card.ID)
	if errors.Is(err, live.ErrFull) {
		writeErr(w, 429, "rate_limited", "too many trace streams are open; try again soon")
		return
	}
	defer sub.Close()
	ctx := r.Context()
	listed, attempts, isLive, err := s.traceRead(ctx, card, since)
	if err != nil {
		writeErr(w, 500, "internal", "trace")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	emit := func(kind string, data any) bool {
		body, err := json.Marshal(data)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, body); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	seen := map[string]bool{}
	cursor := since
	note := func(events []trace.Listed) []trace.Listed {
		fresh := events[:0:0]
		for _, ev := range events {
			if seen[ev.ID] {
				continue
			}
			seen[ev.ID] = true
			fresh = append(fresh, ev)
			if at, err := time.Parse(time.RFC3339Nano, ev.TS); err == nil {
				if next := at.Add(-traceOverlap).UTC().Format(time.RFC3339); next > cursor {
					cursor = next
				}
			}
		}
		return fresh
	}
	sentAttempts, _ := json.Marshal(attempts)
	if !emit("trace", map[string]any{"events": traceEvents(note(listed)), "attempts": attempts, "live": isLive}) {
		return
	}
	for _, f := range snapshot {
		if !emit(f.Kind, f) {
			return
		}
	}
	if !isLive {
		emit("done", map[string]any{"live": false})
		return
	}
	// refresh sends the events stored since the last read, and reports
	// whether the stream should go on.
	refresh := func() bool {
		current, err := board.GetCard(ctx, s.DB, card.ID)
		if err != nil {
			return false
		}
		listed, attempts, isLive, err := s.traceRead(ctx, current, cursor)
		if err != nil {
			return false
		}
		fresh := note(listed)
		encoded, _ := json.Marshal(attempts)
		if len(fresh) > 0 || string(encoded) != string(sentAttempts) || !isLive {
			sentAttempts = encoded
			if !emit("trace", map[string]any{"events": traceEvents(fresh), "attempts": attempts, "live": isLive}) {
				return false
			}
		}
		if !isLive {
			emit("done", map[string]any{"live": false})
			return false
		}
		return true
	}

	poll := time.NewTicker(traceStreamPoll)
	defer poll.Stop()
	ping := time.NewTicker(traceStreamPing)
	defer ping.Stop()
	limit := time.NewTimer(traceStreamMax)
	defer limit.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case f, open := <-sub.C:
			if !open {
				if sub.Dropped() {
					emit(live.Resync, map[string]any{})
				}
				return
			}
			if !emit(f.Kind, f) {
				return
			}
			if f.Kind == live.TurnEnd && !refresh() {
				return
			}
		case <-poll.C:
			if !refresh() {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-limit.C:
			emit("done", map[string]any{"reconnect": true})
			return
		}
	}
}
