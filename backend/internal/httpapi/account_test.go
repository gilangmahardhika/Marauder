package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/identity"
)

func TestPasswordLogin(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "lead_developer", true)
	rec := postJSON(t, srv, "/api/session", "", map[string]string{"email": "ada@example.com", "password": "password-long-1"})
	if rec.Code != 200 {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Bearer string `json:"bearer"`
		User   struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Bearer == "" || body.User.Email != "ada@example.com" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	me := getAuth(t, srv, "/api/me", body.Bearer)
	if me.Code != 200 || !strings.Contains(me.Body.String(), "ada@example.com") {
		t.Fatalf("me = %d %s", me.Code, me.Body.String())
	}
}

func TestWrongPassword(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	rec := postJSON(t, srv, "/api/session", "", map[string]string{"email": "ada@example.com", "password": "not-the-password"})
	if rec.Code != 401 {
		t.Fatalf("status = %d", rec.Code)
	}
	var n int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("sessions = %d", n)
	}
}

func TestSessionLockout(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	seedUser(t, srv, "bea@example.com", "password-long-2", "developer", false)
	var last int
	for i := 0; i < 6; i++ {
		rec := postJSON(t, srv, "/api/session", "", map[string]string{"email": "ada@example.com", "password": "wrong-password"})
		last = rec.Code
	}
	if last != 429 {
		t.Fatalf("6th status = %d", last)
	}
	other := postJSON(t, srv, "/api/session", "", map[string]string{"email": "bea@example.com", "password": "password-long-2"})
	if other.Code != 200 {
		t.Fatalf("other email = %d %s", other.Code, other.Body.String())
	}
}

func TestBearerExpiry(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	rec := postJSON(t, srv, "/api/session", "", map[string]string{"email": "ada@example.com", "password": "password-long-1"})
	if rec.Code != 200 {
		t.Fatalf("login = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Bearer string `json:"bearer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := srv.DB.Exec(`UPDATE sessions SET expires_at = $1`, past); err != nil {
		t.Fatal(err)
	}
	me := getAuth(t, srv, "/api/me", body.Bearer)
	if me.Code != 401 {
		t.Fatalf("expired = %d", me.Code)
	}
}

func TestLastAdmin(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	rec := putJSON(t, srv, "/api/users/"+admin.ID, token, map[string]any{"disabled": true})
	if rec.Code != 422 {
		t.Fatalf("disable last = %d %s", rec.Code, rec.Body.String())
	}
	rec = putJSON(t, srv, "/api/users/"+admin.ID, token, map[string]any{"admin": false})
	if rec.Code != 422 {
		t.Fatalf("drop last = %d %s", rec.Code, rec.Body.String())
	}
}

func TestUserGitHubOmitted(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	rec := putJSON(t, srv, "/api/me/github", token, map[string]any{"host": "github.com", "login": "ada", "token": "github-token-value"})
	if rec.Code != 200 {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "github-token-value") {
		t.Fatalf("put leaked token: %s", rec.Body.String())
	}
	got := getAuth(t, srv, "/api/me/github", token)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"tokenSet":true`) || strings.Contains(got.Body.String(), "github-token-value") {
		t.Fatalf("get = %s", got.Body.String())
	}
}

func TestNonAdminForbidden(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	created := postJSON(t, srv, "/api/users", adminToken, map[string]any{
		"email": "dev@example.com", "password": "password-long-2", "role": "developer", "admin": false,
	})
	if created.Code != 201 {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	devToken := loginToken(t, srv, "dev@example.com", "password-long-2")
	denied := postJSON(t, srv, "/api/users", devToken, map[string]any{
		"email": "other@example.com", "password": "password-long-3", "role": "qa", "admin": false,
	})
	if denied.Code != 403 {
		t.Fatalf("non-admin = %d %s", denied.Code, denied.Body.String())
	}
}

func accountServer(t *testing.T) *httpapi.Server {
	t.Helper()
	db := boardtest.Open(t)
	srv := httpapi.New()
	srv.DB = db
	srv.TokenKey = bytes.Repeat([]byte("k"), 32)
	return srv
}

func seedUser(t *testing.T, srv *httpapi.Server, email, password, role string, admin bool) app.User {
	t.Helper()
	user, err := identity.Insert(context.Background(), srv.DB, email, password, role, admin)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func loginToken(t *testing.T, srv *httpapi.Server, email, password string) string {
	t.Helper()
	rec := postJSON(t, srv, "/api/session", "", map[string]string{"email": email, "password": password})
	if rec.Code != 200 {
		t.Fatalf("login %s = %d %s", email, rec.Code, rec.Body.String())
	}
	var body struct {
		Bearer string `json:"bearer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Bearer
}

func postJSON(t *testing.T, srv *httpapi.Server, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return sendJSON(t, srv, http.MethodPost, path, token, body)
}

func putJSON(t *testing.T, srv *httpapi.Server, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return sendJSON(t, srv, http.MethodPut, path, token, body)
}

func sendJSON(t *testing.T, srv *httpapi.Server, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Host = "127.0.0.1:8787"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func getAuth(t *testing.T, srv *httpapi.Server, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}
