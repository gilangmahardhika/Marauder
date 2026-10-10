package httpapi_test

import (
	"context"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
)

func TestProjectLinkSameTribeOnly(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	lead := seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	platform := newTribe(t, srv, adminToken, "Platform")
	payments := newTribe(t, srv, adminToken, "Payments")
	widget := newProject(t, srv, adminToken, platform, "Widget", "widget")
	gadget := newProject(t, srv, adminToken, platform, "Gadget", "gadget")
	ledger := newProject(t, srv, adminToken, payments, "Ledger", "ledger")
	if err := app.AddMember(context.Background(), srv.DB, admin, platform, lead.ID); err != nil {
		t.Fatal(err)
	}
	token := loginToken(t, srv, "lead@example.com", "password-long-2")
	cross := putJSON(t, srv, "/api/projects/"+widget+"/settings/links", token, map[string]any{
		"links": []map[string]any{{"projectId": ledger}},
	})
	if cross.Code != 422 || !strings.Contains(cross.Body.String(), `"links"`) {
		t.Fatalf("cross tribe = %d %s", cross.Code, cross.Body.String())
	}
	ok := putJSON(t, srv, "/api/projects/"+widget+"/settings/links", token, map[string]any{
		"links": []map[string]any{{"projectId": gadget, "sharedPaths": []string{"docs/api", "openapi.yaml"}}},
	})
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), `"projectId":"`+gadget+`"`) || !strings.Contains(ok.Body.String(), `"docs/api"`) {
		t.Fatalf("same tribe = %d %s", ok.Code, ok.Body.String())
	}
	if strings.Contains(ok.Body.String(), "repoPath") {
		t.Fatalf("link leaks repo path: %s", ok.Body.String())
	}
	var n int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM project_links WHERE project_id = $1`, gadget).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("link is not one-way")
	}
}

func TestProjectLinkNoSelf(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	lead := seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, adminToken, "Platform")
	widget := newProject(t, srv, adminToken, tribe, "Widget", "widget")
	if err := app.AddMember(context.Background(), srv.DB, admin, tribe, lead.ID); err != nil {
		t.Fatal(err)
	}
	token := loginToken(t, srv, "lead@example.com", "password-long-2")
	self := putJSON(t, srv, "/api/projects/"+widget+"/settings/links", token, map[string]any{
		"links": []map[string]any{{"projectId": widget}},
	})
	if self.Code != 422 {
		t.Fatalf("self link = %d %s", self.Code, self.Body.String())
	}
	if _, err := srv.DB.Exec(`
		INSERT INTO project_links (project_id, linked_project_id, created_at) VALUES ($1, $1, 'now')`, widget); err == nil {
		t.Fatal("database allowed a self link")
	}
}

func TestProjectLinkOutsiderMisses(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	seedUser(t, srv, "out@example.com", "password-long-2", "lead_developer", false)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, adminToken, "Platform")
	widget := newProject(t, srv, adminToken, tribe, "Widget", "widget")
	_ = admin
	token := loginToken(t, srv, "out@example.com", "password-long-2")
	if got := getAuth(t, srv, "/api/projects/"+widget+"/settings/links", token); got.Code != 404 {
		t.Fatalf("outsider get = %d %s", got.Code, got.Body.String())
	}
	put := putJSON(t, srv, "/api/projects/"+widget+"/settings/links", token, map[string]any{"links": []map[string]any{}})
	if put.Code != 404 {
		t.Fatalf("outsider put = %d %s", put.Code, put.Body.String())
	}
	if got := getAuth(t, srv, "/api/projects/"+widget+"/settings/repository", token); got.Code != 404 {
		t.Fatalf("outsider repository = %d", got.Code)
	}
}

func TestSharedPathConfined(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	lead := seedUser(t, srv, "lead@example.com", "password-long-2", "lead_developer", false)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := newTribe(t, srv, adminToken, "Platform")
	widget := newProject(t, srv, adminToken, tribe, "Widget", "widget")
	gadget := newProject(t, srv, adminToken, tribe, "Gadget", "gadget")
	if err := app.AddMember(context.Background(), srv.DB, admin, tribe, lead.ID); err != nil {
		t.Fatal(err)
	}
	token := loginToken(t, srv, "lead@example.com", "password-long-2")
	for _, bad := range []string{"../secrets", "/etc/passwd", "docs/../../x", ".git/config", ".orchestrator/state"} {
		rec := putJSON(t, srv, "/api/projects/"+widget+"/settings/links", token, map[string]any{
			"links": []map[string]any{{"projectId": gadget, "sharedPaths": []string{bad}}},
		})
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"sharedPaths"`) {
			t.Fatalf("%q = %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	many := make([]string, 21)
	for i := range many {
		many[i] = "docs/" + string(rune('a'+i))
	}
	rec := putJSON(t, srv, "/api/projects/"+widget+"/settings/links", token, map[string]any{
		"links": []map[string]any{{"projectId": gadget, "sharedPaths": many}},
	})
	if rec.Code != 422 {
		t.Fatalf("21 paths = %d", rec.Code)
	}
}
