package library_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/store"
)

func moveRepo(t *testing.T) (*library.Repo, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return library.NewRepo(db), db
}

func scalar(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s.String
}

func swapPrefix(from, to string) func(string) (string, bool) {
	return func(p string) (string, bool) {
		if strings.HasPrefix(p, from+"/") {
			return to + p[len(from):], true
		}
		return p, false
	}
}

func TestApplyMoveUpdatesEveryPathOfAMovie(t *testing.T) {
	repo, db := moveRepo(t)
	m, _ := repo.Add(library.Movie{TMDBID: 1, Title: "One", Monitored: true})
	other, _ := repo.Add(library.Movie{TMDBID: 2, Title: "Two", Monitored: true})
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE movies SET file_path = '/movies/One (2000)/One (2000).mkv' WHERE id = ?`, m.ID)
	exec(`UPDATE movies SET file_path = '/movies/Two (2001)/Two (2001).mkv' WHERE id = ?`, other.ID)
	exec(`INSERT INTO subtitle_files (kind, media_id, language, file_id, sub_path, downloaded_at) VALUES ('movie', ?, 'en', 1, '/movies/One (2000)/One (2000).en.srt', '2026-01-01')`, m.ID)
	exec(`INSERT INTO subtitle_files (kind, media_id, language, file_id, sub_path, downloaded_at) VALUES ('movie', ?, 'en', 2, '/movies/Two (2001)/Two (2001).en.srt', '2026-01-01')`, other.ID)
	exec(`INSERT INTO media_info (path, size, mtime, scanned_at) VALUES ('/movies/One (2000)/One (2000).mkv', 5, 6, '2026-01-01')`)

	if err := repo.ApplyMove(library.KindMovie, m.ID, "/movies2", swapPrefix("/movies", "/movies2")); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, db, `SELECT file_path FROM movies WHERE id = ?`, m.ID); got != "/movies2/One (2000)/One (2000).mkv" {
		t.Fatalf("file_path = %q", got)
	}
	if got := scalar(t, db, `SELECT root_path FROM movies WHERE id = ?`, m.ID); got != "/movies2" {
		t.Fatalf("root_path = %q", got)
	}
	if got := scalar(t, db, `SELECT sub_path FROM subtitle_files WHERE media_id = ?`, m.ID); got != "/movies2/One (2000)/One (2000).en.srt" {
		t.Fatalf("sub_path = %q", got)
	}
	if got := scalar(t, db, `SELECT path FROM media_info`); got != "/movies2/One (2000)/One (2000).mkv" {
		t.Fatalf("media_info path = %q", got)
	}
	// The other movie is untouched.
	if got := scalar(t, db, `SELECT file_path FROM movies WHERE id = ?`, other.ID); got != "/movies/Two (2001)/Two (2001).mkv" {
		t.Fatalf("other movie = %q", got)
	}
	if got := scalar(t, db, `SELECT sub_path FROM subtitle_files WHERE media_id = ?`, other.ID); got != "/movies/Two (2001)/Two (2001).en.srt" {
		t.Fatalf("other subtitle = %q", got)
	}
}

func TestApplyMoveShowIsAllOrNothing(t *testing.T) {
	repo, db := moveRepo(t)
	s, err := repo.AddSeries(library.Series{TMDBID: 5, Title: "Show", Monitored: true}, []library.Episode{
		{Season: 1, Episode: 1, Title: "a"}, {Season: 1, Episode: 2, Title: "b"}, {Season: 2, Episode: 1, Title: "c"},
	})
	if err != nil {
		t.Fatal(err)
	}
	eps, _ := repo.ListEpisodes(s.ID)
	for i, e := range eps[:2] {
		p := "/tv/Show/Season 01/Show - S01E0" + string(rune('1'+i)) + ".mkv"
		if _, err := db.Exec(`UPDATE episodes SET file_path = ? WHERE id = ?`, p, e.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO subtitle_files (kind, media_id, language, file_id, sub_path, downloaded_at) VALUES ('episode', ?, 'en', 1, '/tv/Show/Season 01/Show - S01E01.en.srt', '2026-01-01')`, eps[0].ID); err != nil {
		t.Fatal(err)
	}

	// Saving fails at the last step: nothing at all may have changed.
	if _, err := db.Exec(`CREATE TRIGGER no_root BEFORE UPDATE OF root_path ON series BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyMove(library.KindShow, s.ID, "/tv2", swapPrefix("/tv", "/tv2")); err == nil {
		t.Fatal("the failure was swallowed")
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM episodes WHERE file_path LIKE '/tv2/%'`); got != "0" {
		t.Fatalf("%s episodes were changed although saving failed", got)
	}
	if got := scalar(t, db, `SELECT sub_path FROM subtitle_files`); got != "/tv/Show/Season 01/Show - S01E01.en.srt" {
		t.Fatalf("subtitle changed: %q", got)
	}
	if got := scalar(t, db, `SELECT root_path FROM series WHERE id = ?`, s.ID); got != "" {
		t.Fatalf("root_path = %q", got)
	}

	// Without the failure everything moves together.
	if _, err := db.Exec(`DROP TRIGGER no_root`); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyMove(library.KindShow, s.ID, "/tv2", swapPrefix("/tv", "/tv2")); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM episodes WHERE file_path LIKE '/tv2/Show/%'`); got != "2" {
		t.Fatalf("%s episodes moved, want 2", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM episodes WHERE COALESCE(file_path,'') = ''`); got != "1" {
		t.Fatalf("the episode without a file changed: %s", got)
	}
	if got := scalar(t, db, `SELECT sub_path FROM subtitle_files`); got != "/tv2/Show/Season 01/Show - S01E01.en.srt" {
		t.Fatalf("subtitle = %q", got)
	}
	if got := scalar(t, db, `SELECT root_path FROM series WHERE id = ?`, s.ID); got != "/tv2" {
		t.Fatalf("root_path = %q", got)
	}
}
