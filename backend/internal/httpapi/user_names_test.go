package httpapi_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
)

func TestUserNameRequired(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	user := map[string]any{"email": "grace@example.com", "password": "password-long-2", "role": "developer"}
	for _, name := range []string{"", "   ", strings.Repeat("a", 81), "Grace\nHopper"} {
		user["name"] = name
		if rec := postJSON(t, srv, "/api/users", token, user); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"name"`) {
			t.Fatalf("name %q = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	user["name"] = "  Grace Hopper  "
	rec := postJSON(t, srv, "/api/users", token, user)
	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"name":"Grace Hopper"`) {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if list := getAuth(t, srv, "/api/users", token); !strings.Contains(list.Body.String(), `"name":"Grace Hopper"`) {
		t.Fatalf("list = %s", list.Body.String())
	}
}

func TestAdminRenamesUser(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "product_owner", true)
	grace := seedUser(t, srv, "grace@example.com", "password-long-2", "developer", false)
	token := loginToken(t, srv, "ada@example.com", "password-long-1")
	path := "/api/users/" + grace.ID
	rec := putJSON(t, srv, path, token, map[string]any{"name": "Grace Hopper", "role": "qa_lead"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"Grace Hopper"`) || !strings.Contains(rec.Body.String(), `"role":"qa_lead"`) {
		t.Fatalf("rename = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, path, token, map[string]any{"name": " "}); rec.Code != 422 || !strings.Contains(rec.Body.String(), `"name"`) {
		t.Fatalf("empty = %d %s", rec.Code, rec.Body.String())
	}
	if rec := putJSON(t, srv, path, token, map[string]any{"disabled": false}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"Grace Hopper"`) {
		t.Fatalf("save without name = %d %s", rec.Code, rec.Body.String())
	}
	graceToken := loginToken(t, srv, "grace@example.com", "password-long-2")
	if rec := putJSON(t, srv, path, graceToken, map[string]any{"name": "Someone"}); rec.Code != 403 {
		t.Fatalf("non-admin rename = %d %s", rec.Code, rec.Body.String())
	}
}

func TestOwnName(t *testing.T) {
	srv := accountServer(t)
	seedUser(t, srv, "ada@example.com", "password-long-1", "developer", false)
	seedUser(t, srv, "grace@example.com", "password-long-2", "developer", false)
	ada := loginToken(t, srv, "ada@example.com", "password-long-1")
	grace := loginToken(t, srv, "grace@example.com", "password-long-2")
	rec := putJSON(t, srv, "/api/me", ada, map[string]string{"name": "Ada Lovelace"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"Ada Lovelace"`) || !strings.Contains(rec.Body.String(), `"tribeAdminOf"`) {
		t.Fatalf("put me = %d %s", rec.Code, rec.Body.String())
	}
	if me := getAuth(t, srv, "/api/me", ada); !strings.Contains(me.Body.String(), `"name":"Ada Lovelace"`) {
		t.Fatalf("me = %s", me.Body.String())
	}
	if rec := putJSON(t, srv, "/api/me", ada, map[string]string{"name": ""}); rec.Code != 422 {
		t.Fatalf("empty = %d %s", rec.Code, rec.Body.String())
	}
	if me := getAuth(t, srv, "/api/me", grace); !strings.Contains(me.Body.String(), `"name":""`) {
		t.Fatalf("other user = %s", me.Body.String())
	}
}

func TestCardPeopleNames(t *testing.T) {
	srv := accountServer(t)
	ctx := context.Background()
	admin := seedUser(t, srv, "admin@example.com", "password-long-0", "product_owner", true)
	owner := verifiedMember(t, srv, admin, "ada@example.com", "ada", 41)
	grace := verifiedMember(t, srv, admin, "grace@example.com", "grace", 42)
	lin := verifiedMember(t, srv, admin, "lin@example.com", "lin", 43)
	if _, err := app.SetOwnName(ctx, srv.DB, owner, "Ada Lovelace"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetOwnName(ctx, srv.DB, grace, "Grace Hopper"); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`
		INSERT INTO requirements (
			id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
			content_hash, project_id, created_by
		) VALUES ('REQ-001', 'prd', 'docs/prd/a.md', 'Export', 'statement', '[]', '[]', 'h1', 'proj_default', $1)`,
		owner.ID); err != nil {
		t.Fatal(err)
	}
	for _, reviewer := range []string{grace.ID, lin.ID} {
		if _, err := srv.DB.Exec(`
			INSERT INTO requirement_reviewers (project_id, requirement_id, user_id, added_by, created_at)
			VALUES ('proj_default', 'REQ-001', $1, $2, 'now')`, reviewer, admin.ID); err != nil {
			t.Fatal(err)
		}
	}
	id := reviewerCard(t, srv, "proj_default", owner.ID)
	token := loginToken(t, srv, "ada@example.com", "password-long-9")
	rec := getAuth(t, srv, "/api/projects/proj_default/cards/"+id, token)
	var card struct {
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Reviewers []struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"reviewers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
		t.Fatalf("card = %d %s", rec.Code, rec.Body.String())
	}
	if card.Owner.Name != "Ada Lovelace" || len(card.Reviewers) != 2 ||
		card.Reviewers[0].Name != "Grace Hopper" || card.Reviewers[1].Email != "lin@example.com" || card.Reviewers[1].Name != "" {
		t.Fatalf("names = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":""`) {
		t.Fatalf("unnamed reviewer has no empty name: %s", rec.Body.String())
	}
}
