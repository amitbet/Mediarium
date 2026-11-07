package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/config"
	"github.com/rdborg/mediarium/internal/crypto"
)

// bigLibrary fills a database like the one in GitHub issue #52: thousands of
// movies and shows with their files read (details JSON saved). Real empty
// files are made on disk so the folder and size lookups do real work.
func bigLibrary(t testing.TB, db *sql.DB, root string, movies, shows, perShow int) {
	t.Helper()
	info := `{"container":"matroska","duration":%d,"video":{"codec":"HEVC","width":3840,"height":2160,"bitDepth":10,"hdr":"HDR10"},"audio":[` +
		strings.Repeat(`{"codec":"EAC3","channels":6,"language":"eng","title":"English 5.1 Dolby Digital Plus"},`, 4) +
		`{"codec":"AAC","channels":2,"language":"jpn"}],"subtitles":[` +
		strings.Repeat(`{"language":"eng","codec":"subrip","title":"English forced and full dialogue track"},`, 6) +
		`{"language":"deu","codec":"subrip"}],"padding":"` + strings.Repeat("x", 1500) + `"}`
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	media := func(path string, secs int) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := tx.Exec(`INSERT INTO media_info (path,size,mtime,scanned_at,checked_at,failed,info,container,vcodec,hdr,res,acodecs,alangs,slangs,duration)
			VALUES (?,?,?,?,?,0,?,?,?,?,?,?,?,?,?)`, path, 1000, 1, "2026-10-01T00:00:00Z", "2026-10-01T00:00:00Z",
			fmt.Sprintf(info, secs), "matroska", "HEVC", "HDR10", "2160p", "EAC3,AAC", "eng,jpn", "eng,deu", secs)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= movies; i++ {
		p := filepath.Join(root, "movies", fmt.Sprintf("Movie %d (2020)", i), "movie.mkv")
		media(p, 7000)
		if _, err := tx.Exec(`INSERT INTO movies (tmdb_id,title,year,overview,poster_path,status,quality,file_path) VALUES (?,?,?,?,?,'downloaded','Bluray-2160p',?)`,
			i, fmt.Sprintf("Movie %d", i), 2020, strings.Repeat("A long plot summary. ", 25), "/p.jpg", p); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= shows; i++ {
		res, err := tx.Exec(`INSERT INTO series (tmdb_id,title,year,overview,poster_path) VALUES (?,?,?,?,?)`,
			i, fmt.Sprintf("Show %d", i), 2015, strings.Repeat("A long plot summary. ", 25), "/p.jpg")
		if err != nil {
			t.Fatal(err)
		}
		sid, _ := res.LastInsertId()
		for e := 1; e <= perShow; e++ {
			season := (e-1)/10 + 1
			p := filepath.Join(root, "tv", fmt.Sprintf("Show %d", i), "Season "+strconv.Itoa(season), fmt.Sprintf("S%02dE%02d.mkv", season, e))
			media(p, 2700)
			if _, err := tx.Exec(`INSERT INTO episodes (series_id,season,episode,title,air_date,status,file_path) VALUES (?,?,?,?,'2020-01-01','downloaded',?)`,
				sid, season, e, "Ep", p); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func bigServer(t testing.TB) (*Server, *sql.DB, string) {
	dir := t.TempDir()
	cfg := config.Config{ConfigDir: filepath.Join(dir, "config"), DownloadsDir: filepath.Join(dir, "dl"),
		DownloadsIncomplete: filepath.Join(dir, "dl", "i"), DownloadsComplete: filepath.Join(dir, "dl", "c"), MoviesDir: filepath.Join(dir, "movies")}
	if err := os.MkdirAll(cfg.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := OpenTestDB(filepath.Join(cfg.ConfigDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	box, err := crypto.LoadOrCreateKey(filepath.Join(cfg.ConfigDir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(db, cfg, box, "", "1.2.3-test")
	if err != nil {
		t.Fatal(err)
	}
	return s, db, dir
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

func timedGet(t testing.TB, h http.HandlerFunc, path string) time.Duration {
	start := time.Now()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", path, nil))
	if rec.Code != 200 {
		t.Fatalf("%s: %d %.100s", path, rec.Code, rec.Body.String())
	}
	return time.Since(start)
}

// TestBigLibraryTimings prints how long the Library lists take on a big
// library, with the file-details cache cold (as after every change by the
// background file reader) and warm. Size it with MEDIARIUM_BIG_MOVIES and
// MEDIARIUM_BIG_SHOWS; the defaults are small.
func TestBigLibraryTimings(t *testing.T) {
	movies, shows := envInt("MEDIARIUM_BIG_MOVIES", 300), envInt("MEDIARIUM_BIG_SHOWS", 60)
	s, db, dir := bigServer(t)
	bigLibrary(t, db, dir, movies, shows, 25)
	for _, name := range []string{"first", "warm"} {
		t.Logf("%-12s movies %v  series %v", name, timedGet(t, s.handleListMovies, "/api/movies?brief=1"), timedGet(t, s.handleListSeries, "/api/series?brief=1"))
	}
	s.mediaInfo.gen.Add(1)
	t.Logf("%-12s movies %v  series %v", "gen bumped", timedGet(t, s.handleListMovies, "/api/movies?brief=1"), timedGet(t, s.handleListSeries, "/api/series?brief=1"))
	st := time.Now()
	_, _ = s.MovieRepo.ShortFileCandidates(600)
	t.Logf("ShortFileCandidates %v", time.Since(st))
	st = time.Now()
	_, _ = s.MovieRepo.MovieMediaSummaries()
	_, _ = s.MovieRepo.SeriesMediaSummaries()
	t.Logf("MediaSummaries %v", time.Since(st))
	st = time.Now()
	_, _, _ = s.MovieRepo.MediaFilePaths()
	t.Logf("MediaFilePaths %v", time.Since(st))
}

// BenchmarkBigLibraryLists measures the movie and show lists while the file
// reader is saving details: "reading" saves one file's details before each
// call (what the reader does several times a second), "changed" throws the
// cached lists away first (what every save did before the lists were renewed
// in the background).
func BenchmarkBigLibraryLists(b *testing.B) {
	movies, shows := envInt("MEDIARIUM_BIG_MOVIES", 300), envInt("MEDIARIUM_BIG_SHOWS", 60)
	s, db, dir := bigServer(b)
	bigLibrary(b, db, dir, movies, shows, 25)
	row, _, err := s.MovieRepo.GetMediaInfo(dir + "/movies/Movie 1 (2020)/movie.mkv")
	if err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"reading", "changed"} {
		b.Run(mode, func(b *testing.B) {
			timedGet(b, s.handleListMovies, "/api/movies?brief=1")
			timedGet(b, s.handleListSeries, "/api/series?brief=1")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if mode == "changed" {
					s.mediaInfo.gen.Add(1)
				} else if err := s.MovieRepo.SaveMediaInfo(row); err != nil {
					b.Fatal(err)
				}
				timedGet(b, s.handleListMovies, "/api/movies?brief=1")
				timedGet(b, s.handleListSeries, "/api/series?brief=1")
			}
		})
	}
}
