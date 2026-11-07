package store_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/store"
)

// Every indexer an install already has starts without extra categories, so it
// is searched as before.
func TestIndexerExtraCategoriesMigration(t *testing.T) {
	before := 0
	for i, n := range migrationNames(t) {
		if strings.HasPrefix(n, "0233_") {
			before = i
		}
	}
	if before == 0 {
		t.Fatal("migration 0233 not found")
	}
	path := oldInstall(t, before, func(db *sql.DB) {
		mustExec(t, db, `INSERT INTO indexers (name, definition_id, base_url, protocol) VALUES ('Old', 'newznab', 'https://a', 'usenet')`)
	})
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer db.Close()
	var s string
	if err := db.QueryRow(`SELECT extra_categories FROM indexers WHERE name = 'Old'`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	if s != "" {
		t.Errorf("extra_categories = %q, want empty", s)
	}
	mustExec(t, db, `INSERT INTO indexers (name, definition_id, base_url, protocol) VALUES ('New', 'newznab', 'https://b', 'usenet')`)
	if err := db.QueryRow(`SELECT extra_categories FROM indexers WHERE name = 'New'`).Scan(&s); err != nil || s != "" {
		t.Errorf("a new row defaults to empty, got %q, %v", s, err)
	}
}
