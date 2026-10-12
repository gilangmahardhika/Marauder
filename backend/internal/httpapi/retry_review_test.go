package httpapi_test

import (
	"context"
	"slices"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/httpapi"
)

// failedReviewCard is an agent_review card whose AI review gave no verdict.
func failedReviewCard(t *testing.T, srv *httpapi.Server) board.Card {
	t.Helper()
	card := statusCard(t, srv, "agent_review")
	if err := board.InsertAttempt(context.Background(), srv.DB, board.Attempt{CardID: card.ID, N: 1, TaskClass: "implement"}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`
		UPDATE attempts SET outcome = 'checks_passed', commit_sha = $2, review_verdict = 'failed', reviewed_sha = $2
		WHERE card_id = $1`, card.ID, "0123456789abcdef0123456789abcdef01234567"); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(`UPDATE cards SET pr_url = 'https://github.com/acme/widget/pull/7' WHERE id = $1`, card.ID); err != nil {
		t.Fatal(err)
	}
	return card
}

func TestRetryReviewAction(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "developer")
	card := failedReviewCard(t, srv)
	path := "/api/projects/proj_default/cards/" + card.ID

	if got := decodeRunCard(t, getAuth(t, srv, path, token).Body.Bytes()); !slices.Contains(got.Actions, "retry_review") {
		t.Fatalf("failed review offers no retry: %+v", got)
	}
	if rec := getAuth(t, srv, path+"/retry-review", token); rec.Code != 405 {
		t.Fatalf("GET retry-review = %d %s, want 405", rec.Code, rec.Body.String())
	}
	rec := postJSON(t, srv, path+"/retry-review", token, map[string]any{})
	if rec.Code != 200 {
		t.Fatalf("retry-review = %d %s", rec.Code, rec.Body.String())
	}
	if got := decodeRunCard(t, rec.Body.Bytes()); got.Status != "pr_open" || slices.Contains(got.Actions, "retry_review") {
		t.Fatalf("after retry = %+v", got)
	}
	if rec := postJSON(t, srv, path+"/retry-review", token, map[string]any{}); rec.Code != 409 {
		t.Fatalf("second retry-review = %d %s, want 409", rec.Code, rec.Body.String())
	}
}

func TestRetryReviewRole(t *testing.T) {
	srv := accountServer(t)
	token := runMember(t, srv, "product_owner")
	card := failedReviewCard(t, srv)
	path := "/api/projects/proj_default/cards/" + card.ID

	if got := decodeRunCard(t, getAuth(t, srv, path, token).Body.Bytes()); slices.Contains(got.Actions, "retry_review") {
		t.Fatalf("product owner sees retry_review: %+v", got)
	}
	if rec := postJSON(t, srv, path+"/retry-review", token, map[string]any{}); rec.Code != 403 {
		t.Fatalf("retry-review = %d %s, want 403", rec.Code, rec.Body.String())
	}
	if got, _ := board.GetCard(context.Background(), srv.DB, card.ID); got.Status != "agent_review" {
		t.Fatalf("card = %s, want agent_review", got.Status)
	}
}
