//go:build !windows

package api_test

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Opt-in: runs the real move code between two real drives. Set
// MEDIARIUM_TWO_DRIVES_A and MEDIARIUM_TWO_DRIVES_B to two folders on
// DIFFERENT filesystems (for example two tmpfs mounts). The rename then really
// fails with EXDEV and the copy, read-back check and delete run for real.

func twoDrives(t *testing.T) (a, b string) {
	t.Helper()
	pa, pb := os.Getenv("MEDIARIUM_TWO_DRIVES_A"), os.Getenv("MEDIARIUM_TWO_DRIVES_B")
	if pa == "" || pb == "" {
		t.Skip("set MEDIARIUM_TWO_DRIVES_A and MEDIARIUM_TWO_DRIVES_B to two folders on different filesystems")
	}
	dev := func(p string) uint64 {
		var st syscall.Stat_t
		if err := syscall.Stat(p, &st); err != nil {
			t.Fatal(err)
		}
		return uint64(st.Dev)
	}
	if dev(pa) == dev(pb) {
		t.Skip("the two folders are on the same filesystem")
	}
	mk := func(p string) string {
		d, err := os.MkdirTemp(p, "mediarium-two-drives-*")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(d) })
		return d
	}
	return mk(pa), mk(pb)
}

// bigFile writes n bytes of repeatable noise and returns its checksum.
func bigFile(t *testing.T, path string, n int, seed int64) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(buf)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(buf))
}

