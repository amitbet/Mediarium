package metadata_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rdborg/mediarium/internal/metadata"
)

const releaseDatesBody = `{
	"id": 7, "title": "Dates", "release_date": "2025-03-12",
	"release_dates": {"results": [
		{"iso_3166_1": "US", "release_dates": [
			{"type": 4, "release_date": "2025-05-20T00:00:00.000Z"},
			{"type": 3, "release_date": "2025-03-14T00:00:00.000Z"},
			{"type": 5, "release_date": "2025-06-10T00:00:00.000Z"}
		]},
		{"iso_3166_1": "DE", "release_dates": [
			{"type": 3, "release_date": "2025-03-20T00:00:00.000Z"},
			{"type": 3, "release_date": "2025-03-13T00:00:00.000Z"},
			{"type": 4, "release_date": "2025-05-30T00:00:00.000Z"},
			{"type": 4, "release_date": "2025-05-14T00:00:00.000Z"}
		]},
		{"iso_3166_1": "FR", "release_dates": [
			{"type": 1, "release_date": "2025-03-01T00:00:00.000Z"}
		]},
		{"iso_3166_1": "JP", "release_dates": [
			{"type": 2, "release_date": "2025-03-10T00:00:00.000Z"},
			{"type": 4, "release_date": "2025-05-01T00:00:00.000Z"}
		]}
	]}
}`

func releaseDatesFixture(t *testing.T, body string) *metadata.MovieDetail {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Query().Get("append_to_response") != "videos,release_dates,credits" {
			t.Errorf("the dates must come with the detail call, got %s", r.URL.String())
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := metadata.NewWithBaseURL("k", srv.URL)
	d, err := c.GetMovieDetail(context.Background(), 7)
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if _, err := c.GetMovieDetail(context.Background(), 7); err != nil || hits.Load() != 1 {
		t.Fatalf("one request per title, got %d (%v)", hits.Load(), err)
	}
	return d
}

func TestReleaseDatesFor(t *testing.T) {
	d := releaseDatesFixture(t, releaseDatesBody)
	tests := []struct {
		name   string
		region string
		want   metadata.MovieReleaseDates
	}{
		{"region has both, earliest of several entries wins", "DE", metadata.MovieReleaseDates{Cinema: "2025-03-13", Digital: "2025-05-14", Region: "DE"}},
		{"lower case region", "de", metadata.MovieReleaseDates{Cinema: "2025-03-13", Digital: "2025-05-14", Region: "DE"}},
		{"limited theatrical counts when there is no wide one", "JP", metadata.MovieReleaseDates{Cinema: "2025-03-10", Digital: "2025-05-01", Region: "JP"}},
		{"premiere only, digital falls back to the US", "FR", metadata.MovieReleaseDates{Cinema: "2025-03-01", Digital: "2025-05-20", DigitalRegion: "US", Region: "FR"}},
		{"unknown region falls back to the US", "NL", metadata.MovieReleaseDates{Cinema: "2025-03-14", Digital: "2025-05-20", CinemaRegion: "US", DigitalRegion: "US", Region: "NL"}},
		{"the US itself needs no fallback", "US", metadata.MovieReleaseDates{Cinema: "2025-03-14", Digital: "2025-05-20", Region: "US"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := d.ReleaseDatesFor(tc.region); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReleaseDatesForMissingTypes(t *testing.T) {
	// No US entry: an unlisted region has no date, never one from another country.
	d := releaseDatesFixture(t, `{"id": 7, "release_dates": {"results": [
		{"iso_3166_1": "GB", "release_dates": [{"type": 3, "release_date": "2025-04-02T00:00:00.000Z"}, {"type": 5, "release_date": "2025-07-01T00:00:00.000Z"}]},
		{"iso_3166_1": "IT", "release_dates": [{"type": 3, "release_date": "2025-04-01T00:00:00.000Z"}]}
	]}}`)
	got := d.ReleaseDatesFor("FR")
	want := metadata.MovieReleaseDates{Region: "FR"}
	if got != want {
		t.Fatalf("got %+v, want %+v (dates only in other countries stay empty)", got, want)
	}

	empty := releaseDatesFixture(t, `{"id": 7}`)
	if got := empty.ReleaseDatesFor("DE"); got != (metadata.MovieReleaseDates{Region: "DE"}) {
		t.Fatalf("a film without any dates gave %+v", got)
	}

	short := releaseDatesFixture(t, `{"id": 7, "release_dates": {"results": [{"iso_3166_1": "DE", "release_dates": [{"type": 3, "release_date": ""}, {"type": 4, "release_date": "2025-5"}]}]}}`)
	if got := short.ReleaseDatesFor("DE"); got.Cinema != "" || got.Digital != "" {
		t.Fatalf("unreadable dates must be ignored, got %+v", got)
	}
}

func TestRegionOfLanguage(t *testing.T) {
	for tag, want := range map[string]string{"en-US": "US", "de-DE": "DE", "pt-BR": "BR", "es-MX": "MX", "": "US", "en": "US"} {
		if got := metadata.RegionOfLanguage(tag); got != want {
			t.Errorf("RegionOfLanguage(%q) = %q, want %q", tag, got, want)
		}
	}
}

// With no US dates and none in the person's country there is no date at all:
// a date from some other country (here BR and IN) is never shown.
func TestReleaseDatesForNeverUsesAnotherCountry(t *testing.T) {
	d := releaseDatesFixture(t, `{
		"id": 7, "title": "Far away",
		"release_dates": {"results": [
			{"iso_3166_1": "BR", "release_dates": [{"type": 3, "release_date": "2026-10-27T00:00:00.000Z"}]},
			{"iso_3166_1": "IN", "release_dates": [{"type": 4, "release_date": "2019-05-21T00:00:00.000Z"}]}
		]}
	}`)
	tests := []struct {
		name   string
		region string
		want   metadata.MovieReleaseDates
	}{
		{"own country has it, no tag", "IN", metadata.MovieReleaseDates{Digital: "2019-05-21", Region: "IN"}},
		{"neither the country nor the US has a date", "DE", metadata.MovieReleaseDates{Region: "DE"}},
		{"the US has nothing either", "US", metadata.MovieReleaseDates{Region: "US"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := d.ReleaseDatesFor(tc.region); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestIsReleaseCountry(t *testing.T) {
	for code, want := range map[string]bool{"IN": true, "US": true, "in": false, "": false, "ZZ": false, "USA": false} {
		if got := metadata.IsReleaseCountry(code); got != want {
			t.Errorf("IsReleaseCountry(%q) = %v, want %v", code, got, want)
		}
	}
}
