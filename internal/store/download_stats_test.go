package store_test

import (
	"database/sql"
	"testing"

	migrations "github.com/rdborg/mediarium/db"
)

type statTotals struct{ grabs, completed, failed, bytes int64 }

func statSums(t *testing.T, db *sql.DB, where string) statTotals {
	t.Helper()
	var s statTotals
	if err := db.QueryRow(`SELECT COALESCE(SUM(grabs), 0), COALESCE(SUM(completed), 0), COALESCE(SUM(failed), 0), COALESCE(SUM(bytes), 0) FROM download_stats `+where).
		Scan(&s.grabs, &s.completed, &s.failed, &s.bytes); err != nil {
		t.Fatal(err)
	}
	return s
}

// The download totals are counted as downloads happen, by day, kind and
// protocol, and nothing that takes a download off the list touches them.
func TestDownloadStatsAreCountedAndSurviveClearing(t *testing.T) {
	db := openTemp(t)
	mustExec(t, db, `INSERT INTO movies (id, tmdb_id, title) VALUES (1, 1, 'A Movie')`)

	mustExec(t, db, `INSERT INTO download_queue (movie_id, release_title, size_bytes, protocol, added_at) VALUES (1, 'a', 1000, 'usenet', '2026-10-01T10:00:00.000Z')`)
	mustExec(t, db, `INSERT INTO download_queue (movie_id, release_title, size_bytes, protocol, added_at) VALUES (1, 'b', 500, 'torrent', '2026-10-01T11:00:00.000Z')`)
	mustExec(t, db, `INSERT INTO download_queue (release_title, size_bytes, protocol, added_at) VALUES ('c', 200, 'usenet', '2026-10-02T11:00:00.000Z')`)
	if got := statSums(t, db, ""); got != (statTotals{grabs: 3}) {
		t.Fatalf("after three grabs: %+v", got)
	}

	mustExec(t, db, `UPDATE download_queue SET status = 'completed', completed_at = '2026-10-03T08:00:00.000Z' WHERE release_title = 'a'`)
	mustExec(t, db, `UPDATE download_queue SET status = 'failed', completed_at = '2026-10-03T09:00:00.000Z' WHERE release_title = 'b'`)
	// The same status written again, and a download stopped, count for nothing.
	mustExec(t, db, `UPDATE download_queue SET status = 'completed' WHERE release_title = 'a'`)
	mustExec(t, db, `UPDATE download_queue SET status = 'stopped' WHERE release_title = 'c'`)
	want := statTotals{grabs: 3, completed: 1, failed: 1, bytes: 1000}
	if got := statSums(t, db, ""); got != want {
		t.Fatalf("after finishing: %+v, want %+v", got, want)
	}
	if got := statSums(t, db, `WHERE day = '2026-10-03' AND kind = 'movie' AND protocol = 'usenet'`); got != (statTotals{completed: 1, bytes: 1000}) {
		t.Errorf("the usenet movie on the 3rd: %+v", got)
	}
	if got := statSums(t, db, `WHERE kind = 'other'`); got.grabs != 1 {
		t.Errorf("a grab for no title is kind other: %+v", got)
	}

	// Clearing the list, and deleting the movie, leave every total alone.
	mustExec(t, db, `DELETE FROM download_queue WHERE status IN ('completed', 'failed', 'stopped')`)
	mustExec(t, db, `DELETE FROM movies WHERE id = 1`)
	mustExec(t, db, `DELETE FROM download_queue`)
	if got := statSums(t, db, ""); got != want {
		t.Fatalf("after clearing: %+v, want %+v", got, want)
	}
}

// Migration 0223 fills the table once from the downloads on the list (and from
// the daily snapshots for days the list no longer explains), and running it
// again changes nothing.
func TestDownloadStatsMigrationBackfillsOnce(t *testing.T) {
	sqlBytes, err := migrations.MigrationsFS.ReadFile("migrations/0223_download_stats.sql")
	if err != nil {
		t.Fatal(err)
	}
	db := openTemp(t)
	// Start from an install that has not had the migration: no table, no triggers.
	for _, stmt := range []string{
		`DROP TRIGGER download_stats_grab`, `DROP TRIGGER download_stats_completed`, `DROP TRIGGER download_stats_failed`, `DROP TABLE download_stats`,
	} {
		mustExec(t, db, stmt)
	}
	mustExec(t, db, `INSERT INTO download_queue (release_title, size_bytes, protocol, status, added_at, completed_at) VALUES ('a', 1000, 'usenet', 'completed', '2026-10-01T10:00:00.000Z', '2026-10-01T10:30:00.000Z')`)
	mustExec(t, db, `INSERT INTO download_queue (release_title, size_bytes, protocol, status, added_at, completed_at) VALUES ('b', 400, 'torrent', 'completed', '2026-10-01T11:00:00.000Z', '2026-10-02T01:00:00.000Z')`)
	mustExec(t, db, `INSERT INTO download_queue (release_title, size_bytes, protocol, status, added_at, completed_at) VALUES ('c', 0, 'usenet', 'failed', '2026-10-02T11:00:00.000Z', '2026-10-02T11:05:00.000Z')`)
	mustExec(t, db, `INSERT INTO download_queue (release_title, size_bytes, protocol, status, added_at) VALUES ('d', 0, 'usenet', 'downloading', '2026-10-02T12:00:00.000Z')`)
	// A day whose downloads were cleared long ago: only the snapshot remembers.
	mustExec(t, db, `INSERT INTO stats_daily (day, completed, failed, download_bytes) VALUES ('2026-09-15', 5, 2, 9000)`)
	// A snapshot that agrees with the list adds nothing.
	mustExec(t, db, `INSERT INTO stats_daily (day, completed, failed, download_bytes) VALUES ('2026-10-01', 1, 0, 1000)`)

	run := func() {
		t.Helper()
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("run migration: %v", err)
		}
	}
	run()
	want := statTotals{grabs: 4, completed: 2 + 5, failed: 1 + 2, bytes: 1400 + 9000}
	if got := statSums(t, db, ""); got != want {
		t.Fatalf("after the backfill: %+v, want %+v", got, want)
	}
	if got := statSums(t, db, `WHERE day = '2026-09-15' AND kind = 'other' AND protocol = 'unknown'`); got != (statTotals{completed: 5, failed: 2, bytes: 9000}) {
		t.Errorf("the cleared day: %+v", got)
	}
	if got := statSums(t, db, `WHERE protocol = 'torrent'`); got != (statTotals{grabs: 1, completed: 1, bytes: 400}) {
		t.Errorf("torrents: %+v", got)
	}

	// Running it again, with more happening in between, changes nothing it
	// should not: the table is not empty, so nothing is filled twice.
	mustExec(t, db, `UPDATE download_queue SET status = 'completed', completed_at = '2026-10-03T00:00:00.000Z', size_bytes = 50 WHERE release_title = 'd'`)
	run()
	run()
	want.completed++
	want.bytes += 50
	if got := statSums(t, db, ""); got != want {
		t.Fatalf("after running it again: %+v, want %+v", got, want)
	}
}
