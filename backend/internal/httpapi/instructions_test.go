package httpapi_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

type instructionsCard struct {
	Actions      []string `json:"actions"`
	Instructions *string  `json:"instructions"`
	Queued       bool     `json:"queued"`
	Notes        []struct {
		Source string `json:"source"`
		Note   string `json:"note"`
	} `json:"notes"`
}

func decodeInstructions(t *testing.T, body []byte) instructionsCard {
	t.Helper()
	var card instructionsCard
	if err := json.Unmarshal(body, &card); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return card
}

func TestInstructionsRoute(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "developer")
	card := statusCard(t, srv, "changes_requested")
	path := "/api/projects/proj_default/cards/" + card.ID

	got := decodeInstructions(t, getAuth(t, srv, path, token).Body.Bytes())
	if !slices.Contains(got.Actions, "instructions") || got.Instructions != nil {
		t.Fatalf("before = %+v", got)
	}

	rec := putJSON(t, srv, path+"/instructions", token, map[string]any{"instructions": " Use the lint script. "})
	if rec.Code != 200 {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}
	got = decodeInstructions(t, rec.Body.Bytes())
	if got.Instructions == nil || *got.Instructions != "Use the lint script." {
		t.Fatalf("after put = %+v", got)
	}
	if len(got.Notes) != 1 || got.Notes[0].Source != "instructions" {
		t.Fatalf("notes = %+v", got.Notes)
	}

	rec = putJSON(t, srv, path+"/instructions", token, map[string]any{"instructions": strings.Repeat("x", 4001)})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"instructions"`) {
		t.Fatalf("long put = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := srv.DB.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ('proj_other', 'tribe_default', 'Other', '/tmp/other', '2026-10-12T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if rec := putJSON(t, srv, "/api/projects/proj_other/cards/"+card.ID+"/instructions", token, map[string]any{"instructions": "x"}); rec.Code != 404 {
		t.Fatalf("other project put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, path+"/instructions", token, map[string]any{"instructions": "x"}); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post = %d", rec.Code)
	}
	if rec := postJSON(t, srv, path+"/run", token, map[string]any{"instructions": 5}); rec.Code != 422 {
		t.Fatalf("run with a number = %d %s", rec.Code, rec.Body.String())
	}

	rec = postJSON(t, srv, path+"/run", token, map[string]any{"instructions": "Keep tsc."})
	if rec.Code != 200 {
		t.Fatalf("run = %d %s", rec.Code, rec.Body.String())
	}
	got = decodeInstructions(t, rec.Body.Bytes())
	if !got.Queued || got.Instructions == nil || *got.Instructions != "Keep tsc." {
		t.Fatalf("after run = %+v", got)
	}
	if board := getAuth(t, srv, "/api/projects/proj_default/cards", token).Body.String(); strings.Contains(board, "Keep tsc.") {
		t.Fatalf("board list carries instruction text: %s", board)
	}

	rec = putJSON(t, srv, path+"/instructions", token, map[string]any{"instructions": ""})
	if got := decodeInstructions(t, rec.Body.Bytes()); rec.Code != 200 || got.Instructions != nil {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body.String())
	}

	done := statusCard(t, srv, "done")
	donePath := "/api/projects/proj_default/cards/" + done.ID
	if got := decodeInstructions(t, getAuth(t, srv, donePath, token).Body.Bytes()); slices.Contains(got.Actions, "instructions") {
		t.Fatalf("done card offers instructions: %+v", got)
	}
	if rec := putJSON(t, srv, donePath+"/instructions", token, map[string]any{"instructions": "x"}); rec.Code != 409 {
		t.Fatalf("done put = %d %s", rec.Code, rec.Body.String())
	}
}

func TestInstructionsRole(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "product_owner")
	card := statusCard(t, srv, "changes_requested")
	path := "/api/projects/proj_default/cards/" + card.ID
	if got := decodeInstructions(t, getAuth(t, srv, path, token).Body.Bytes()); slices.Contains(got.Actions, "instructions") {
		t.Fatalf("product owner sees instructions: %+v", got)
	}
	if rec := putJSON(t, srv, path+"/instructions", token, map[string]any{"instructions": "x"}); rec.Code != 403 {
		t.Fatalf("put = %d %s, want 403", rec.Code, rec.Body.String())
	}
}
