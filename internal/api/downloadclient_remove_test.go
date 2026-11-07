package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/api"
	"github.com/rdborg/mediarium/internal/library"
)

// removeEnv is a client environment with a torrent that keeps downloading.
type removeEnv struct {
	*clientEnv
	movie library.Movie
	id    int64
}

// startHeld grabs a magnet and waits until Mediarium has handed it to the
// client and is following it (the torrent never finishes).
func startHeld(t *testing.T, body map[string]any) *removeEnv {
	t.Helper()
	e := newClientEnv(t)
	e.qb.hold = true
	e.save(t, body)
	m, err := e.server.MovieRepo.Add(library.Movie{TMDBID: 603, Title: "The Fixture Movie", Year: 1999, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	id, err := e.server.TestGrabTorrentAs(m, "The.Fixture.Movie.1999.1080p.WEB-DL.x264-FIXTURE", "magnet:?xt=urn:btih:"+fixtureHash+"&dn=fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	r := &removeEnv{clientEnv: e, movie: m, id: id}
	// A test that fails (t.Fatal) before it stops the download would leave the
	// download polling for ever, and the cleanup that waits for every download
	// to end would hang. Cleanups run last in, first out, so this runs before
	// that wait. Stopping a download that already ended is harmless here.
	t.Cleanup(func() {
		if resp, err := r.client.Post(fmt.Sprintf("%s/api/queue/%d/stop", r.url, r.id), "application/json", strings.NewReader("{}")); err == nil {
			resp.Body.Close()
		}
	})
	// The status checks before the torrent is added (is it there already?) also
	// count in infos, so wait for two listings of the torrent itself: the second
	// is only asked for once the first look at it has been dealt with. A test
	// that changes the torrent's state afterwards then changes it for a download
	// that is past its first look.
	r.waitFor(t, "the torrent to be added and followed", func() bool {
		e.qb.mu.Lock()
		defer e.qb.mu.Unlock()
		return len(e.qb.tags) == 1 && e.qb.polls >= 2
	})
	return r
}

func (r *removeEnv) waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (r *removeEnv) deleted() []string {
	r.server.TestWaitBackground()
	r.qb.mu.Lock()
	defer r.qb.mu.Unlock()
	return append([]string(nil), r.qb.deleted...)
}

func (r *removeEnv) call(t *testing.T, method, path string, body any, want int) {
	t.Helper()
	postJSONMethod[map[string]any](t, r.client, method, r.url+path, body, want)
}

func (r *removeEnv) wantRemoved(t *testing.T) {
	t.Helper()
	r.waitFor(t, "the torrent to be removed from the client", func() bool {
		r.qb.mu.Lock()
		defer r.qb.mu.Unlock()
		return len(r.qb.deleted) > 0
	})
	got := r.deleted()
	if len(got) != 1 || got[0] != fixtureHash+" true" {
		t.Fatalf("the client was asked to delete %v, want the torrent with its files", got)
	}
}

func (r *removeEnv) wantUntouched(t *testing.T) {
	t.Helper()
	if got := r.deleted(); len(got) != 0 {
		t.Fatalf("the client was asked to delete %v, want nothing", got)
	}
}

// Stopping a download that is still downloading takes the torrent out of the
// client, with what it downloaded.
func TestStopRemovesTheTorrentFromTheClient(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	r := startHeld(t, map[string]any{})
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/stop", r.id), map[string]any{}, http.StatusOK)
	r.wantRemoved(t)
}

// A paused download that is then removed from the list goes the same way.
func TestRemovingAPausedDownloadRemovesTheTorrent(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	r := startHeld(t, map[string]any{})
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/pause", r.id), map[string]any{}, http.StatusOK)
	r.wantUntouched(t) // paused is not given up on
	r.call(t, http.MethodDelete, fmt.Sprintf("/api/queue/%d", r.id), nil, http.StatusOK)
	r.wantRemoved(t)
}

// Removing the movie cancels its download, torrent included.
func TestRemovingTheTitleRemovesTheTorrent(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	r := startHeld(t, map[string]any{})
	r.call(t, http.MethodDelete, fmt.Sprintf("/api/movies/%d", r.movie.ID), nil, http.StatusOK)
	r.wantRemoved(t)
}

// Blocklist & search again throws the release away, whatever state it is in.
func TestBlocklistRemovesTheTorrent(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	r := startHeld(t, map[string]any{})
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/pause", r.id), map[string]any{}, http.StatusOK)
	r.wantUntouched(t)
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/blocklist", r.id), map[string]any{}, http.StatusOK)
	r.wantRemoved(t)
}

// The switch turns all of it off.
func TestStopLeavesTheTorrentWhenTheSettingIsOff(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	r := startHeld(t, map[string]any{"removeOnStop": false})
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/stop", r.id), map[string]any{}, http.StatusOK)
	r.wantUntouched(t)
	r.call(t, http.MethodDelete, fmt.Sprintf("/api/queue/%d", r.id), nil, http.StatusOK)
	r.wantUntouched(t)
}

// A client that is down or has lost the torrent never stops the removal.
func TestClientErrorDoesNotBlockTheRemoval(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	r := startHeld(t, map[string]any{})
	r.qb.mu.Lock()
	r.qb.failDelete = true
	r.qb.mu.Unlock()
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/stop", r.id), map[string]any{}, http.StatusOK)
	r.call(t, http.MethodDelete, fmt.Sprintf("/api/queue/%d", r.id), nil, http.StatusOK)
	r.server.TestWaitBackground()
	q := getJSON[[]map[string]any](t, r.client, r.url+"/api/queue")
	if len(q) != 0 {
		t.Fatalf("the entry should be gone: %v", q)
	}
}

// A torrent that was in the client before Mediarium sent it is never deleted.
func TestTorrentThatWasThereBeforeIsNotDeleted(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	e := newClientEnv(t)
	e.qb.extra = []map[string]any{{"hash": fixtureHash, "name": "Mine.Already", "progress": 0.4, "state": "downloading", "category": "linux", "added_on": 1}}
	e.save(t, map[string]any{})
	m, _ := e.server.MovieRepo.Add(library.Movie{TMDBID: 603, Title: "The Fixture Movie", Year: 1999, Monitored: true})
	id, err := e.server.TestGrabTorrentAs(m, "The.Fixture.Movie.1999.1080p.WEB-DL.x264-FIXTURE", "magnet:?xt=urn:btih:"+fixtureHash+"&dn=fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	r := &removeEnv{clientEnv: e, movie: m, id: id}
	r.waitFor(t, "the item to be followed", func() bool {
		e.qb.mu.Lock()
		defer e.qb.mu.Unlock()
		return e.qb.infos >= 3
	})
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/stop", id), map[string]any{}, http.StatusOK)
	r.wantUntouched(t)
	e.qb.mu.Lock()
	adds := len(e.qb.tags)
	e.qb.mu.Unlock()
	if adds != 0 {
		t.Fatalf("the torrent was added again: %d", adds)
	}
}

// Only a torrent that carries Mediarium's tag for that download is touched, and
// a torrent that has finished downloading is left to seed unless the release
// is being thrown away.
func TestRemoveFromClientChecksTheTagAndTheProgress(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	other := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	cases := []struct {
		name    string
		torrent map[string]any
		hash    string
		force   bool
		want    int
	}{
		{"no tag", map[string]any{"hash": other, "progress": 0.3, "state": "downloading", "tags": ""}, other, false, 0},
		{"another download's tag", map[string]any{"hash": other, "progress": 0.3, "state": "downloading", "tags": "mediarium-q999"}, other, false, 0},
		{"its own tag, unfinished", map[string]any{"hash": other, "progress": 0.3, "state": "downloading", "tags": "mediarium-q%d"}, other, false, 1},
		{"its own tag, finished and seeding", map[string]any{"hash": other, "progress": 1.0, "state": "uploading", "tags": "mediarium-q%d"}, other, false, 0},
		{"its own tag, finished, thrown away", map[string]any{"hash": other, "progress": 1.0, "state": "uploading", "tags": "mediarium-q%d"}, other, true, 1},
		{"found by tag when the hash is unknown", map[string]any{"hash": other, "progress": 0.3, "state": "downloading", "tags": "mediarium-q%d"}, "", false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newClientEnv(t)
			e.save(t, map[string]any{})
			torrent := map[string]any{}
			for k, v := range tc.torrent {
				torrent[k] = v
			}
			if tag, ok := torrent["tags"].(string); ok && strings.Contains(tag, "%d") {
				torrent["tags"] = fmt.Sprintf(tag, 7)
			}
			e.qb.extra = []map[string]any{torrent}
			e.server.TestRemoveFromClient(7, tc.hash, tc.force)
			e.server.TestWaitBackground()
			e.qb.mu.Lock()
			defer e.qb.mu.Unlock()
			if len(e.qb.deleted) != tc.want {
				t.Errorf("deleted = %v, want %d call(s)", e.qb.deleted, tc.want)
			}
		})
	}
}

// fakeTransmission keeps one torrent that never finishes and records what it is
// told to remove.
type fakeTransmission struct {
	mu      sync.Mutex
	labels  []string
	added   bool
	removed []string // "ids delete-local-data"
}

func (f *fakeTransmission) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Transmission-Session-Id") != "sid-1" {
		w.Header().Set("X-Transmission-Session-Id", "sid-1")
		http.Error(w, "409: Conflict", http.StatusConflict)
		return
	}
	var req struct {
		Method    string         `json:"method"`
		Arguments map[string]any `json:"arguments"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	f.mu.Lock()
	defer f.mu.Unlock()
	args := map[string]any{}
	switch req.Method {
	case "torrent-add":
		f.added = true
		f.labels = nil
		for _, l := range req.Arguments["labels"].([]any) {
			f.labels = append(f.labels, l.(string))
		}
	case "torrent-get":
		list := []map[string]any{}
		if f.added && len(f.removed) == 0 {
			list = append(list, map[string]any{
				"hashString": fixtureHash, "name": "x", "percentDone": 0.5, "status": 4, "peersConnected": 3, "peersSendingToUs": 3,
				"rateDownload": 100, "downloadDir": "/downloads", "totalSize": 1000, "sizeWhenDone": 1000, "labels": f.labels,
			})
		}
		args["torrents"] = list
	case "torrent-remove":
		ids, _ := req.Arguments["ids"].([]any)
		f.removed = append(f.removed, fmt.Sprintf("%v %v", ids, req.Arguments["delete-local-data"]))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"result": "success", "arguments": args})
}

func TestStopRemovesTheTorrentFromTransmission(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	e := newClientEnv(t)
	tm := &fakeTransmission{}
	srv := httptest.NewServer(tm)
	t.Cleanup(srv.Close)
	putJSONStatus(t, e.client, e.url+"/api/download-client", map[string]any{"kind": "transmission", "url": srv.URL}, http.StatusOK)
	m, _ := e.server.MovieRepo.Add(library.Movie{TMDBID: 603, Title: "The Fixture Movie", Year: 1999, Monitored: true})
	id, err := e.server.TestGrabTorrentAs(m, "The.Fixture.Movie.1999.1080p.WEB-DL.x264-FIXTURE", "magnet:?xt=urn:btih:"+fixtureHash+"&dn=fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	r := &removeEnv{clientEnv: e, movie: m, id: id}
	r.waitFor(t, "the torrent to be added and followed", func() bool {
		q := getJSON[[]map[string]any](t, e.client, e.url+"/api/queue")
		tm.mu.Lock()
		defer tm.mu.Unlock()
		return tm.added && len(q) == 1 && q[0]["status"] == "downloading"
	})
	time.Sleep(100 * time.Millisecond)
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/stop", id), map[string]any{}, http.StatusOK)
	r.waitFor(t, "the torrent to be removed from Transmission", func() bool {
		tm.mu.Lock()
		defer tm.mu.Unlock()
		return len(tm.removed) > 0
	})
	e.server.TestWaitBackground()
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if len(tm.removed) != 1 || tm.removed[0] != fmt.Sprintf("[%s] true", fixtureHash) {
		t.Fatalf("removed = %v, want the torrent with its data", tm.removed)
	}
}
