package api

import (
	"strings"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/tvmaze"
)

func TestMatchAirTimes(t *testing.T) {
	stored := []library.Episode{
		{Season: 1, Episode: 1, AirDate: "2024-03-04"},
		{Season: 1, Episode: 2, AirDate: "2024-03-11"},
		{Season: 1, Episode: 3, AirDate: "2024-03-18"},
		{Season: 1, Episode: 4, AirDate: ""},
		{Season: 1, Episode: 5, AirDate: "2024-04-01"},
		{Season: 2, Episode: 1, AirDate: "2024-09-02"},
	}
	listed := []tvmaze.Episode{
		{Season: 1, Number: 1, Airdate: "2024-03-04", Airtime: "21:00", Airstamp: "2024-03-05T05:00:00+00:00"}, // same day
		{Season: 1, Number: 2, Airdate: "2024-03-12", Airtime: "21:00", Airstamp: "2024-03-13T05:00:00+00:00"}, // one day apart: still taken
		{Season: 1, Number: 3, Airdate: "2024-03-25", Airtime: "21:00", Airstamp: "2024-03-26T05:00:00+00:00"}, // a week off: rejected
		{Season: 1, Number: 4, Airdate: "2024-03-25", Airtime: "21:00", Airstamp: "2024-03-26T05:00:00+00:00"}, // TMDB has no date: nothing to check against
		{Season: 1, Number: 5, Airdate: "2024-04-01", Airtime: "", Airstamp: "2024-04-01T12:00:00+00:00"},      // TVmaze has no time: never guessed
		{Season: 2, Number: 1, Airdate: "2024-09-02", Airtime: "20:00", Airstamp: "2024-09-02T19:00:00+00:00"}, // BST evening
		{Season: 0, Number: 0, Airdate: "2024-03-04", Airtime: "21:00", Airstamp: "2024-03-05T05:00:00+00:00"}, // an unnumbered special
		{Season: 3, Number: 1, Airdate: "2025-01-01", Airtime: "21:00", Airstamp: "2025-01-02T05:00:00+00:00"}, // not in the library
		{Season: 1, Number: 1, Airdate: "2024-03-04", Airtime: "21:00", Airstamp: "1999-01-01T00:00:00+00:00"}, // a duplicate: the first one counts
	}
	got := matchAirTimes(stored, listed)
	want := map[[2]int]string{
		{1, 1}: "2024-03-05T05:00:00Z",
		{1, 2}: "2024-03-13T05:00:00Z",
		{2, 1}: "2024-09-02T19:00:00Z",
	}
	if len(got) != len(want) {
		t.Fatalf("matched %d episodes (%v), want %d", len(got), got, len(want))
	}
	for _, a := range got {
		if w, ok := want[[2]int{a.Season, a.Episode}]; !ok || a.At.Format(time.RFC3339) != w {
			t.Errorf("S%02dE%02d = %s, want %q", a.Season, a.Episode, a.At.Format(time.RFC3339), w)
		}
	}
}

func TestTVmazeNeeded(t *testing.T) {
	now := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	future := []library.Episode{{AirDate: "2024-07-01"}}
	past := []library.Episode{{AirDate: "2024-01-01", AirAt: "2024-01-02T01:00:00Z"}}
	tests := []struct {
		name string
		sr   library.Series
		eps  []library.Episode
		want bool
	}{
		{"never asked", library.Series{}, nil, true},
		{"id known, running show", library.Series{TVmazeID: 5, TVmazeCheckedAt: ago(time.Hour), TMDBStatus: "Returning Series"}, past, true},
		{"id known, ended show asked before", library.Series{TVmazeID: 5, TVmazeCheckedAt: ago(time.Hour), TMDBStatus: "Ended"}, past, false},
		{"ended show with an undated-time episode ahead", library.Series{TVmazeID: 5, TVmazeCheckedAt: ago(time.Hour), TMDBStatus: "Ended"}, future, true},
		{"unknown to TVmaze, tried yesterday", library.Series{TVmazeID: -1, TVmazeCheckedAt: ago(24 * time.Hour)}, nil, false},
		{"unknown to TVmaze, tried last month", library.Series{TVmazeID: -1, TVmazeCheckedAt: ago(30 * 24 * time.Hour)}, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tvmazeNeeded(tc.sr, tc.eps, now); got != tc.want {
				t.Fatalf("tvmazeNeeded = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRenderICSTimedAndAllDayEvents(t *testing.T) {
	entries := []calendarEntryPayload{
		{Kind: "episode", ID: 1, Title: "Timed", Subtitle: "S01E01", Season: 1, Episode: 1, ReleaseDate: "2024-03-04", AirAt: "2024-03-05T05:00:00Z", Status: "missing"},
		{Kind: "episode", ID: 2, Title: "Plain", Subtitle: "S01E02", Season: 1, Episode: 2, ReleaseDate: "2024-03-11", Status: "missing"},
		{Kind: "movie", ID: 3, Title: "Film", ReleaseDate: "2024-03-12", AirAt: "2024-03-12T20:00:00Z", Status: "missing"}, // only episodes are timed
		{Kind: "episode", ID: 4, Title: "Broken", ReleaseDate: "2024-03-13", AirAt: "not a time", Status: "missing"},
	}
	ics := renderICS(entries, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{
		"DTSTART:20240305T050000Z\r\nDTEND:20240305T060000Z\r\n",
		"DTSTART;VALUE=DATE:20240311\r\nDTEND;VALUE=DATE:20240312\r\n",
		"DTSTART;VALUE=DATE:20240312\r\n",
		"DTSTART;VALUE=DATE:20240313\r\n",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("feed lacks %q:\n%s", want, ics)
		}
	}
	if strings.Count(ics, "DTSTART:") != 1 {
		t.Errorf("exactly one event should be timed:\n%s", ics)
	}
}

func TestUpcomingEntries(t *testing.T) {
	now := time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC)
	entries := []calendarEntryPayload{
		{Title: "yesterday, no time", ReleaseDate: "2024-03-04"},
		{Title: "dated yesterday, airs today UTC", ReleaseDate: "2024-03-04", AirAt: "2024-03-05T05:00:00Z"},
		{Title: "today, no time", ReleaseDate: "2024-03-05"},
		{Title: "today, airs tomorrow UTC", ReleaseDate: "2024-03-05", AirAt: "2024-03-06T01:00:00Z"},
		{Title: "next week", ReleaseDate: "2024-03-12"},
	}
	got := upcomingEntries(entries, now, 3)
	var titles []string
	for _, e := range got {
		titles = append(titles, e.Title)
	}
	// An episode with no time sorts at the start of its day.
	want := "today, no time|dated yesterday, airs today UTC|today, airs tomorrow UTC"
	if strings.Join(titles, "|") != want {
		t.Fatalf("upcoming = %v, want %s", titles, want)
	}
}

func TestArrAirDateUTC(t *testing.T) {
	tests := []struct {
		name string
		ep   library.Episode
		want string
	}{
		{"exact time", library.Episode{AirDate: "2024-03-04", AirAt: "2024-03-05T05:00:00Z"}, "2024-03-05T05:00:00Z"},
		{"date only", library.Episode{AirDate: "2024-03-04"}, "2024-03-04T00:00:00Z"},
		{"nothing", library.Episode{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := arrAirDateUTC(tc.ep); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
