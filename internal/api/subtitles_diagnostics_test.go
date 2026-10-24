package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type subtitleSearchAnswer struct {
	Results     []map[string]any `json:"results"`
	Diagnostics []map[string]any `json:"diagnostics"`
}

func searchSubtitlesAnswer(t *testing.T, env *tvAutoEnv, url string) subtitleSearchAnswer {
	t.Helper()
	return getJSON[subtitleSearchAnswer](t, env.client, url)
}

// subtitleSearchResults is the results part of a search answer.
func subtitleSearchResults(t *testing.T, client *http.Client, url string) []map[string]any {
	t.Helper()
	return getJSON[subtitleSearchAnswer](t, client, url).Results
}

// A movie that already has its English subtitle still lists what SubDL has when
// searched by hand, and an answer with no subtitles at first does not stop the
// next lookup.
func TestManualSubDLSearchListsResultsAndExplainsAnEmptyOne(t *testing.T) {
	env := newTVAutoEnv(t, nil, nil)
	env.server.TestSetSubtitlesBaseURL("", "http://127.0.0.1:1") // OpenSubtitles off

	var mu sync.Mutex
	var queries []string
	titleHasEnglish := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		has := titleHasEnglish
		mu.Unlock()
		// Shaped like SubDL's real answer: "status true", the matching titles,
		// and subtitles with a long "lang" and a short "language".
		if q.Get("film_name") != "" && has {
			fmt.Fprint(w, `{"status":true,"results":[{"sd_id":1638677,"type":"movie","name":"Some Movie","imdb_id":"tt0000009","tmdb_id":9,"year":2001}],"subtitles":[
			 {"release_name":"Some.Movie.2001.1080p.WEB-GRP","name":"SUBDL::Some.Movie","lang":"english","language":"EN","author":"a","url":"/subtitle/11-22.zip","subtitlePage":"/s/x","season":0,"episode":null,"hi":false,"full_season":false},
			 {"release_name":"Some.Movie.2001.HI","name":"SUBDL::Some.Movie.hi","lang":"english","language":"EN","url":"/subtitle/11-23.zip","hi":true},
			 {"release_name":"Some.Movie.2001.fr","name":"SUBDL::Some.Movie.fr","lang":"french","language":"FR","url":"/subtitle/11-24.zip"}]}`)
			return
		}
		fmt.Fprint(w, `{"status":true,"results":[],"subtitles":[]}`)
	}))
	defer srv.Close()
	env.server.TestSetSubDLBaseURL("subdl-key", srv.URL)

	dir := t.TempDir()
	video := filepath.Join(dir, "Some Movie (2001).mkv")
	os.WriteFile(video, []byte("v"), 0o644)
	os.WriteFile(filepath.Join(dir, "Some Movie (2001).en.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nHave\n"), 0o644)
	m := addDownloadedMovie(t, env, video)
	url := fmt.Sprintf("%s/api/movies/%d/subtitles?lang=en", env.baseURL, m.ID)

	got := searchSubtitlesAnswer(t, env, url)
	if len(got.Results) != 2 || got.Results[0]["link"] != "/subtitle/11-22.zip" || got.Results[1]["variant"] != "sdh" {
		t.Fatalf("results (plain first, hearing-impaired kept but lower): %+v", got.Results)
	}
	if len(got.Diagnostics) < 2 || got.Diagnostics[0]["lookup"] != "tmdb" || got.Diagnostics[0]["count"] != float64(0) {
		t.Fatalf("diagnostics: %+v", got.Diagnostics)
	}
	if last := got.Diagnostics[len(got.Diagnostics)-1]; last["lookup"] != "title" || last["count"] != float64(2) {
		t.Fatalf("last line: %+v", last)
	}

	// Nothing anywhere: the answer says what was asked, and no key is in it.
	mu.Lock()
	titleHasEnglish = false
	queries = nil
	mu.Unlock()
	got = searchSubtitlesAnswer(t, env, url)
	if len(got.Results) != 0 || len(got.Diagnostics) < 4 {
		t.Fatalf("empty search: %+v", got)
	}
	if fmt.Sprint(got.Diagnostics) == "" || strings.Contains(fmt.Sprint(got.Diagnostics), "subdl-key") {
		t.Fatalf("diagnostics leak or are empty: %+v", got.Diagnostics)
	}
	if bare := got.Diagnostics[len(got.Diagnostics)-1]; bare["bare"] != true {
		t.Fatalf("the last try should be the one with fewer filters: %+v", bare)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(queries) < 4 {
		t.Fatalf("queries: %v", queries)
	}
}
