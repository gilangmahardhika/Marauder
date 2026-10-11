package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/httpapi"
)

type tribeFixture struct {
	srv        *httpapi.Server
	admin      app.User
	adminToken string
	tribe      string
	other      string
	chief      app.User
	chiefToken string
	member     app.User
	outsider   app.User
}

// newTribeFixture makes a tribe whose tribe admin is a plain developer, one
// more member, one user outside the tribe, and a second tribe.
func newTribeFixture(t *testing.T) tribeFixture {
	t.Helper()
	ctx := context.Background()
	srv := accountServer(t)
	f := tribeFixture{srv: srv}
	f.admin = seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	f.chief = seedUser(t, srv, "chief@example.com", "password-long-2", "developer", false)
	f.member = seedUser(t, srv, "member@example.com", "password-long-3", "developer", false)
	f.outsider = seedUser(t, srv, "out@example.com", "password-long-4", "developer", false)
	f.adminToken = loginToken(t, srv, "ada@example.com", "password-long-1")
	f.tribe = newTribe(t, srv, f.adminToken, "Platform")
	f.other = newTribe(t, srv, f.adminToken, "Payments")
	for _, user := range []app.User{f.chief, f.member} {
		if err := app.AddMember(ctx, srv.DB, f.admin, f.tribe, user.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.SetTribeAdmin(ctx, srv.DB, f.admin, f.tribe, f.chief.ID, true); err != nil {
		t.Fatal(err)
	}
	f.chiefToken = loginToken(t, srv, "chief@example.com", "password-long-2")
	return f
}

func isMember(t *testing.T, f tribeFixture, userID string) bool {
	t.Helper()
	var n int
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM tribe_members WHERE tribe_id = $1 AND user_id = $2`, f.tribe, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestTribeAdminManagesMembers(t *testing.T) {
	f := newTribeFixture(t)
	members := "/api/tribes/" + f.tribe + "/members"
	candidates := getAuth(t, f.srv, "/api/tribes/"+f.tribe+"/candidates", f.chiefToken)
	if candidates.Code != 200 || !strings.Contains(candidates.Body.String(), "out@example.com") || strings.Contains(candidates.Body.String(), "member@example.com") {
		t.Fatalf("candidates = %d %s", candidates.Code, candidates.Body.String())
	}
	if rec := postJSON(t, f.srv, members, f.chiefToken, map[string]string{"userId": f.outsider.ID}); rec.Code != 204 {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
	if !isMember(t, f, f.outsider.ID) {
		t.Fatal("tribe admin did not add the member")
	}
	if rec := sendJSON(t, f.srv, http.MethodDelete, members+"/"+f.member.ID, f.chiefToken, nil); rec.Code != 204 {
		t.Fatalf("remove = %d %s", rec.Code, rec.Body.String())
	}
	if isMember(t, f, f.member.ID) {
		t.Fatal("tribe admin did not remove the member")
	}
	self := sendJSON(t, f.srv, http.MethodDelete, members+"/"+f.chief.ID, f.chiefToken, nil)
	if self.Code != 422 || !isMember(t, f, f.chief.ID) {
		t.Fatalf("remove self = %d %s", self.Code, self.Body.String())
	}
	if list := getAuth(t, f.srv, members, f.chiefToken); list.Code != 200 || !strings.Contains(list.Body.String(), `"tribeAdmin":true`) {
		t.Fatalf("members = %d %s", list.Code, list.Body.String())
	}
}

func TestTribeAdminGrantsTribeAdmin(t *testing.T) {
	f := newTribeFixture(t)
	path := "/api/tribes/" + f.tribe + "/members/" + f.member.ID
	if rec := putJSON(t, f.srv, path, f.chiefToken, map[string]bool{"tribeAdmin": true}); rec.Code != 204 {
		t.Fatalf("grant = %d %s", rec.Code, rec.Body.String())
	}
	var on bool
	if err := f.srv.DB.QueryRow(`SELECT admin FROM tribe_members WHERE tribe_id = $1 AND user_id = $2`, f.tribe, f.member.ID).Scan(&on); err != nil || !on {
		t.Fatalf("flag = %v %v", on, err)
	}
	memberToken := loginToken(t, f.srv, "member@example.com", "password-long-3")
	if me := getAuth(t, f.srv, "/api/me", memberToken); !strings.Contains(me.Body.String(), `"tribeAdminOf":["`+f.tribe+`"]`) {
		t.Fatalf("me = %s", me.Body.String())
	}
	own := putJSON(t, f.srv, "/api/tribes/"+f.tribe+"/members/"+f.chief.ID, f.chiefToken, map[string]bool{"tribeAdmin": false})
	if own.Code != 422 {
		t.Fatalf("clear own flag = %d %s", own.Code, own.Body.String())
	}
	if rec := putJSON(t, f.srv, path, f.chiefToken, map[string]bool{"tribeAdmin": false}); rec.Code != 204 {
		t.Fatalf("revoke = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, f.srv, "/api/tribes/"+f.tribe+"/members/"+f.outsider.ID, f.chiefToken, map[string]bool{"tribeAdmin": true}); rec.Code != 404 {
		t.Fatalf("grant non-member = %d %s", rec.Code, rec.Body.String())
	}
}

func TestNonTribeAdminCannotManage(t *testing.T) {
	f := newTribeFixture(t)
	memberToken := loginToken(t, f.srv, "member@example.com", "password-long-3")
	members := "/api/tribes/" + f.tribe + "/members"
	checks := map[string]int{
		"add":        postJSON(t, f.srv, members, memberToken, map[string]string{"userId": f.outsider.ID}).Code,
		"remove":     sendJSON(t, f.srv, http.MethodDelete, members+"/"+f.chief.ID, memberToken, nil).Code,
		"grant":      putJSON(t, f.srv, members+"/"+f.member.ID, memberToken, map[string]bool{"tribeAdmin": true}).Code,
		"candidates": getAuth(t, f.srv, "/api/tribes/"+f.tribe+"/candidates", memberToken).Code,
		"create":     postJSON(t, f.srv, "/api/projects", memberToken, map[string]string{"tribeId": f.tribe, "name": "Widget", "repo": "widget"}).Code,
		"other tribe": postJSON(t, f.srv, "/api/tribes/"+f.other+"/members", f.chiefToken,
			map[string]string{"userId": f.outsider.ID}).Code,
		"create other tribe": postJSON(t, f.srv, "/api/projects", f.chiefToken,
			map[string]string{"tribeId": f.other, "name": "Ledger", "repo": "ledger"}).Code,
	}
	for name, code := range checks {
		if code != 403 {
			t.Errorf("%s = %d, want 403", name, code)
		}
	}
	if isMember(t, f, f.outsider.ID) || !isMember(t, f, f.chief.ID) {
		t.Fatal("membership changed")
	}
}

func TestTribeAdminCreatesProjectWithLinks(t *testing.T) {
	f := newTribeFixture(t)
	gadget := newProject(t, f.srv, f.adminToken, f.tribe, "Gadget", "gadget")
	rec := postJSON(t, f.srv, "/api/projects", f.chiefToken, map[string]any{
		"tribeId": f.tribe, "name": "Widget", "repo": "widget",
		"links": []map[string]any{{"projectId": gadget, "sharedPaths": []string{"docs/api"}}},
	})
	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"projectId":"`+gadget+`"`) || !strings.Contains(rec.Body.String(), `"docs/api"`) {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var n int
	if err := f.srv.DB.QueryRow(`
		SELECT count(*) FROM project_links l JOIN projects p ON p.id = l.project_id
		WHERE p.name = 'Widget' AND l.linked_project_id = $1`, gadget).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("links = %d", n)
	}
	if list := getAuth(t, f.srv, "/api/projects", f.chiefToken); !strings.Contains(list.Body.String(), `"Widget"`) {
		t.Fatalf("tribe admin cannot see the new project: %s", list.Body.String())
	}
}

func TestCreateProjectLinkOtherTribeRollsBack(t *testing.T) {
	f := newTribeFixture(t)
	ledger := newProject(t, f.srv, f.adminToken, f.other, "Ledger", "ledger")
	rec := postJSON(t, f.srv, "/api/projects", f.adminToken, map[string]any{
		"tribeId": f.tribe, "name": "Widget", "repo": "widget",
		"links": []map[string]any{{"projectId": ledger}},
	})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"links"`) {
		t.Fatalf("cross tribe create = %d %s", rec.Code, rec.Body.String())
	}
	var n int
	if err := f.srv.DB.QueryRow(`SELECT count(*) FROM projects WHERE name = 'Widget'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("a failed create stored the project")
	}
}

func TestTribeAdminSavesLinks(t *testing.T) {
	f := newTribeFixture(t)
	widget := newProject(t, f.srv, f.adminToken, f.tribe, "Widget", "widget")
	gadget := newProject(t, f.srv, f.adminToken, f.tribe, "Gadget", "gadget")
	path := "/api/projects/" + widget + "/settings/links"
	body := map[string]any{"links": []map[string]any{{"projectId": gadget}}}
	memberToken := loginToken(t, f.srv, "member@example.com", "password-long-3")
	if rec := putJSON(t, f.srv, path, memberToken, body); rec.Code != 403 {
		t.Fatalf("developer = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, f.srv, path, f.chiefToken, body); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"projectId":"`+gadget+`"`) {
		t.Fatalf("tribe admin = %d %s", rec.Code, rec.Body.String())
	}
}
