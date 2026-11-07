package api_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func rawGet(t *testing.T, client *http.Client, url, etag string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	var buf []byte
	buf = make([]byte, 0, 512)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return resp, buf
}

func TestLibraryOwnedIsSlimSharedWithMembersAndRevalidates(t *testing.T) {
	server, base, admin, member, _ := familyServer(t)
	tmdb, _ := newFamilyTMDB(t)
	server.TestSetTMDBBaseURL("k", tmdb.URL)
	postJSON[map[string]any](t, member, base+"/api/movies", map[string]any{"tmdbId": 603}, http.StatusCreated)
	postJSON[map[string]any](t, member, base+"/api/movies", map[string]any{"tmdbId": 604}, http.StatusCreated)
	postJSON[map[string]any](t, member, base+"/api/series", map[string]any{"tmdbId": 1399}, http.StatusCreated)

	movies := getJSON[[]map[string]any](t, admin, base+"/api/movies")
	shows := getJSON[[]map[string]any](t, admin, base+"/api/series")
	if len(movies) != 2 || len(shows) != 1 {
		t.Fatalf("setup: %d movies, %d shows", len(movies), len(shows))
	}
	m603 := int64(movies[0]["id"].(float64))
	if movies[0]["tmdbId"] != float64(603) {
		m603 = int64(movies[1]["id"].(float64))
	}
	showID := int64(shows[0]["id"].(float64))
	exec := func(q string, args ...any) {
		t.Helper()
		if err := server.TestExec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO download_queue (movie_id, release_title, nzb_url, size_bytes, status, progress_pct) VALUES (?, 'a', 'http://example.invalid/a.nzb', 0, 'downloading', 40)`, m603)
	exec(`INSERT INTO download_queue (movie_id, release_title, nzb_url, size_bytes, status, progress_pct) VALUES (?, 'b', 'http://example.invalid/b.nzb', 0, 'failed', 0)`, m603)
	exec(`INSERT INTO download_queue (movie_id, release_title, nzb_url, size_bytes, status, progress_pct) VALUES (?, 'c', 'http://example.invalid/c.nzb', 0, 'completed', 100)`, m603)
	exec(`INSERT INTO download_queue (series_id, season, episode, release_title, nzb_url, size_bytes, status) VALUES (?, 1, 1, 'd', 'http://example.invalid/d.nzb', 0, 'queued')`, showID)

	type owned struct {
		MovieCount int `json:"movieCount"`
		ShowCount  int `json:"showCount"`
		Movies     []struct {
			ID        int64  `json:"id"`
			TMDBID    int    `json:"tmdbId"`
			Status    string `json:"status"`
			Monitored bool   `json:"monitored"`
		} `json:"movies"`
		Shows []struct {
			ID              int64 `json:"id"`
			TMDBID          int   `json:"tmdbId"`
			Monitored       bool  `json:"monitored"`
			EpisodeCount    int   `json:"episodeCount"`
			DownloadedCount int   `json:"downloadedCount"`
		} `json:"shows"`
		Queue []struct {
			MovieID     int64   `json:"movieId"`
			SeriesID    int64   `json:"seriesId"`
			Status      string  `json:"status"`
			ProgressPct float64 `json:"progressPct"`
		} `json:"queue"`
	}
	got := getJSON[owned](t, member, base+"/api/library/owned")
	if got.MovieCount != 2 || got.ShowCount != 1 || len(got.Movies) != 2 || len(got.Shows) != 1 {
		t.Fatalf("owned: %+v", got)
	}
	// The same answer as the full lists, field by field.
	for _, o := range got.Movies {
		var full map[string]any
		for _, m := range movies {
			if int64(m["id"].(float64)) == o.ID {
				full = m
			}
		}
		if full == nil || full["tmdbId"] != float64(o.TMDBID) || full["status"] != o.Status || full["monitored"] != o.Monitored {
			t.Fatalf("owned movie %+v differs from the list entry %+v", o, full)
		}
	}
	sh := got.Shows[0]
	if sh.ID != showID || sh.TMDBID != 1399 || sh.EpisodeCount != int(shows[0]["episodeCount"].(float64)) ||
		sh.DownloadedCount != int(shows[0]["downloadedCount"].(float64)) || sh.Monitored != shows[0]["monitored"] {
		t.Fatalf("owned show %+v differs from the list entry %+v", sh, shows[0])
	}
	// One line per title and status, finished downloads left out.
	if len(got.Queue) != 3 {
		t.Fatalf("queue summary: %+v", got.Queue)
	}
	var sawDownloading, sawFailed, sawQueued bool
	for _, q := range got.Queue {
		switch {
		case q.MovieID == m603 && q.Status == "downloading" && q.ProgressPct == 40:
			sawDownloading = true
		case q.MovieID == m603 && q.Status == "failed":
			sawFailed = true
		case q.SeriesID == showID && q.Status == "queued":
			sawQueued = true
		}
	}
	if !sawDownloading || !sawFailed || !sawQueued {
		t.Fatalf("queue summary missing a line: %+v", got.Queue)
	}

	// Members and administrators see the same, and it carries nothing but the
	// listed fields (no paths, names or accounts).
	_, memberBody := rawGet(t, member, base+"/api/library/owned", "")
	resp, adminBody := rawGet(t, admin, base+"/api/library/owned", "")
	if string(memberBody) != string(adminBody) {
		t.Fatalf("members and administrators got different answers:\n%s\n%s", memberBody, adminBody)
	}
	var generic map[string]any
	if err := json.Unmarshal(adminBody, &generic); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ownedKeys(generic), []string{"movieCount", "movies", "queue", "showCount", "shows"}) {
		t.Fatalf("unexpected top-level fields: %v", ownedKeys(generic))
	}
	for _, m := range generic["movies"].([]any) {
		if !reflect.DeepEqual(ownedKeys(m.(map[string]any)), []string{"id", "monitored", "status", "tmdbId"}) {
			t.Fatalf("unexpected movie fields: %v", ownedKeys(m.(map[string]any)))
		}
	}

	// Asking again with the tag costs a 304 and no body.
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("the answer has no ETag")
	}
	resp2, body2 := rawGet(t, member, base+"/api/library/owned", etag)
	if resp2.StatusCode != http.StatusNotModified || len(body2) != 0 {
		t.Fatalf("with the tag: status %d, %d bytes", resp2.StatusCode, len(body2))
	}

	// A change shows at once (the kept answer is dropped when the library
	// changes) and the old tag no longer matches.
	exec(`UPDATE movies SET status = 'downloaded' WHERE id = ?`, m603)
	resp3, body3 := rawGet(t, member, base+"/api/library/owned", etag)
	if resp3.StatusCode != http.StatusOK || string(body3) == string(adminBody) {
		t.Fatalf("after a change: status %d, same body %v", resp3.StatusCode, string(body3) == string(adminBody))
	}
	after := getJSON[owned](t, member, base+"/api/library/owned")
	for _, o := range after.Movies {
		if o.ID == m603 && o.Status != "downloaded" {
			t.Fatalf("status after the change: %s", o.Status)
		}
	}
}

func ownedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func TestQueueCountMatchesTheActiveQueue(t *testing.T) {
	server, base, admin, member, _ := familyServer(t)
	tmdb, _ := newFamilyTMDB(t)
	server.TestSetTMDBBaseURL("k", tmdb.URL)
	postJSON[map[string]any](t, member, base+"/api/movies", map[string]any{"tmdbId": 603}, http.StatusCreated)
	movies := getJSON[[]map[string]any](t, admin, base+"/api/movies")
	id := int64(movies[0]["id"].(float64))
	for i, status := range []string{"queued", "downloading", "importing", "paused", "failed", "completed"} {
		if err := server.TestExec(`INSERT INTO download_queue (movie_id, release_title, nzb_url, size_bytes, status) VALUES (?, ?, ?, 0, ?)`,
			id, "r"+status, "http://example.invalid/"+status+".nzb", status); err != nil {
			t.Fatalf("row %d: %v", i, err)
		}
	}
	type count struct{ Active, Queued, Downloading, Importing int }
	for _, c := range []*http.Client{admin, member} {
		got := getJSON[count](t, c, base+"/api/queue/count")
		if got != (count{Active: 3, Queued: 1, Downloading: 1, Importing: 1}) {
			t.Fatalf("count = %+v", got)
		}
		active := getJSON[[]map[string]any](t, c, base+"/api/queue?active=1")
		if len(active) != got.Active {
			t.Fatalf("the active queue has %d rows, the count says %d", len(active), got.Active)
		}
	}
}
