package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
)

// The library_search inputs a real assistant sends (Home Assistant sent
// kind "movies", query "action", limit 5 and got an empty error back): every
// one of them must end in a normal tool result, never a protocol error.
func TestMCPLibrarySearchInputs(t *testing.T) {
	e := newMCPEnv(t)
	e.enable(t)
	e.library(t)
	s := e.server
	if _, err := s.MovieRepo.Add(library.Movie{TMDBID: 801, Title: "Rooftop Run", Year: 2020, Monitored: true, Genres: []string{"Action", "Thriller"}, Overview: "A courier races across the city."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MovieRepo.Add(library.Movie{TMDBID: 802, Title: "Quiet Orbit", Year: 2022, Monitored: true, Genres: []string{"Science Fiction"}, Overview: "A drama on a station."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MovieRepo.AddSeries(library.Series{TMDBID: 9100, Title: "Skyline Patrol", Year: 2021, Monitored: true, Genres: []string{"Action & Adventure"}}, nil); err != nil {
		t.Fatal(err)
	}

	type result struct {
		isErr bool
		text  string
		p     map[string]any
	}
	total := func(n float64) func(*testing.T, result) {
		return func(t *testing.T, r result) {
			if r.isErr || r.p["total"] != n {
				t.Fatalf("want total %v, got isError=%v %s", n, r.isErr, r.text)
			}
		}
	}
	first := func(title string) func(*testing.T, result) {
		return func(t *testing.T, r result) {
			rows, _ := r.p["results"].([]any)
			if r.isErr || len(rows) == 0 || rows[0].(map[string]any)["title"] != title {
				t.Fatalf("want %q first, got %s", title, r.text)
			}
		}
	}
	failsWith := func(s string) func(*testing.T, result) {
		return func(t *testing.T, r result) {
			if !r.isErr || !strings.Contains(r.text, s) {
				t.Fatalf("want an error mentioning %q, got isError=%v %s", s, r.isErr, r.text)
			}
		}
	}
	tests := []struct {
		name string
		args map[string]any
		ok   func(*testing.T, result)
	}{
		{"home assistant call: movies, action, 5", map[string]any{"kind": "movies", "query": "action", "limit": 5}, func(t *testing.T, r result) { total(1)(t, r); first("Rooftop Run")(t, r) }},
		{"genre argument", map[string]any{"genre": "action"}, func(t *testing.T, r result) { total(1)(t, r); first("Rooftop Run")(t, r) }},
		{"genre sci-fi spelling", map[string]any{"genre": "Sci-Fi"}, func(t *testing.T, r result) { total(1)(t, r); first("Quiet Orbit")(t, r) }},
		{"query matches the description", map[string]any{"query": "courier"}, total(1)},
		{"nothing matches", map[string]any{"query": "zzzz-nothing"}, func(t *testing.T, r result) {
			total(0)(t, r)
			if m, _ := r.p["message"].(string); !strings.Contains(m, "No titles matched") || !strings.Contains(m, "5 movies") {
				t.Fatalf("message: %s", r.text)
			}
		}},
		{"singular kind", map[string]any{"kind": "movie", "query": "tide"}, total(1)},
		{"capitals and spaces", map[string]any{"kind": " Films ", "query": "tide"}, total(1)},
		{"limit as text", map[string]any{"limit": "2"}, func(t *testing.T, r result) {
			if r.isErr || len(r.p["results"].([]any)) != 2 {
				t.Fatalf("%s", r.text)
			}
		}},
		{"limit with a fraction", map[string]any{"limit": 2.0}, total(5)},
		{"nulls for options left out", map[string]any{"limit": nil, "query": nil, "has_file": nil, "kind": nil}, total(5)},
		{"boolean as text", map[string]any{"has_file": "false"}, total(3)},
		{"empty strings", map[string]any{"limit": "", "has_file": ""}, total(5)},
		{"series kind", map[string]any{"kind": "series"}, total(2)},
		{"shows kind with genre", map[string]any{"kind": "shows", "genre": "action"}, total(1)},
		{"tv shows", map[string]any{"kind": "TV Shows", "query": "salt"}, total(1)},
		{"nothing matches in tv", map[string]any{"kind": "tv", "query": "zzzz"}, func(t *testing.T, r result) {
			if m, _ := r.p["message"].(string); !strings.Contains(m, "No titles matched") {
				t.Fatalf("%s", r.text)
			}
		}},
		{"music kind (module is off)", map[string]any{"kind": "music"}, failsWith("switched off")},
		{"book kind (module is off)", map[string]any{"kind": "book"}, failsWith("switched off")},
		{"unknown kind", map[string]any{"kind": "games"}, failsWith("kind must be")},
		{"limit that is not a number", map[string]any{"limit": "many"}, failsWith("schema")},
		{"query is a list", map[string]any{"query": []string{"a"}}, failsWith("schema")},
		{"size filter", map[string]any{"min_size_gb": "1"}, total(0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isErr, text, parsed := e.call(t, e.roKey, "library_search", tc.args) // e.call fails the test on any HTTP or JSON-RPC error
			tc.ok(t, result{isErr, text, parsed})
		})
	}

	// A client that only accepts an event stream gets the answer as one event.
	status, _, resp := e.rpc(t, e.roKey, rpcBody(1, "tools/call", map[string]any{"name": "library_search", "arguments": map[string]any{"query": "zzzz"}}), "Accept", "text/event-stream")
	if status != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream framing: %d %s", status, resp.Header.Get("Content-Type"))
	}
}
