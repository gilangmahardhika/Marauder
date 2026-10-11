package httpapi_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
)

type settingsSave struct {
	section string
	body    any
}

func settingsSaves(repo string) []settingsSave {
	return []settingsSave{
		{"repository", map[string]string{"repo": repo, "baseBranch": "main"}},
		{"links", map[string]any{"links": []any{}}},
		{"deploy", map[string]any{"environment": "dev", "sandboxEngine": "docker"}},
		{"conventions", map[string]any{"manualStackIds": []string{}, "skillIds": []string{}}},
	}
}

func wantCanEdit(t *testing.T, rec *httptest.ResponseRecorder, what string, want bool) {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("%s = %d %s", what, rec.Code, rec.Body.String())
	}
	flag := `"canEdit":false`
	if want {
		flag = `"canEdit":true`
	}
	if !strings.Contains(rec.Body.String(), flag) {
		t.Fatalf("%s: want %s in %s", what, flag, rec.Body.String())
	}
}

func TestAdminEditsProjectSettings(t *testing.T) {
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	seedUser(t, srv, "po@example.com", "password-long-2", "product_owner", true)
	creator := loginToken(t, srv, "ada@example.com", "password-long-1")
	token := loginToken(t, srv, "po@example.com", "password-long-2")
	tribe := newTribe(t, srv, creator, "Platform")
	project := newProject(t, srv, creator, tribe, "Widget", "widget")
	prefix := "/api/projects/" + project

	for _, save := range settingsSaves("widget") {
		path := prefix + "/settings/" + save.section
		wantCanEdit(t, getAuth(t, srv, path, token), "get "+save.section, true)
		wantCanEdit(t, putJSON(t, srv, path, token, save.body), "put "+save.section, true)
	}
	if rec := postJSON(t, srv, prefix+"/sync", token, map[string]any{}); rec.Code == 403 || rec.Code == 404 {
		t.Fatalf("sync = %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAuth(t, srv, prefix+"/cards", token); rec.Code != 404 {
		t.Fatalf("cards = %d, want 404 for an admin outside the tribe", rec.Code)
	}
	if rec := getAuth(t, srv, "/api/projects/proj_missing/settings/repository", token); rec.Code != 404 {
		t.Fatalf("missing project = %d, want 404", rec.Code)
	}
}

func TestTribeAdminEditsProjectSettings(t *testing.T) {
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	chief := seedUser(t, srv, "chief@example.com", "password-long-2", "developer", false)
	dev := seedUser(t, srv, "dev@example.com", "password-long-3", "developer", false)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	own := newTribe(t, srv, adminToken, "Platform")
	other := newTribe(t, srv, adminToken, "Payments")
	widget := newProject(t, srv, adminToken, own, "Widget", "widget")
	gadget := newProject(t, srv, adminToken, other, "Gadget", "gadget")
	for _, user := range []app.User{chief, dev} {
		if err := app.AddMember(ctx, srv.DB, admin, own, user.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.SetTribeAdmin(ctx, srv.DB, admin, own, chief.ID, true); err != nil {
		t.Fatal(err)
	}
	chiefToken := loginToken(t, srv, "chief@example.com", "password-long-2")
	devToken := loginToken(t, srv, "dev@example.com", "password-long-3")

	for _, save := range settingsSaves("widget") {
		path := "/api/projects/" + widget + "/settings/" + save.section
		wantCanEdit(t, putJSON(t, srv, path, chiefToken, save.body), "tribe admin put "+save.section, true)
		if rec := getAuth(t, srv, "/api/projects/"+gadget+"/settings/"+save.section, chiefToken); rec.Code != 404 {
			t.Fatalf("other tribe %s = %d, want 404", save.section, rec.Code)
		}
		wantCanEdit(t, getAuth(t, srv, path, devToken), "developer get "+save.section, false)
		if rec := putJSON(t, srv, path, devToken, save.body); rec.Code != 403 {
			t.Fatalf("developer put %s = %d %s, want 403", save.section, rec.Code, rec.Body.String())
		}
	}
}