func sum(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// driveLibrary is the move library with the main folders on drive A and the
// extra folders on drive B, and big files in the titles.
type driveLibrary struct {
	*moveLibrary
	aMovies, aTV, bMovies, bTV string
	movieDir, seed             string
	files                      map[string]string // path -> checksum
}

func newDriveLibrary(t *testing.T) *driveLibrary {
	t.Helper()
	a, b := twoDrives(t)
	d := &driveLibrary{moveLibrary: newMoveLibrary(t), files: map[string]string{}}
	d.aMovies, d.aTV = filepath.Join(a, "movies"), filepath.Join(a, "tv")
	d.bMovies, d.bTV = filepath.Join(b, "movies2"), filepath.Join(b, "tv2")
	for _, p := range []string{d.aMovies, d.aTV, d.bMovies, d.bTV, filepath.Join(a, "downloads")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	putJSONStatus(t, d.client, d.base+"/api/settings", map[string]any{
		"moviesPath": d.aMovies, "tvPath": d.aTV,
		"moviesExtraPaths": []string{d.bMovies}, "tvExtraPaths": []string{d.bTV},
	}, http.StatusOK)

	// The movie, with sidecars, an extra and a hard link to a "seeding" file.
	d.movieDir = filepath.Join(d.aMovies, "The Matrix (1999)")
	video := filepath.Join(d.movieDir, "The Matrix (1999).mkv")
	d.files[video] = bigFile(t, video, 3<<20, 1)
	sub := filepath.Join(d.movieDir, "The Matrix (1999).en.srt")
	d.files[sub] = bigFile(t, sub, 20<<10, 2)
	nfo := filepath.Join(d.movieDir, "movie.nfo")
	d.files[nfo] = bigFile(t, nfo, 4<<10, 3)
	poster := filepath.Join(d.movieDir, "poster.jpg")
	d.files[poster] = bigFile(t, poster, 600<<10, 4)
	extra := filepath.Join(d.movieDir, "extras", "Making of.mkv")
	d.files[extra] = bigFile(t, extra, 2<<20, 5)
	d.seed = filepath.Join(a, "downloads", "seeding.mkv")
	if err := os.Link(video, d.seed); err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(d.server.TestExec(`UPDATE movies SET file_path = ? WHERE id = ?`, video, int64(d.movieID)))
	must(d.server.TestExec(`UPDATE subtitle_files SET sub_path = ? WHERE kind = 'movie'`, sub))

	// The show: two seasons.
	e1 := filepath.Join(d.aTV, "Fixture Show", "Season 01", "Fixture Show - S01E01-E02.mkv")
	e2 := filepath.Join(d.aTV, "Fixture Show", "Season 02", "Fixture Show - S02E01.mkv")
	d.files[e1] = bigFile(t, e1, 4<<20, 6)
	d.files[e2] = bigFile(t, e2, 3<<20, 7)
	nfoShow := filepath.Join(d.aTV, "Fixture Show", "tvshow.nfo")
	d.files[nfoShow] = bigFile(t, nfoShow, 2<<10, 8)
	must(d.server.TestExec(`UPDATE episodes SET file_path = ? WHERE series_id = ? AND season = 1 AND file_path != ''`, e1, int64(d.showID)))
	must(d.server.TestExec(`UPDATE episodes SET file_path = ? WHERE series_id = ? AND season = 2 AND file_path != ''`, e2, int64(d.showID)))
	return d
}

func (d *driveLibrary) move(t *testing.T, kind string, id float64, to string) map[string]any {
	t.Helper()
	postJSON[map[string]any](t, d.client, d.base+"/api/library/move-files", map[string]any{"items": d.items(kind, id), "rootPath": to}, http.StatusAccepted)
	return d.waitMove(t)
}

// expectAt checks every file that lives under from is now under to with the same bytes,
// nothing is left under from, and no hidden staging leftovers sit beside the new folder.
func (d *driveLibrary) expectAt(t *testing.T, from, to string) {
	t.Helper()
	next := map[string]string{}
	for p, h := range d.files {
		if !strings.HasPrefix(p, from+"/") {
			next[p] = h
			continue
		}
		np := to + strings.TrimPrefix(p, from)
		if sum(t, np) != h {
			t.Fatalf("%s differs after the move", np)
		}
		if exists(p) {
			t.Fatalf("%s is still in the old place", p)
		}
		next[np] = h
	}
	d.files = next
	entries, _ := os.ReadDir(to)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && e.Name() != ".mediarium-root" {
			t.Fatalf("a leftover in the new folder: %s", e.Name())
		}
	}
}

func TestTwoRealDrivesMovieMovesThereAndBack(t *testing.T) {
	d := newDriveLibrary(t)
	prev := postJSON[map[string]any](t, d.client, d.base+"/api/library/move-files/preview", map[string]any{"items": d.items("movie", d.movieID), "rootPath": d.bMovies}, http.StatusOK)
	first := prev["titles"].([]any)[0].(map[string]any)
	if first["method"] != "copy" || first["linkNote"] == nil || first["linkNote"] == "" {
		t.Fatalf("preview = %v", first)
	}
	if st := d.move(t, "movie", d.movieID, d.bMovies); st["moved"] != float64(1) || st["left"] != float64(0) {
		t.Fatalf("status = %v", st)
	}
	d.expectAt(t, d.aMovies, d.bMovies)
	if sum(t, d.seed) == "" || exists(d.movieDir) {
		t.Fatal("the seeding file or the old folder is wrong")
	}
	if got, _ := d.server.TestScalar(`SELECT file_path FROM movies`); !strings.HasPrefix(got, d.bMovies+"/") {
		t.Fatalf("movie record = %q", got)
	}
	if got, _ := d.server.TestScalar(`SELECT sub_path FROM subtitle_files`); !strings.HasPrefix(got, d.bMovies+"/") {
		t.Fatalf("subtitle record = %q", got)
	}
	if st := d.move(t, "movie", d.movieID, d.aMovies); st["moved"] != float64(1) || st["left"] != float64(0) {
		t.Fatalf("back: status = %v", st)
	}
	d.expectAt(t, d.bMovies, d.aMovies)
	if got, _ := d.server.TestScalar(`SELECT file_path FROM movies`); !strings.HasPrefix(got, d.aMovies+"/") {
		t.Fatalf("movie record = %q", got)
	}
}

func TestTwoRealDrivesShowMovesThereAndBack(t *testing.T) {
	d := newDriveLibrary(t)
	if st := d.move(t, "tv", d.showID, d.bTV); st["moved"] != float64(1) || st["left"] != float64(0) {
		t.Fatalf("status = %v", st)
	}
	d.expectAt(t, d.aTV, d.bTV)
	if got, _ := d.server.TestScalar(`SELECT COUNT(*) FROM episodes WHERE file_path LIKE ?`, d.bTV+"/%"); got != "3" {
		t.Fatalf("%s episodes in the new folder, want 3", got)
	}
	if st := d.move(t, "tv", d.showID, d.aTV); st["moved"] != float64(1) || st["left"] != float64(0) {
		t.Fatalf("back: status = %v", st)
	}
	d.expectAt(t, d.bTV, d.aTV)
	if got, _ := d.server.TestScalar(`SELECT COUNT(*) FROM episodes WHERE file_path LIKE ?`, d.aTV+"/%"); got != "3" {
		t.Fatalf("%s episodes back in the old folder, want 3", got)
	}
}

// After a real copy the records cannot be saved: the copy is removed again and
// both the files and the paths in the database are as they were.
func TestTwoRealDrivesFailureAfterTheCopyPutsEverythingBack(t *testing.T) {
	d := newDriveLibrary(t)
	if err := d.server.TestExec(`CREATE TRIGGER refuse_move BEFORE UPDATE OF root_path ON movies BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	st := d.move(t, "movie", d.movieID, d.bMovies)
	if st["moved"] != float64(0) || st["left"] != float64(1) {
		t.Fatalf("status = %v", st)
	}
	for p, h := range d.files {
		if !strings.HasPrefix(p, d.aMovies+"/") {
			continue
		}
		if sum(t, p) != h {
			t.Fatalf("%s changed", p)
		}
	}
	if sum(t, d.seed) != d.files[filepath.Join(d.movieDir, "The Matrix (1999).mkv")] {
		t.Fatal("the seeding file changed")
	}
	entries, _ := os.ReadDir(d.bMovies)
	for _, e := range entries {
		if e.Name() != ".mediarium-root" { // the folder's own marker file is fine
			t.Fatalf("the new drive was left with %s", e.Name())
		}
	}
	if got, _ := d.server.TestScalar(`SELECT file_path FROM movies`); !strings.HasPrefix(got, d.aMovies+"/") {
		t.Fatalf("movie record = %q", got)
	}
	if got, _ := d.server.TestScalar(`SELECT sub_path FROM subtitle_files`); !strings.HasPrefix(got, d.aMovies+"/") {
		t.Fatalf("subtitle record = %q", got)
	}
}
