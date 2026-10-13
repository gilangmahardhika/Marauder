package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/loop"
	"orchestrator/backend/internal/providers/fake"
	"orchestrator/backend/internal/runtime"
	"orchestrator/backend/internal/tenancy"
	"orchestrator/backend/internal/vcs"
)

const (
	ownerToken   = "owner-token-value"
	companyToken = "company-token-value"
	prURL        = "https://github.com/acme/widget/pull/7"
)

var tokenKey = bytes.Repeat([]byte("k"), 32)

func TestIngestRecordsCreator(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	ada := githubUser(t, db, "ada@example.com", "", 0, "")
	grace := githubUser(t, db, "grace@example.com", "", 0, "")
	repo := t.TempDir()
	data := t.TempDir()
	writeFile(t, filepath.Join(repo, "docs", "prd", "one.md"), "### REQ-001 Parse records\n\nThe orchestrator reads markdown.\n")
	if _, err := app.Ingest(ctx, db, "proj_default", ada.ID, repo, data); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "docs", "prd", "one.md"), "### REQ-001 Parse records\n\nThe orchestrator reads markdown.\n\n### REQ-002 Second\n\nAnother.\n")
	if _, err := app.Ingest(ctx, db, "proj_default", grace.ID, repo, data); err != nil {
		t.Fatal(err)
	}
	if got := creatorOf(t, db, "REQ-001"); got != ada.ID {
		t.Fatalf("REQ-001 created_by = %q, want the first ingester", got)
	}
	if got := creatorOf(t, db, "REQ-002"); got != grace.ID {
		t.Fatalf("REQ-002 created_by = %q", got)
	}
	writeFile(t, filepath.Join(repo, "docs", "prd", "one.md"), "### REQ-003 Third\n\nFrom the CLI.\n")
	if _, err := app.Ingest(ctx, db, "proj_default", "", repo, data); err != nil {
		t.Fatal(err)
	}
	if got := creatorOf(t, db, "REQ-003"); got != "" {
		t.Fatalf("CLI ingest created_by = %q", got)
	}
}

