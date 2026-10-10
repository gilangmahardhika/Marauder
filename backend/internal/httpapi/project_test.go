package httpapi_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
)

func TestMemberSeesProject(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	tribe := postJSON(t, srv, "/api/tribes", token, map[string]string{"name": "Platform"})
	if tribe.Code != 201 {
		t.Fatalf("tribe = %d %s", tribe.Code, tribe.Body.String())
	}
	var tribeBody struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(tribe.Body.Bytes(), &tribeBody); err != nil {
		t.Fatal(err)
	}
	if rec := postJSON(t, srv, "/api/tribes/"+tribeBody.ID+"/members", token, map[string]string{"userId": admin.ID}); rec.Code != 204 {
		t.Fatalf("member = %d %s", rec.Code, rec.Body.String())
	}
	created := postJSON(t, srv, "/api/projects", token, map[string]string{
		"tribeId": tribeBody.ID, "name": "Widget", "repo": "widget",
	})
	if created.Code != 201 {
		t.Fatalf("project = %d %s", created.Code, created.Body.String())
	}
	var project struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET project_id = $2 WHERE id = $1`, card.ID, project.ID); err != nil {
		t.Fatal(err)
	}
	list := getAuth(t, srv, "/api/projects/"+project.ID+"/cards", token)
	if list.Code != 200 || !strings.Contains(list.Body.String(), card.ID) {
		t.Fatalf("cards = %d %s", list.Code, list.Body.String())
	}
	mine := getAuth(t, srv, "/api/projects", token)
	if mine.Code != 200 || !strings.Contains(mine.Body.String(), project.ID) {
		t.Fatalf("projects = %s", mine.Body.String())
	}
}

func TestOutsiderMissesProject(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	seedUser(t, srv, "bea@example.com", "password-long-2", "developer", false)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	outsider := loginToken(t, srv, "bea@example.com", "password-long-2")
	tribe := postJSON(t, srv, "/api/tribes", adminToken, map[string]string{"name": "Platform"})
	var tribeBody struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(tribe.Body.Bytes(), &tribeBody); err != nil {
		t.Fatal(err)
	}
	postJSON(t, srv, "/api/tribes/"+tribeBody.ID+"/members", adminToken, map[string]string{"userId": admin.ID})
	created := postJSON(t, srv, "/api/projects", adminToken, map[string]string{
		"tribeId": tribeBody.ID, "name": "Widget", "repo": "widget",
	})
	var project struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	card, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET project_id = $2 WHERE id = $1`, card.ID, project.ID); err != nil {
		t.Fatal(err)
	}
	miss := getAuth(t, srv, "/api/projects/"+project.ID+"/cards", outsider)
	if miss.Code != 404 {
		t.Fatalf("outsider cards = %d %s", miss.Code, miss.Body.String())
	}
	one := getAuth(t, srv, "/api/projects/"+project.ID+"/cards/"+card.ID, outsider)
	if one.Code != 404 {
		t.Fatalf("outsider card = %d", one.Code)
	}
	unknown := getAuth(t, srv, "/api/projects/proj_missing/cards", outsider)
	if unknown.Code != 404 {
		t.Fatalf("unknown = %d", unknown.Code)
	}
}

func TestReqIdsPerProject(t *testing.T) {
	srv := accountServer(t)
	insertReq(t, srv, "proj_default")
	if _, err := srv.DB.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_other', 'tribe_default', 'Other', '/tmp/other', $1)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	insertReq(t, srv, "proj_other")
	var n int
	if err := srv.DB.QueryRow(`SELECT count(*) FROM requirements WHERE id = 'REQ-001'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("requirements = %d", n)
	}
}

func TestBranchPerProject(t *testing.T) {
	srv := accountServer(t)
	if _, err := srv.DB.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_other', 'tribe_default', 'Other', '/tmp/other', $1)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	for _, project := range []string{"proj_default", "proj_other"} {
		card, err := board.InsertCard(context.Background(), srv.DB, board.Card{Status: "ready"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := srv.DB.Exec(`UPDATE cards SET project_id = $2, branch = 'orch/same' WHERE id = $1`, card.ID, project); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAdminManagesMembership(t *testing.T) {
	srv := accountServer(t)
	admin := seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	dev := seedUser(t, srv, "dev@example.com", "password-long-2", "developer", false)
	adminToken := loginToken(t, srv, "ada@example.com", "password-long-1")
	devToken := loginToken(t, srv, "dev@example.com", "password-long-2")
	denied := postJSON(t, srv, "/api/tribes", devToken, map[string]string{"name": "Nope"})
	if denied.Code != 403 {
		t.Fatalf("non-admin = %d", denied.Code)
	}
	tribe := postJSON(t, srv, "/api/tribes", adminToken, map[string]string{"name": "Platform"})
	if tribe.Code != 201 {
		t.Fatalf("tribe = %d %s", tribe.Code, tribe.Body.String())
	}
	var tribeBody struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(tribe.Body.Bytes(), &tribeBody); err != nil {
		t.Fatal(err)
	}
	added := postJSON(t, srv, "/api/tribes/"+tribeBody.ID+"/members", adminToken, map[string]string{"userId": dev.ID})
	if added.Code != 204 {
		t.Fatalf("add = %d %s", added.Code, added.Body.String())
	}
	listed := getAuth(t, srv, "/api/tribes/"+tribeBody.ID+"/members", adminToken)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), dev.Email) {
		t.Fatalf("members = %d %s", listed.Code, listed.Body.String())
	}
	if got := getAuth(t, srv, "/api/tribes/"+tribeBody.ID+"/members", devToken); got.Code != 403 {
		t.Fatalf("member list for a non-admin = %d", got.Code)
	}
	if missing := getAuth(t, srv, "/api/tribes/tribe_missing/members", adminToken); missing.Code != 404 {
		t.Fatalf("missing tribe = %d %s", missing.Code, missing.Body.String())
	}
	_ = admin
}

func insertReq(t *testing.T, srv *httpapi.Server, project string) {
	t.Helper()
	_, err := srv.DB.Exec(`
		INSERT INTO requirements (
			id, project_id, source_kind, source_path, heading, statement,
			acceptance_json, non_goals_json, content_hash
		) VALUES ('REQ-001', $1, 'prd', 'docs/prd', 'REQ-001', 'same', '[]', '[]', 'hash')`, project)
	if err != nil {
		t.Fatal(err)
	}
}
