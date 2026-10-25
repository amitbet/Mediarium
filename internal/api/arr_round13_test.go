package api_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/queue"
)

// The Radarr movie card's history has to answer under every name an app asks
// with, for a movie that came in by a download, one that has only a file, and
// a movie with nothing; and a download from this week shows in the recent lists.
func TestArrRadarrMovieHistoryRound13(t *testing.T) {
	server, base, admin, _, _ := familyServer(t)
	key := postJSON[map[string]any](t, admin, base+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	radarr := base + "/radarr/api/v3"
	dir := t.TempDir()
	withFile := func(m library.Movie, name string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := server.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, "Bluray-1080p", p); err != nil {
			t.Fatal(err)
		}
	}
	add := func(tmdb int, title string) library.Movie {
		m, err := server.MovieRepo.Add(library.Movie{TMDBID: tmdb, Title: title, Year: 2020, Monitored: true, ReleaseDate: "2020-01-01"})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	downloaded := add(801, "Harbour Lights")
	withFile(downloaded, "a.mkv")
	id, err := server.QueueRepo.Enqueue(queue.Item{MovieID: downloaded.ID, ReleaseTitle: "Harbour.Lights.2020.1080p.BluRay.x264-GRP", SizeBytes: 4000, NZBURL: "http://127.0.0.1:1/x.nzb", Protocol: queue.ProtocolUsenet})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.QueueRepo.SetStatus(id, queue.StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	fileOnly := add(802, "Quiet Pier")
	withFile(fileOnly, "b.mkv")
	twin := add(803, "Twin Tide")
	withFile(twin, "c.mkv")
	missing := add(804, "Not Yet")

	cases := []struct {
		name  string
		movie library.Movie
		want  int
		event string // the event type of the newest line
	}{
		{"a download gives its grabbed and import line", downloaded, 2, "downloadFolderImported"},
		{"a library import gives one line", fileOnly, 1, "movieFolderImported"},
		{"a movie with no file and no download gives none", missing, 0, ""},
	}
	for _, tc := range cases {
		for _, path := range []string{
			"/history/movie?movieId=%d",
			"/history?movieId=%d&page=1&pageSize=20&sortKey=date&sortDirection=descending",
			"/history?movieIds=%d&includeMovie=true",
			"/history/since?movieId=%d&date=2000-01-01T00:00:00Z",
		} {
			url := radarr + fmt.Sprintf(path, tc.movie.ID)
			t.Run(tc.name+" "+path, func(t *testing.T) {
				var got []any
				switch v := arrCall(t, "GET", url, key, nil, http.StatusOK).(type) {
				case []any:
					got = v
				case map[string]any:
					got = list(v["records"])
				}
				if len(got) != tc.want {
					t.Fatalf("%s: %d lines, want %d: %v", url, len(got), tc.want, got)
				}
				for _, l := range got {
					if obj(l)["movieId"] != float64(tc.movie.ID) || obj(obj(l)["movie"])["title"] != tc.movie.Title {
						t.Errorf("a line for another movie or without the movie: %v", l)
					}
				}
				if tc.want > 0 && obj(got[0])["eventType"] != tc.event {
					t.Errorf("newest line is %v, want %s", obj(got[0])["eventType"], tc.event)
				}
			})
		}
	}

	// The recent list a dashboard card fills from: the download of this week
	// and the library imports added this week show; a movie with nothing does not.
	recent := obj(arrCall(t, "GET", radarr+"/history?page=1&pageSize=20&sortKey=date&sortDirection=descending&eventType=1,3&includeMovie=true", key, nil, http.StatusOK))
	if recent["totalRecords"] != float64(4) { // the download's two lines and the two library imports
		t.Errorf("dashboard list with eventType=1,3: %v", recent)
	}
	recentAll := obj(arrCall(t, "GET", radarr+"/history?page=1&pageSize=20&sortKey=date&sortDirection=descending&includeMovie=true", key, nil, http.StatusOK))
	titles := map[string]bool{}
	for _, l := range list(recentAll["records"]) {
		titles[obj(obj(l)["movie"])["title"].(string)] = true
	}
	if !titles["Harbour Lights"] || !titles["Quiet Pier"] || !titles["Twin Tide"] || titles["Not Yet"] {
		t.Errorf("dashboard list without a filter: %v", titles)
	}
}
