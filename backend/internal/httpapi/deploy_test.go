package httpapi_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/httpapi"
)

const deployToken = "deploy-token-value-1"

type deployFixture struct {
	srv      *httpapi.Server
	lead     string
	dev      string
	projectA string
	projectB string
}

func newDeployFixture(t *testing.T) deployFixture {
	t.Helper()
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	lead := seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	dev := seedUser(t, srv, "dev@example.com", "password-long-3", "developer", false)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, adminToken, "Platform")
	a := newProject(t, srv, adminToken, tribe, "Widget", "widget")
	b := newProject(t, srv, adminToken, tribe, "Gadget", "gadget")
	for _, user := range []app.User{lead, dev} {
		if err := app.AddMember(context.Background(), srv.DB, admin, tribe, user.ID); err != nil {
			t.Fatal(err)
		}
	}
	return deployFixture{
		srv:      srv,
		lead:     loginToken(t, srv, "lead@example.com", "password-long-2"),
		dev:      loginToken(t, srv, "dev@example.com", "password-long-3"),
		projectA: a,
		projectB: b,
	}
}

func TestProjectDeploySettingsIsolated(t *testing.T) {
	f := newDeployFixture(t)
	saved := putJSON(t, f.srv, "/api/projects/"+f.projectA+"/settings/deploy", f.lead, map[string]any{
		"environment": "dev", "devHosts": []string{"dev.example.com"}, "devUrl": "https://dev.example.com",
		"sandboxEngine": "docker", "implementationCap": 3,
	})
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"sandboxEngine":"docker"`) {
		t.Fatalf("put = %d %s", saved.Code, saved.Body.String())
	}
	other := getAuth(t, f.srv, "/api/projects/"+f.projectB+"/settings/deploy", f.lead)
	if other.Code != 200 {
		t.Fatalf("get B = %d %s", other.Code, other.Body.String())
	}
	body := other.Body.String()
	if !strings.Contains(body, `"sandboxEngine":"podman"`) || !strings.Contains(body, `"devUrl":""`) || !strings.Contains(body, `"implementationCap":2`) {
		t.Fatalf("project B saw A's deploy: %s", body)
	}
	again := getAuth(t, f.srv, "/api/projects/"+f.projectA+"/settings/deploy", f.lead)
	if !strings.Contains(again.Body.String(), `"devUrl":"https://dev.example.com"`) {
		t.Fatalf("get A = %s", again.Body.String())
	}
	if strings.Contains(again.Body.String(), "operatorLogin") {
		t.Fatalf("deploy still carries operatorLogin: %s", again.Body.String())
	}
}

func TestProjectDeployTokenNeverReturned(t *testing.T) {
	f := newDeployFixture(t)
	saved := putJSON(t, f.srv, "/api/projects/"+f.projectA+"/settings/deploy", f.lead, map[string]any{
		"environment": "dev", "deployToken": deployToken,
	})
	if saved.Code != 200 || strings.Contains(saved.Body.String(), deployToken) || !strings.Contains(saved.Body.String(), `"deployTokenSet":true`) {
		t.Fatalf("put = %d %s", saved.Code, saved.Body.String())
	}
	got := getAuth(t, f.srv, "/api/projects/"+f.projectA+"/settings/deploy", f.dev)
	if got.Code != 200 || strings.Contains(got.Body.String(), deployToken) {
		t.Fatalf("get = %d %s", got.Code, got.Body.String())
	}
	path := filepath.Join(f.srv.Data, "projects", f.projectA, "secrets.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets mode = %v", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), deployToken) {
		t.Fatal("token not stored in project secrets")
	}
	otherRaw, err := os.ReadFile(filepath.Join(f.srv.Data, "projects", f.projectB, "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(otherRaw), deployToken) {
		t.Fatal("token leaked into another project")
	}
	other := getAuth(t, f.srv, "/api/projects/"+f.projectB+"/settings/deploy", f.lead)
	if !strings.Contains(other.Body.String(), `"deployTokenSet":false`) {
		t.Fatalf("get B = %s", other.Body.String())
	}
}

func TestDeployLeadOnly(t *testing.T) {
	f := newDeployFixture(t)
	denied := putJSON(t, f.srv, "/api/projects/"+f.projectA+"/settings/deploy", f.dev, map[string]any{"environment": "dev"})
	if denied.Code != 403 {
		t.Fatalf("developer put = %d %s", denied.Code, denied.Body.String())
	}
	if got := getAuth(t, f.srv, "/api/projects/"+f.projectA+"/settings/deploy", f.dev); got.Code != 200 {
		t.Fatalf("developer get = %d", got.Code)
	}
	bad := putJSON(t, f.srv, "/api/projects/"+f.projectA+"/settings/deploy", f.lead, map[string]any{"environment": "prod"})
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), `"environment"`) {
		t.Fatalf("lead bad put = %d %s", bad.Code, bad.Body.String())
	}
	if gone := getAuth(t, f.srv, "/api/settings/deploy", f.lead); gone.Code != 404 {
		t.Fatalf("global deploy route = %d", gone.Code)
	}
}
