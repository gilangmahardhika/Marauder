package worker_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/worker"
)

const testProject = "proj_default"

func insertUser(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO users (id, email, password_hash, role, created_at)
		VALUES ($1, $1 || '@example.com', 'x', 'developer', '2026-10-11T00:00:00Z')`, id); err != nil {
		t.Fatal(err)
	}
}

func insertRequirement(t *testing.T, db *sql.DB, project, id, heading string, accepted bool, acceptance, creator string) {
	t.Helper()
	var acceptedAt any
	if accepted {
		acceptedAt = "2026-10-11T00:00:00Z"
	}
	var createdBy any
	if creator != "" {
		createdBy = creator
	}
	if _, err := db.Exec(`
		INSERT INTO requirements (
			id, project_id, source_kind, source_path, heading, statement,
			acceptance_json, non_goals_json, content_hash, accepted_at, created_by
		) VALUES ($1, $2, 'prd', 'docs/prd', $3, 'text', $4, '[]', 'hash', $5, $6)`,
		id, project, heading, acceptance, acceptedAt, createdBy); err != nil {
		t.Fatal(err)
	}
}

func insertProject(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
		VALUES ($1, 'tribe_default', $1, '/tmp/' || $1, '2026-10-11T00:00:00Z')`, id); err != nil {
		t.Fatal(err)
	}
}

func dataDir(t *testing.T) string {
	t.Helper()
	data := t.TempDir()
	dir := filepath.Join(data, "projects", testProject)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("checks:\n  local: go test ./...\n  dev: ./dev-check\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "config.yaml"), []byte("router:\n  budgets:\n    maxAttempts: 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func projectCards(t *testing.T, db *sql.DB, project string) []board.Card {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM cards WHERE project_id = $1 ORDER BY title`, project)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	var cards []board.Card
	for _, id := range ids {
		card, err := board.GetCard(context.Background(), db, id)
		if err != nil {
			t.Fatal(err)
		}
		cards = append(cards, card)
	}
	return cards
}

func TestPassCreatesCardPerRequirement(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	data := dataDir(t)
	insertUser(t, db, "user_ada")
	insertRequirement(t, db, testProject, "REQ-001", "Alpha export", true, `["exports CSV"]`, "user_ada")
	insertRequirement(t, db, testProject, "REQ-002", "", true, `[]`, "")
	insertRequirement(t, db, testProject, "REQ-003", "Gamma", false, `["later"]`, "user_ada")

	result, err := worker.Pass(ctx, db, data, testProject)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 2 {
		t.Fatalf("created = %d, want 2", result.Created)
	}
	cards := projectCards(t, db, testProject)
	if len(cards) != 2 {
		t.Fatalf("cards = %+v", cards)
	}
	byTitle := map[string]board.Card{}
	for _, card := range cards {
		byTitle[card.Title] = card
	}
	alpha, ok := byTitle["Alpha export"]
	if !ok || alpha.Status != "backlog" || alpha.OwnerID != "user_ada" || alpha.RequirementIDs[0] != "REQ-001" {
		t.Fatalf("alpha = %+v", alpha)
	}
	if alpha.LocalCheck != "go test ./..." || alpha.DevCheck != "./dev-check" || alpha.MaxAttempts != 5 {
		t.Fatalf("alpha settings = %+v", alpha)
	}
	if untitled, ok := byTitle["REQ-002"]; !ok || untitled.OwnerID != "" {
		t.Fatalf("REQ-002 card = %+v", byTitle)
	}

	again, err := worker.Pass(ctx, db, data, testProject)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created != 0 {
		t.Fatalf("second pass created %d", again.Created)
	}
}

func TestPassPromotesReady(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	insertRequirement(t, db, testProject, "REQ-001", "With lines", true, `["a line"]`, "")
	insertRequirement(t, db, testProject, "REQ-002", "No lines", true, `[]`, "")
	blocker, err := board.InsertCard(ctx, db, board.Card{Title: "blocker", Status: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := board.InsertCard(ctx, db, board.Card{Title: "ready", Status: "backlog", RequirementIDs: []string{"REQ-001"}})
	if err != nil {
		t.Fatal(err)
	}
	bare, err := board.InsertCard(ctx, db, board.Card{Title: "bare", Status: "backlog", RequirementIDs: []string{"REQ-002"}})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := board.InsertCard(ctx, db, board.Card{
		Title: "waiting", Status: "backlog", RequirementIDs: []string{"REQ-001"}, DependsOn: []string{blocker.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := worker.Pass(ctx, db, "", testProject)
	if err != nil {
		t.Fatal(err)
	}
	if result.Promoted != 1 {
		t.Fatalf("promoted = %d, want 1", result.Promoted)
	}
	for id, want := range map[string]string{ready.ID: "ready", bare.ID: "backlog", waiting.ID: "backlog"} {
		card, err := board.GetCard(ctx, db, id)
		if err != nil {
			t.Fatal(err)
		}
		if card.Status != want {
			t.Fatalf("%s status = %s, want %s", card.Title, card.Status, want)
		}
	}
	var transitions int
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE card_id = $1 AND kind = 'transition'`, ready.ID).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	if transitions != 1 {
		t.Fatalf("transition events = %d", transitions)
	}
}

func TestPassMarksRunsProcessed(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	card, err := board.InsertCard(ctx, db, board.Card{Title: "queued", Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO runs (id, trigger, created_at, project_id) VALUES ('run_project0000', 'accept', '2026-10-11T00:00:00Z', $1);
	`, testProject); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO runs (id, trigger, created_at, project_id, card_id) VALUES ('run_card00000000', 'api', '2026-10-11T00:00:00Z', $1, $2)`,
		testProject, card.ID); err != nil {
		t.Fatal(err)
	}
	if open, err := worker.Pending(ctx, db, testProject); err != nil || !open {
		t.Fatalf("pending = %t %v", open, err)
	}
	if _, err := worker.Pass(ctx, db, "", testProject); err != nil {
		t.Fatal(err)
	}
	var projectDone, cardDone sql.NullString
	if err := db.QueryRow(`SELECT processed_at FROM runs WHERE id = 'run_project0000'`).Scan(&projectDone); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT processed_at FROM runs WHERE id = 'run_card00000000'`).Scan(&cardDone); err != nil {
		t.Fatal(err)
	}
	if !projectDone.Valid || cardDone.Valid {
		t.Fatalf("project processed %v, card processed %v", projectDone, cardDone)
	}
	if open, err := worker.Pending(ctx, db, testProject); err != nil || open {
		t.Fatalf("pending after = %t %v", open, err)
	}
}
