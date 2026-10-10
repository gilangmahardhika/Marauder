package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/trace"
)

func TestActionsEmpty(t *testing.T) {
	body := get(t, "/api/cards/card_0123456789ab?status=in_progress", "127.0.0.1:8787", bearer())
	if !strings.Contains(body, `"actions":[]`) {
		t.Fatalf("body = %s", body)
	}
}

func TestActionsApproved(t *testing.T) {
	body := get(t, "/api/cards/card_0123456789ab?status=approved", "127.0.0.1:8787", bearer())
	if !strings.Contains(body, "request_changes") || !strings.Contains(body, "reject") || strings.Contains(body, `"approve"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestCORS(t *testing.T) {
	rec := request(t, "/api/cards", "127.0.0.1:8787", "http://127.0.0.1:9999", "")
	if rec.Code != 403 {
		t.Fatalf("evil origin = %d", rec.Code)
	}
	ok := request(t, "/api/cards", "127.0.0.1:8787", "http://127.0.0.1:4173", bearer())
	if ok.Code != 200 || ok.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:4173" {
		t.Fatalf("allowed = %d %s", ok.Code, ok.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestHostHeader(t *testing.T) {
	rec := request(t, "/api/cards", "evil.example:8787", "", bearer())
	if rec.Code != 403 {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSessionLockout(t *testing.T) {
	srv := httpapi.New("startup-token")
	var last int
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(`{"token":"nope"}`))
		req.Host = "127.0.0.1:8787"
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != 429 {
		t.Fatalf("6th status = %d", last)
	}
}

func TestRedactPack(t *testing.T) {
	const token = "deploy-token-value"
	stored := trace.Redact("log "+token, []string{token})
	if strings.Contains(stored, token) || !strings.Contains(stored, "[redacted]") {
		t.Fatalf("stored = %s", stored)
	}
}

func bearer() string { return "bearer-token-for-tests" }

func get(t *testing.T, path, host, token string) string {
	t.Helper()
	return request(t, path, host, "", token).Body.String()
}

func request(t *testing.T, path, host, origin, token string) *httptest.ResponseRecorder {
	t.Helper()
	srv := httpapi.New("startup-token")
	if token == "" {
		token = bearer()
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}
