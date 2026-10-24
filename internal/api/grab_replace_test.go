package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/queue"
)

// "Cancel it and grab this one": a grab that names the download in the way
// cancels it and goes ahead in one step; naming anything else is refused and
// leaves the running download alone.
func TestGrabReplacingTheDownloadInTheWay(t *testing.T) {
	server, httpSrv, client := newHuntTestServer(t)
	base := httpSrv.URL
	signUpAdmin(t, client, base)

	m, err := server.MovieRepo.Add(library.Movie{TMDBID: 1, Title: "Some Movie", Year: 2001})
	if err != nil {
		t.Fatal(err)
	}
	other, err := server.MovieRepo.Add(library.Movie{TMDBID: 2, Title: "Other Movie", Year: 2002})
	if err != nil {
		t.Fatal(err)
	}
	busy, err := server.QueueRepo.Enqueue(queue.Item{MovieID: m.ID, ReleaseTitle: "Some.Movie.2001.1080p.WEB-DL.x264-GRP", NZBURL: "http://127.0.0.1:1/a.nzb", SizeBytes: 5 << 30})
	if err != nil {
		t.Fatal(err)
	}
	elsewhere, err := server.QueueRepo.Enqueue(queue.Item{MovieID: other.ID, ReleaseTitle: "Other.Movie.2002.1080p.WEB-DL.x264-GRP", NZBURL: "http://127.0.0.1:1/o.nzb"})
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/movies/%d/grab", base, m.ID)
	grab := func(replace int64) map[string]any {
		return map[string]any{"releaseTitle": "Some.Movie.2001.1080p.BluRay.x264-GRP", "downloadUrl": "http://127.0.0.1:1/b.nzb", "replaceQueueId": replace}
	}

	// The refusal says what is in the way.
	plain := postJSON[map[string]any](t, client, url, map[string]any{"releaseTitle": "Some.Movie.2001.1080p.BluRay.x264-GRP", "downloadUrl": "http://127.0.0.1:1/b.nzb"}, http.StatusConflict)
	blocking, _ := plain["blocking"].(map[string]any)
	if blocking["id"] != float64(busy) || blocking["title"] != "Some.Movie.2001.1080p.WEB-DL.x264-GRP" || blocking["sizeBytes"] != float64(5<<30) {
		t.Fatalf("409 body: %+v", plain)
	}

	// A wrong id, one for another movie, or none that exists: refused, and the
	// running download stays.
	for _, wrong := range []int64{busy + 1000, elsewhere} {
		postJSON[map[string]any](t, client, url, grab(wrong), http.StatusConflict)
		if _, err := server.QueueRepo.Get(busy); err != nil {
			t.Fatalf("the download was removed by a wrong id %d: %v", wrong, err)
		}
	}
	if _, err := server.QueueRepo.Get(elsewhere); err != nil {
		t.Fatalf("another movie's download was removed: %v", err)
	}

	// The right id cancels it, takes it off the list and queues the new release.
	got := postJSON[map[string]any](t, client, url, grab(busy), http.StatusAccepted)
	newID, _ := got["queueId"].(float64)
	if newID == 0 || int64(newID) == busy {
		t.Fatalf("answer: %+v", got)
	}
	if _, err := server.QueueRepo.Get(busy); err == nil {
		t.Error("the cancelled download is still on the list")
	}
	if it, err := server.QueueRepo.Get(int64(newID)); err != nil || it.ReleaseTitle != "Some.Movie.2001.1080p.BluRay.x264-GRP" {
		t.Errorf("the new release is not queued: %+v %v", it, err)
	}

}

// For a show, the download named has to be one of those covering the same
// episodes; a season pack in the way can be replaced by one episode.
func TestGrabReplacingForAShow(t *testing.T) {
	server, httpSrv, client := newHuntTestServer(t)
	base := httpSrv.URL
	signUpAdmin(t, client, base)

	sr, err := server.MovieRepo.AddSeries(library.Series{TMDBID: 9, Title: "Show"},
		[]library.Episode{{Season: 1, Episode: 1}, {Season: 1, Episode: 2}, {Season: 2, Episode: 1}})
	if err != nil {
		t.Fatal(err)
	}
	pack, err := server.QueueRepo.Enqueue(queue.Item{SeriesID: sr.ID, Season: 1, ReleaseTitle: "Show.S01.1080p.WEB-DL.x264-GRP", NZBURL: "http://127.0.0.1:1/p.nzb"})
	if err != nil {
		t.Fatal(err)
	}
	seasonTwo, err := server.QueueRepo.Enqueue(queue.Item{SeriesID: sr.ID, Season: 2, ReleaseTitle: "Show.S02.1080p.WEB-DL.x264-GRP", NZBURL: "http://127.0.0.1:1/p2.nzb"})
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/series/%d/grab", base, sr.ID)
	ep := func(replace int64) map[string]any {
		return map[string]any{"releaseTitle": "Show.S01E02.1080p.WEB-DL.x264-GRP", "downloadUrl": "http://127.0.0.1:1/e.nzb", "replaceQueueId": replace}
	}

	body := postJSON[map[string]any](t, client, url, ep(seasonTwo), http.StatusConflict)
	if b, _ := body["blocking"].(map[string]any); b["id"] != float64(pack) {
		t.Fatalf("it should name the pack that is really in the way: %+v", body)
	}
	if _, err := server.QueueRepo.Get(seasonTwo); err != nil {
		t.Fatalf("an unrelated season was cancelled: %v", err)
	}
	postJSON[map[string]any](t, client, url, ep(pack), http.StatusAccepted)
	if _, err := server.QueueRepo.Get(pack); err == nil {
		t.Error("the pack is still on the list")
	}
	if _, err := server.QueueRepo.Get(seasonTwo); err != nil {
		t.Errorf("the other season's download is gone: %v", err)
	}
}
