package subtitles

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// SubDL's tmdb_id search says "can't find movie or tv" for a title it does
// have under its IMDb id or name. The search must not stop at the first miss.
func TestSubDLFallsBackToImdbThenName(t *testing.T) {
	const hit = `{"status":true,"subtitles":[{"release_name":"The.Sixth.Sense.1999.1080p.BluRay-X","name":"a.zip","url":"/subtitle/1-2.zip"}]}`
	const miss = `{"status":false,"error":"can't find movie or tv"}`
	tests := []struct {
		name     string
		answers  func(q map[string][]string) string
		wantKeys []string // the lookups made, in order
		want     int
	}{
		{"tmdb works", func(q map[string][]string) string {
			if len(q["tmdb_id"]) > 0 {
				return hit
			}
			return miss
		}, []string{"tmdb_id"}, 1},
		{"imdb works", func(q map[string][]string) string {
			if len(q["imdb_id"]) > 0 {
				return hit
			}
			return miss
		}, []string{"tmdb_id", "imdb_id"}, 1},
		{"only the name works", func(q map[string][]string) string {
			if len(q["film_name"]) > 0 {
				return hit
			}
			return miss
		}, []string{"tmdb_id", "imdb_id", "film_name"}, 1},
		// Each lookup is tried again with fewer filters before giving up.
		{"nothing works", func(q map[string][]string) string { return miss }, []string{"tmdb_id", "imdb_id", "film_name", "tmdb_id", "imdb_id", "film_name"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var keys []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				mu.Lock()
				for _, k := range []string{"tmdb_id", "imdb_id", "film_name"} {
					if q.Get(k) != "" {
						keys = append(keys, k)
					}
				}
				mu.Unlock()
				if q.Get("film_name") != "" && ((q.Get("year") != "1999" && q.Get("hi") != "") || q.Get("film_name") != "The Sixth Sense") {
					t.Errorf("name search: %s", q.Encode())
				}
				if q.Get("type") != "movie" || (q.Get("languages") != "EN" && q.Get("hi") != "") {
					t.Errorf("type/languages: %s", q.Encode())
				}
				fmt.Fprint(w, tc.answers(q))
			}))
			defer srv.Close()
			c := NewSubDLWithBaseURL("k", srv.URL, srv.URL)
			res, err := c.Find(context.Background(), Query{TMDBID: 745, IMDBID: "tt0167404", Title: "The Sixth Sense", Year: 1999, Type: "movie", Language: "en"})
			if err != nil {
				t.Fatal(err)
			}
			if len(res) != tc.want {
				t.Fatalf("results: %d, want %d", len(res), tc.want)
			}
			if strings.Join(keys, ",") != strings.Join(tc.wantKeys, ",") {
				t.Fatalf("lookups %v, want %v", keys, tc.wantKeys)
			}
		})
	}
}

// A title with more than one page of subtitles is read page by page, up to a limit.
func TestSubDLPages(t *testing.T) {
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("page")
		pages = append(pages, p)
		var items []string
		for i := 0; i < 30; i++ {
			items = append(items, fmt.Sprintf(`{"release_name":"R%s-%d","name":"a.zip","url":"/subtitle/%s-%d.zip"}`, p, i, p, i))
		}
		fmt.Fprintf(w, `{"status":true,"subtitles":[%s]}`, strings.Join(items, ","))
	}))
	defer srv.Close()
	res, err := NewSubDLWithBaseURL("k", srv.URL, srv.URL).Find(context.Background(), Query{TMDBID: 745, Type: "movie", Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 90 || len(pages) != 3 {
		t.Fatalf("results %d, pages %v", len(res), pages)
	}
}

// SubDL download paths come in more shapes than "/subtitle/<id>-<id>.zip":
// any plain path under /subtitle/ is fine, a query, a ".." part or another
// start is not.
func TestSubDLLinkShapes(t *testing.T) {
	for link, want := range map[string]bool{
		"/subtitle/3197651-3213944.zip":       true,
		"/subtitle/3197651-3213944.1.zip":     true,
		"/subtitle/sd1638677/3213944":         true,
		"/subtitle/a%20b.zip":                 true,
		"/subtitle/3197651-3213944.zip?x=1":   false,
		"/subtitle/../etc/passwd":             false,
		"/subtitle/a/../b.zip":                false,
		"/other/3197651.zip":                  false,
		"https://evil.example/subtitle/1.zip": false,
		"":                                    false,
	} {
		if got := ValidSubDLLink(link); got != want {
			t.Errorf("ValidSubDLLink(%q) = %v, want %v", link, got, want)
		}
	}
	if got := shortSample("/subtitle/1.zip?api_key=SECRET"); strings.Contains(got, "SECRET") || strings.Contains(got, "?") {
		t.Errorf("sample shows the query: %q", got)
	}
}

// SubDL's "url" may carry a query or a fragment, or come with the download
// host in front: only the path is asked for.
func TestSubDLDownloadPathDropsQueryAndHost(t *testing.T) {
	for in, want := range map[string]string{
		"/subtitle/3672321-8589191.zip":                         "/subtitle/3672321-8589191.zip",
		"/subtitle/3672321-8589191.zip?download=1&x=y":          "/subtitle/3672321-8589191.zip",
		"/subtitle/3672321-8589191.zip#top":                     "/subtitle/3672321-8589191.zip",
		"https://dl.subdl.com/subtitle/3672321-8589191.zip?a=b": "/subtitle/3672321-8589191.zip",
		"//dl.subdl.com/subtitle/3672321-8589191.zip":           "/subtitle/3672321-8589191.zip",
		"  /subtitle/3672321-8589191.zip  ":                     "/subtitle/3672321-8589191.zip",
	} {
		got := downloadPath(in)
		if got != want || !ValidSubDLLink(got) {
			t.Errorf("downloadPath(%q) = %q (valid %v), want %q", in, got, ValidSubDLLink(got), want)
		}
	}
	if got := shortSample("/subtitle/1.zip?api_key=SECRET"); strings.Contains(got, "SECRET") {
		t.Errorf("sample shows the query: %q", got)
	}
}
