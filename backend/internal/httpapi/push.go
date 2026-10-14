package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/webpush"
)

// pushUser is the signed-in viewer when browser notifications are set up.
func (s *Server) pushUser(w http.ResponseWriter, r *http.Request) (app.User, bool) {
	user, ok := app.CurrentUser(r.Context())
	if !ok || s.DB == nil {
		writeErr(w, 401, "unauthenticated", "bearer")
		return app.User{}, false
	}
	if s.Push == nil || s.PushKey == "" {
		writeErr(w, 503, "unavailable", "browser notifications are not set up")
		return app.User{}, false
	}
	return user, true
}

// pushKey returns the VAPID public key a browser subscribes with, and how
// many subscriptions the viewer has.
func (s *Server) pushKey(w http.ResponseWriter, r *http.Request) {
	user, ok := s.pushUser(w, r)
	if !ok {
		return
	}
	n, err := webpush.Count(r.Context(), s.DB, user.ID)
	if err != nil {
		writeErr(w, 500, "internal", "push")
		return
	}
	writeJSON(w, 200, map[string]any{"publicKey": s.PushKey, "subscriptions": n})
}

func (s *Server) savePush(w http.ResponseWriter, r *http.Request) {
	user, ok := s.pushUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeErr(w, 422, "validation", "send endpoint and keys")
		return
	}
	err := webpush.Save(r.Context(), s.DB, user.ID, body.Endpoint, body.Keys.P256dh, body.Keys.Auth)
	switch {
	case errors.Is(err, webpush.ErrEndpoint):
		writeFields(w, 422, "validation", err.Error(), map[string]string{"endpoint": "This browser's push service is not supported."})
	case errors.Is(err, webpush.ErrKeys):
		writeFields(w, 422, "validation", err.Error(), map[string]string{"keys": "The subscription keys are not valid."})
	case err != nil:
		writeErr(w, 500, "internal", "push")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) deletePush(w http.ResponseWriter, r *http.Request) {
	user, ok := s.pushUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Endpoint == "" {
		writeErr(w, 422, "validation", "send endpoint")
		return
	}
	if err := webpush.Delete(r.Context(), s.DB, user.ID, body.Endpoint); err != nil {
		writeErr(w, 500, "internal", "push")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testPush sends a test notification to the viewer's browsers.
func (s *Server) testPush(w http.ResponseWriter, r *http.Request) {
	user, ok := s.pushUser(w, r)
	if !ok {
		return
	}
	sent, err := s.Push.SendTest(r.Context(), user.ID)
	if err != nil {
		writeErr(w, 500, "internal", "push")
		return
	}
	writeJSON(w, 200, map[string]any{"sent": sent})
}
