package app_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchestrator/backend/internal/app"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/intake"
)

func TestDraftIds(t *testing.T) {
	t.Run("no requirement headings", func(t *testing.T) {
		db := boardtest.Open(t)
		repo := t.TempDir()
		data := t.TempDir()
		writeFile(t, filepath.Join(repo, "docs", "prd", "draft.md"), "# Notes\n\nThis file has no requirement ids.\n")
		writeFile(t, filepath.Join(repo, "docs", "extra", "note.md"), "### REQ-999 Outside\n\nNot ingested.\n")

		result, err := app.Ingest(context.Background(), db, "proj_default", "", repo, data)
		if err != nil {
			t.Fatal(err)
		}
		if !result.DraftPending {
			t.Fatal("draftPending = false")
		}
		if accepted := countAccepted(t, db); accepted != 0 {
			t.Fatalf("accepted requirements = %d", accepted)
		}
		if cards := countCards(t, db); cards != 0 {
			t.Fatalf("cards = %d", cards)
		}
		if hasRequirement(t, db, "REQ-999") {
			t.Fatal("stored REQ-999 from outside the configured path")
		}
	})

	t.Run("keeps ids and clears a changed hash", func(t *testing.T) {
		db := boardtest.Open(t)
		repo := t.TempDir()
		data := t.TempDir()
		prd := filepath.Join(repo, "docs", "prd", "one.md")
		rfc := filepath.Join(repo, "docs", "rfc", "one.md")
		writeFile(t, prd, "### REQ-001 Parse records\n\nThe orchestrator reads markdown.\n\n#### Acceptance\n\n- A fixture PRD yields one record per id.\n")
		writeFile(t, rfc, "## RFC-001 Keep the id\n\nExisting ids stay as written.\n")
		writeFile(t, filepath.Join(data, "config.yaml"), "operatorLogin: ada\n")

		result, err := app.Ingest(context.Background(), db, "proj_default", "", repo, data)
		if err != nil {
			t.Fatal(err)
		}
		if result.DraftPending {
			t.Fatal("draftPending = true")
		}
		if result.Requirements != 1 || result.Decisions != 1 {
			t.Fatalf("ingest = %+v", result)
		}
		var acceptance string
		if err := db.QueryRow(`SELECT acceptance_json FROM requirements WHERE id = 'REQ-001'`).Scan(&acceptance); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(acceptance, "A fixture PRD yields one record per id.") {
			t.Fatalf("acceptance = %s", acceptance)
		}
		if err := app.Accept(context.Background(), db, "proj_default", data, []string{"REQ-404"}); err != intake.ErrUnknownID {
			t.Fatalf("unknown accept err = %v", err)
		}
		if runs := countRuns(t, db); runs != 0 {
			t.Fatalf("runs after unknown accept = %d", runs)
		}
		if err := app.Accept(context.Background(), db, "proj_default", data, []string{"REQ-001"}); err != nil {
			t.Fatal(err)
		}
		if !requirementAccepted(t, db, "REQ-001", "ada") {
			t.Fatal("REQ-001 was not accepted")
		}

		result, err = app.Ingest(context.Background(), db, "proj_default", "", repo, data)
		if err != nil {
			t.Fatal(err)
		}
		if result.Decisions != 1 {
			t.Fatalf("decisions = %d", result.Decisions)
		}
		if !requirementAccepted(t, db, "REQ-001", "ada") {
			t.Fatal("unchanged ingest cleared acceptance")
		}
		if decisions := countDecisions(t, db); decisions != 1 {
			t.Fatalf("decision rows = %d", decisions)
		}

		writeFile(t, prd, "### REQ-001 Parse records\n\nThe statement changed.\n\n#### Acceptance\n\n- A fixture PRD yields one record per id.\n")
		if _, err := app.Ingest(context.Background(), db, "proj_default", "", repo, data); err != nil {
			t.Fatal(err)
		}
		if requirementAccepted(t, db, "REQ-001", "ada") {
			t.Fatal("changed statement stayed accepted")
		}
		if decisions := countDecisions(t, db); decisions != 1 {
			t.Fatalf("decision rows = %d", decisions)
		}
		if cards := countCards(t, db); cards != 0 {
			t.Fatalf("cards = %d", cards)
		}
	})
}

