package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/settings"
)

// tvmazeStandIn answers like TVmaze for one show (id 82, TheTVDB 121361, IMDb
// tt0944947, named "Fixture Show") and counts the calls by kind.
type tvmazeStandIn struct {
	srv                                 *httptest.Server
	tvdb, imdb, search, episodes, total atomic.Int32
	episodesBody                        string
	tooManyRequests                     bool
	knowsTVDB, knowsIMDb, knowsName     bool
}

const standInShow = `{"id":82,"name":"Fixture Show","status":"Running","premiered":"2020-01-05","network":{"country":{"timezone":"America/Los_Angeles"}}}`

func newTVmazeStandIn(t *testing.T) *tvmazeStandIn {
	t.Helper()
	s := &tvmazeStandIn{knowsTVDB: true, knowsIMDb: true, knowsName: true}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.total.Add(1)
		if s.tooManyRequests {
			if r.URL.Query().Get("thetvdb") != "" {
				s.tvdb.Add(1)
			}
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/lookup/shows" && q.Get("thetvdb") != "":
			s.tvdb.Add(1)
			if s.knowsTVDB && q.Get("thetvdb") == "121361" {
				w.Write([]byte(standInShow))
				return
			}
		case r.URL.Path == "/lookup/shows" && q.Get("imdb") != "":
			s.imdb.Add(1)
			if s.knowsIMDb && q.Get("imdb") == "tt0944947" {
				w.Write([]byte(standInShow))
				return
			}
		case r.URL.Path == "/singlesearch/shows":
			s.search.Add(1)
			if s.knowsName && q.Get("q") == "Fixture Show" {
				w.Write([]byte(standInShow))
				return
			}
		case r.URL.Path == "/shows/82/episodes":
			s.episodes.Add(1)
			w.Write([]byte(s.episodesBody))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// A show at 21:00 on the US west coast: the evening of 2 March there is
// 05:00 UTC on 3 March.
const standInEpisodes = `[
 {"season":1,"number":1,"airdate":"2020-03-02","airtime":"21:00","airstamp":"2020-03-03T05:00:00+00:00","runtime":60},
 {"season":1,"number":2,"airdate":"2020-03-09","airtime":"21:00","airstamp":"2020-03-10T05:00:00+00:00","runtime":60},
 {"season":1,"number":3,"airdate":"2020-03-16","airtime":"","airstamp":"2020-03-16T12:00:00+00:00","runtime":60},
 {"season":1,"number":4,"airdate":"2020-06-01","airtime":"21:00","airstamp":"2020-06-02T04:00:00+00:00","runtime":60}
]`

func airAtOf(t *testing.T, env *tvAutoEnv, episode int) string {
	t.Helper()
	ep, err := env.server.MovieRepo.GetEpisode(env.seriesID, 1, episode)
	if err != nil {
		t.Fatal(err)
	}
	return ep.AirAt
}

func TestSyncAirTimes(t *testing.T) {
	tests := []struct {
		name        string
		tvdb        int
		imdb        string
		mutate      func(*tvmazeStandIn)
		wantAt      map[int]string // episode -> air_at ("" = none)
		wantCalls   [4]int32       // tvdb, imdb, search, episodes
		wantTVmaze  int
		turnSwitch  string // "0" switches the setting off
		wantNoCalls bool
	}{
		{
			name: "found by TheTVDB id", tvdb: 121361, imdb: "tt0944947",
			wantAt:    map[int]string{1: "2020-03-03T05:00:00Z", 2: "2020-03-10T05:00:00Z", 3: "", 4: "2020-06-02T04:00:00Z"},
			wantCalls: [4]int32{1, 0, 0, 1}, wantTVmaze: 82,
		},
		{
			name: "found by IMDb id when there is no TheTVDB id", imdb: "tt0944947",
			wantAt:    map[int]string{1: "2020-03-03T05:00:00Z", 2: "2020-03-10T05:00:00Z"},
			wantCalls: [4]int32{0, 1, 0, 1}, wantTVmaze: 82,
		},
		{
			name: "IMDb id when TheTVDB does not know the show", tvdb: 5, imdb: "tt0944947",
			wantAt:    map[int]string{1: "2020-03-03T05:00:00Z"},
			wantCalls: [4]int32{1, 1, 0, 1}, wantTVmaze: 82,
		},
		{
			name:      "falls back to a search by name and year",
			wantAt:    map[int]string{1: "2020-03-03T05:00:00Z"},
			wantCalls: [4]int32{0, 0, 1, 1}, wantTVmaze: 82,
		},
		{
			name: "TVmaze's day is far from TMDB's: nothing is stored", tvdb: 121361,
			mutate: func(s *tvmazeStandIn) {
				s.episodesBody = strings.ReplaceAll(standInEpisodes, `"airdate":"2020-03-02"`, `"airdate":"2020-03-12"`)
			},
			wantAt:    map[int]string{1: "", 2: "2020-03-10T05:00:00Z"},
			wantCalls: [4]int32{1, 0, 0, 1}, wantTVmaze: 82,
		},
		{
			name: "an episode with no time on TVmaze gets none", tvdb: 121361,
			wantAt:    map[int]string{3: ""},
			wantCalls: [4]int32{1, 0, 0, 1}, wantTVmaze: 82,
		},
		{
			name: "TVmaze does not know the show: remembered as -1", tvdb: 121361, imdb: "tt0944947",
			mutate:    func(s *tvmazeStandIn) { s.knowsTVDB, s.knowsIMDb, s.knowsName = false, false, false },
			wantAt:    map[int]string{1: ""},
			wantCalls: [4]int32{1, 1, 1, 0}, wantTVmaze: -1,
		},
		{
			name: "the switch off makes no calls at all", tvdb: 121361, imdb: "tt0944947", turnSwitch: "0",
			wantAt: map[int]string{1: ""}, wantNoCalls: true,
		},
		{
			name: "TVmaze asking to slow down leaves everything as it was", tvdb: 121361,
			mutate:    func(s *tvmazeStandIn) { s.tooManyRequests = true },
			wantAt:    map[int]string{1: ""},
			wantCalls: [4]int32{1, 0, 0, 0}, wantTVmaze: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stand := newTVmazeStandIn(t)
			stand.episodesBody = standInEpisodes
			if tc.mutate != nil {
				tc.mutate(stand)
			}
			env := newTVAutoEnv(t, nil, []episodeSpec{
				{1, 1, "2020-03-02", library.StatusMissing, ""},
				{1, 2, "2020-03-09", library.StatusMissing, ""},
				{1, 3, "2020-03-16", library.StatusMissing, ""},
				{1, 4, "2020-06-01", library.StatusMissing, ""},
			})
			env.server.SetTVmazeServer(stand.srv.URL)
			if tc.turnSwitch != "" {
				if err := env.server.Settings.Set(settings.KeyTVmazeAirTimes, tc.turnSwitch, false); err != nil {
					t.Fatal(err)
				}
			}
			if err := env.server.MovieRepo.SetSeriesExternalIDs(env.seriesID, tc.tvdb, tc.imdb, "Returning Series"); err != nil {
				t.Fatal(err)
			}
			sr, err := env.server.MovieRepo.GetSeries(env.seriesID)
			if err != nil {
				t.Fatal(err)
			}
			env.server.TestSyncAirTimes(context.Background(), sr)

			if tc.wantNoCalls && stand.total.Load() != 0 {
				t.Fatalf("TVmaze was called %d times with the switch off", stand.total.Load())
			}
			if !tc.wantNoCalls {
				got := [4]int32{stand.tvdb.Load(), stand.imdb.Load(), stand.search.Load(), stand.episodes.Load()}
				if got != tc.wantCalls {
					t.Fatalf("calls (tvdb, imdb, search, episodes) = %v, want %v", got, tc.wantCalls)
				}
				after, _ := env.server.MovieRepo.GetSeries(env.seriesID)
				if after.TVmazeID != tc.wantTVmaze {
					t.Fatalf("TVmaze id = %d, want %d", after.TVmazeID, tc.wantTVmaze)
				}
			}
			for ep, want := range tc.wantAt {
				if got := airAtOf(t, env, ep); got != want {
					t.Errorf("episode %d air_at = %q, want %q", ep, got, want)
				}
			}
		})
	}
}

func TestSyncAirTimesAsksOnceAndKeepsTheId(t *testing.T) {
	stand := newTVmazeStandIn(t)
	stand.episodesBody = standInEpisodes
	env := newTVAutoEnv(t, nil, []episodeSpec{{1, 1, "2020-03-02", library.StatusMissing, ""}})
	env.server.SetTVmazeServer(stand.srv.URL)
	if err := env.server.MovieRepo.SetSeriesExternalIDs(env.seriesID, 121361, "", "Ended"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		sr, _ := env.server.MovieRepo.GetSeries(env.seriesID)
		env.server.TestSyncAirTimes(context.Background(), sr)
	}
	// An ended show is asked once: one look-up and one episode list, then nothing.
	if stand.tvdb.Load() != 1 || stand.episodes.Load() != 1 {
		t.Fatalf("tvdb=%d episodes=%d, want 1 and 1", stand.tvdb.Load(), stand.episodes.Load())
	}

	// A running show is asked on every refresh, but the id is not looked up again.
	if err := env.server.MovieRepo.SetSeriesExternalIDs(env.seriesID, 0, "", "Returning Series"); err != nil {
		t.Fatal(err)
	}
	sr, _ := env.server.MovieRepo.GetSeries(env.seriesID)
	env.server.TestSyncAirTimes(context.Background(), sr)
	if stand.tvdb.Load() != 1 || stand.episodes.Load() != 2 {
		t.Fatalf("tvdb=%d episodes=%d, want 1 and 2", stand.tvdb.Load(), stand.episodes.Load())
	}
}

func TestCalendarSendsAirAt(t *testing.T) {
	day := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	env := newTVAutoEnv(t, nil, []episodeSpec{
		{1, 1, day, library.StatusMissing, ""},
		{1, 2, day, library.StatusMissing, ""},
	})
	at := time.Now().UTC().AddDate(0, 0, 4).Truncate(time.Hour)
	if _, err := env.server.MovieRepo.SetEpisodeAirTimes(env.seriesID, []library.AirTime{{Season: 1, Episode: 1, At: at}}); err != nil {
		t.Fatal(err)
	}
	entries := getJSON[[]map[string]any](t, env.client, env.baseURL+"/api/calendar")
	got := map[string]any{}
	for _, e := range entries {
		if e["kind"] == "episode" {
			got[e["subtitle"].(string)] = e["airAt"]
		}
	}
	if got["S01E01"] != at.Format(time.RFC3339) {
		t.Errorf("S01E01 airAt = %v, want %s", got["S01E01"], at.Format(time.RFC3339))
	}
	if v, ok := got["S01E02"]; !ok || v != nil {
		t.Errorf("S01E02 should have no airAt, got %v (present %v)", v, ok)
	}
}

func TestEpisodeThatAirsLaterTodayIsNotMissing(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	env := newTVAutoEnv(t, nil, []episodeSpec{
		{1, 1, today, library.StatusMissing, ""}, // dated today, airs in 10 hours
		{1, 2, today, library.StatusMissing, ""}, // dated today, aired an hour ago
		{1, 3, today, library.StatusMissing, ""}, // dated today, no time known
	})
	_, err := env.server.MovieRepo.SetEpisodeAirTimes(env.seriesID, []library.AirTime{
		{Season: 1, Episode: 1, At: time.Now().UTC().Add(10 * time.Hour)},
		{Season: 1, Episode: 2, At: time.Now().UTC().Add(-time.Hour)},
	})
	if err != nil {
		t.Fatal(err)
	}
	wanted, err := env.server.MovieRepo.ListWantedEpisodes(context.Background(), library.StatusMissing, today, env.seriesID)
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, w := range wanted {
		got = append(got, w.Episode.Episode)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("wanted episodes = %v, want [2 3]", got)
	}
}
