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

// What nzb360's home screens ask for after the 3.7.1 history change: the
// movie card's history (by movie id, ids a phone app can read), the recent
// lists, the missing and recommended movies, the credits, the show's pictures
// on every episode list, and the /MediaCover addresses its images point to.
func TestArrHomeScreensRound4(t *testing.T) {
	server, base, admin, _, _ := familyServer(t)
	server.TestSetTMDBBaseURL("fixture-key", arrTMDB(t).URL)
	key := postJSON[map[string]any](t, admin, base+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	radarr, sonarr := base+"/radarr/api/v3", base+"/sonarr/api/v3"
	dir := t.TempDir()
	file := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	addMovie := func(tmdb int, title string, monitored bool, poster string) library.Movie {
		m, err := server.MovieRepo.Add(library.Movie{TMDBID: tmdb, Title: title, Year: 2021, Monitored: monitored, ReleaseDate: "2021-05-14", PosterPath: poster})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	withFile := func(m library.Movie) {
		if err := server.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, "Bluray-1080p", file(fmt.Sprintf("movie-%d.mkv", m.ID))); err != nil {
			t.Fatal(err)
		}
	}
	downloaded := addMovie(701, "Night Harbour", true, "/nh.jpg") // finished a download of Mediarium's
	withFile(downloaded)
	fileOnly := addMovie(705, "Low Water", true, "/lw.jpg") // came in with a library import
	withFile(fileOnly)
	missing := addMovie(703, "Missing One", true, "/m1.jpg")
	unmonitored := addMovie(704, "Zed Unmonitored", false, "/zu.jpg")

	id, err := server.QueueRepo.Enqueue(queue.Item{MovieID: downloaded.ID, ReleaseTitle: "Night.Harbour.2021.1080p.BluRay.x264-GRP", SizeBytes: 4000, NZBURL: "http://127.0.0.1:1/x.nzb", Protocol: queue.ProtocolUsenet})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.QueueRepo.SetStatus(id, queue.StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}

	// (d) The movie card's history: a download gives its grabbed and import
	// lines, a movie with only a file one import line, and no id is bigger than
	// the 32-bit number the apps read it as.
	smallIDs := func(label string, lines []any) {
		t.Helper()
		for _, l := range lines {
			if id := obj(l)["id"].(float64); id <= 0 || id >= 1<<31 {
				t.Errorf("%s: the id %v does not fit in 32 bits", label, id)
			}
		}
	}
	for _, url := range []string{
		fmt.Sprintf("%s/history/movie?movieId=%d", radarr, downloaded.ID),
		fmt.Sprintf("%s/history/movie?movieId=%d&eventType=1&eventType=3&includeMovie=true", radarr, downloaded.ID),
	} {
		got := list(arrCall(t, "GET", url, key, nil, http.StatusOK))
		if len(got) != 2 || obj(got[0])["movieId"] != float64(downloaded.ID) {
			t.Fatalf("history of a downloaded movie (%s): %v", url, got)
		}
		smallIDs("downloaded movie", got)
	}
	got := list(arrCall(t, "GET", fmt.Sprintf("%s/history/movie?movieId=%d", radarr, fileOnly.ID), key, nil, http.StatusOK))
	if len(got) != 1 || obj(got[0])["eventType"] != "movieFolderImported" || obj(obj(got[0])["movie"])["title"] != "Low Water" {
		t.Fatalf("history of a movie with only a file: %v", got)
	}
	smallIDs("file-only movie", got)
	paged := obj(arrCall(t, "GET", fmt.Sprintf("%s/history?page=1&pageSize=10&movieId=%d&sortKey=date&sortDirection=descending", radarr, fileOnly.ID), key, nil, http.StatusOK))
	if paged["totalRecords"] != float64(1) {
		t.Fatalf("paged history of a movie with only a file: %v", paged)
	}

	// (a) The recent list: newest first, a page size is kept, a start date
	// under any of its names narrows it, and a library import older than a
	// week stays out of a list that is not about one title.
	all := obj(arrCall(t, "GET", radarr+"/history?page=1&pageSize=50&sortKey=date&sortDirection=descending", key, nil, http.StatusOK))
	recs := list(all["records"])
	if all["totalRecords"] != float64(3) || len(recs) != 3 {
		t.Fatalf("radarr history: %v", all)
	}
	smallIDs("history", recs)
	for i := 1; i < len(recs); i++ {
		if obj(recs[i-1])["date"].(string) < obj(recs[i])["date"].(string) {
			t.Errorf("history is not newest first: %v then %v", obj(recs[i-1])["date"], obj(recs[i])["date"])
		}
	}
	if one := list(arrCall(t, "GET", radarr+"/history/since?date=2000-01-01T00:00:00Z&pageSize=1", key, nil, http.StatusOK)); len(one) != 1 {
		t.Errorf("a page size of 1 on /history/since: %v", one)
	}
	for _, name := range []string{"date", "since", "start", "startDate"} {
		if none := list(arrCall(t, "GET", radarr+"/history/since?"+name+"=2999-01-01", key, nil, http.StatusOK)); len(none) != 0 {
			t.Errorf("%s in the future should leave nothing: %v", name, none)
		}
	}

	// (e) Pictures: poster, fanart and the local address next to the TMDB one.
	m := obj(arrCall(t, "GET", fmt.Sprintf("%s/movie/%d", radarr, downloaded.ID), key, nil, http.StatusOK))
	types := map[string]map[string]any{}
	for _, im := range list(m["images"]) {
		types[obj(im)["coverType"].(string)] = obj(im)
	}
	if types["poster"] == nil || types["fanart"] == nil {
		t.Fatalf("movie images: %v", m["images"])
	}
	if types["poster"]["url"] != fmt.Sprintf("/MediaCover/%d/poster.jpg", downloaded.ID) || !strings.HasSuffix(types["poster"]["remoteUrl"].(string), "/nh.jpg") {
		t.Errorf("poster image: %v", types["poster"])
	}

	// (c) Missing movies: monitored, no file, released; the paged envelope
	// with the sort the app sends; monitored=false lists the other ones too.
	miss := obj(arrCall(t, "GET", radarr+"/wanted/missing?page=1&pageSize=20&sortKey=movieMetadata.sortTitle&sortDirection=ascending&monitored=true", key, nil, http.StatusOK))
	mr := list(miss["records"])
	if miss["totalRecords"] != float64(1) || len(mr) != 1 || obj(mr[0])["title"] != "Missing One" || obj(mr[0])["movieFile"] != nil || len(list(obj(mr[0])["images"])) == 0 {
		t.Fatalf("radarr missing: %v", miss)
	}
	if miss["page"] != float64(1) || miss["pageSize"] != float64(20) || miss["sortKey"] != "movieMetadata.sortTitle" || miss["sortDirection"] != "ascending" {
		t.Errorf("radarr missing envelope: %v", miss)
	}
	both := list(obj(arrCall(t, "GET", radarr+"/wanted/missing?monitored=false&sortKey=movieMetadata.sortTitle&sortDirection=descending", key, nil, http.StatusOK))["records"])
	if len(both) != 2 || obj(both[0])["title"] != "Zed Unmonitored" || obj(both[1])["title"] != "Missing One" {
		t.Errorf("radarr missing with unmonitored, descending: %v", both)
	}
	filtered := list(arrCall(t, "GET", radarr+"/movie?monitored=true&hasFile=false", key, nil, http.StatusOK))
	if len(filtered) != 1 || obj(filtered[0])["id"] != float64(missing.ID) {
		t.Errorf("/movie?monitored=true&hasFile=false: %v", filtered)
	}
	_ = unmonitored

	// (c) Recommended: trending movies not in the library, in the lookup shape
	// with the flags and pictures.
	recsList := list(arrCall(t, "GET", radarr+"/importlist/movie?includeRecommendations=true", key, nil, http.StatusOK))
	if len(recsList) != 1 { // 701 is owned, 702 stays
		t.Fatalf("recommendations: %v", recsList)
	}
	r0 := obj(recsList[0])
	if r0["tmdbId"] != float64(702) || r0["isRecommendation"] != true || r0["isExisting"] != false || r0["remotePoster"] == nil || len(list(r0["images"])) == 0 {
		t.Errorf("a recommendation: %v", r0)
	}
	if im := obj(list(r0["images"])[0]); im["remoteUrl"] == "" || im["coverType"] == "" {
		t.Errorf("recommendation image: %v", im)
	}

	// (d) Credits: cast and crew of the movie, nothing for a movie TMDB does
	// not know and nothing without an id.
	credits := list(arrCall(t, "GET", fmt.Sprintf("%s/credit?movieId=%d", radarr, downloaded.ID), key, nil, http.StatusOK))
	if len(credits) != 3 {
		t.Fatalf("credits: %v", credits)
	}
	cast, director, editor := obj(credits[0]), obj(credits[1]), obj(credits[2])
	if cast["type"] != "cast" || cast["personName"] != "Ada Reef" || cast["character"] != "Captain" || cast["movieMetadataId"] != float64(downloaded.ID) || len(list(cast["images"])) != 1 {
		t.Errorf("cast credit: %v", cast)
	}
	if director["type"] != "crew" || director["job"] != "Director" || director["department"] != "Directing" || editor["department"] != "Editing" {
		t.Errorf("crew credits: %v %v", director, editor)
	}
	for _, url := range []string{fmt.Sprintf("%s/credit?movieId=%d", radarr, fileOnly.ID), radarr + "/credit", radarr + "/credit?movieId=999"} {
		if l, ok := arrCall(t, "GET", url, key, nil, http.StatusOK).([]any); !ok || len(l) != 0 {
			t.Errorf("%s should answer an empty list: %v", url, l)
		}
	}

	// A show with a file that never came through a download, and a missing
	// episode.
	show, err := server.MovieRepo.AddSeries(library.Series{TMDBID: 9001, Title: "Salt & Static", Year: 2019, Monitored: true, PosterPath: "/ss.jpg"}, []library.Episode{
		{Season: 1, Episode: 1, AirDate: "2019-02-01", Title: "Static"}, {Season: 1, Episode: 2, AirDate: "2019-02-08", Title: "Salt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	eps, _ := server.MovieRepo.ListEpisodes(show.ID)
	if err := server.MovieRepo.SetEpisodeStatus(eps[0].ID, library.StatusDownloaded, "WEBDL-1080p", file("S01E01.mkv")); err != nil {
		t.Fatal(err)
	}
	showImages := func(label string, series map[string]any) {
		t.Helper()
		got := map[string]map[string]any{}
		for _, im := range list(series["images"]) {
			got[obj(im)["coverType"].(string)] = obj(im)
		}
		for _, kind := range []string{"poster", "fanart", "banner"} {
			im := got[kind]
			if im == nil || im["url"] != fmt.Sprintf("/MediaCover/%d/%s.jpg", show.ID, kind) || !strings.HasSuffix(im["remoteUrl"].(string), "/ss.jpg") {
				t.Errorf("%s: series %s image: %v", label, kind, im)
			}
		}
	}

	// (b) The show's history has an import line for the episode with its file,
	// with the series (and its pictures) and the episode.
	sh := obj(arrCall(t, "GET", sonarr+"/history?page=1&pageSize=50&sortKey=date&sortDirection=descending&eventType=3&includeSeries=true&includeEpisode=true", key, nil, http.StatusOK))
	srecs := list(sh["records"])
	if sh["totalRecords"] != float64(1) || len(srecs) != 1 {
		t.Fatalf("sonarr history: %v", sh)
	}
	s0 := obj(srecs[0])
	if s0["eventType"] != "downloadFolderImported" || s0["seriesId"] != float64(show.ID) || s0["episodeId"] != float64(eps[0].ID) || obj(s0["episode"])["episodeNumber"] != float64(1) || s0["sourceTitle"] != "S01E01" {
		t.Errorf("sonarr import line: %v", s0)
	}
	smallIDs("sonarr history", srecs)
	showImages("history", obj(s0["series"]))
	if since := list(arrCall(t, "GET", sonarr+"/history/since?date=2000-01-01T00:00:00Z&eventType=3&pageSize=5", key, nil, http.StatusOK)); len(since) != 1 {
		t.Errorf("sonarr history/since: %v", since)
	}
	if none := list(arrCall(t, "GET", sonarr+"/history/since?date=2999-01-01T00:00:00Z", key, nil, http.StatusOK)); len(none) != 0 {
		t.Errorf("sonarr history/since in the future: %v", none)
	}

	// (e) Every list of episodes carries the show with its pictures, and the
	// file when there is one.
	cal := list(arrCall(t, "GET", sonarr+"/calendar?start=2019-01-01&end=2019-12-31", key, nil, http.StatusOK))
	if len(cal) != 2 || obj(obj(cal[0])["episodeFile"])["relativePath"] != "S01E01.mkv" || obj(cal[1])["episodeFile"] != nil {
		t.Fatalf("sonarr calendar: %v", cal)
	}
	showImages("calendar", obj(obj(cal[0])["series"]))
	wm := obj(arrCall(t, "GET", sonarr+"/wanted/missing?page=1&pageSize=10&sortKey=episodes.airDateUtc&sortDirection=descending&includeSeries=true", key, nil, http.StatusOK))
	if wm["totalRecords"] != float64(1) {
		t.Fatalf("sonarr missing: %v", wm)
	}
	showImages("wanted", obj(obj(list(wm["records"])[0])["series"]))
	el := list(arrCall(t, "GET", fmt.Sprintf("%s/episode?seriesId=%d&includeEpisodeFile=true", sonarr, show.ID), key, nil, http.StatusOK))
	if len(el) != 2 || obj(obj(el[0])["episodeFile"])["relativePath"] != "S01E01.mkv" {
		t.Fatalf("sonarr episodes: %v", el)
	}
	showImages("episode list", obj(obj(el[1])["series"]))
	showImages("one episode", obj(obj(arrCall(t, "GET", fmt.Sprintf("%s/episode/%d", sonarr, eps[1].ID), key, nil, http.StatusOK))["series"]))
	showImages("series", obj(arrCall(t, "GET", fmt.Sprintf("%s/series/%d", sonarr, show.ID), key, nil, http.StatusOK)))

	// The local addresses answer under the app's own prefix and under its API,
	// in any casing and with a size, by sending the app to the TMDB picture.
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cover := func(url, apiKey string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", url, nil)
		if apiKey != "" {
			req.Header.Set("X-Api-Key", apiKey)
		}
		resp, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	for _, c := range []struct{ url, want string }{
		{fmt.Sprintf("%s/radarr/MediaCover/%d/poster-250.jpg", base, downloaded.ID), "/t/p/w342/nh.jpg"},
		{fmt.Sprintf("%s/radarr/MediaCover/%d/poster.jpg", base, downloaded.ID), "/t/p/w500/nh.jpg"},
		{fmt.Sprintf("%s/radarr/api/v3/mediacover/%d/fanart-1280.jpg", base, downloaded.ID), "/t/p/w500/nh.jpg"}, // no wide picture known: the poster
		{fmt.Sprintf("%s/sonarr/MediaCover/%d/banner.jpg", base, show.ID), "/t/p/w500/ss.jpg"},
		{fmt.Sprintf("%s/sonarr/MediaCover/%d/poster-500.jpg?apikey=%s", base, show.ID, key), "/t/p/w500/ss.jpg"},
	} {
		resp := cover(c.url, key)
		if resp.StatusCode != http.StatusFound || !strings.HasSuffix(resp.Header.Get("Location"), c.want) || !strings.Contains(resp.Header.Get("Cache-Control"), "max-age") {
			t.Errorf("%s: status %d, location %q, cache %q", c.url, resp.StatusCode, resp.Header.Get("Location"), resp.Header.Get("Cache-Control"))
		}
	}
	if resp := cover(fmt.Sprintf("%s/radarr/MediaCover/%d/poster.jpg", base, downloaded.ID), ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a cover without a key: %d", resp.StatusCode)
	}
	if resp := cover(base+"/radarr/MediaCover/99999/poster.jpg", key); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a cover of nothing: %d", resp.StatusCode)
	}
}
