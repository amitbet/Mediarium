package api_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/queue"
)

// What nzb360's "recently downloaded" cards ask for: history as the paged
// envelope, filtered by event type, with the movie, show and episode when the
// app asks for them; the same lines as plain lists; the calendar with the
// episode file; and the file, parse and empty-list answers.
func TestArrHistoryAndFiles(t *testing.T) {
	server, base, admin, _, _ := familyServer(t)
	key := postJSON[map[string]any](t, admin, base+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	radarr, sonarr := base+"/radarr/api/v3", base+"/sonarr/api/v3"

	movie, err := server.MovieRepo.Add(library.Movie{TMDBID: 701, Title: "Night Harbour", Year: 2021, Monitored: true, ReleaseDate: "2021-05-14"})
	if err != nil {
		t.Fatal(err)
	}
	show, err := server.MovieRepo.AddSeries(library.Series{TMDBID: 9001, Title: "Salt & Static", Year: 2019, Monitored: true}, []library.Episode{
		{Season: 1, Episode: 1, AirDate: "2019-02-01", Title: "Static"}, {Season: 1, Episode: 2, AirDate: "2019-02-08", Title: "Salt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	eps, _ := server.MovieRepo.ListEpisodes(show.ID)
	file := filepath.Join(t.TempDir(), "S01E01.mkv")
	if err := os.WriteFile(file, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := server.MovieRepo.SetEpisodeStatus(eps[0].ID, library.StatusDownloaded, "WEBDL-1080p", file); err != nil {
		t.Fatal(err)
	}

	enqueue := func(it queue.Item, status queue.Status, errText string) int64 {
		it.NZBURL, it.Protocol = "http://127.0.0.1:1/x.nzb", queue.ProtocolUsenet
		id, err := server.QueueRepo.Enqueue(it)
		if err != nil {
			t.Fatal(err)
		}
		if err := server.QueueRepo.SetStatus(id, status, errText); err != nil {
			t.Fatal(err)
		}
		return id
	}
	enqueue(queue.Item{MovieID: movie.ID, ReleaseTitle: "Night.Harbour.2021.1080p.BluRay.x264-GRP", SizeBytes: 4000}, queue.StatusCompleted, "")
	enqueue(queue.Item{MovieID: movie.ID, ReleaseTitle: "Night.Harbour.2021.720p.WEB-DL.x264-BAD", SizeBytes: 2000}, queue.StatusFailed, "download failed")
	enqueue(queue.Item{SeriesID: show.ID, Season: 1, ReleaseTitle: "Salt.and.Static.S01.1080p.WEB-DL.x264-GRP", SizeBytes: 3000}, queue.StatusCompleted, "")

	// Radarr: the envelope, newest first, only imports when asked for 3.
	h := obj(arrCall(t, "GET", radarr+"/history?page=1&pageSize=20&sortKey=date&sortDirection=descending&eventType=3&includeMovie=true", key, nil, http.StatusOK))
	recs := list(h["records"])
	if h["page"] != float64(1) || h["pageSize"] != float64(20) || h["totalRecords"] != float64(1) || len(recs) != 1 {
		t.Fatalf("radarr history: %v", h)
	}
	r := obj(recs[0])
	for _, k := range []string{"id", "movieId", "sourceTitle", "quality", "customFormats", "languages", "date", "eventType", "data", "downloadId", "movie"} {
		if r[k] == nil {
			t.Errorf("radarr history record misses %q: %v", k, r)
		}
	}
	if r["eventType"] != "downloadFolderImported" || r["sourceTitle"] != "Night.Harbour.2021.1080p.BluRay.x264-GRP" || obj(r["movie"])["title"] != "Night Harbour" {
		t.Errorf("radarr history record: %v", r)
	}
	if obj(obj(r["quality"])["quality"])["name"] != "Bluray-1080p" {
		t.Errorf("quality: %v", r["quality"])
	}
	// Without a filter: 2 grabbed + 1 imported + 1 failed, paged.
	h = obj(arrCall(t, "GET", radarr+"/history?page=1&pageSize=3", key, nil, http.StatusOK))
	if h["totalRecords"] != float64(4) || len(list(h["records"])) != 3 {
		t.Fatalf("radarr history paging: %v", h)
	}
	for _, rec := range list(h["records"]) {
		// The movie is always there: an app that forgets includeMovie still needs the poster.
		if obj(rec)["movie"] == nil {
			t.Error("a history line came without its movie")
		}
	}
	if f := obj(arrCall(t, "GET", radarr+"/history?eventType=4&eventType=1", key, nil, http.StatusOK)); f["totalRecords"] != float64(3) {
		t.Fatalf("grabbed and failed: %v", f)
	}
	if got := list(arrCall(t, "GET", fmt.Sprintf("%s/history/movie?movieId=%d&eventType=4", radarr, movie.ID), key, nil, http.StatusOK)); len(got) != 1 || obj(got[0])["eventType"] != "downloadFailed" {
		t.Fatalf("history/movie: %v", got)
	}
	if got := list(arrCall(t, "GET", radarr+"/history/since?date=2000-01-01T00:00:00Z&eventType=1", key, nil, http.StatusOK)); len(got) != 2 {
		t.Fatalf("history/since: %v", got)
	}
	if got := list(arrCall(t, "GET", radarr+"/history/since?date=2999-01-01T00:00:00Z", key, nil, http.StatusOK)); len(got) != 0 {
		t.Fatalf("history/since in the future: %v", got)
	}

	// Sonarr: the season pack is one line per episode.
	sh := obj(arrCall(t, "GET", sonarr+"/history?pageSize=50&eventType=3&includeSeries=true&includeEpisode=true", key, nil, http.StatusOK))
	srecs := list(sh["records"])
	if sh["totalRecords"] != float64(2) || len(srecs) != 2 {
		t.Fatalf("sonarr history: %v", sh)
	}
	s0 := obj(srecs[0])
	if s0["seriesId"] == nil || s0["episodeId"] == nil || obj(s0["series"])["title"] != "Salt & Static" || obj(s0["episode"])["seasonNumber"] != float64(1) {
		t.Errorf("sonarr history record: %v", s0)
	}
	if got := list(arrCall(t, "GET", fmt.Sprintf("%s/history/series?seriesId=%d&seasonNumber=1&eventType=1", sonarr, show.ID), key, nil, http.StatusOK)); len(got) != 2 {
		t.Fatalf("history/series: %v", got)
	}
	if got := list(arrCall(t, "GET", fmt.Sprintf("%s/history/since?date=2000-01-01&episodeId=%d", sonarr, eps[1].ID), key, nil, http.StatusOK)); len(got) != 2 {
		t.Fatalf("history of one episode: %v", got)
	}

	// Files.
	ef := list(arrCall(t, "GET", fmt.Sprintf("%s/episodefile?seriesId=%d", sonarr, show.ID), key, nil, http.StatusOK))
	if len(ef) != 1 || obj(ef[0])["relativePath"] != "S01E01.mkv" || obj(ef[0])["size"] != float64(5) || obj(ef[0])["seasonNumber"] != float64(1) {
		t.Fatalf("episodefile: %v", ef)
	}
	arrCall(t, "GET", fmt.Sprintf("%s/episodefile/%d", sonarr, eps[0].ID), key, nil, http.StatusOK)
	arrCall(t, "GET", fmt.Sprintf("%s/episodefile/%d", sonarr, eps[1].ID), key, nil, http.StatusNotFound)
	cal := list(arrCall(t, "GET", sonarr+"/calendar?start=2019-01-01&end=2019-12-31&includeSeries=true&includeEpisodeFile=true", key, nil, http.StatusOK))
	if len(cal) != 2 || obj(obj(cal[0])["episodeFile"])["relativePath"] != "S01E01.mkv" || obj(cal[1])["episodeFile"] != nil {
		t.Fatalf("sonarr calendar: %v", cal)
	}
	arrCall(t, "GET", fmt.Sprintf("%s/moviefile/%d", radarr, movie.ID), key, nil, http.StatusNotFound)

	// Parse, rename, manual import.
	p := obj(arrCall(t, "GET", radarr+"/parse?title=Night.Harbour.2021.1080p.BluRay.x264-GRP", key, nil, http.StatusOK))
	if obj(p["parsedMovieInfo"])["year"] != float64(2021) || obj(p["movie"])["tmdbId"] != float64(701) {
		t.Fatalf("radarr parse: %v", p)
	}
	sp := obj(arrCall(t, "GET", sonarr+"/parse?title=Salt.and.Static.S01E02.1080p.WEB-DL.x264-GRP", key, nil, http.StatusOK))
	info := obj(sp["parsedEpisodeInfo"])
	if info["seasonNumber"] != float64(1) || len(list(info["episodeNumbers"])) != 1 || len(list(sp["episodes"])) != 1 {
		t.Fatalf("sonarr parse: %v", sp)
	}
	arrCall(t, "GET", radarr+"/parse", key, nil, http.StatusBadRequest)
	for _, u := range []string{radarr + "/rename?movieId=1", sonarr + "/rename?seriesId=1", radarr + "/manualimport?folder=/x", sonarr + "/manualimport?downloadId=a"} {
		if got := arrCall(t, "GET", u, key, nil, http.StatusOK); got == nil || len(list(got)) != 0 {
			t.Errorf("%s: %v", u, got)
		}
	}
	if q := obj(arrCall(t, "GET", radarr+"/queue?page=1&pageSize=10&includeMovie=true", key, nil, http.StatusOK)); q["totalRecords"] != float64(0) || q["records"] == nil {
		t.Errorf("queue envelope: %v", q)
	}
}

// The ways the apps ask Radarr for what was imported: the event as a number or a
// name, 3 and 6 together, repeated or comma-separated. Each import is answered
// once, under the id that was asked for, and the movie comes with its poster.
func TestArrHistoryEventTypeVariants(t *testing.T) {
	server, base, admin, _, _ := familyServer(t)
	key := postJSON[map[string]any](t, admin, base+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	radarr := base + "/radarr/api/v3"

	var ids []int64
	for i, title := range []string{"Alpha Film", "Bravo Film", "Charlie Film"} {
		m, err := server.MovieRepo.Add(library.Movie{TMDBID: 800 + i, Title: title, Year: 2020 + i, Monitored: true, PosterPath: fmt.Sprintf("/p%d.jpg", i)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
		id, err := server.QueueRepo.Enqueue(queue.Item{MovieID: m.ID, ReleaseTitle: fmt.Sprintf("%s.%d.1080p.BluRay.x264-GRP", strings.ReplaceAll(title, " ", "."), 2020+i), SizeBytes: 100, NZBURL: "http://127.0.0.1:1/x.nzb", Protocol: queue.ProtocolUsenet})
		if err != nil {
			t.Fatal(err)
		}
		if err := server.QueueRepo.SetStatus(id, queue.StatusCompleted, ""); err != nil {
			t.Fatal(err)
		}
	}
	// A movie that was imported from a folder has no download of its own.
	file := filepath.Join(t.TempDir(), "Delta Film (2023).mkv")
	if err := os.WriteFile(file, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	delta, err := server.MovieRepo.Add(library.Movie{TMDBID: 810, Title: "Delta Film", Year: 2023, Monitored: true, PosterPath: "/p9.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.MovieRepo.SetStatus(delta.ID, library.StatusDownloaded, "WEBDL-1080p", file); err != nil {
		t.Fatal(err)
	}

	count := func(query string) (total int, types map[string]int, first map[string]any) {
		h := obj(arrCall(t, "GET", radarr+"/history?"+query, key, nil, http.StatusOK))
		recs := list(h["records"])
		types = map[string]int{}
		for _, r := range recs {
			types[obj(r)["eventType"].(string)]++
		}
		if len(recs) > 0 {
			first = obj(recs[0])
		}
		return int(h["totalRecords"].(float64)), types, first
	}
	cases := []struct {
		name  string
		query string
		total int
		types map[string]int
	}{
		{"3 as a number", "eventType=3", 4, map[string]int{"downloadFolderImported": 4}},
		{"3 by name", "eventType=downloadFolderImported", 4, map[string]int{"downloadFolderImported": 4}},
		{"3 by name, other case", "eventType=DOWNLOADFOLDERIMPORTED", 4, map[string]int{"downloadFolderImported": 4}},
		{"6 as a number", "eventType=6", 4, map[string]int{"movieFolderImported": 4}},
		{"6 by name", "eventType=movieFolderImported", 4, map[string]int{"movieFolderImported": 4}},
		{"3 and 6 repeated", "eventType=3&eventType=6", 8, map[string]int{"downloadFolderImported": 4, "movieFolderImported": 4}},
		{"3 and 6 in a list", "eventType=3,6", 8, map[string]int{"downloadFolderImported": 4, "movieFolderImported": 4}},
		{"names in a list", "eventType=downloadFolderImported,movieFolderImported", 8, map[string]int{"downloadFolderImported": 4, "movieFolderImported": 4}},
		{"with brackets", "eventType[]=6", 4, map[string]int{"movieFolderImported": 4}},
		{"grabbed", "eventType=grabbed", 3, map[string]int{"grabbed": 3}},
		{"nothing readable asks for everything", "eventType=banana", 7, map[string]int{"grabbed": 3, "downloadFolderImported": 3, "movieFolderImported": 1}},
		{"deleted: none happened", "eventType=5", 0, map[string]int{}},
		{"no filter", "", 7, map[string]int{"grabbed": 3, "downloadFolderImported": 3, "movieFolderImported": 1}},
	}
	for _, c := range cases {
		total, types, _ := count(c.query)
		if total != c.total || fmt.Sprint(types) != fmt.Sprint(c.types) {
			t.Errorf("%s (?%s): total %d, types %v; want %d, %v", c.name, c.query, total, types, c.total, c.types)
		}
	}

	// Paging, sorting and the fields the Dashboard card reads.
	total, _, first := count("page=1&pageSize=2&sortKey=date&sortDirection=descending&eventType=3&eventType=6&includeMovie=true")
	if total != 8 {
		t.Fatalf("total with a page of 2: %d", total)
	}
	for _, k := range []string{"id", "movieId", "sourceTitle", "quality", "customFormats", "languages", "date", "downloadId", "eventType", "data", "movie"} {
		if first[k] == nil {
			t.Errorf("history line misses %q: %v", k, first)
		}
	}
	mv := obj(first["movie"])
	if mv["title"] == nil || mv["year"] == nil || mv["tmdbId"] == nil || mv["sortTitle"] == nil || len(list(mv["images"])) == 0 {
		t.Errorf("the movie in a history line is thin: %v", mv)
	}
	img := obj(list(mv["images"])[0])
	if img["coverType"] != "poster" || img["url"] == "" || img["remoteUrl"] == "" {
		t.Errorf("poster: %v", img)
	}
	if !strings.HasPrefix(first["date"].(string), "20") || obj(first["data"]) == nil {
		t.Errorf("date or data: %v", first)
	}
	if big := obj(arrCall(t, "GET", radarr+"/history?page=1&pageSize=250&eventType=3", key, nil, http.StatusOK)); big["pageSize"] != float64(250) || big["totalRecords"] != float64(4) {
		t.Errorf("a page size over 100: %v", big)
	}
	// Sorting by title, both ways.
	title := func(dir string) string {
		h := obj(arrCall(t, "GET", radarr+"/history?eventType=3&sortKey=movieMetadata.sortTitle&sortDirection="+dir, key, nil, http.StatusOK))
		if h["sortKey"] != "movieMetadata.sortTitle" {
			t.Errorf("sortKey echoed as %v", h["sortKey"])
		}
		return obj(obj(list(h["records"])[0])["movie"])["title"].(string)
	}
	if a, d := title("ascending"), title("descending"); a != "Alpha Film" || d != "Delta Film" {
		t.Errorf("sorted by title: first %q ascending, %q descending", a, d)
	}
	// Only some movies.
	if got, _, _ := count(fmt.Sprintf("movieIds=%d&movieIds=%d&eventType=3", ids[0], ids[1])); got != 2 {
		t.Errorf("movieIds: %d", got)
	}
	if got, _, _ := count(fmt.Sprintf("movieIds=%d,%d&eventType=6", delta.ID, ids[0])); got != 2 {
		t.Errorf("movieIds as a list: %d", got)
	}
	if got := list(arrCall(t, "GET", fmt.Sprintf("%s/history/movie?movieId=%d&eventType=6", radarr, delta.ID), key, nil, http.StatusOK)); len(got) != 1 || obj(got[0])["eventType"] != "movieFolderImported" {
		t.Errorf("history/movie for the folder import: %v", got)
	}
	// The same line is not repeated across ids (each is unique).
	h := obj(arrCall(t, "GET", radarr+"/history?pageSize=100&eventType=3,6", key, nil, http.StatusOK))
	seen := map[float64]bool{}
	for _, r := range list(h["records"]) {
		id := obj(r)["id"].(float64)
		if seen[id] {
			t.Errorf("history id %v is repeated", id)
		}
		seen[id] = true
	}
}
