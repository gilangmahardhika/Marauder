package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/notify"
)

// Stream timing (RFC-113); tests shorten them.
var (
	streamPing = 15 * time.Second
	streamMax  = 10 * time.Minute
)

// stream sends the viewer's board changes and notifications as server-sent
// events (RFC-113). Frames: ready (the unread count, first), cards (changed
// cards with the viewer's actions, every card when full), board (readyCount,
// running, canRun), notification, resync (read the notifications again), and
// done.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	user, ok := app.CurrentUser(r.Context())
	if !ok || s.DB == nil {
		writeErr(w, 401, "unauthenticated", "bearer")
		return
	}
	ctx := r.Context()
	projectID := r.URL.Query().Get("project")
	if projectID != "" {
		if err := app.SeeProject(ctx, s.DB, projectID, user.ID); err != nil {
			writeAccountErr(w, err)
			return
		}
	}
	flusher, ok := w.(http.Flusher)
	if !ok || s.Notify == nil {
		writeErr(w, 500, "internal", "stream")
		return
	}
	sub, err := s.Notify.Subscribe(user.ID, projectID)
	if errors.Is(err, notify.ErrFull) {
		writeErr(w, 429, "rate_limited", "too many streams are open; try again soon")
		return
	}
	if err != nil {
		writeErr(w, 500, "internal", "stream")
		return
	}
	defer sub.Close()
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

	sent := map[string]string{}
	sentBoard := ""
	// sendBoard sends the cards whose JSON changed since the last frame and
	// the board summary when it changed; full sends every card. A viewer
	// who left the project's tribe gets done.
	sendBoard := func(full bool) bool {
		if projectID == "" {
			return true
		}
		if err := app.SeeProject(ctx, s.DB, projectID, user.ID); err != nil {
			emit("done", map[string]any{"reconnect": false})
			return false
		}
		body, err := s.projectBoard(r, projectID)
		if err != nil {
			return ctx.Err() == nil
		}
		cards, _ := body["cards"].([]any)
		changed := []any{}
		seen := map[string]bool{}
		for _, c := range cards {
			card, _ := c.(map[string]any)
			id, _ := card["id"].(string)
			raw, _ := json.Marshal(card)
			seen[id] = true
			if full || sent[id] != string(raw) {
				changed = append(changed, card)
				sent[id] = string(raw)
			}
		}
		removed := []string{}
		for id := range sent {
			if !seen[id] {
				removed = append(removed, id)
				delete(sent, id)
			}
		}
		if full || len(changed) > 0 || len(removed) > 0 {
			if !emit("cards", map[string]any{"full": full, "cards": changed, "removed": removed}) {
				return false
			}
		}
		summary := map[string]any{"readyCount": body["readyCount"], "running": body["running"], "canRun": body["canRun"]}
		raw, _ := json.Marshal(summary)
		if full || string(raw) != sentBoard {
			sentBoard = string(raw)
			return emit("board", summary)
		}
		return true
	}

	unread, _ := notify.Unread(ctx, s.DB, user.ID)
	if !emit("ready", map[string]any{"unread": unread}) || !sendBoard(true) {
		return
	}
	ping := time.NewTicker(streamPing)
	defer ping.Stop()
	limit := time.NewTimer(streamMax)
	defer limit.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case f, open := <-sub.C:
			if !open {
				return
			}
			switch f.Kind {
			case notify.Changed:
				if !sendBoard(false) {
					return
				}
			case notify.Notified:
				row, err := notify.Get(ctx, s.DB, user.ID, f.NotificationID)
				if err != nil {
					continue
				}
				unread, _ := notify.Unread(ctx, s.DB, user.ID)
				if !emit("notification", map[string]any{"notification": row, "unread": unread}) {
					return
				}
			case notify.Resync:
				if !emit("resync", map[string]any{}) || !sendBoard(true) {
					return
				}
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

// notifications lists the viewer's newest notifications in projects they
// see; unread=1 lists only unread ones.
func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	user, ok := app.CurrentUser(r.Context())
	if !ok || s.DB == nil {
		writeErr(w, 401, "unauthenticated", "bearer")
		return
	}
	rows, unread, err := notify.List(r.Context(), s.DB, user.ID, r.URL.Query().Get("unread") == "1")
	if err != nil {
		writeErr(w, 500, "internal", "notifications")
		return
	}
	writeJSON(w, 200, map[string]any{"notifications": rows, "unread": unread})
}

// readNotifications marks {ids} or {all: true} of the viewer's
// notifications read.
func (s *Server) readNotifications(w http.ResponseWriter, r *http.Request) {
	user, ok := app.CurrentUser(r.Context())
	if !ok || s.DB == nil {
		writeErr(w, 401, "unauthenticated", "bearer")
		return
	}
	var body struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (!body.All && len(body.IDs) == 0) || len(body.IDs) > notify.ListMax {
		writeErr(w, 422, "validation", "send ids or all")
		return
	}
	if err := notify.MarkRead(r.Context(), s.DB, user.ID, body.IDs, body.All); err != nil {
		writeErr(w, 500, "internal", "notifications")
		return
	}
	unread, err := notify.Unread(r.Context(), s.DB, user.ID)
	if err != nil {
		writeErr(w, 500, "internal", "notifications")
		return
	}
	writeJSON(w, 200, map[string]any{"unread": unread})
}
