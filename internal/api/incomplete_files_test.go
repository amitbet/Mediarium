package api_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/api"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/mediainfo/mediainfotest"
	"github.com/rdborg/mediarium/internal/settings"
)

// Files already in the library that look cut off: the Library lists carry the
// note, the pages say so, the assistant tool can filter on it, and the redo
// call deletes only those, blocklists their releases and leaves the rest.
func TestLibraryFilesThatLookIncomplete(t *testing.T) {
	restore := api.SetMediaInfoSettle(0)
	defer restore()
	e := newMCPEnv(t)
	e.enable(t)
	s := e.server
	root := t.TempDir()
	movies, tv := filepath.Join(root, "movies"), filepath.Join(root, "tv")
	for k, v := range map[string]string{settings.KeyMoviesPath: movies, settings.KeyTVPath: tv} {
		if err := s.Settings.Set(k, v, false); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string, seconds float64) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, mediainfotest.MKV(seconds), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	addMovie := func(tmdb int, title, path, release string) library.Movie {
		t.Helper()
		m, err := s.MovieRepo.Add(library.Movie{TMDBID: tmdb, Title: title, Year: 2020, Monitored: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, "WEBDL-1080p", path); err != nil {
			t.Fatal(err)
		}
		if release != "" {
			_ = s.MovieRepo.SetReleaseTitle(m.ID, release)
		}
		return m
	}
	fake := addMovie(1, "Fake Film", write(filepath.Join(movies, "Fake Film (2020)", "Fake.Film.2020.mkv"), 90), "Fake.Film.2020.1080p.WEB-DL-GRP")
	good := addMovie(2, "Good Film", write(filepath.Join(movies, "Good Film (2020)", "Good.Film.2020.mkv"), 95*60), "Good.Film.2020.1080p.WEB-DL-GRP")
	unknown := addMovie(3, "Unknown Film", write(filepath.Join(movies, "Unknown Film (2020)", "Unknown.Film.2020.mkv"), 4*60), "")
	outsidePath := write(filepath.Join(root, "elsewhere", "Outside.Film.2020.mkv"), 60)
	outside := addMovie(4, "Outside Film", outsidePath, "")
	for _, id := range []int{1, 2} {
		if err := s.MovieRepo.SaveRuntime(library.RuntimeMovie, id, 100, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	// The runtime of "Unknown Film" and "Outside Film" is not known.
	_ = s.MovieRepo.SaveRuntime(library.RuntimeMovie, 3, 0, time.Now())
	_ = s.MovieRepo.SaveRuntime(library.RuntimeMovie, 4, 0, time.Now())

	sr, err := s.MovieRepo.AddSeries(library.Series{TMDBID: 55, Title: "Fixture Series", Year: 2020, Monitored: true}, []library.Episode{
		{Season: 1, Episode: 1, Title: "One", AirDate: "2020-01-01", Monitored: true},
		{Season: 1, Episode: 2, Title: "Two", AirDate: "2020-01-08", Monitored: true},
		{Season: 0, Episode: 1, Title: "Special", AirDate: "2020-01-15", Monitored: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.MovieRepo.SaveRuntime(library.RuntimeTV, 55, 45, time.Now())
	eps, _ := s.MovieRepo.ListEpisodes(sr.ID)
	byNum := map[string]library.Episode{}
	for _, ep := range eps {
		byNum[fmt.Sprintf("%d-%d", ep.Season, ep.Episode)] = ep
	}
	season := filepath.Join(tv, "Fixture Series (2020)", "Season 01")
	for num, spec := range map[string]struct {
		file    string
		seconds float64
		release string
	}{
		"1-1": {filepath.Join(season, "S01E01.mkv"), 60, "Fixture.Series.S01.2160p.WEB-DL-GRP"},
		"1-2": {filepath.Join(season, "S01E02.mkv"), 44 * 60, "Fixture.Series.S01.2160p.WEB-DL-GRP"},
		"0-1": {filepath.Join(tv, "Fixture Series (2020)", "Specials", "S00E01.mkv"), 5 * 60, ""},
	} {
		ep := byNum[num]
		write(spec.file, spec.seconds)
		if err := s.MovieRepo.SetEpisodeStatus(ep.ID, library.StatusDownloaded, "WEBDL-2160p", spec.file); err != nil {
			t.Fatal(err)
		}
		if spec.release != "" {
			_ = s.MovieRepo.SetEpisodeReleaseTitle(ep.ID, spec.release)
		}
	}
	s.RunMediaInfoJob(context.Background())

	// The lists and pages carry the note.
	list := getJSON[[]map[string]any](t, e.admin, e.base+"/api/movies?brief=1")
	short := map[string]bool{}
	for _, m := range list {
		if n, ok := m["shortFile"].(map[string]any); ok {
			short[m["title"].(string)] = true
			if m["title"] == "Fake Film" && (n["minutes"].(float64) != 2 && n["minutes"].(float64) != 1 || n["expected"].(float64) != 100) {
				t.Errorf("note of the fake film = %v", n)
			}
		}
	}
	if !short["Fake Film"] || !short["Unknown Film"] || !short["Outside Film"] || short["Good Film"] || len(short) != 3 {
		t.Fatalf("movies that look incomplete = %v", short)
	}
	shows := getJSON[[]map[string]any](t, e.admin, e.base+"/api/series?brief=1")
	if len(shows) != 1 || shows[0]["shortFiles"] != float64(1) {
		t.Fatalf("shows = %v (the special of five minutes is not judged, only the one-minute episode is)", shows)
	}
	detail := getJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/series/%d", e.base, sr.ID))
	flagged := 0
	for _, raw := range detail["episodes"].([]any) {
		if raw.(map[string]any)["shortFile"] != nil {
			flagged++
			if raw.(map[string]any)["episode"] != float64(1) || raw.(map[string]any)["season"] != float64(1) {
				t.Errorf("the wrong episode is flagged: %v", raw)
			}
		}
	}
	if flagged != 1 {
		t.Fatalf("flagged episodes = %d", flagged)
	}
	page := getJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/movies/%d", e.base, fake.ID))
	if page["shortFile"] == nil {
		t.Fatalf("the movie page does not say the file is short: %v", page)
	}
	if goodPage := getJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/movies/%d", e.base, good.ID)); goodPage["shortFile"] != nil {
		t.Fatalf("a good file is flagged: %v", goodPage["shortFile"])
	}

	// The assistant tool.
	count := func(args map[string]any) float64 {
		t.Helper()
		_, text, p := e.call(t, e.roKey, "library_search", args)
		if p == nil {
			t.Fatalf("answer: %s", text)
		}
		return p["total"].(float64)
	}
	if got := count(map[string]any{"incomplete": true}); got != 3 {
		t.Errorf("movies with incomplete=true: %v", got)
	}
	if got := count(map[string]any{"incomplete": false}); got != 1 {
		t.Errorf("movies with incomplete=false: %v", got)
	}
	if got := count(map[string]any{"kind": "tv", "incomplete": true}); got != 1 {
		t.Errorf("shows with incomplete=true: %v", got)
	}

	// Redo: only what still looks incomplete, inside the library, is touched.
	redo := postJSON[map[string]any](t, e.admin, e.base+"/api/library/incomplete/redo", map[string]any{
		"movieIds": []int64{fake.ID, good.ID, outside.ID}, "seriesIds": []int64{sr.ID},
	}, http.StatusOK)
	if redo["titles"] != float64(2) || redo["skipped"] != float64(2) {
		t.Fatalf("redo answered %v, want 2 titles done (the fake film and the fake episode) and 2 skipped (the good film and the one outside the library)", redo)
	}
	if m, _ := s.MovieRepo.Get(fake.ID); m.Status != library.StatusMissing || m.FilePath != "" {
		t.Errorf("the fake film should be wanted again: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(movies, "Fake Film (2020)", "Fake.Film.2020.mkv")); err == nil {
		t.Error("the fake film's file should be gone")
	}
	if m, _ := s.MovieRepo.Get(good.ID); m.Status != library.StatusDownloaded {
		t.Errorf("the good film was touched: %+v", m)
	}
	if _, err := os.Stat(outsidePath); err != nil {
		t.Errorf("a file outside the library folders must never be touched: %v", err)
	}
	if m, _ := s.MovieRepo.Get(outside.ID); m.Status != library.StatusDownloaded {
		t.Errorf("the film outside the library was touched: %+v", m)
	}
	if m, _ := s.MovieRepo.Get(unknown.ID); m.Status != library.StatusDownloaded {
		t.Errorf("a film that was not asked for was touched: %+v", m)
	}
	if e1, _ := s.MovieRepo.GetEpisodeByID(byNum["1-1"].ID); e1.Status != library.StatusMissing {
		t.Errorf("the one-minute episode should be wanted again: %+v", e1)
	}
	if e2, _ := s.MovieRepo.GetEpisodeByID(byNum["1-2"].ID); e2.Status != library.StatusDownloaded {
		t.Errorf("the full episode was touched: %+v", e2)
	}
	for _, release := range []string{"Fake.Film.2020.1080p.WEB-DL-GRP", "Fixture.Series.S01.2160p.WEB-DL-GRP"} {
		if reason, listed, _ := s.Blocklist.Reason(release); !listed || reason != "incomplete file" {
			t.Errorf("release %s: blocklist reason %q, listed %v", release, reason, listed)
		}
	}
	if _, listed, _ := s.Blocklist.Reason("Good.Film.2020.1080p.WEB-DL-GRP"); listed {
		t.Error("the release of a good file was blocklisted")
	}

	// Only an administrator may do it.
	req, _ := http.NewRequest(http.MethodPost, e.base+"/api/library/incomplete/redo", strings.NewReader(fmt.Sprintf(`{"movieIds":[%d]}`, unknown.ID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.member)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a member could call redo: status %d", resp.StatusCode)
	}
	if m, _ := s.MovieRepo.Get(unknown.ID); m.Status != library.StatusDownloaded {
		t.Errorf("the film was deleted by a member: %+v", m)
	}
}
