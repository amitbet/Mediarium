package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func detailsOf(ev Event) map[string]string {
	m := map[string]string{}
	for _, d := range ev.Details {
		m[d.Label] = d.Value
	}
	return m
}

func TestComposeShowsReleaseDetailsOnlyWhenKnown(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	full := Item{
		Media: "movie", Title: "Dune", Year: 2021, Quality: "Bluray-1080p", SizeBytes: 8 << 30, Release: "Dune.2021.1080p.BluRay.x264-GRP",
		ScoreKnown: true, Score: 1750, Formats: []string{"HDR", "Remux", "x264", "Atmos", "Repack", "Extra", "Another"},
		Group: "GRP", Indexer: "NZBgeek",
	}
	tests := []struct {
		name      string
		event     string
		item      Item
		wantRows  map[string]string
		wantNoRow []string
	}{
		{"grab with everything", "grabbed", full, map[string]string{
			"Release group": "GRP", "Indexer": "NZBgeek", "Custom format score": "+1750", "Custom formats": "HDR, Remux, x264, Atmos, Repack", "Quality": "Bluray-1080p",
		}, nil},
		{"import with everything", "imported", full, map[string]string{"Release group": "GRP", "Custom format score": "+1750"}, nil},
		{"negative score has its sign", "grabbed", Item{Title: "A", ScoreKnown: true, Score: -200}, map[string]string{"Custom format score": "-200"}, []string{"Custom formats"}},
		{"zero score is shown when the profile scores", "grabbed", Item{Title: "A", ScoreKnown: true}, map[string]string{"Custom format score": "0"}, nil},
		{"nothing known adds no rows", "grabbed", Item{Title: "A", Quality: "WEBDL-1080p"}, nil, []string{"Release group", "Indexer", "Custom format score", "Custom formats"}},
		{"a failure does not carry them", "failed", full, nil, []string{"Release group", "Indexer", "Custom format score"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows := detailsOf(Compose(tc.event, tc.item, Links{}, at))
			for k, v := range tc.wantRows {
				if rows[k] != v {
					t.Errorf("row %q = %q, want %q (rows: %v)", k, rows[k], v, rows)
				}
			}
			for _, k := range tc.wantNoRow {
				if _, ok := rows[k]; ok {
					t.Errorf("row %q should not be there: %v", k, rows)
				}
			}
		})
	}
}

func TestTopFormats(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"a", "", " b ", "a"}, "a,b"},
		{[]string{"1", "2", "3", "4", "5", "6", "7"}, "1,2,3,4,5"},
	}
	for _, tc := range tests {
		if got := strings.Join(topFormats(tc.in), ","); got != tc.want {
			t.Errorf("topFormats(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWebhookAddsReleaseFields(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	it := Item{Media: "movie", Title: "Dune", Quality: "Bluray-1080p", SizeBytes: 1000, ScoreKnown: true, Score: 5, Formats: []string{"HDR"}, Group: "GRP", Indexer: "Geek"}
	tests := []struct {
		name     string
		item     Item
		template string
		check    func(t *testing.T, body string)
	}{
		{"default payload has the new fields", it, "", func(t *testing.T, body string) {
			var m map[string]any
			if err := json.Unmarshal([]byte(body), &m); err != nil {
				t.Fatal(err)
			}
			if m["customFormatScore"] != float64(5) || m["releaseGroup"] != "GRP" || m["indexer"] != "Geek" || m["size"] != float64(1000) {
				t.Errorf("payload: %v", m)
			}
			if f, _ := m["customFormats"].([]any); len(f) != 1 || f[0] != "HDR" {
				t.Errorf("customFormats: %v", m["customFormats"])
			}
			if m["event"] != "grabbed" || m["title"] == "" {
				t.Errorf("the old fields are gone: %v", m)
			}
		}},
		{"unknown details are left out", Item{Media: "movie", Title: "Dune"}, "", func(t *testing.T, body string) {
			for _, k := range []string{"customFormatScore", "customFormats", "releaseGroup", "indexer", "size"} {
				if strings.Contains(body, `"`+k+`"`) {
					t.Errorf("%s should not be in %s", k, body)
				}
			}
		}},
		{"template placeholders", it, `{"s":"{{customFormatScore}}","f":"{{customFormats}}","g":"{{releaseGroup}}","i":"{{indexer}}","b":"{{size}}","t":"{{sizeText}}"}`, func(t *testing.T, body string) {
			want := `{"s":"5","f":"HDR","g":"GRP","i":"Geek","b":"1000","t":"1000 B"}`
			if body != want {
				t.Errorf("got %s, want %s", body, want)
			}
		}},
		{"template placeholders empty when unknown", Item{Title: "x"}, `{"s":"{{customFormatScore}}","g":"{{releaseGroup}}"}`, func(t *testing.T, body string) {
			if body != `{"s":"","g":""}` {
				t.Errorf("got %s", body)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				got = string(b)
			}))
			defer srv.Close()
			s := &WebhookSender{URL: srv.URL, Client: srv.Client(), Template: tc.template}
			if err := s.Send(context.Background(), Compose("grabbed", tc.item, Links{}, at)); err != nil {
				t.Fatal(err)
			}
			tc.check(t, got)
		})
	}
}
