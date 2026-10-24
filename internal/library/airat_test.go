package library_test

import (
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/library"
)

func TestAirAtSurvivesARefreshOnlyWhileTheDateIsUnchanged(t *testing.T) {
	repo := newRepo(t)
	series, err := repo.AddSeries(library.Series{TMDBID: 7, Title: "S", Monitored: true}, []library.Episode{
		{Season: 1, Episode: 1, AirDate: "2030-03-02"},
		{Season: 1, Episode: 2, AirDate: "2030-03-09"},
	})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2030, 3, 3, 5, 0, 0, 0, time.UTC)
	n, err := repo.SetEpisodeAirTimes(series.ID, []library.AirTime{{Season: 1, Episode: 1, At: at}, {Season: 1, Episode: 2, At: at.AddDate(0, 0, 7)}, {Season: 9, Episode: 9, At: at}})
	if err != nil || n != 2 {
		t.Fatalf("changed %d rows, err %v; want 2", n, err)
	}
	// The same times again change nothing.
	if n, _ := repo.SetEpisodeAirTimes(series.ID, []library.AirTime{{Season: 1, Episode: 1, At: at}}); n != 0 {
		t.Fatalf("an unchanged time changed %d rows", n)
	}
	// TMDB repeats episode 1's date and moves episode 2's.
	if err := repo.UpsertEpisodes(series.ID, []library.Episode{
		{Season: 1, Episode: 1, AirDate: "2030-03-02"},
		{Season: 1, Episode: 2, AirDate: "2030-03-16"},
	}, true); err != nil {
		t.Fatal(err)
	}
	one, _ := repo.GetEpisode(series.ID, 1, 1)
	two, _ := repo.GetEpisode(series.ID, 1, 2)
	if one.AirAt != "2030-03-03T05:00:00Z" {
		t.Errorf("episode 1 air time = %q, want it kept", one.AirAt)
	}
	if two.AirAt != "" {
		t.Errorf("episode 2 air time = %q, want it cleared when its date moved", two.AirAt)
	}
}

func TestSecondCopyKeepsAirTimes(t *testing.T) {
	repo := newRepo(t)
	series, _ := repo.AddSeries(library.Series{TMDBID: 8, Title: "S", Monitored: true}, []library.Episode{{Season: 1, Episode: 1, AirDate: "2030-03-02"}})
	at := time.Date(2030, 3, 3, 5, 0, 0, 0, time.UTC)
	if _, err := repo.SetEpisodeAirTimes(series.ID, []library.AirTime{{Season: 1, Episode: 1, At: at}}); err != nil {
		t.Fatal(err)
	}
	cp, err := repo.AddSeriesCopy(series.ID, 0)
	if err != nil {
		t.Skipf("second copy: %v", err)
	}
	ep, err := repo.GetEpisode(cp.ID, 1, 1)
	if err != nil || ep.AirAt != "2030-03-03T05:00:00Z" {
		t.Fatalf("copy episode = %+v, err %v", ep, err)
	}
}

func TestAirsLater(t *testing.T) {
	now := time.Date(2030, 3, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   string
		want bool
	}{
		{"no time known keeps the date rule", "", false},
		{"in ten hours", "2030-03-02T22:00:00Z", true},
		{"in two hours is within the slack", "2030-03-02T14:00:00Z", false},
		{"just over the slack", "2030-03-02T15:00:01Z", true},
		{"already aired", "2030-03-02T01:00:00Z", false},
		{"unreadable time keeps the date rule", "tonight", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (library.Episode{AirAt: tc.at}).AirsLater(now); got != tc.want {
				t.Fatalf("AirsLater = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSetSeriesTVmaze(t *testing.T) {
	repo := newRepo(t)
	series, _ := repo.AddSeries(library.Series{TMDBID: 9, Title: "S", Monitored: true}, nil)
	when := time.Date(2030, 1, 1, 8, 0, 0, 0, time.UTC)
	if err := repo.SetSeriesTVmaze(series.ID, 82, time.Time{}); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetSeries(series.ID)
	if got.TVmazeID != 82 || got.TVmazeCheckedAt != "" {
		t.Fatalf("after the id: %+v", got)
	}
	if err := repo.SetSeriesTVmaze(series.ID, 0, when); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetSeries(series.ID)
	if got.TVmazeID != 82 || got.TVmazeCheckedAt != "2030-01-01T08:00:00Z" {
		t.Fatalf("after the check: %+v", got)
	}
}
