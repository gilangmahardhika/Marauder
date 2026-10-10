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

		result, err := app.Ingest(context.Background(), db, "proj_default", repo, data)
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

		result, err := app.Ingest(context.Background(), db, "proj_default", repo, data)
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

		result, err = app.Ingest(context.Background(), db, "proj_default", repo, data)
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
		if _, err := app.Ingest(context.Background(), db, "proj_default", repo, data); err != nil {
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
