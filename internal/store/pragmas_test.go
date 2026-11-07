package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/store"
)

func pragmaInt(t *testing.T, c *sql.Conn, name string) int {
	t.Helper()
	var v int
	if err := c.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&v); err != nil {
		t.Fatalf("PRAGMA %s: %v", name, err)
	}
	return v
}

// Every connection of the pool gets the same settings: the write-ahead log,
// the 10 second wait for a busy database and the foreign keys stay as they
// were, and the page cache is bigger than SQLite's 2 MB default.
func TestEveryConnectionHasTheSameSettings(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var conns []*sql.Conn
	for i := 0; i < 4; i++ {
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		defer c.Close()
	}
	for i, c := range conns {
		if got := pragmaInt(t, c, "busy_timeout"); got != 10000 {
			t.Errorf("conn %d busy_timeout = %d, want 10000", i, got)
		}
		if got := pragmaInt(t, c, "foreign_keys"); got != 1 {
			t.Errorf("conn %d foreign_keys = %d, want 1", i, got)
		}
		if got := pragmaInt(t, c, "synchronous"); got != 1 { // NORMAL
			t.Errorf("conn %d synchronous = %d, want 1 (NORMAL)", i, got)
		}
		if got := pragmaInt(t, c, "cache_size"); got != -8000 {
			t.Errorf("conn %d cache_size = %d, want -8000 (8 MB)", i, got)
		}
		var mode string
		if err := c.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
			t.Errorf("conn %d journal_mode = %q (%v), want wal", i, mode, err)
		}
	}
	if m := store.Mode(db); m.JournalMode != store.ModeWAL {
		t.Errorf("Mode = %+v, want WAL", m)
	}
}
