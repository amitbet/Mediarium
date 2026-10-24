package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/api"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/queue"
	"github.com/rdborg/mediarium/internal/settings"
)

// locateEnv is a server with a movies folder, a TV folder, a downloads folder
// and a folder that is not part of the library.
type locateEnv struct {
	server                              *api.Server
	base                                string
	admin, member                       *http.Client
	movies, tv, downloads, outside, tmp string
}

func newLocateEnv(t *testing.T) *locateEnv {
	t.Helper()
	server, base, admin, member, _ := familyServer(t)
	tmp := t.TempDir()
	e := &locateEnv{server: server, base: base, admin: admin, member: member, tmp: tmp,
		movies: filepath.Join(tmp, "movies"), tv: filepath.Join(tmp, "tv"), downloads: filepath.Join(tmp, "downloads"), outside: filepath.Join(tmp, "outside")}
	for _, d := range []string{e.movies, e.tv, e.downloads, e.outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for k, v := range map[string]string{settings.KeyMoviesPath: e.movies, settings.KeyTVPath: e.tv, settings.KeyDownloadsPath: e.downloads} {
		if err := server.Settings.Set(k, v, false); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func (e *locateEnv) addMovie(t *testing.T, tmdb int, title string) library.Movie {
	t.Helper()
	m, err := e.server.MovieRepo.Add(library.Movie{TMDBID: tmdb, Title: title, Year: 1999, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// A different file is read again: a name that says nothing about the
// quality leaves the movie Not stated, not with the old file's quality.
func TestLocateMovieFileReadsTheNewFilesQuality(t *testing.T) {
	e := newLocateEnv(t)
	m := e.addMovie(t, 603, "The Matrix")
	old := filepath.Join(e.movies, "The.Matrix.1999.720p.BluRay.x264-GRP.mkv")
	writeBody(t, old, "video")
	if err := e.server.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, "Bluray-720p", old); err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(e.movies, "Old drive", "matrix by me.mkv")
	writeBody(t, renamed, "video")
	postJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/movies/%d/locate-file", e.base, m.ID), map[string]string{"path": renamed}, http.StatusOK)
	got, err := e.server.MovieRepo.Get(m.ID)
	if err != nil || got.FilePath != renamed || got.Quality != "Unknown" {
		t.Fatalf("after locate: %+v %v", got, err)
	}
	hd := filepath.Join(e.movies, "The Matrix (1999)", "The.Matrix.1999.1080p.WEB-DL.mkv")
	writeBody(t, hd, "video")
	postJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/movies/%d/locate-file", e.base, m.ID), map[string]string{"path": hd}, http.StatusOK)
	if got, _ := e.server.MovieRepo.Get(m.ID); got.Quality != "WEBDL-1080p" {
		t.Fatalf("the new name's quality: %q", got.Quality)
	}
}

func TestLocateMovieFilePathChecks(t *testing.T) {
	tests := []struct {
		name string
		// setup returns the path to send
		setup      func(t *testing.T, e *locateEnv, other library.Movie) string
		wantStatus int
		wantSaved  bool
	}{
		{
			name: "a video in the movies folder",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				p := filepath.Join(e.movies, "The Matrix Renamed", "matrix.1080p.mkv")
				writeBody(t, p, "video")
				return p
			},
			wantStatus: http.StatusOK, wantSaved: true,
		},
		{
			name: "a path with dots that walks out of the library",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				writeBody(t, filepath.Join(e.outside, "secret.mkv"), "video")
				return e.movies + "/../outside/secret.mkv"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "a file outside every library folder",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				p := filepath.Join(e.outside, "secret.mkv")
				writeBody(t, p, "video")
				return p
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "a link inside the library that points outside",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				target := filepath.Join(e.outside, "secret.mkv")
				writeBody(t, target, "video")
				link := filepath.Join(e.movies, "sneaky.mkv")
				if err := os.Symlink(target, link); err != nil {
					t.Skip("symlinks are not available here")
				}
				return link
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "a link to a folder outside, with the file below it",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				writeBody(t, filepath.Join(e.outside, "secret.mkv"), "video")
				if err := os.Symlink(e.outside, filepath.Join(e.movies, "elsewhere")); err != nil {
					t.Skip("symlinks are not available here")
				}
				return filepath.Join(e.movies, "elsewhere", "secret.mkv")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "a file that is not a video",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				p := filepath.Join(e.movies, "The Matrix (1999)", "The Matrix (1999).nfo")
				writeBody(t, p, "text")
				return p
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "a folder",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				p := filepath.Join(e.movies, "The Matrix (1999)")
				if err := os.MkdirAll(p, 0o755); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "a file that is not there",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				return filepath.Join(e.movies, "nothing.mkv")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "a path that is not a full path",
			setup:      func(t *testing.T, e *locateEnv, _ library.Movie) string { return "matrix.mkv" },
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "the file of another movie",
			setup: func(t *testing.T, e *locateEnv, other library.Movie) string {
				p := filepath.Join(e.movies, "Another (2001)", "another.mkv")
				writeBody(t, p, "video")
				if err := e.server.MovieRepo.SetStatus(other.ID, library.StatusDownloaded, "", p); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "the file of an episode",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				p := filepath.Join(e.tv, "Show", "Season 1", "Show.S01E01.mkv")
				writeBody(t, p, "video")
				sr, err := e.server.MovieRepo.AddSeries(library.Series{TMDBID: 5, Title: "Show"}, []library.Episode{{Season: 1, Episode: 1}})
				if err != nil {
					t.Fatal(err)
				}
				eps, _ := e.server.MovieRepo.ListEpisodes(sr.ID)
				if err := e.server.MovieRepo.SetEpisodeStatus(eps[0].ID, library.StatusDownloaded, "", p); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "a video in the downloads folder is not a library file",
			setup: func(t *testing.T, e *locateEnv, _ library.Movie) string {
				p := filepath.Join(e.downloads, "complete", "matrix.mkv")
				writeBody(t, p, "video")
				return p
			},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newLocateEnv(t)
			m := e.addMovie(t, 603, "The Matrix")
			other := e.addMovie(t, 604, "Another Film")
			path := tc.setup(t, e, other)
			url := fmt.Sprintf("%s/api/movies/%d/locate-file", e.base, m.ID)

			if status, _ := doStatus(t, e.member, http.MethodPost, url); status != http.StatusForbidden {
				t.Fatalf("a member's request = %d, want 403", status)
			}
			postJSON[map[string]any](t, e.admin, url, map[string]any{"path": path}, tc.wantStatus)
			after, _ := e.server.MovieRepo.Get(m.ID)
			if tc.wantSaved {
				if after.Status != library.StatusDownloaded || after.FilePath != path {
					t.Fatalf("movie = %+v, want downloaded with %s", after, path)
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("the file was touched: %v", err)
				}
			} else if after.FilePath != "" || after.Status == library.StatusDownloaded {
				t.Fatalf("nothing should have been saved: %+v", after)
			}
		})
	}
}

func TestLocateMovieFileRefusedWhileDownloading(t *testing.T) {
	e := newLocateEnv(t)
	m := e.addMovie(t, 603, "The Matrix")
	p := filepath.Join(e.movies, "matrix.mkv")
	writeBody(t, p, "video")
	if _, err := e.server.QueueRepo.Enqueue(queue.Item{MovieID: m.ID, ReleaseTitle: "The.Matrix.1999.1080p.BluRay.x264-GRP"}); err != nil {
		t.Fatal(err)
	}
	postJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/movies/%d/locate-file", e.base, m.ID), map[string]any{"path": p}, http.StatusConflict)
}

func TestLocateMovieFolder(t *testing.T) {
	e := newLocateEnv(t)
	m := e.addMovie(t, 603, "The Matrix")
	url := fmt.Sprintf("%s/api/movies/%d/locate-folder", e.base, m.ID)

	// Not allowed: outside, a whole library folder, a file instead of a folder.
	writeBody(t, filepath.Join(e.outside, "a.mkv"), "video")
	postJSON[map[string]any](t, e.admin, url, map[string]any{"path": e.outside}, http.StatusBadRequest)
	postJSON[map[string]any](t, e.admin, url, map[string]any{"path": e.movies}, http.StatusBadRequest)
	postJSON[map[string]any](t, e.admin, url, map[string]any{"path": filepath.Join(e.outside, "a.mkv")}, http.StatusBadRequest)

	// An empty folder finds nothing and says where it looked.
	empty := filepath.Join(e.movies, "Empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	got := postJSON[map[string]any](t, e.admin, url, map[string]any{"path": empty}, http.StatusOK)
	if got["looked"] != empty || got["added"] != nil {
		t.Fatalf("report = %v", got)
	}

	// The biggest video wins; extras and samples don't count.
	dir := filepath.Join(e.movies, "Matrix new name")
	writeBody(t, filepath.Join(dir, "small.mkv"), "12")
	writeBody(t, filepath.Join(dir, "main.1080p.mkv"), "1234567890")
	writeBody(t, filepath.Join(dir, "Extras", "making-of.mkv"), strings.Repeat("x", 100))
	got = postJSON[map[string]any](t, e.admin, url, map[string]any{"path": dir}, http.StatusOK)
	if got["added"] != "main.1080p.mkv" {
		t.Fatalf("report = %v", got)
	}
	after, _ := e.server.MovieRepo.Get(m.ID)
	if after.Status != library.StatusDownloaded || after.FilePath != filepath.Join(dir, "main.1080p.mkv") {
		t.Fatalf("movie = %+v", after)
	}
}

type browseReply struct {
	Path    string `json:"path"`
	Parent  string `json:"parent"`
	Entries []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Dir  bool   `json:"dir"`
	} `json:"entries"`
}

func TestBrowseStaysInsideTheLibraryFolders(t *testing.T) {
	e := newLocateEnv(t)
	writeBody(t, filepath.Join(e.movies, "Film (2000)", "film.mkv"), "video")
	writeBody(t, filepath.Join(e.movies, "Film (2000)", "film.nfo"), "text")
	writeBody(t, filepath.Join(e.movies, "Film (2000)", ".hidden.mkv"), "video")
	writeBody(t, filepath.Join(e.outside, "secret.mkv"), "video")
	if err := os.Symlink(e.outside, filepath.Join(e.movies, "elsewhere")); err != nil {
		t.Skip("symlinks are not available here")
	}
	if err := os.Symlink(filepath.Join(e.outside, "secret.mkv"), filepath.Join(e.movies, "Film (2000)", "link.mkv")); err != nil {
		t.Skip("symlinks are not available here")
	}

	if status, _ := doStatus(t, e.member, http.MethodGet, e.base+"/api/library/browse"); status != http.StatusForbidden {
		t.Fatalf("a member's browse = %d, want 403", status)
	}

	top := getJSON[browseReply](t, e.admin, e.base+"/api/library/browse")
	var names []string
	for _, en := range top.Entries {
		names = append(names, en.Path)
	}
	if got, want := strings.Join(names, ","), strings.Join([]string{e.movies, e.tv, e.downloads}, ","); got != want {
		t.Fatalf("top = %s, want %s", got, want)
	}

	list := getJSON[browseReply](t, e.admin, e.base+"/api/library/browse?path="+url.QueryEscape(e.movies))
	if len(list.Entries) != 1 || list.Entries[0].Name != "Film (2000)" || !list.Entries[0].Dir || list.Parent != "" {
		t.Fatalf("movies folder = %+v", list)
	}
	inner := getJSON[browseReply](t, e.admin, e.base+"/api/library/browse?path="+url.QueryEscape(filepath.Join(e.movies, "Film (2000)")))
	if len(inner.Entries) != 1 || inner.Entries[0].Name != "film.mkv" || inner.Parent != e.movies {
		t.Fatalf("title folder = %+v", inner)
	}

	// A folder that is gone opens at the nearest one above it.
	gone := getJSON[browseReply](t, e.admin, e.base+"/api/library/browse?path="+url.QueryEscape(filepath.Join(e.movies, "Film (2000)", "Gone", "Deeper")))
	if len(gone.Entries) != 1 || gone.Entries[0].Name != "film.mkv" {
		t.Fatalf("a gone folder = %+v", gone)
	}

	for _, bad := range []string{e.outside, e.movies + "/../outside", filepath.Join(e.movies, "elsewhere"), "relative/path", filepath.Join(e.outside, "gone")} {
		if status, _ := doStatus(t, e.admin, http.MethodGet, e.base+"/api/library/browse?path="+url.QueryEscape(bad)); status != http.StatusBadRequest {
			t.Errorf("browse of %q = %d, want 400", bad, status)
		}
	}
}

func TestChangeSeriesFolder(t *testing.T) {
	type ep struct {
		season, episode int
		airDate         string
	}
	aired := func(s, e int) ep { return ep{s, e, "2020-01-01"} }
	tests := []struct {
		name       string
		anime      bool
		episodes   []ep
		files      []string       // under the new folder
		oldFiles   map[int]string // episode index -> file that still exists elsewhere (relative to the TV folder)
		goneFiles  []int          // episode indexes whose recorded file is not on the disk
		wantFound  int
		wantTotal  int
		wantAfter  string // part of the message
		wantFilled []int  // episode indexes that have a file in the new folder
	}{
		{
			name:      "most episodes are found, one is missing, one file has no number",
			episodes:  []ep{aired(1, 1), aired(1, 2), aired(1, 3), aired(1, 4)},
			files:     []string{"Show.S01E01.mkv", "Season 1/Show.S01E02.mkv", "Show.S01E03.720p.mkv", "Pilot.mkv"},
			goneFiles: []int{0},
			wantFound: 3, wantTotal: 4,
			wantAfter:  "Found 3 of 4 episodes in %s. 1 is still missing. 1 video file there wasn't placed on an episode.",
			wantFilled: []int{0, 1, 2},
		},
		{
			name:      "everything is found",
			episodes:  []ep{aired(1, 1), aired(1, 2)},
			files:     []string{"Show.S01E01.mkv", "Show.S01E02.mkv"},
			wantFound: 2, wantTotal: 2,
			wantAfter:  "Found 2 of 2 episodes in %s.",
			wantFilled: []int{0, 1},
		},
		{
			name:      "an episode that has not aired yet is not counted as missing",
			episodes:  []ep{aired(1, 1), {1, 2, "2999-01-01"}},
			files:     []string{"Show.S01E01.mkv"},
			wantFound: 1, wantTotal: 1,
			wantAfter:  "Found 1 of 1 episodes in %s.",
			wantFilled: []int{0},
		},
		{
			name:      "an empty folder changes nothing",
			episodes:  []ep{aired(1, 1)},
			files:     nil,
			goneFiles: []int{0},
			wantFound: 0, wantTotal: 1,
			wantAfter: "Found no episode files for this show in %s. Nothing was changed.",
		},
		{
			name:      "an episode whose own file is still there is left alone",
			episodes:  []ep{aired(1, 1), aired(1, 2)},
			files:     []string{"Show.S01E01.mkv", "Show.S01E02.mkv"},
			oldFiles:  map[int]string{0: "Elsewhere/Show.S01E01.mkv"},
			wantFound: 1, wantTotal: 2,
			wantAfter:  "Found 1 of 2 episodes in %s.",
			wantFilled: []int{1},
		},
		{
			name:      "an anime numbered from the first episode",
			anime:     true,
			episodes:  []ep{aired(1, 1), aired(1, 2), aired(2, 1)},
			files:     []string{"[Group] Show - 02.mkv", "[Group] Show - 03.mkv"},
			wantFound: 2, wantTotal: 3,
			wantAfter:  "Found 2 of 3 episodes in %s. 1 is still missing.",
			wantFilled: []int{1, 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newLocateEnv(t)
			var eps []library.Episode
			for _, x := range tc.episodes {
				eps = append(eps, library.Episode{Season: x.season, Episode: x.episode, AirDate: x.airDate})
			}
			series := library.Series{TMDBID: 99, Title: "Show", Year: 2020, Monitored: true}
			if tc.anime {
				series.SeriesType = library.SeriesAnime
			}
			sr, err := e.server.MovieRepo.AddSeries(series, eps)
			if err != nil {
				t.Fatal(err)
			}
			listed, _ := e.server.MovieRepo.ListEpisodes(sr.ID)
			oldDir := filepath.Join(e.tv, "Old Folder")
			for _, i := range tc.goneFiles {
				if err := e.server.MovieRepo.SetEpisodeStatus(listed[i].ID, library.StatusDownloaded, "", filepath.Join(oldDir, fmt.Sprintf("gone-%d.mkv", i))); err != nil {
					t.Fatal(err)
				}
			}
			for i, rel := range tc.oldFiles {
				p := filepath.Join(e.tv, filepath.FromSlash(rel))
				writeBody(t, p, "old")
				if err := e.server.MovieRepo.SetEpisodeStatus(listed[i].ID, library.StatusDownloaded, "", p); err != nil {
					t.Fatal(err)
				}
			}
			newDir := filepath.Join(e.tv, "New Folder")
			if err := os.MkdirAll(newDir, 0o755); err != nil {
				t.Fatal(err)
			}
			before := map[string]string{}
			for _, rel := range tc.files {
				writeBody(t, filepath.Join(newDir, filepath.FromSlash(rel)), "video")
				before[rel] = "video"
			}
			url := fmt.Sprintf("%s/api/series/%d/change-folder", e.base, sr.ID)

			if status, _ := doStatus(t, e.member, http.MethodPost, url); status != http.StatusForbidden {
				t.Fatalf("a member's request = %d, want 403", status)
			}
			got := postJSON[map[string]any](t, e.admin, url, map[string]any{"path": newDir}, http.StatusOK)
			if got["found"] != float64(tc.wantFound) || got["total"] != float64(tc.wantTotal) {
				t.Fatalf("report = %v, want %d of %d", got, tc.wantFound, tc.wantTotal)
			}
			if want := fmt.Sprintf(tc.wantAfter, newDir); got["message"] != want {
				t.Fatalf("message = %q, want %q", got["message"], want)
			}
			after, _ := e.server.MovieRepo.ListEpisodes(sr.ID)
			filled := map[int]bool{}
			for _, i := range tc.wantFilled {
				filled[i] = true
			}
			for i, x := range after {
				inNew := strings.HasPrefix(x.FilePath, newDir+string(filepath.Separator))
				if inNew != filled[i] {
					t.Errorf("episode %d: file %q, in the new folder = %v, want %v", i, x.FilePath, inNew, filled[i])
				}
				if inNew && x.Status != library.StatusDownloaded {
					t.Errorf("episode %d has a file but is %s", i, x.Status)
				}
			}
			// An episode that kept a path that is gone stays as it was.
			for _, i := range tc.goneFiles {
				if !filled[i] && after[i].FilePath != filepath.Join(oldDir, fmt.Sprintf("gone-%d.mkv", i)) {
					t.Errorf("episode %d lost its recorded file: %q", i, after[i].FilePath)
				}
			}
			// Nothing was moved, renamed or deleted.
			for rel := range before {
				if _, err := os.Stat(filepath.Join(newDir, filepath.FromSlash(rel))); err != nil {
					t.Errorf("%s was touched: %v", rel, err)
				}
			}
			for _, rel := range tc.oldFiles {
				if _, err := os.Stat(filepath.Join(e.tv, filepath.FromSlash(rel))); err != nil {
					t.Errorf("%s was touched: %v", rel, err)
				}
			}
		})
	}
}

func TestChangeSeriesFolderRefusesOtherFolders(t *testing.T) {
	e := newLocateEnv(t)
	sr, err := e.server.MovieRepo.AddSeries(library.Series{TMDBID: 99, Title: "Show"}, []library.Episode{{Season: 1, Episode: 1, AirDate: "2020-01-01"}})
	if err != nil {
		t.Fatal(err)
	}
	writeBody(t, filepath.Join(e.outside, "Show.S01E01.mkv"), "video")
	if err := os.Symlink(e.outside, filepath.Join(e.tv, "link")); err != nil {
		t.Skip("symlinks are not available here")
	}
	url := fmt.Sprintf("%s/api/series/%d/change-folder", e.base, sr.ID)
	for _, bad := range []string{e.outside, e.tv, filepath.Join(e.tv, "link"), e.tv + "/../outside", filepath.Join(e.tv, "nope"), "Show", e.downloads} {
		postJSON[map[string]any](t, e.admin, url, map[string]any{"path": bad}, http.StatusBadRequest)
	}
	eps, _ := e.server.MovieRepo.ListEpisodes(sr.ID)
	if eps[0].FilePath != "" {
		t.Fatalf("a refused folder changed an episode: %+v", eps[0])
	}
}

func TestLocateEpisodeFile(t *testing.T) {
	e := newLocateEnv(t)
	sr, err := e.server.MovieRepo.AddSeries(library.Series{TMDBID: 7, Title: "Fixture Show", Year: 2026, Monitored: true}, []library.Episode{
		{Season: 1, Episode: 1, AirDate: "2026-01-01", Monitored: true},
		{Season: 1, Episode: 2, AirDate: "2026-01-08", Monitored: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ep1, err := e.server.MovieRepo.GetEpisode(sr.ID, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	ep2, _ := e.server.MovieRepo.GetEpisode(sr.ID, 1, 2)
	url := func(id int64) string { return fmt.Sprintf("%s/api/episodes/%d/locate-file", e.base, id) }

	inShow := filepath.Join(e.tv, "Fixture Show", "S15E01.mkv")
	elsewhere := filepath.Join(e.tv, "Old drive", "clip.1080p.WEB-DL.mkv")
	outside := filepath.Join(e.outside, "a.mkv")
	for _, p := range []string{inShow, elsewhere, outside} {
		writeBody(t, p, "video")
	}
	if status, _ := doStatus(t, e.member, http.MethodPost, url(ep1.ID)); status != http.StatusForbidden {
		t.Fatalf("a member's request = %d, want 403", status)
	}
	// Outside the library folders, or not a file: refused.
	postJSON[map[string]any](t, e.admin, url(ep1.ID), map[string]any{"path": outside}, http.StatusBadRequest)
	postJSON[map[string]any](t, e.admin, url(ep1.ID), map[string]any{"path": e.tv}, http.StatusBadRequest)
	postJSON[map[string]any](t, e.admin, url(99999), map[string]any{"path": inShow}, http.StatusNotFound)

	postJSON[map[string]any](t, e.admin, url(ep1.ID), map[string]any{"path": inShow}, http.StatusOK)
	got, _ := e.server.MovieRepo.GetEpisodeByID(ep1.ID)
	if got.Status != library.StatusDownloaded || got.FilePath != inShow {
		t.Fatalf("episode 1 = %+v", got)
	}
	if _, err := os.Stat(inShow); err != nil {
		t.Fatalf("the file was touched: %v", err)
	}
	// One file is not two episodes' file; a file outside the show's folder can be used.
	postJSON[map[string]any](t, e.admin, url(ep2.ID), map[string]any{"path": inShow}, http.StatusConflict)
	postJSON[map[string]any](t, e.admin, url(ep2.ID), map[string]any{"path": elsewhere}, http.StatusOK)
	if got, _ := e.server.MovieRepo.GetEpisodeByID(ep2.ID); got.FilePath != elsewhere || got.Status != library.StatusDownloaded {
		t.Fatalf("episode 2 = %+v", got)
	}
}
