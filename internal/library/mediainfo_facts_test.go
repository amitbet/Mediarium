package library_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/store"
)

// The Library lists read a file's short facts and length from the media_info
// row without touching its details JSON (GitHub issue #52: on a big library
// every list load walked the JSON of every file).
func TestMediaInfoFactsDoNotNeedTheJSON(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := library.NewRepo(db)

	add := func(tmdb int, path string, row library.MediaInfoRow) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO movies (tmdb_id, title, status, file_path) VALUES (?, ?, 'downloaded', ?)`, tmdb, path, path); err != nil {
			t.Fatal(err)
		}
		row.Path = path
		row.ScannedAt = "2026-10-01T00:00:00Z"
		if err := repo.SaveMediaInfo(row); err != nil {
			t.Fatal(err)
		}
	}
	add(1, "/m/short.mkv", library.MediaInfoRow{Info: `{"duration":300.5,"container":"matroska"}`, MediaSummary: library.MediaSummary{VCodec: "HEVC"}})
	add(2, "/m/long.mkv", library.MediaInfoRow{Info: `{"duration":7200}`})
	add(3, "/m/broken.mkv", library.MediaInfoRow{Failed: true, Info: `{"duration":100}`})
	add(4, "/m/nojson.mkv", library.MediaInfoRow{Info: `not json`})

	got, err := repo.ShortFileCandidates(600)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "/m/short.mkv" || got[0].Seconds != 300.5 {
		t.Fatalf("short files = %+v, want only /m/short.mkv at 300.5s", got)
	}
	sum, err := repo.MovieMediaSummaries()
	if err != nil || len(sum) != 3 {
		t.Fatalf("summaries = %v, %v; want the 3 files that could be read", sum, err)
	}

	// The queries are answered from the index alone.
	for _, q := range []string{
		`SELECT m.id, mi.vcodec, mi.hdr, mi.res, mi.acodecs, mi.alangs, mi.slangs FROM movies m JOIN media_info mi ON mi.path = m.file_path WHERE mi.failed = 0 AND m.status = 'downloaded'`,
		`SELECT m.id, mi.duration FROM movies m JOIN media_info mi ON mi.path = m.file_path WHERE mi.failed = 0`,
	} {
		rows, err := db.Query(`EXPLAIN QUERY PLAN ` + q)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(detail + "\n")
		}
		rows.Close()
		if !strings.Contains(plan.String(), "COVERING INDEX media_info_facts") {
			t.Errorf("the plan for %q does not use the covering index:\n%s", q, plan.String())
		}
	}
}
