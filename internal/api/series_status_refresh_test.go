package api_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/library"
)

// A show added while TMDB called it running is marked ended once TMDB says so:
// the scheduled refresh keeps the status up to date, also for a show that is
// not monitored.
func TestRefreshAllSeriesUpdatesTheShowStatus(t *testing.T) {
	server, _, _ := loginNewServer(t)
	server.TestSetSeriesRefreshPause(0)
	var status atomic.Value
	status.Store("Returning Series")
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1399,"name":"Show","status":"` + status.Load().(string) + `","seasons":[]}`))
	}))
	t.Cleanup(tmdb.Close)
	server.TestSetTMDBBaseURL("key", tmdb.URL)

	running, err := server.MovieRepo.AddSeries(library.Series{TMDBID: 1399, Title: "Show", Monitored: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := server.MovieRepo.AddSeries(library.Series{TMDBID: 1400, Title: "Quiet show", Monitored: false}, nil)
	if err != nil {
		t.Fatal(err)
	}

	statusOf := func(id int64) string {
		t.Helper()
		sr, err := server.MovieRepo.GetSeries(id)
		if err != nil {
			t.Fatal(err)
		}
		return sr.TMDBStatus
	}

	server.TestRefreshAllSeries()
	if got := statusOf(running.ID); got != "Returning Series" {
		t.Fatalf("monitored show status = %q, want Returning Series", got)
	}
	if got := statusOf(quiet.ID); got != "Returning Series" {
		t.Fatalf("unmonitored show status = %q, want Returning Series (it was skipped before)", got)
	}

	// TMDB now says the show has ended. The monitored one is read on every
	// run; the unmonitored one was read an hour ago and waits for the day.
	status.Store("Ended")
	server.TestRefreshAllSeries()
	if got := statusOf(running.ID); got != "Ended" {
		t.Fatalf("monitored show status = %q, want Ended", got)
	}
	if got := statusOf(quiet.ID); got != "Returning Series" {
		t.Fatalf("unmonitored show was read again within the day: status = %q", got)
	}
	if err := server.MovieRepo.MarkSeriesChecked(quiet.ID, time.Now().Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	server.TestRefreshAllSeries()
	if got := statusOf(quiet.ID); got != "Ended" {
		t.Fatalf("unmonitored show status = %q, want Ended after a day", got)
	}
}
