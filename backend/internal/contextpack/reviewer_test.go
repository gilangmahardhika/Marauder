package contextpack_test

import (
	"os"
	"strings"
	"testing"

	"orchestrator/backend/internal/contextpack"
)

func TestReviewerPack(t *testing.T) {
	raw, err := os.ReadFile("testdata/reviewer.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "diff --git") || strings.Contains(string(raw), "author-transcript") {
		t.Fatalf("fixture = %s", raw)
	}
	_, user, _, err := contextpack.BuildReviewer(
		[]string{"The orchestrator reads markdown."},
		"diff --git a/hello.txt b/hello.txt\n+ok\n",
		"exit 0",
		"pull request",
		"author-transcript should be omitted",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(user, "diff --git") || strings.Contains(user, "author-transcript") {
		t.Fatalf("user = %s", user)
	}
}
