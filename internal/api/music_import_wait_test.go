package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// busyMusicBrainz sits in front of the fake MusicBrainz and answers "busy"
// to the track list requests while failures are left.
type busyMusicBrainz struct {
	*httptest.Server
	failures atomic.Int32 // track list requests still to be refused
	status   atomic.Int32 // what they are refused with
	listHits atomic.Int32 // track list requests seen, refused or not
}

func newBusyMusicBrainz(t *testing.T, backend string) *busyMusicBrainz {
	t.Helper()
	target, err := url.Parse(backend)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	b := &busyMusicBrainz{}
	b.status.Store(http.StatusServiceUnavailable)
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/ws/2/release") && !strings.HasPrefix(r.URL.Path, "/ws/2/release-group") {
			b.listHits.Add(1)
			if b.failures.Add(-1) >= 0 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(int(b.status.Load()))
				return
			}
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(b.Close)
	return b
}

// musicDownload is an album with a release ready to be grabbed from a fake
// indexer and Usenet server, and a MusicBrainz that can be made busy.
type musicDownload struct {
	*musicEnv
	mb       *busyMusicBrainz
	idx      *musicIndexer
	albumURL string
	albumID  float64
}

func newMusicDownload(t *testing.T) *musicDownload {
	t.Helper()
	e := newMusicEnv(t)
	e.useLosslessByDefault(t)
	artist := e.addArtist(t, "none") // while MusicBrainz still answers
	first := albumByTitle(t, artist, "First Album")
	d := &musicDownload{musicEnv: e, albumURL: fmt.Sprintf("%s/api/music/albums/%v", e.base, first["id"]), albumID: first["id"].(float64)}

	d.mb = newBusyMusicBrainz(t, e.mbURL)
	e.server.TestSetMusicBrainz(d.mb.URL)

	articles := map[string]nntpArticle{
		"t1@x":  {fileName: "01 - Opening.flac", content: []byte("flac one " + strings.Repeat("a", 300))},
		"t2@x":  {fileName: "02 - Middle Song.flac", content: []byte("flac two " + strings.Repeat("b", 300))},
		"t3@x":  {fileName: "03 - Closing Time.flac", content: []byte("flac three " + strings.Repeat("c", 300))},
		"art@x": {fileName: "cover.jpg", content: []byte("jpeg bytes")},
	}
	host, port := newArticleNNTPServer(t, articles)
	d.idx = newMusicIndexer(t, []string{"Fixture Band - First Album (1999) [FLAC]"}, nzbFor(articles))
	postJSON[map[string]any](t, e.client, e.base+"/api/indexers", map[string]any{"name": "Music Indexer", "definitionId": "x", "baseUrl": d.idx.URL, "apiKey": "k", "protocol": "usenet"}, http.StatusCreated)
	postJSON[map[string]any](t, e.client, e.base+"/api/usenet-servers", map[string]any{"name": "Fixture Usenet", "host": host, "port": port, "useSsl": false, "connections": 2}, http.StatusCreated)
	return d
}

// grab downloads the release and waits for the pipeline to end.
func (d *musicDownload) grab(t *testing.T) {
	t.Helper()
	now := postJSON[map[string]any](t, d.client, d.albumURL+"/search-now", nil, http.StatusOK)
	if now["grabbed"] != float64(1) {
		t.Fatalf("search now: %v", now)
	}
	waitBackground(t, d.server)
}

func (d *musicDownload) queue(t *testing.T) []map[string]any {
	t.Helper()
	return getJSON[[]map[string]any](t, d.client, d.base+"/api/queue")
}

// onlyItem is the one download in the list.
func (d *musicDownload) onlyItem(t *testing.T) map[string]any {
	t.Helper()
	q := d.queue(t)
	if len(q) != 1 {
		t.Fatalf("want one download in the list, got %v", q)
	}
	return q[0]
}

func (d *musicDownload) albumStatus(t *testing.T) string {
	t.Helper()
	return getJSON[map[string]any](t, d.client, d.albumURL)["status"].(string)
}

func (d *musicDownload) wantImported(t *testing.T, id float64) {
	t.Helper()
	item := d.onlyItem(t)
	if item["id"] != id || item["status"] != "completed" || item["waiting"] != nil || item["error"] != nil {
		t.Fatalf("the same download should be done: %v", item)
	}
	if got := d.albumStatus(t); got != "downloaded" {
		t.Fatalf("album: %s", got)
	}
	folder := filepath.Join(d.musicRoot, "Fixture Band", "First Album (1999)")
	if _, err := os.Stat(filepath.Join(folder, "02 - Middle Song.flac")); err != nil {
		t.Fatalf("the files should be in the album folder: %v", err)
	}
}

const waitingNote = "MusicBrainz is busy. Mediarium will try again later."

