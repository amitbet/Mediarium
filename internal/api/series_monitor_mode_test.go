package api_test

import (
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/library"
)

func TestSeriesMonitorModeFromTheShowPage(t *testing.T) {
	server, base, client := loginNewServer(t)
	day := func(offset int) string { return time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02") }
	s, err := server.MovieRepo.AddSeries(library.Series{TMDBID: 5, Title: "Old Show", Year: 2020, Monitored: true}, []library.Episode{
		{Season: 1, Episode: 1, AirDate: day(-400)},
		{Season: 1, Episode: 2, AirDate: day(-1)},
		{Season: 2, Episode: 1, AirDate: day(0)},
		{Season: 2, Episode: 2, AirDate: day(30)},
		{Season: 2, Episode: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.MovieRepo.SetSeriesMonitored(s.ID, false); err != nil {
		t.Fatal(err)
	}
	url := base + "/api/series/" + itoa(s.ID) + "/monitor-mode"
	flags := func() (out []bool) {
		eps, _ := server.MovieRepo.ListEpisodes(s.ID)
		for _, e := range eps {
			out = append(out, e.Monitored)
		}
		return out
	}
	same := func(got, want []bool) bool {
		for i := range want {
			if got[i] != want[i] {
				return false
			}
		}
		return len(got) == len(want)
	}
	type answer = library.MonitorChange

	postJSONMethod[answer](t, client, "POST", url, map[string]any{"mode": "sometimes"}, 400)
	postJSONMethod[answer](t, client, "POST", base+"/api/series/9999/monitor-mode", map[string]any{"mode": "all"}, 404)

	// Counting changes nothing.
	if a := postJSONMethod[answer](t, client, "POST", url, map[string]any{"mode": "future", "dryRun": true}, 200); a.Stopped != 2 || a.Started != 0 {
		t.Fatalf("dry run should count the two aired episodes, got %+v", a)
	}
	if !same(flags(), []bool{true, true, true, true, true}) {
		t.Fatalf("a dry run changed something: %v", flags())
	}
	if got, _ := server.MovieRepo.GetSeries(s.ID); got.Monitored {
		t.Fatal("a dry run switched the show on")
	}

	if a := postJSONMethod[answer](t, client, "POST", url, map[string]any{"mode": "future"}, 200); a.Stopped != 2 {
		t.Fatalf("changed %+v, want 2 stopped", a)
	}
	if !same(flags(), []bool{false, false, true, true, true}) {
		t.Fatalf("only what has not aired yet should stay monitored: %v", flags())
	}
	if got, _ := server.MovieRepo.GetSeries(s.ID); !got.Monitored {
		t.Fatal("the show should be monitored so new episodes are picked up")
	}
	// A refresh that finds a new episode monitors it.
	if err := server.MovieRepo.UpsertEpisodes(s.ID, []library.Episode{{Season: 3, Episode: 1, AirDate: day(90)}}, true); err != nil {
		t.Fatal(err)
	}
	if f := flags(); !f[len(f)-1] {
		t.Fatalf("a new episode should be monitored: %v", f)
	}

	if a := postJSONMethod[answer](t, client, "POST", url, map[string]any{"mode": "none"}, 200); a.Stopped != 4 {
		t.Fatalf("changed %+v, want 4 stopped", a)
	}
	if a := postJSONMethod[answer](t, client, "POST", url, map[string]any{"mode": "all"}, 200); a.Started != 6 {
		t.Fatalf("changed %+v, want 6 started", a)
	}
	if q, _ := server.QueueRepo.List(); len(q) != 0 {
		t.Fatalf("changing monitoring queued %d downloads", len(q))
	}
}

// A show with a monitored episode is a monitored show: switching on one
// episode or a season from the show page switches the show on.
func TestMonitoringAnEpisodeSwitchesTheShowOn(t *testing.T) {
	server, base, client := loginNewServer(t)
	s, err := server.MovieRepo.AddSeries(library.Series{TMDBID: 6, Title: "Show", Year: 2020, Monitored: true}, []library.Episode{
		{Season: 1, Episode: 1, AirDate: "2020-01-01"},
		{Season: 2, Episode: 1, AirDate: "2020-02-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	eps, _ := server.MovieRepo.ListEpisodes(s.ID)
	off := func() {
		t.Helper()
		if err := server.MovieRepo.SetSeriesMonitored(s.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	on := func() bool { got, _ := server.MovieRepo.GetSeries(s.ID); return got.Monitored }

	off()
	putJSONStatus(t, client, base+"/api/episodes/"+itoa(eps[0].ID)+"/monitored", map[string]any{"monitored": false}, 200)
	if on() {
		t.Fatal("unmonitoring an episode must not switch the show on")
	}
	putJSONStatus(t, client, base+"/api/episodes/"+itoa(eps[0].ID)+"/monitored", map[string]any{"monitored": true}, 200)
	if !on() {
		t.Fatal("monitoring an episode should switch the show on")
	}
	off()
	putJSONStatus(t, client, base+"/api/series/"+itoa(s.ID)+"/seasons/2/monitored", map[string]any{"monitored": true}, 200)
	if !on() {
		t.Fatal("monitoring a season should switch the show on")
	}
}