func TestCardOwnerFromRequirement(t *testing.T) {
	db := boardtest.Open(t)
	ada := githubUser(t, db, "ada@example.com", "", 0, "")
	mallory := githubUser(t, db, "mallory@example.com", "", 0, "")
	acceptedReq(t, db, "REQ-001", ada.ID)
	card, err := app.CreateCard(context.Background(), loop.CreateInput{
		DB: db,
		Args: map[string]any{
			"title": "Hello", "requirementIds": []any{"REQ-001"},
			"ownerId": mallory.ID, "owner": mallory.ID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := board.GetCard(context.Background(), db, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.OwnerID != ada.ID {
		t.Fatalf("owner_id = %q, want %q", stored.OwnerID, ada.ID)
	}
}

func TestCreateCardMixedOwners(t *testing.T) {
	db := boardtest.Open(t)
	ada := githubUser(t, db, "ada@example.com", "", 0, "")
	grace := githubUser(t, db, "grace@example.com", "", 0, "")
	acceptedReq(t, db, "REQ-001", ada.ID)
	acceptedReq(t, db, "REQ-002", grace.ID)
	_, err := app.CreateCard(context.Background(), loop.CreateInput{
		DB:   db,
		Args: map[string]any{"title": "Both", "requirementIds": []any{"REQ-001", "REQ-002"}},
	})
	if !errors.Is(err, loop.ErrOwners) {
		t.Fatalf("err = %v, want ErrOwners", err)
	}
	if n := countCards(t, db); n != 0 {
		t.Fatalf("cards = %d", n)
	}
}

func TestCommitIdentityOwner(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	repo := openFixture(t)
	data := dataDir(t)
	grace := githubUser(t, db, "grace@example.com", "grace", 4242, ownerToken)
	card, err := board.InsertCard(ctx, db, board.Card{Status: "ready", LocalCheck: "true", OwnerID: grace.ID})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := app.OwnerGitHub(ctx, db, tokenKey, card, "github.com")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := fake.Load([]byte(`{"implement":[{"tool":"edit_file","path":"hello.txt","text":"owner\n"},{"tool":"commit","message":"Owner commit"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RunImplementer(ctx, loop.Input{
		DB: db, Git: runtime.Git{Bin: lookGit(t), Repo: repo, Data: data, Author: owner.Author},
		CardID: card.ID, Model: "fake-model", TaskClass: "implement", MaxTurns: 4,
		Provider: provider, Allow: []string{"true"},
	}); err != nil {
		t.Fatal(err)
	}
	defer cleanupWorktree(t, repo, data, card.ID)
	body, err := os.ReadFile(filepath.Join(data, "worktrees", card.ID, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(body)), "gitdir:"))
	out, err := exec.Command(lookGit(t), "--git-dir="+gitdir, "log", "-1", "--format=%an|%ae|%cn|%ce").Output()
	if err != nil {
		t.Fatal(err)
	}
	want := "grace|4242+grace@users.noreply.github.com|grace|4242+grace@users.noreply.github.com"
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestPushUsesOwnerToken(t *testing.T) {
	f := newPublishFixture(t)
	rec := &recorder{}
	card, err := app.Publish(context.Background(), f.db, f.card.ID, f.input(rec))
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" || card.PRURL != prURL {
		t.Fatalf("card = %s %s", card.Status, card.PRURL)
	}
	calls := rec.all()
	var pushed, created bool
	for _, call := range calls {
		joined := strings.Join(call.Argv, " ")
		if strings.Contains(joined, ownerToken) || strings.Contains(joined, companyToken) {
			t.Fatalf("token in argv: %v", call.Argv)
		}
		if has(call.Env, "GH_TOKEN="+companyToken) {
			t.Fatalf("company token used: %v", call.Argv)
		}
		if !has(call.Env, "GH_TOKEN="+ownerToken) {
			t.Fatalf("call without the owner token: %v", call.Argv)
		}
		if has(call.Argv, "push") {
			pushed = true
			if !has(call.Argv, "https://github.com/acme/widget.git") || !has(call.Argv, "refs/heads/orch/"+f.card.ID+":refs/heads/orch/"+f.card.ID) {
				t.Fatalf("push argv = %v", call.Argv)
			}
		}
		if has(call.Argv, "create") {
			created = true
			if !has(call.Argv, "acme/widget") || !has(call.Argv, "orch/"+f.card.ID) {
				t.Fatalf("pr create argv = %v", call.Argv)
			}
		}
	}
	if !pushed || !created {
		t.Fatalf("pushed=%v created=%v calls=%d", pushed, created, len(calls))
	}
}

func TestPublishEditsExistingPR(t *testing.T) {
	f := newPublishFixture(t)
	if _, err := f.db.Exec(`UPDATE cards SET pr_url = $2 WHERE id = $1`, f.card.ID, prURL); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	card, err := app.Publish(context.Background(), f.db, f.card.ID, f.input(rec))
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" || card.PRURL != prURL {
		t.Fatalf("card = %s %s", card.Status, card.PRURL)
	}
	var patched bool
	for _, call := range rec.all() {
		if has(call.Argv, "pr") && (has(call.Argv, "edit") || has(call.Argv, "create")) {
			t.Fatalf("existing pull request edited through gh pr: %v", call.Argv)
		}
		if has(call.Argv, "repos/acme/widget/pulls/7") && has(call.Argv, "PATCH") && has(call.Argv, "--input") {
			patched = true
		}
	}
	if !patched || len(rec.inputs) != 1 {
		t.Fatalf("patched=%v inputs=%d", patched, len(rec.inputs))
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(rec.inputs[0]), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["title"] != "REQ-001: Export CSV" || !strings.Contains(payload["body"], vcs.BodyStart+"\n## Summary\n\nAdds the export.") {
		t.Fatalf("payload = %v", payload)
	}
}

func TestOwnerGitHubBlocks(t *testing.T) {
	f := newPublishFixture(t)
	if _, err := f.db.Exec(`UPDATE user_github SET verified_at = NULL WHERE user_id = $1`, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	card, err := app.Publish(context.Background(), f.db, f.card.ID, f.input(rec))
	if !errors.Is(err, app.ErrOwnerGitHub) {
		t.Fatalf("err = %v", err)
	}
	if card.Status != "blocked" || card.BlockedReason != "owner_github" {
		t.Fatalf("card = %s %s", card.Status, card.BlockedReason)
	}
	if n := len(rec.all()); n != 0 {
		t.Fatalf("calls = %d, want none", n)
	}
}

func TestRequestReviewers(t *testing.T) {
	f := newPublishFixture(t)
	rec := &recorder{}
	if _, err := app.Publish(context.Background(), f.db, f.card.ID, f.input(rec)); err != nil {
		t.Fatal(err)
	}
	var request *execx.Child
	for _, call := range rec.all() {
		if strings.Contains(strings.Join(call.Argv, " "), "requested_reviewers") {
			c := call
			request = &c
		}
	}
	if request == nil {
		t.Fatal("no reviewer request")
	}
	if !has(request.Argv, "repos/acme/widget/pulls/7/requested_reviewers") || !has(request.Argv, "reviewers[]=hopper") {
		t.Fatalf("request argv = %v", request.Argv)
	}
	for _, unwanted := range []string{"reviewers[]=grace", "reviewers[]=acme-bot", "reviewers[]=unverified"} {
		if has(request.Argv, unwanted) {
			t.Fatalf("request names %s: %v", unwanted, request.Argv)
		}
	}
	var asked string
	if err := f.db.QueryRow(`
		SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'reviewers_requested'`, f.card.ID).Scan(&asked); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked, `"reviewers":["hopper"]`) || !strings.Contains(asked, `"waiting":["u@example.com"]`) {
		t.Fatalf("payload = %s", asked)
	}

	failing := newPublishFixture(t)
	bad := &recorder{fail: "requested_reviewers"}
	card, err := app.Publish(context.Background(), failing.db, failing.card.ID, failing.input(bad))
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" {
		t.Fatalf("status = %s after a failed reviewer request", card.Status)
	}
	var payload string
	if err := failing.db.QueryRow(`
		SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'reviewers_requested'`, card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"ok":false`) || strings.Contains(payload, ownerToken) {
		t.Fatalf("payload = %s", payload)
	}
}

func TestRequestReviewersOnlyWaiting(t *testing.T) {
	f := newPublishFixture(t)
	if _, err := f.db.Exec(`
		DELETE FROM requirement_reviewers
		WHERE user_id IN (SELECT user_id FROM user_github WHERE verified_at IS NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	card, err := app.Publish(context.Background(), f.db, f.card.ID, f.input(rec))
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != "pr_open" {
		t.Fatalf("status = %s", card.Status)
	}
	for _, call := range rec.all() {
		if strings.Contains(strings.Join(call.Argv, " "), "requested_reviewers") {
			t.Fatalf("reviewer request with no verified reviewer: %v", call.Argv)
		}
	}
	var payload string
	if err := f.db.QueryRow(`
		SELECT payload_json FROM events WHERE card_id = $1 AND kind = 'reviewers_requested'`, card.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"reviewers":[]`) || !strings.Contains(payload, `"waiting":["u@example.com"]`) || !strings.Contains(payload, `"ok":true`) {
		t.Fatalf("payload = %s", payload)
	}
}

type publishFixture struct {
	db    *sql.DB
	data  string
	owner identity.User
	card  board.Card
}

func newPublishFixture(t *testing.T) publishFixture {
	t.Helper()
	db := boardtest.Open(t)
	ctx := context.Background()
	data := dataDir(t)
	admin := githubUser(t, db, "admin@example.com", "", 0, "")
	if _, err := db.Exec(`UPDATE users SET admin = true WHERE id = $1`, admin.ID); err != nil {
		t.Fatal(err)
	}
	admin.Admin = true
	token := companyToken
	if _, err := app.SaveCompany(ctx, db, admin, "github.com", "acme", &token, false, tokenKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE workspace_github SET bot_login = 'acme-bot' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := tenancy.PrepareProject(data, "proj_default"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE projects SET github_repo = 'widget', github_base = 'main', repo_path = $1 WHERE id = 'proj_default'`,
		filepath.Join(data, "projects", "proj_default", "repo")); err != nil {
		t.Fatal(err)
	}
	owner := githubUser(t, db, "grace@example.com", "grace", 11, ownerToken)
	reviewer := githubUser(t, db, "hopper@example.com", "hopper", 12, "hopper-token")
	bot := githubUser(t, db, "bot@example.com", "acme-bot", 13, "bot-token-x")
	unverified := githubUser(t, db, "u@example.com", "", 0, "")
	if err := identity.SaveGitHub(ctx, db, unverified.ID, "github.com", "unverified", nil, false, tokenKey); err != nil {
		t.Fatal(err)
	}
	for _, user := range []identity.User{owner, reviewer, bot, unverified} {
		if _, err := db.Exec(`INSERT INTO tribe_members (tribe_id, user_id) VALUES ('tribe_default', $1)`, user.ID); err != nil {
			t.Fatal(err)
		}
	}
	acceptedReq(t, db, "REQ-001", owner.ID)
	for _, user := range []identity.User{owner, reviewer, bot, unverified} {
		if _, err := db.Exec(`
			INSERT INTO requirement_reviewers (project_id, requirement_id, user_id, added_by, created_at)
			VALUES ('proj_default', 'REQ-001', $1, $2, '2026-10-10T00:00:00Z')`, user.ID, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	card, err := board.InsertCard(ctx, db, board.Card{
		ProjectID: "proj_default", OwnerID: owner.ID, Title: "Export CSV",
		Status: "self_check", RequirementIDs: []string{"REQ-001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return publishFixture{db: db, data: data, owner: owner, card: card}
}

func (f publishFixture) input(rec *recorder) app.PublishInput {
	return app.PublishInput{
		Data: f.data, Key: tokenKey,
		Tools: app.Tools{Git: "/usr/bin/git", GH: "/usr/bin/gh", Exec: rec.run},
		Text:  app.PRText{Title: "REQ-001: Export CSV", Body: "## Summary\n\nAdds the export."},
	}
}

// githubUser inserts a user. A login with a token is stored and verified.
func githubUser(t *testing.T, db *sql.DB, email, login string, id int64, token string) identity.User {
	t.Helper()
	ctx := context.Background()
	user, err := identity.Insert(ctx, db, email, "password-long-1", "developer", false)
	if err != nil {
		t.Fatal(err)
	}
	if login == "" {
		return user
	}
	if err := identity.SaveGitHub(ctx, db, user.ID, "github.com", login, &token, false, tokenKey); err != nil {
		t.Fatal(err)
	}
	if err := identity.VerifyGitHub(ctx, db, user.ID, login, id); err != nil {
		t.Fatal(err)
	}
	return user
}

func TestCardSelfReview(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	ada := githubUser(t, db, "ada@example.com", "", 0, "")
	for _, id := range []string{"REQ-001", "REQ-002", "REQ-003"} {
		acceptedReq(t, db, id, ada.ID)
	}
	if _, err := db.Exec(`UPDATE requirements SET self_review = true WHERE id IN ('REQ-001', 'REQ-002')`); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		ids  []string
		want bool
	}{
		"all flagged":     {[]string{"REQ-001", "REQ-002"}, true},
		"mixed":           {[]string{"REQ-001", "REQ-003"}, false},
		"none flagged":    {[]string{"REQ-003"}, false},
		"no requirements": {nil, false},
		"unknown id":      {[]string{"REQ-001", "REQ-404"}, false},
	} {
		card, err := board.InsertCard(ctx, db, board.Card{Status: "human_review", RequirementIDs: tc.ids})
		if err != nil {
			t.Fatal(err)
		}
		got, err := app.CardSelfReview(ctx, db, card)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: self review = %v, want %v", name, got, tc.want)
		}
	}
}

func acceptedReq(t *testing.T, db *sql.DB, id, createdBy string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO requirements (
			id, source_kind, source_path, heading, statement, acceptance_json, non_goals_json,
			content_hash, accepted_at, accepted_by, project_id, created_by
		) VALUES ($1, 'prd', 'docs/prd/a.md', $1, 'statement', '[]', '[]', $1, '2026-01-01T00:00:00Z', 'ada', 'proj_default', $2)`,
		id, createdBy); err != nil {
		t.Fatal(err)
	}
}

func creatorOf(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var creator sql.NullString
	if err := db.QueryRow(`SELECT created_by FROM requirements WHERE id = $1 AND project_id = 'proj_default'`, id).Scan(&creator); err != nil {
		t.Fatal(err)
	}
	return creator.String
}

type recorder struct {
	mu     sync.Mutex
	calls  []execx.Child
	inputs []string
	fail   string
}

func (r *recorder) run(_ context.Context, child execx.Child) (execx.Result, error) {
	r.mu.Lock()
	r.calls = append(r.calls, child)
	for i, arg := range child.Argv {
		if arg == "--input" && i+1 < len(child.Argv) {
			text, _ := os.ReadFile(child.Argv[i+1])
			r.inputs = append(r.inputs, string(text))
		}
	}
	r.mu.Unlock()
	joined := strings.Join(child.Argv, " ")
	if r.fail != "" && strings.Contains(joined, r.fail) {
		return execx.Result{Exit: 1, Out: []byte("HTTP 422: reviewer is not a collaborator\n")}, nil
	}
	if has(child.Argv, "create") {
		return execx.Result{Out: []byte(prURL + "\n")}, nil
	}
	return execx.Result{}, nil
}

func (r *recorder) all() []execx.Child {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]execx.Child(nil), r.calls...)
}

func has(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
