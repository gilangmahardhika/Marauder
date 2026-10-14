package httpapi_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/httpapi"
)

// closeGH records gh calls like ghWrites. A state read answers state, and a
// PATCH fails when refuse is set.
type closeGH struct {
	ghWrites
	state  string
	refuse bool
}

func (c *closeGH) run(ctx context.Context, child execx.Child) (execx.Result, error) {
	_, _ = c.ghWrites.run(ctx, child)
	if slices.Contains(child.Argv, ".state") {
		return execx.Result{Out: []byte(c.state + "\n")}, nil
	}
	if c.refuse && slices.Contains(child.Argv, "PATCH") {
		return execx.Result{Exit: 1, Out: []byte("gh: Validation Failed (HTTP 422)\n")}, nil
	}
	return execx.Result{Out: []byte("{}\n")}, nil
}

// wontFixServer has a card blocked for attempt_cap with a pull request, and a
// lead whose GitHub token is verified when withToken is set.
func wontFixServer(t *testing.T, withToken bool) (*httpapi.Server, *closeGH, board.Card, string) {
	t.Helper()
	srv, _, card, token := mergeServer(t, withToken)
	gh := &closeGH{state: "open"}
	srv.Tools = app.Tools{GH: "/usr/bin/gh", Exec: gh.run}
	if _, err := srv.DB.Exec(`UPDATE cards SET status = 'blocked', blocked_reason = 'attempt_cap', approved_sha = NULL WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	return srv, gh, card, token
}

func wontFix(t *testing.T, srv *httpapi.Server, cardID, token, reason string) (int, string) {
	t.Helper()
	rec := postJSON(t, srv, "/api/projects/proj_default/cards/"+cardID+"/wont-fix", token, map[string]string{"reason": reason})
	return rec.Code, rec.Body.String()
}

func TestActionsWontFix(t *testing.T) {
	srv, _, card, lead := wontFixServer(t, true)
	ctx := context.Background()
	leadUser := app.User{ID: emailUserID(t, srv, "lead@example.com"), Role: "lead_developer", Admin: true}
	po := seedUser(t, srv, "po@example.com", "password-long-2", "product_owner", false)
	qa := seedUser(t, srv, "qa@example.com", "password-long-3", "qa_lead", false)
	owner := seedUser(t, srv, "owner@example.com", "password-long-4", "developer", false)
	for _, u := range []app.User{po, qa, owner} {
		if err := app.AddMember(ctx, srv.DB, leadUser, "tribe_default", u.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET owner_id = $2 WHERE id = $1`, card.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	poToken := loginToken(t, srv, "po@example.com", "password-long-2")
	qaToken := loginToken(t, srv, "qa@example.com", "password-long-3")
	ownerToken := loginToken(t, srv, "owner@example.com", "password-long-4")
	for name, token := range map[string]string{"lead": lead, "product owner": poToken, "owner": ownerToken} {
		if got := mergeActions(t, srv, card.ID, token); !strings.Contains(got, " wont_fix ") {
			t.Fatalf("%s actions =%s", name, got)
		}
	}
	if got := mergeActions(t, srv, card.ID, qaToken); strings.Contains(got, " wont_fix ") {
		t.Fatalf("QA lead actions =%s", got)
	}
	if code, body := wontFix(t, srv, card.ID, qaToken, "Out of scope"); code != 403 {
		t.Fatalf("QA lead wont-fix = %d %s", code, body)
	}

	if _, err := srv.DB.Exec(`UPDATE cards SET status = 'wont_fix' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	rec := getAuth(t, srv, "/api/projects/proj_default/cards/"+card.ID, lead)
	if !strings.Contains(rec.Body.String(), `"group":"closed"`) {
		t.Fatalf("closed card = %s", rec.Body.String())
	}
	got := mergeActions(t, srv, card.ID, lead)
	if !strings.Contains(got, " reopen ") || strings.Contains(got, " wont_fix ") || strings.Contains(got, " instructions ") {
		t.Fatalf("wont_fix lead actions =%s", got)
	}
}

func TestWontFixClosesPR(t *testing.T) {
	srv, gh, card, lead := wontFixServer(t, true)
	if code, body := wontFix(t, srv, card.ID, lead, ""); code != 422 || !strings.Contains(body, `"reason"`) {
		t.Fatalf("empty reason = %d %s", code, body)
	}
	if code, body := wontFix(t, srv, card.ID, lead, strings.Repeat("x", 501)); code != 422 {
		t.Fatalf("long reason = %d %s", code, body)
	}
	if calls := gh.all(); len(calls) != 0 {
		t.Fatalf("gh calls before a valid reason = %+v", calls)
	}
	code, body := wontFix(t, srv, card.ID, lead, "Superseded by the new export")
	if code != 200 || !strings.Contains(body, `"status":"wont_fix"`) {
		t.Fatalf("wont-fix = %d %s", code, body)
	}
	calls := gh.all()
	if len(calls) != 3 {
		t.Fatalf("gh calls = %+v", calls)
	}
	read, comment, patch := calls[0], calls[1], calls[2]
	if read.path != "repos/acme/widget/pulls/7" || read.input != "" {
		t.Fatalf("state read = %+v", read)
	}
	if comment.path != "repos/acme/widget/issues/7/comments" ||
		!strings.Contains(comment.input, "Closed as won't fix by @lead: Superseded by the new export") {
		t.Fatalf("comment = %+v", comment)
	}
	if patch.path != "repos/acme/widget/pulls/7" || !strings.Contains(strings.Join(patch.argv, " "), "--method PATCH") ||
		!strings.Contains(patch.input, `"state":"closed"`) {
		t.Fatalf("close = %+v", patch)
	}
	for _, call := range calls {
		if !contains(call.env, "GH_TOKEN="+leadToken) || strings.Contains(strings.Join(call.argv, " "), leadToken) {
			t.Fatalf("token handling = %+v", call)
		}
	}
	var payload string
	if err := srv.DB.QueryRow(`SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'wont_fix'`, card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"prClosed":true`) || !strings.Contains(payload, "Superseded") || strings.Contains(payload, leadToken) {
		t.Fatalf("wont_fix event = %s", payload)
	}
}

func TestWontFixNoPR(t *testing.T) {
	srv, gh, card, lead := wontFixServer(t, false)
	if _, err := srv.DB.Exec(`UPDATE cards SET pr_url = NULL WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	if code, body := wontFix(t, srv, card.ID, lead, "Not needed"); code != 200 {
		t.Fatalf("wont-fix = %d %s", code, body)
	}
	if status := storedStatus(t, srv, card.ID); status != "wont_fix" {
		t.Fatalf("status = %s", status)
	}
	if calls := gh.all(); len(calls) != 0 {
		t.Fatalf("gh calls = %+v", calls)
	}
}

func TestWontFixNoToken(t *testing.T) {
	srv, gh, card, lead := wontFixServer(t, false)
	code, body := wontFix(t, srv, card.ID, lead, "Not needed")
	if code != 422 || !strings.Contains(body, "to close the pull request") {
		t.Fatalf("wont-fix = %d %s", code, body)
	}
	if status := storedStatus(t, srv, card.ID); status != "blocked" {
		t.Fatalf("status = %s, want blocked", status)
	}
	if calls := gh.all(); len(calls) != 0 {
		t.Fatalf("gh calls = %+v", calls)
	}
}

func TestWontFixClosedPR(t *testing.T) {
	for _, state := range []string{"closed", "merged"} {
		srv, gh, card, lead := wontFixServer(t, true)
		gh.state = state
		if code, body := wontFix(t, srv, card.ID, lead, "Not needed"); code != 200 {
			t.Fatalf("%s: wont-fix = %d %s", state, code, body)
		}
		if calls := gh.all(); len(calls) != 1 || calls[0].input != "" {
			t.Fatalf("%s: gh calls = %+v, want the state read only", state, calls)
		}
	}
}

func TestWontFixRefused(t *testing.T) {
	srv, gh, card, lead := wontFixServer(t, true)
	gh.refuse = true
	code, body := wontFix(t, srv, card.ID, lead, "Not needed")
	if code != 422 || !strings.Contains(body, "Validation Failed") || strings.Contains(body, leadToken) {
		t.Fatalf("wont-fix = %d %s", code, body)
	}
	if status := storedStatus(t, srv, card.ID); status != "blocked" {
		t.Fatalf("status = %s, want blocked", status)
	}
}

func TestReopen(t *testing.T) {
	srv, _, card, lead := wontFixServer(t, true)
	if code, body := wontFix(t, srv, card.ID, lead, "Not needed"); code != 200 {
		t.Fatalf("wont-fix = %d %s", code, body)
	}
	ctx := context.Background()
	leadUser := app.User{ID: emailUserID(t, srv, "lead@example.com"), Role: "lead_developer", Admin: true}
	qa := seedUser(t, srv, "qa@example.com", "password-long-3", "qa_lead", false)
	if err := app.AddMember(ctx, srv.DB, leadUser, "tribe_default", qa.ID); err != nil {
		t.Fatal(err)
	}
	path := "/api/projects/proj_default/cards/" + card.ID + "/reopen"
	if rec := postJSON(t, srv, path, loginToken(t, srv, "qa@example.com", "password-long-3"), map[string]string{}); rec.Code != 403 {
		t.Fatalf("QA lead reopen = %d %s", rec.Code, rec.Body.String())
	}
	rec := postJSON(t, srv, path, lead, map[string]string{})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"blocked"`) {
		t.Fatalf("reopen = %d %s", rec.Code, rec.Body.String())
	}
	var reason string
	if err := srv.DB.QueryRow(`SELECT blocked_reason FROM cards WHERE id = $1`, card.ID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "attempt_cap" {
		t.Fatalf("blocked_reason = %q, want attempt_cap", reason)
	}
	if rec := postJSON(t, srv, path, lead, map[string]string{}); rec.Code != 409 {
		t.Fatalf("second reopen = %d %s", rec.Code, rec.Body.String())
	}
}
