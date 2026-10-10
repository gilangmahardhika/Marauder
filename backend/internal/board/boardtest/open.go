package boardtest

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"

	"orchestrator/backend/internal/board"
	"orchestrator/backend/internal/envfile"
)

func Open(t *testing.T) *sql.DB {
	t.Helper()
	if err := envfile.Load(); err != nil {
		t.Fatal(err)
	}
	url := os.Getenv("ORCHESTRATOR_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("ORCHESTRATOR_TEST_DATABASE_URL is required")
	}

	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatal(err)
	}
	schema := "s" + hex.EncodeToString(buf[:])

	admin, err := board.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}

	var db *sql.DB
	t.Cleanup(func() {
		if db != nil {
			db.Close()
		}
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
	})

	db, err = board.OpenInSchema(url, schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := board.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}
