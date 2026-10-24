package store_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/store"
)

// Migration 0227 turns every saved 0 seeding limit into its old meaning (no
// limit), now written as empty, so nobody's torrents change behaviour when 0
// starts to mean "do not seed".
func TestSeedLimitsZeroMigration(t *testing.T) {
	before := 0
	for i, n := range migrationNames(t) {
		if strings.HasPrefix(n, "0227_") {
			before = i
		}
	}
	if before == 0 {
		t.Fatal("migration 0227 not found")
	}
	f := func(v float64) *float64 { return &v }
	type rule struct{ ratio, hours *float64 }
	cases := []struct {
		name                         string
		ratio, hours, upload         string // saved settings, "-" = not saved
		rule                         rule
		wantRatio, wantHours, wantUp string // "-" = not saved
		wantRule                     *rule  // nil = no rule left
	}{
		{"zeros become empty", "0", "0", "0", rule{}, "", "", "", nil},
		{"zero with decimals", "0.0", "0,00", "00", rule{}, "", "", "", nil},
		{"real numbers stay", "2.5", "48", "300", rule{}, "2.5", "48", "300", nil},
		{"not saved stays not saved", "-", "-", "-", rule{}, "-", "-", "-", nil},
		{"empty stays empty", "", "", "", rule{}, "", "", "", nil},
		{"text that is no number is left alone", "abc", "-", "-", rule{}, "abc", "-", "-", nil},
		{"a rule zero under a global limit says no limit here", "2", "48", "-", rule{f(0), f(0)}, "2", "48", "-", &rule{f(-1), f(-1)}},
		{"a rule zero under no global limit follows it", "0", "0", "-", rule{f(0), f(0)}, "", "", "-", nil},
		{"only the zero field of a rule changes", "2", "-", "-", rule{f(0), f(12)}, "2", "-", "-", &rule{f(-1), f(12)}},
		{"a rule with real numbers stays", "2", "48", "-", rule{f(1.5), nil}, "2", "48", "-", &rule{f(1.5), nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := oldInstall(t, before, func(db *sql.DB) {
				for k, v := range map[string]string{"torrent.seed_ratio_limit": tc.ratio, "torrent.seed_time_limit_h": tc.hours, "torrent.upload_limit_kb": tc.upload} {
					if v != "-" {
						mustExec(t, db, `INSERT INTO settings (key, value, encrypted) VALUES (?, ?, 0)`, k, v)
					}
				}
				mustExec(t, db, `INSERT INTO indexers (name, definition_id, base_url, protocol) VALUES ('T', 'torznab', 'https://a', 'torrent')`)
				if tc.rule.ratio != nil || tc.rule.hours != nil {
					mustExec(t, db, `INSERT INTO indexer_seed_rules (indexer_id, ratio, hours) VALUES (1, ?, ?)`, tc.rule.ratio, tc.rule.hours)
				}
			})
			db, err := store.Open(path)
			if err != nil {
				t.Fatalf("upgrade: %v", err)
			}
			defer db.Close()
			for k, want := range map[string]string{"torrent.seed_ratio_limit": tc.wantRatio, "torrent.seed_time_limit_h": tc.wantHours, "torrent.upload_limit_kb": tc.wantUp} {
				var got string
				err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, k).Scan(&got)
				switch {
				case want == "-" && err == sql.ErrNoRows:
				case err != nil:
					t.Errorf("%s: %v (want %q)", k, err, want)
				case got != want:
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
			var ratio, hours sql.NullFloat64
			err = db.QueryRow(`SELECT ratio, hours FROM indexer_seed_rules WHERE indexer_id = 1`).Scan(&ratio, &hours)
			if tc.wantRule == nil {
				if err != sql.ErrNoRows {
					t.Errorf("the rule should be gone, got %v %v (%v)", ratio, hours, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("read the rule: %v", err)
			}
			same := func(got sql.NullFloat64, want *float64) bool {
				return got.Valid == (want != nil) && (want == nil || got.Float64 == *want)
			}
			if !same(ratio, tc.wantRule.ratio) || !same(hours, tc.wantRule.hours) {
				t.Errorf("rule = %v %v, want %v %v", ratio, hours, deref(tc.wantRule.ratio), deref(tc.wantRule.hours))
			}
		})
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
