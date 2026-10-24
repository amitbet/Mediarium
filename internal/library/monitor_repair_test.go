package library_test

import (
	"testing"

	"github.com/rdborg/mediarium/internal/library"
)

func TestRepairFutureOnlyMonitoring(t *testing.T) {
	const today = "2026-10-18"
	aired := func(n int) library.Episode { return library.Episode{Season: 1, Episode: n, AirDate: "2024-01-01"} }
	cases := []struct {
		name      string
		eps       []library.Episode
		unmonitor []int // episodes (1-based) switched off before the repair
		monitored bool  // the show's own switch before the repair
		want      bool  // ... and after it
	}{
		{"future-only show with the switch off is repaired", []library.Episode{aired(1), aired(2), {Season: 2, Episode: 1, AirDate: "2027-01-01"}}, []int{1, 2}, false, true},
		{"an undated episode counts as not aired", []library.Episode{aired(1), {Season: 2, Episode: 1}}, []int{1}, false, true},
		{"turned off on purpose, every episode still monitored: left alone", []library.Episode{aired(1), {Season: 2, Episode: 1, AirDate: "2027-01-01"}}, nil, false, false},
		{"an aired episode is still monitored: left alone", []library.Episode{aired(1), aired(2), {Season: 2, Episode: 1, AirDate: "2027-01-01"}}, []int{1}, false, false},
		{"nothing monitored: left alone", []library.Episode{aired(1), {Season: 2, Episode: 1, AirDate: "2027-01-01"}}, []int{1, 2}, false, false},
		{"nothing has aired, so it is not future-only mode: left alone", []library.Episode{{Season: 1, Episode: 1, AirDate: "2027-01-01"}}, nil, false, false},
		{"already on stays on", []library.Episode{aired(1), {Season: 2, Episode: 1, AirDate: "2027-01-01"}}, []int{1}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newRepo(t)
			s, err := repo.AddSeries(library.Series{TMDBID: 1, Title: "Show", Year: 2020, Monitored: true}, c.eps)
			if err != nil {
				t.Fatal(err)
			}
			list, _ := repo.ListEpisodes(s.ID)
			for _, n := range c.unmonitor {
				if err := repo.SetEpisodeMonitored(list[n-1].ID, false); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.SetSeriesMonitored(s.ID, c.monitored); err != nil {
				t.Fatal(err)
			}
			changed, err := repo.RepairFutureOnlyMonitoring(today)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := repo.GetSeries(s.ID)
			if got.Monitored != c.want {
				t.Fatalf("monitored = %v, want %v", got.Monitored, c.want)
			}
			if again, _ := repo.RepairFutureOnlyMonitoring(today); again != 0 {
				t.Fatalf("a second repair changed %d shows (first changed %d)", again, changed)
			}
		})
	}
}

func TestEnableSeriesOfEpisode(t *testing.T) {
	repo := newRepo(t)
	s, err := repo.AddSeries(library.Series{TMDBID: 1, Title: "Show", Year: 2020, Monitored: true}, []library.Episode{{Season: 1, Episode: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSeriesMonitored(s.ID, false); err != nil {
		t.Fatal(err)
	}
	list, _ := repo.ListEpisodes(s.ID)
	if err := repo.EnableSeriesOfEpisode(list[0].ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetSeries(s.ID); !got.Monitored {
		t.Fatal("the show should be monitored once one of its episodes is")
	}
}