// A busy MusicBrainz during the first import: a few refusals are waited out
// inside the same import; a longer spell leaves the download waiting, with
// its files, and the import (never the download) runs again by itself or on
// Retry.
func TestMusicImportSurvivesAnUnavailableMusicBrainz(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		t.Run(fmt.Sprintf("short spell of %d answers is waited out", status), func(t *testing.T) {
			d := newMusicDownload(t)
			d.mb.status.Store(int32(status))
			d.mb.failures.Store(3)
			d.grab(t)
			item := d.onlyItem(t)
			d.wantImported(t, item["id"].(float64))
			if d.mb.listHits.Load() != 3+2 { // three refusals, then the release list and the release
				t.Fatalf("track list requests: %d", d.mb.listHits.Load())
			}
		})
	}

	t.Run("long spell leaves the download waiting and the scheduled try finishes it", func(t *testing.T) {
		d := newMusicDownload(t)
		d.mb.failures.Store(1000)
		d.grab(t)
		downloads := d.idx.nzbHits.Load()
		item := d.onlyItem(t)
		id := item["id"].(float64)
		if item["status"] != "importing" || item["waiting"] != true || item["note"] != waitingNote || item["error"] != nil {
			t.Fatalf("the download should wait for MusicBrainz, not fail: %v", item)
		}
		if got := d.albumStatus(t); got != "downloading" {
			t.Fatalf("the album stays claimed while it waits: %s", got)
		}
		if strings.Contains(fmt.Sprint(item["note"]), "503") {
			t.Fatal("the note is plain words")
		}

		// A try while MusicBrainz is still busy changes nothing.
		d.server.TestRunMusicWaits()
		if again := d.onlyItem(t); again["id"] != id || again["waiting"] != true {
			t.Fatalf("still waiting: %v", again)
		}

		d.mb.failures.Store(0)
		d.server.TestRunMusicWaits()
		d.wantImported(t, id)
		if d.idx.nzbHits.Load() != downloads {
			t.Fatalf("the release was downloaded again: %d -> %d", downloads, d.idx.nzbHits.Load())
		}
		var waited, imported bool
		for _, ev := range getJSON[[]map[string]any](t, d.client, d.albumURL+"/events") {
			waited = waited || ev["kind"] == "waiting"
			imported = imported || ev["kind"] == "imported"
		}
		if !waited || !imported {
			t.Fatalf("the album's log should say it waited and then imported (waited %v, imported %v)", waited, imported)
		}
	})

	t.Run("Retry on the waiting row runs the import again at once, without a new download", func(t *testing.T) {
		d := newMusicDownload(t)
		d.mb.failures.Store(1000)
		d.grab(t)
		downloads := d.idx.nzbHits.Load()
		id := d.onlyItem(t)["id"].(float64)
		retry := fmt.Sprintf("%s/api/queue/%v/retry", d.base, id)

		// MusicBrainz still busy: Retry tries, and the row keeps waiting.
		hits := d.mb.listHits.Load()
		got := postJSON[map[string]any](t, d.client, retry, nil, http.StatusAccepted)
		waitBackground(t, d.server)
		if got["queueId"] != id || d.mb.listHits.Load() == hits {
			t.Fatalf("retry should try the import itself again: %v (requests %d -> %d)", got, hits, d.mb.listHits.Load())
		}
		if item := d.onlyItem(t); item["id"] != id || item["waiting"] != true {
			t.Fatalf("still waiting: %v", item)
		}

		// MusicBrainz back after two refusals: Retry finishes the import.
		d.mb.failures.Store(2)
		postJSON[map[string]any](t, d.client, retry, nil, http.StatusAccepted)
		waitBackground(t, d.server)
		d.wantImported(t, id)
		if d.idx.nzbHits.Load() != downloads {
			t.Fatalf("retry downloaded the release again: %d -> %d", downloads, d.idx.nzbHits.Load())
		}
	})

	t.Run("after the waiting is over the row fails, and Retry still only imports", func(t *testing.T) {
		d := newMusicDownload(t)
		d.server.TestSetMusicWaits(time.Minute, time.Nanosecond)
		d.mb.failures.Store(1000)
		d.grab(t)
		downloads := d.idx.nzbHits.Load()
		id := d.onlyItem(t)["id"].(float64)

		d.server.TestRunMusicWaits() // the day is up
		item := d.onlyItem(t)
		if item["id"] != id || item["status"] != "failed" || item["waiting"] != nil ||
			item["error"] != "MusicBrainz stayed busy, so this album wasn't imported. The downloaded files are kept. Press Retry to try the import again." {
			t.Fatalf("the row should fail in plain words: %v", item)
		}
		if got := d.albumStatus(t); got != "missing" {
			t.Fatalf("a failed import frees the album: %s", got)
		}

		d.mb.failures.Store(0)
		got := postJSON[map[string]any](t, d.client, fmt.Sprintf("%s/api/queue/%v/retry", d.base, id), nil, http.StatusAccepted)
		waitBackground(t, d.server)
		if got["queueId"] != id {
			t.Fatalf("retry should reuse the same row: %v", got)
		}
		d.wantImported(t, id)
		if d.idx.nzbHits.Load() != downloads {
			t.Fatalf("retry downloaded the release again: %d -> %d", downloads, d.idx.nzbHits.Load())
		}
	})

	t.Run("a waiting download can be stopped, and the waiting ends", func(t *testing.T) {
		d := newMusicDownload(t)
		d.mb.failures.Store(1000)
		d.grab(t)
		id := d.onlyItem(t)["id"].(float64)
		postJSON[map[string]any](t, d.client, fmt.Sprintf("%s/api/queue/%v/stop", d.base, id), nil, http.StatusOK)
		hits := d.mb.listHits.Load()
		d.server.TestRunMusicWaits()
		if item := d.onlyItem(t); item["status"] != "stopped" || d.mb.listHits.Load() != hits {
			t.Fatalf("a stopped download is left alone: %v", item)
		}
		if got := d.albumStatus(t); got != "missing" {
			t.Fatalf("album: %s", got)
		}
	})
}
