package httpapi_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/httpapi"
	"orchestrator/backend/internal/identity"
)

// ghPost is one gh api write: the REST path and the JSON it sent.
type ghPost struct {
	path, input string
	env         []string
	argv        []string
}

type ghWrites struct {
	mu    sync.Mutex
	posts []ghPost
	fail  bool
}

func (g *ghWrites) run(_ context.Context, child execx.Child) (execx.Result, error) {
	var post ghPost
	for i, arg := range child.Argv {
		if arg == "api" && i+1 < len(child.Argv) {
			post.path = child.Argv[i+1]
		}
		if arg == "--input" && i+1 < len(child.Argv) {
			body, _ := os.ReadFile(child.Argv[i+1])
			post.input = string(body)
		}
	}
	post.env, post.argv = child.Env, child.Argv
	g.mu.Lock()
	defer g.mu.Unlock()
	g.posts = append(g.posts, post)
	if g.fail {
		return execx.Result{Exit: 1, Out: []byte("gh: Server Error (HTTP 500)\n")}, nil
	}
	return execx.Result{Out: []byte("{}\n")}, nil
}

func (g *ghWrites) all() []ghPost {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]ghPost(nil), g.posts...)
}

// approvalServer has a lead with GitHub login "lead" and a human_review card
// with a pull request.
func approvalServer(t *testing.T) (*httpapi.Server, *ghWrites, board.Card, string) {
	t.Helper()
	srv := accountServer(t)
	srv.Data = dataRoot(t)
	gh := &ghWrites{}
	srv.Tools = app.Tools{GH: "/usr/bin/gh", Exec: gh.run}
	ctx := context.Background()
	lead := seedUser(t, srv, "lead@example.com", "password-long-1", "lead_developer", true)
	if err := identity.SaveGitHub(ctx, srv.DB, lead.ID, "github.com", "lead", nil, false, srv.TokenKey); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMember(ctx, srv.DB, lead, "tribe_default", lead.ID); err != nil {
		t.Fatal(err)
	}
	card := reviewCard(t, srv, "proj_default")
	if _, err := srv.DB.Exec(`UPDATE cards SET pr_url = 'https://github.com/acme/widget/pull/7' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	return srv, gh, card, loginToken(t, srv, "lead@example.com", "password-long-1")
}

func TestApprovalPostsCommentAndStatus(t *testing.T) {
	srv, gh, card, token := approvalServer(t)
	path := "/api/projects/proj_default/cards/" + card.ID
	if rec := postJSON(t, srv, path+"/approve", token, map[string]string{"sha": headSHA}); rec.Code != 200 {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	posts := gh.all()
	if len(posts) != 2 {
		t.Fatalf("posts = %+v", posts)
	}
	comment, status := posts[0], posts[1]
	if comment.path != "repos/acme/widget/issues/7/comments" || !strings.Contains(comment.input, "Approved in the orchestrator") ||
		!strings.Contains(comment.input, "@lead") || !strings.Contains(comment.input, headSHA[:7]) {
		t.Fatalf("comment = %+v", comment)
	}
	if status.path != "repos/acme/widget/statuses/"+headSHA || !strings.Contains(status.input, `"state":"success"`) ||
		!strings.Contains(status.input, `"context":"`+app.ApprovalContext+`"`) {
		t.Fatalf("status = %+v", status)
	}
	for _, post := range posts {
		if !contains(post.env, "GH_TOKEN="+companyToken) || strings.Contains(strings.Join(post.argv, " "), companyToken) {
			t.Fatalf("token handling: argv=%v", post.argv)
		}
	}
	if rec := postJSON(t, srv, path+"/approve", token, map[string]string{"sha": headSHA}); rec.Code != 200 {
		t.Fatalf("second approve = %d %s", rec.Code, rec.Body.String())
	}
	if again := gh.all(); len(again) != 2 {
		t.Fatalf("a repeated approve posted again: %+v", again[2:])
	}
}

func TestApprovalAnnounceFailureKeepsApproved(t *testing.T) {
	srv, gh, card, token := approvalServer(t)
	gh.fail = true
	rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+card.ID+"/approve", token, map[string]string{"sha": headSHA})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"approved"`) {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	var comment, status int
	if err := srv.DB.QueryRow(`
		SELECT count(*) FILTER (WHERE payload_json LIKE '%approval comment:%'),
			count(*) FILTER (WHERE payload_json LIKE '%approval status:%')
		FROM events WHERE card_id = $1 AND kind = 'job_error'`, card.ID).Scan(&comment, &status); err != nil {
		t.Fatal(err)
	}
	if comment != 1 || status != 1 {
		t.Fatalf("job errors comment=%d status=%d", comment, status)
	}
}

func TestRequestChangesWithdrawsApprovalStatus(t *testing.T) {
	srv, gh, card, token := approvalServer(t)
	path := "/api/projects/proj_default/cards/" + card.ID
	if rec := postJSON(t, srv, path+"/approve", token, map[string]string{"sha": headSHA}); rec.Code != 200 {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, path+"/request-changes", token, map[string]string{}); rec.Code != 200 {
		t.Fatalf("request changes = %d %s", rec.Code, rec.Body.String())
	}
	posts := gh.all()
	last := posts[len(posts)-1]
	if len(posts) != 3 || last.path != "repos/acme/widget/statuses/"+headSHA || !strings.Contains(last.input, `"state":"failure"`) {
		t.Fatalf("posts = %+v", posts)
	}
}

func TestRequestChangesReason(t *testing.T) {
	srv, _, card, token := approvalServer(t)
	path := "/api/projects/proj_default/cards/" + card.ID
	type detail struct {
		Status string `json:"status"`
		Notes  []struct {
			Source   string `json:"source"`
			Note     string `json:"note"`
			AttemptN int    `json:"attemptN"`
		} `json:"notes"`
	}
	decode := func(body []byte) detail {
		t.Helper()
		var got detail
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	long := postJSON(t, srv, path+"/request-changes", token, map[string]string{"reason": strings.Repeat("é", 4001)})
	if long.Code != 422 || !strings.Contains(long.Body.String(), `"reason":"Keep the reason to 4000 characters or fewer."`) {
		t.Fatalf("long reason = %d %s", long.Code, long.Body.String())
	}
	if got := decode(getAuth(t, srv, path, token).Body.Bytes()); got.Status != "human_review" || len(got.Notes) != 0 {
		t.Fatalf("after long reason = %+v", got)
	}

	rec := postJSON(t, srv, path+"/request-changes", token, map[string]string{"reason": "  Use the **lint** scripts.\r\n\r\n- Keep `.nvmrc`.\r\n "})
	if rec.Code != 200 {
		t.Fatalf("request changes = %d %s", rec.Code, rec.Body.String())
	}
	got := decode(rec.Body.Bytes())
	if got.Status != "changes_requested" || len(got.Notes) != 1 || got.Notes[0].Source != "approver" ||
		got.Notes[0].Note != "Use the **lint** scripts.\n\n- Keep `.nvmrc`." || got.Notes[0].AttemptN != 0 {
		t.Fatalf("request changes = %+v", got)
	}

	if _, err := srv.DB.Exec(`UPDATE failure_notes SET created_at = '2026-01-01T00:00:00Z' WHERE card_id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET status = 'human_review' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	rec = postJSON(t, srv, path+"/request-changes", token, map[string]string{})
	if rec.Code != 200 {
		t.Fatalf("request changes without reason = %d %s", rec.Code, rec.Body.String())
	}
	if got := decode(rec.Body.Bytes()); len(got.Notes) != 2 || got.Notes[0].Note != "changes requested" {
		t.Fatalf("request changes without reason = %+v", got)
	}
}

func TestCardDiffRoute(t *testing.T) {
	srv, _, card, token := approvalServer(t)
	path := "/api/projects/proj_default/cards/" + card.ID + "/diff"
	got := getAuth(t, srv, path, token)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"sha":"`+headSHA+`"`) || !strings.Contains(got.Body.String(), `"missing":true`) {
		t.Fatalf("member diff = %d %s", got.Code, got.Body.String())
	}
	seedUser(t, srv, "out@example.com", "password-long-9", "lead_developer", false)
	outsider := loginToken(t, srv, "out@example.com", "password-long-9")
	if denied := getAuth(t, srv, path, outsider); denied.Code != 404 {
		t.Fatalf("non-member diff = %d %s", denied.Code, denied.Body.String())
	}
}
