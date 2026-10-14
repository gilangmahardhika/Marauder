package httpapi_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/httpapi"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func runnerSend(t *testing.T, srv *httpapi.Server, method, path, secret string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = "127.0.0.1:8787"
	if secret != "" {
		req.Header.Set("Authorization", "Runner "+secret)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func mintToken(t *testing.T, srv *httpapi.Server, bearer string) string {
	t.Helper()
	rec := postJSON(t, srv, "/api/runners/tokens", bearer, map[string]any{})
	if rec.Code != 200 {
		t.Fatalf("mint = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Token) != 64 || body.ExpiresAt == "" {
		t.Fatalf("mint body = %s", rec.Body.String())
	}
	return body.Token
}

type registered struct {
	RunnerID string `json:"runnerId"`
	Secret   string `json:"secret"`
}

func registerRunner(t *testing.T, srv *httpapi.Server, token, name string, slots, protocol int) registered {
	t.Helper()
	rec := runnerSend(t, srv, http.MethodPost, "/api/runner/register", "", map[string]any{
		"token": token, "name": name, "labels": []string{"podman"}, "slots": slots, "version": "0.1.0", "protocol": protocol,
	})
	if rec.Code != 200 {
		t.Fatalf("register = %d %s", rec.Code, rec.Body.String())
	}
	var out registered
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func runnerAdmin(t *testing.T) (*httpapi.Server, string) {
	t.Helper()
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	return srv, loginToken(t, srv, "ada@example.com", "password-long-1")
}

type runnerList struct {
	Executor string `json:"executor"`
	Protocol int    `json:"protocol"`
	Runners  []struct {
		ID             string   `json:"id"`
		Name           string   `json:"name"`
		Labels         []string `json:"labels"`
		Slots          int      `json:"slots"`
		Busy           int      `json:"busy"`
		Status         string   `json:"status"`
		UpdateRequired bool     `json:"updateRequired"`
		Actions        []string `json:"actions"`
	} `json:"runners"`
}

func listRunners(t *testing.T, srv *httpapi.Server, bearer string) runnerList {
	t.Helper()
	rec := getAuth(t, srv, "/api/runners", bearer)
	if rec.Code != 200 {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	var out runnerList
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRunnerTokenHashed(t *testing.T) {
	srv, admin := runnerAdmin(t)
	token := mintToken(t, srv, admin)
	reg := registerRunner(t, srv, token, "agent-1", 2, 1)
	if len(reg.Secret) != 64 || !strings.HasPrefix(reg.RunnerID, "rnr_") {
		t.Fatalf("registered = %+v", reg)
	}
	var dump string
	if err := srv.DB.QueryRow(`
		SELECT COALESCE((SELECT string_agg(id || token_hash || COALESCE(runner_id, ''), ' ') FROM runner_registrations), '') ||
			COALESCE((SELECT string_agg(id || name || secret_hash, ' ') FROM runners), '')`).Scan(&dump); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dump, token) || strings.Contains(dump, reg.Secret) {
		t.Fatalf("a raw token is stored: %s", dump)
	}
	var tokenHash, secretHash string
	if err := srv.DB.QueryRow(`SELECT token_hash FROM runner_registrations`).Scan(&tokenHash); err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.QueryRow(`SELECT secret_hash FROM runners WHERE id = $1`, reg.RunnerID).Scan(&secretHash); err != nil {
		t.Fatal(err)
	}
	if tokenHash != sha256Hex(token) || secretHash != sha256Hex(reg.Secret) {
		t.Fatalf("hashes = %s %s", tokenHash, secretHash)
	}
	list := getAuth(t, srv, "/api/runners", admin)
	if strings.Contains(list.Body.String(), reg.Secret) || strings.Contains(list.Body.String(), secretHash) {
		t.Fatalf("list leaks the secret: %s", list.Body.String())
	}
}

func TestRegistrationSingleUse(t *testing.T) {
	srv, admin := runnerAdmin(t)
	seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	lead := loginToken(t, srv, "lead@example.com", "password-long-2")
	if rec := postJSON(t, srv, "/api/runners/tokens", lead, map[string]any{}); rec.Code != 403 {
		t.Fatalf("non-admin mint = %d", rec.Code)
	}
	token := mintToken(t, srv, admin)
	bad := runnerSend(t, srv, http.MethodPost, "/api/runner/register", "", map[string]any{
		"token": token, "name": "", "slots": 0, "protocol": 1,
	})
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), `"name"`) || !strings.Contains(bad.Body.String(), `"slots"`) {
		t.Fatalf("invalid register = %d %s", bad.Code, bad.Body.String())
	}
	registerRunner(t, srv, token, "agent-1", 2, 1)
	again := runnerSend(t, srv, http.MethodPost, "/api/runner/register", "", map[string]any{
		"token": token, "name": "agent-2", "slots": 1, "protocol": 1,
	})
	if again.Code != 401 {
		t.Fatalf("second use = %d %s", again.Code, again.Body.String())
	}
	old := mintToken(t, srv, admin)
	if _, err := srv.DB.Exec(`UPDATE runner_registrations SET expires_at = $1 WHERE token_hash = $2`,
		time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), sha256Hex(old)); err != nil {
		t.Fatal(err)
	}
	expired := runnerSend(t, srv, http.MethodPost, "/api/runner/register", "", map[string]any{
		"token": old, "name": "agent-3", "slots": 1, "protocol": 1,
	})
	if expired.Code != 401 {
		t.Fatalf("expired = %d %s", expired.Code, expired.Body.String())
	}
	unknown := runnerSend(t, srv, http.MethodPost, "/api/runner/register", "", map[string]any{
		"token": strings.Repeat("a", 64), "name": "agent-4", "slots": 1, "protocol": 1,
	})
	if unknown.Code != 401 {
		t.Fatalf("unknown = %d", unknown.Code)
	}
}

func TestRunnerAuthScope(t *testing.T) {
	srv, admin := runnerAdmin(t)
	reg := registerRunner(t, srv, mintToken(t, srv, admin), "agent-1", 1, 1)
	if rec := runnerSend(t, srv, http.MethodPost, "/api/runner/heartbeat", reg.Secret, map[string]any{"version": "0.1.0", "protocol": 1}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"state":"active"`) {
		t.Fatalf("heartbeat = %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/cards", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Authorization", "Runner "+reg.Secret)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("runner on /api/cards = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/runners", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Authorization", "Bearer "+reg.Secret)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("runner secret as bearer = %d", rec.Code)
	}
	userOnRunner := httptest.NewRequest(http.MethodPost, "/api/runner/heartbeat", strings.NewReader(`{}`))
	userOnRunner.Host = "127.0.0.1:8787"
	userOnRunner.Header.Set("Authorization", "Bearer "+admin)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, userOnRunner)
	if rec.Code != 401 {
		t.Fatalf("user bearer on heartbeat = %d", rec.Code)
	}
	if rec := postJSON(t, srv, "/api/runners/"+reg.RunnerID+"/remove", admin, map[string]any{}); rec.Code != 204 {
		t.Fatalf("remove = %d %s", rec.Code, rec.Body.String())
	}
	if rec := runnerSend(t, srv, http.MethodPost, "/api/runner/heartbeat", reg.Secret, map[string]any{"protocol": 1}); rec.Code != 401 {
		t.Fatalf("removed runner heartbeat = %d", rec.Code)
	}
	if list := listRunners(t, srv, admin); len(list.Runners) != 0 {
		t.Fatalf("removed runner listed: %+v", list.Runners)
	}
}

func TestProtocolMismatchRefused(t *testing.T) {
	srv, admin := runnerAdmin(t)
	reg := registerRunner(t, srv, mintToken(t, srv, admin), "agent-1", 1, 1)
	beat := runnerSend(t, srv, http.MethodPost, "/api/runner/heartbeat", reg.Secret, map[string]any{"version": "0.0.9", "protocol": 0})
	if beat.Code != 200 || !strings.Contains(beat.Body.String(), `"protocol":1`) {
		t.Fatalf("old heartbeat = %d %s", beat.Code, beat.Body.String())
	}
	claim := runnerSend(t, srv, http.MethodPost, "/api/runner/claim", reg.Secret, map[string]any{"waitSeconds": 0})
	if claim.Code != 409 || !strings.Contains(claim.Body.String(), `"protocol"`) {
		t.Fatalf("old claim = %d %s", claim.Code, claim.Body.String())
	}
	if got := listRunners(t, srv, admin).Runners[0]; !got.UpdateRequired {
		t.Fatalf("old runner = %+v", got)
	}
	runnerSend(t, srv, http.MethodPost, "/api/runner/heartbeat", reg.Secret, map[string]any{"version": "0.1.0", "protocol": 1})
	if claim := runnerSend(t, srv, http.MethodPost, "/api/runner/claim", reg.Secret, map[string]any{"waitSeconds": 0}); claim.Code != 204 {
		t.Fatalf("updated claim = %d %s", claim.Code, claim.Body.String())
	}
	if got := listRunners(t, srv, admin).Runners[0]; got.UpdateRequired {
		t.Fatalf("updated runner = %+v", got)
	}
}

func TestRunnerReleaseFiles(t *testing.T) {
	srv, _ := runnerAdmin(t)
	if rec := runnerSend(t, srv, http.MethodGet, "/runner/install.sh", "", nil); rec.Code != 404 {
		t.Fatalf("install without releases = %d", rec.Code)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{"install-runner.sh": "#!/bin/sh\n", "SHA256SUMS": "abc  orchestrator-linux-amd64\n", "orchestrator-linux-amd64": "binary", "orchestrator-runner.service": "[Unit]\n", "notes.txt": "private"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv.RunnerReleases = dir
	for path, want := range map[string]string{
		"/runner/install.sh":                           "#!/bin/sh\n",
		"/runner/download/SHA256SUMS":                  "abc  orchestrator-linux-amd64\n",
		"/runner/download/orchestrator-linux-amd64":    "binary",
		"/runner/download/orchestrator-runner.service": "[Unit]\n",
	} {
		if rec := runnerSend(t, srv, http.MethodGet, path, "", nil); rec.Code != 200 || rec.Body.String() != want {
			t.Fatalf("%s = %d %q", path, rec.Code, rec.Body.String())
		}
	}
	for _, path := range []string{"/runner/download/notes.txt", "/runner/download/orchestrator-linux-arm64", "/runner/download/..%2Fnotes.txt"} {
		if rec := runnerSend(t, srv, http.MethodGet, path, "", nil); rec.Code != 404 {
			t.Fatalf("%s = %d", path, rec.Code)
		}
	}
}

func TestRunnerStatusDerived(t *testing.T) {
	srv, admin := runnerAdmin(t)
	reg := registerRunner(t, srv, mintToken(t, srv, admin), "agent-1", 2, 1)
	list := listRunners(t, srv, admin)
	if list.Executor != "local" || list.Protocol != 1 || len(list.Runners) != 1 {
		t.Fatalf("list = %+v", list)
	}
	got := list.Runners[0]
	if got.Status != "idle" || got.Busy != 0 || got.Slots != 2 || strings.Join(got.Actions, ",") != "drain,remove" || got.UpdateRequired {
		t.Fatalf("idle runner = %+v", got)
	}
	card := statusCard(t, srv, "in_progress")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := srv.DB.Exec(`INSERT INTO runs (id, trigger, created_at, processed_at, project_id, card_id) VALUES ('run_lease', 'api', $1, $1, 'proj_default', $2)`,
		now, card.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`
		INSERT INTO runner_leases (id, run_id, runner_id, card_id, project_id, leased_until, created_at)
		VALUES ('lse_test', 'run_lease', $1, $2, 'proj_default', $3, $3)`, reg.RunnerID, card.ID, now); err != nil {
		t.Fatal(err)
	}
	got = listRunners(t, srv, admin).Runners[0]
	if got.Status != "busy" || got.Busy != 1 {
		t.Fatalf("busy runner = %+v", got)
	}
	if rec := postJSON(t, srv, "/api/runners/"+reg.RunnerID+"/resume", admin, map[string]any{}); rec.Code != 422 ||
		!strings.Contains(rec.Body.String(), "transition_rejected") {
		t.Fatalf("resume of an active runner = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/runners/"+reg.RunnerID+"/drain", admin, map[string]any{}); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"status":"draining"`) {
		t.Fatalf("drain = %d %s", rec.Code, rec.Body.String())
	}
	got = listRunners(t, srv, admin).Runners[0]
	if got.Status != "draining" || strings.Join(got.Actions, ",") != "resume,remove" {
		t.Fatalf("draining runner = %+v", got)
	}
	old := time.Now().Add(-61 * time.Second).UTC().Format(time.RFC3339)
	if _, err := srv.DB.Exec(`UPDATE runners SET heartbeat_at = $1 WHERE id = $2`, old, reg.RunnerID); err != nil {
		t.Fatal(err)
	}
	if got = listRunners(t, srv, admin).Runners[0]; got.Status != "offline" {
		t.Fatalf("stale runner = %+v", got)
	}
	if _, err := srv.DB.Exec(`UPDATE runners SET protocol = 0 WHERE id = $1`, reg.RunnerID); err != nil {
		t.Fatal(err)
	}
	if got = listRunners(t, srv, admin).Runners[0]; !got.UpdateRequired {
		t.Fatalf("old protocol runner = %+v", got)
	}
	seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	lead := loginToken(t, srv, "lead@example.com", "password-long-2")
	if rec := getAuth(t, srv, "/api/runners", lead); rec.Code != 403 {
		t.Fatalf("non-admin list = %d", rec.Code)
	}
}
