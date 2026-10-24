package metadata_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rdborg/mediarium/internal/metadata"
)

func TestDiscoverQueryParams(t *testing.T) {
	var last url.Values
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last, lastPath = r.URL.Query(), r.URL.Path
		w.Write([]byte(`{"page":2,"total_pages":900,"results":[{"id":1,"title":"A","name":"A"}]}`))
	}))
	defer srv.Close()
	c := metadata.NewWithBaseURL("k", srv.URL)
	ctx := context.Background()

	cases := []struct {
		name  string
		tv    bool
		q     metadata.DiscoverQuery
		want  map[string]string // "" = must be absent
		wantP string
	}{
		{"movie defaults", false, metadata.DiscoverQuery{}, map[string]string{
			"sort_by": "popularity.desc", "page": "1", "with_genres": "", "primary_release_year": "", "vote_count.gte": "",
		}, "/discover/movie"},
		{"movie genre, exact year, rating", false, metadata.DiscoverQuery{Genre: 18, Year: 1999, YearFrom: 1980, Sort: metadata.SortRating, Page: 3}, map[string]string{
			"with_genres": "18", "primary_release_year": "1999", "primary_release_date.gte": "",
			"sort_by": "vote_average.desc", "vote_count.gte": "200", "page": "3",
		}, "/discover/movie"},
		{"movie range, newest", false, metadata.DiscoverQuery{YearFrom: 1990, YearTo: 1999, Sort: metadata.SortNewest}, map[string]string{
			"primary_release_date.gte": "1990-01-01", "primary_release_date.lte": "1999-12-31", "sort_by": "primary_release_date.desc",
		}, "/discover/movie"},
		{"tv exact year, oldest", true, metadata.DiscoverQuery{Year: 2011, Sort: metadata.SortOldest}, map[string]string{
			"first_air_date_year": "2011", "sort_by": "first_air_date.asc",
		}, "/discover/tv"},
		{"tv range from only, page capped", true, metadata.DiscoverQuery{YearFrom: 2020, Page: 9999}, map[string]string{
			"first_air_date.gte": "2020-01-01", "first_air_date.lte": "", "page": "500",
		}, "/discover/tv"},
		{"movie korean, 7+, long", false, metadata.DiscoverQuery{Language: "ko", MinRating: 7, Runtime: metadata.RuntimeLong}, map[string]string{
			"with_original_language": "ko", "vote_average.gte": "7", "vote_count.gte": "100", "with_runtime.gte": "121", "with_runtime.lte": "",
		}, "/discover/movie"},
		{"tv japanese, 8+, short", true, metadata.DiscoverQuery{Language: "ja", MinRating: 8, Runtime: metadata.RuntimeShort}, map[string]string{
			"with_original_language": "ja", "vote_average.gte": "8", "vote_count.gte": "50", "with_runtime.lte": "89", "with_runtime.gte": "",
		}, "/discover/tv"},
		{"movie 90 to 120", false, metadata.DiscoverQuery{Runtime: metadata.RuntimeMedium}, map[string]string{
			"with_runtime.gte": "90", "with_runtime.lte": "120", "vote_count.gte": "",
		}, "/discover/movie"},
		{"best rated keeps the higher vote floor", false, metadata.DiscoverQuery{MinRating: 6, Sort: metadata.SortRating}, map[string]string{
			"vote_average.gte": "6", "vote_count.gte": "200", "sort_by": "vote_average.desc",
		}, "/discover/movie"},
	}
	for _, tc := range cases {
		var total int
		if tc.tv {
			p, err := c.DiscoverTV(ctx, tc.q)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			total = p.TotalPages
		} else {
			p, err := c.DiscoverMovies(ctx, tc.q)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			total = p.TotalPages
		}
		if total != 500 {
			t.Fatalf("%s: total pages should be capped at 500, got %d", tc.name, total)
		}
		if lastPath != tc.wantP {
			t.Fatalf("%s: path %s, want %s", tc.name, lastPath, tc.wantP)
		}
		for k, v := range tc.want {
			if got := last.Get(k); got != v {
				t.Fatalf("%s: %s = %q, want %q (query %v)", tc.name, k, got, v, last)
			}
		}
	}
}

func TestGenresListSorted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"},{"id":28,"name":"Action"}]}`))
	}))
	defer srv.Close()
	c := metadata.NewWithBaseURL("k", srv.URL)
	got, err := c.Genres(context.Background(), "tv")
	if err != nil || len(got) != 2 || got[0].Name != "Action" || got[1].ID != 18 {
		t.Fatalf("unexpected genres %+v err=%v", got, err)
	}
	if _, err := c.Genres(context.Background(), "books"); err == nil {
		t.Fatal("unknown kind should be an error")
	}
}

func TestDiscoverFilterValidation(t *testing.T) {
	languages := map[string]bool{"": true, "ko": true, "en": true, "yue": true, "cn": true, "K": false, "k": false, "korean": false, "k0": false, "ko ": false, "ko&x": false, "é": false, "KO": false}
	for in, want := range languages {
		if got := metadata.ValidLanguage(in); got != want {
			t.Errorf("ValidLanguage(%q) = %v, want %v", in, got, want)
		}
	}
	ratings := map[int]bool{0: true, 6: true, 7: true, 8: true, 9: true, 1: false, 5: false, 10: false, -1: false}
	for in, want := range ratings {
		if got := metadata.ValidMinRating(in); got != want {
			t.Errorf("ValidMinRating(%d) = %v, want %v", in, got, want)
		}
	}
	runtimes := map[string]bool{"": true, "under90": true, "90to120": true, "over120": true, "120": false, "Over120": false, "over120&x=1": false}
	for in, want := range runtimes {
		if got := metadata.ValidRuntime(in); got != want {
			t.Errorf("ValidRuntime(%q) = %v, want %v", in, got, want)
		}
	}
}

// Two browses that differ only in a new filter must not share a cached page.
func TestDiscoverFiltersHaveOwnCacheEntries(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{"page":1,"total_pages":1,"results":[]}`))
	}))
	defer srv.Close()
	c := metadata.NewWithBaseURL("k", srv.URL)
	ctx := context.Background()
	for _, q := range []metadata.DiscoverQuery{{}, {Language: "ko"}, {MinRating: 7}, {Runtime: metadata.RuntimeLong}, {Language: "ko"}} {
		if _, err := c.DiscoverTV(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 4 {
		t.Fatalf("want 4 TMDB calls (the last browse is cached), got %d", hits)
	}
}
