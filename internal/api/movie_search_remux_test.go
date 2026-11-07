package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
)

// GitHub #121 (reported by Hoopes80): "The Nightmare Before Christmas" (1993)
// showed "No matching releases right now" although NZBgeek had a 1080p remux of
// it. This runs that exact release name, at the size of a real remux, through
// the whole manual search (indexer, parse, title and year match, profile, size
// limits) for every built-in quality profile. The list must always show the
// release: a profile that does not take a remux marks it rejected with the
// reason, it never hides it.
func TestManualSearchListsAFramestorRemuxUnderEveryProfile(t *testing.T) {
	const release = "The Nightmare Before Christmas 1993 PROPER BluRay 1080p DTS HD MA 7 1 AVC REMUX FraMeSToR"
	const remuxBytes = 28 * 1024 * 1024 * 1024
	indexer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0"?><rss version="2.0" xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/"><channel><item><title>%s</title><guid>g1</guid><enclosure url="http://%s/nzb/x.nzb" length="%d" type="application/x-nzb"/><newznab:attr name="size" value="%d"/><newznab:attr name="category" value="2040"/></item></channel></rss>`, release, r.Host, remuxBytes, remuxBytes)
	}))
	t.Cleanup(indexer.Close)

	for _, profile := range []string{"Cinema recordings", "720p", "1080p", "4K & over", "Any"} {
		for _, lib := range []struct {
			name  string
			title string
			year  int
			shown bool
		}{
			{"library title as TMDB writes it", "The Nightmare Before Christmas", 1993, true},
			{"library year unknown", "The Nightmare Before Christmas", 0, true},
			// A different year in the library is a different film to the matcher.
			{"library year differs from the release", "The Nightmare Before Christmas", 2006, false},
		} {
			t.Run(profile+"/"+lib.name, func(t *testing.T) {
				server, base, client := loginNewServer(t)
				postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
					"name": "NZBgeek", "definitionId": "fixture", "baseUrl": indexer.URL, "apiKey": "k",
				}, http.StatusCreated)
				m, err := server.MovieRepo.Add(library.Movie{TMDBID: 9479, Title: lib.title, Year: lib.year, Monitored: true})
				if err != nil {
					t.Fatal(err)
				}
				list := getJSON[map[string]any](t, client, base+"/api/quality-profiles")
				putJSONStatus(t, client, fmt.Sprintf("%s/api/movies/%d/profile", base, m.ID), map[string]any{"profileId": profileIDByName(t, list, profile)}, http.StatusOK)

				got := getJSON[map[string]any](t, client, fmt.Sprintf("%s/api/movies/%d/search?detail=1", base, m.ID))
				results, _ := got["results"].([]any)
				if lib.shown && len(results) != 1 {
					t.Fatalf("the remux is not in the list under %q: %+v", profile, got)
				}
				if !lib.shown && len(results) != 0 {
					t.Fatalf("a release of another year was listed: %+v", got)
				}
				// A release left out for its year is counted and named, so the page
				// does not claim the indexers had nothing.
				ignored, _ := got["ignored"].(float64)
				samples, _ := got["ignoredSamples"].([]any)
				if !lib.shown && (ignored != 1 || len(samples) != 1 || samples[0] != release) {
					t.Fatalf("the left-out release is not reported: %+v", got)
				}
				if lib.shown && ignored != 0 {
					t.Fatalf("a listed release was also counted as left out: %+v", got)
				}
				if lib.shown {
					r := results[0].(map[string]any)
					if r["title"] != release {
						t.Fatalf("listed as %v", r["title"])
					}
					t.Logf("rejections under %q: %v", profile, r["rejections"])
				}
			})
		}
	}
}
