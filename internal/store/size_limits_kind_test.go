package store_test

import (
	"testing"

	migrations "github.com/rdborg/mediarium/db"
)

// Migration 0218 copies the saved size table to the show table once, so
// nobody's checks change, and never overwrites a show table that exists.
func TestSizeLimitsPerKindMigration(t *testing.T) {
	sqlBytes, err := migrations.MigrationsFS.ReadFile("migrations/0218_size_limits_per_kind.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	const table = `{"enabled":true,"limits":{"WEBDL-1080p":{"min":5,"max":90}}}`
	cases := []struct {
		name string
		seed []string
		want string // the show table afterwards, "" = unset
	}{
		{"fresh install", nil, ""},
		{"a saved table is copied", []string{`INSERT INTO settings (key, value, encrypted) VALUES ('quality.size_limits', '` + table + `', 0)`}, table},
		{"a show table that exists stays",
			[]string{
				`INSERT INTO settings (key, value, encrypted) VALUES ('quality.size_limits', '` + table + `', 0)`,
				`INSERT INTO settings (key, value, encrypted) VALUES ('quality.size_limits_tv', '{"enabled":false}', 0)`,
			}, `{"enabled":false}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTemp(t)
			if _, err := db.Exec(`DELETE FROM settings WHERE key IN ('quality.size_limits', 'quality.size_limits_tv')`); err != nil {
				t.Fatal(err)
			}
			for _, stmt := range tc.seed {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			if _, err := db.Exec(string(sqlBytes)); err != nil {
				t.Fatalf("run migration: %v", err)
			}
			var got string
			if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'quality.size_limits_tv'`).Scan(&got); err != nil && tc.want != "" {
				t.Fatalf("read the show table: %v", err)
			}
			if got != tc.want {
				t.Fatalf("show table = %q, want %q", got, tc.want)
			}
			// The movie table is never touched.
			var movies string
			_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'quality.size_limits'`).Scan(&movies)
			if len(tc.seed) > 0 && movies != table {
				t.Fatalf("movie table = %q", movies)
			}
		})
	}
}
