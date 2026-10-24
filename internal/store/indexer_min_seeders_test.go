package store_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/store"
)

// Every indexer an install already has starts with the minimum off.
func TestIndexerMinSeedersMigration(t *testing.T) {
	before := 0
	for i, n := range migrationNames(t) {
		if strings.HasPrefix(n, "0217_") {
			before = i
		}
	}
	if before == 0 {
		t.Fatal("migration 0217 not found")
	}
	path := oldInstall(t, before, func(db *sql.DB) {
		mustExec(t, db, `INSERT INTO indexers (name, definition_id, base_url, protocol) VALUES ('Old', 'torznab', 'https://a', 'torrent')`)
	})
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT min_seeders FROM indexers WHERE name = 'Old'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("min_seeders = %d, want 0 (off)", n)
	}
	mustExec(t, db, `INSERT INTO indexers (name, definition_id, base_url, protocol) VALUES ('New', 'torznab', 'https://b', 'torrent')`)
	if err := db.QueryRow(`SELECT min_seeders FROM indexers WHERE name = 'New'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("a new row defaults to 0, got %d, %v", n, err)
	}
}