func TestIngestKeepsManual(t *testing.T) {
	db := boardtest.Open(t)
	repo := t.TempDir()
	data := t.TempDir()
	ctx := context.Background()
	if _, err := intake.Add(ctx, db, "proj_default", "user_0123456789ab", "REQ-001", intake.Draft{Statement: "Typed by hand.", Acceptance: []string{"Kept."}}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "docs", "prd", "one.md"), "### REQ-001 From the repo\n\nThe repo text.\n\n### REQ-002 Second\n\nAnother.\n")

	result, err := app.Ingest(ctx, db, "proj_default", "", repo, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "REQ-001" {
		t.Fatalf("skipped = %v", result.Skipped)
	}
	var kind, statement string
	if err := db.QueryRow(`SELECT source_kind, statement FROM requirements WHERE project_id = 'proj_default' AND id = 'REQ-001'`).Scan(&kind, &statement); err != nil {
		t.Fatal(err)
	}
	if kind != "manual" || statement != "Typed by hand." {
		t.Fatalf("REQ-001 = %s %q", kind, statement)
	}
	if !hasRequirement(t, db, "REQ-002") {
		t.Fatal("REQ-002 was not ingested")
	}
}

func TestIngestMentions(t *testing.T) {
	db := boardtest.Open(t)
	repo := t.TempDir()
	data := t.TempDir()
	ctx := context.Background()
	prd := filepath.Join(repo, "docs", "prd", "one.md")
	writeFile(t, filepath.Join(data, "config.yaml"), "operatorLogin: ada\n")
	writeFile(t, prd, "### REQ-001 Tickets\n\nLook up the ticket with @tickets.\n\n#### Acceptance\n\n- The docs come from @docs.\n- Mail ada@example.com.\n")
	if _, err := app.Ingest(ctx, db, "proj_default", "", repo, data); err != nil {
		t.Fatal(err)
	}
	if got := storedMentions(t, db, "REQ-001"); got != `["tickets","docs"]` {
		t.Fatalf("mentions = %s", got)
	}
	if err := app.Accept(ctx, db, "proj_default", data, []string{"REQ-001"}); err != nil {
		t.Fatal(err)
	}

	writeFile(t, prd, "### REQ-001 Tickets\n\nLook up the ticket with `@tickets`.\n\n#### Acceptance\n\n- The docs come from @docs.\n- Mail ada@example.com.\n")
	if _, err := app.Ingest(ctx, db, "proj_default", "", repo, data); err != nil {
		t.Fatal(err)
	}
	if got := storedMentions(t, db, "REQ-001"); got != `["docs"]` {
		t.Fatalf("mentions after backticks = %s", got)
	}
	if requirementAccepted(t, db, "REQ-001", "ada") {
		t.Fatal("wrapping a mention in backticks kept acceptance")
	}
}

func storedMentions(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT mentions_json FROM requirements WHERE project_id = 'proj_default' AND id = $1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func countAccepted(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM requirements WHERE accepted_at IS NOT NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countCards(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM cards`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countRuns(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM runs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countDecisions(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM decisions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func hasRequirement(t *testing.T, db *sql.DB, id string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM requirements WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func requirementAccepted(t *testing.T, db *sql.DB, id, operator string) bool {
	t.Helper()
	var by sql.NullString
	var at sql.NullString
	err := db.QueryRow(`SELECT accepted_at, accepted_by FROM requirements WHERE id = $1`, id).Scan(&at, &by)
	if err != nil {
		t.Fatal(err)
	}
	return at.Valid && by.Valid && by.String == operator
}
