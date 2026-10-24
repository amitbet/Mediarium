package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Open Library answers 422 when the search text isn't valid in its query
// syntax. A search that only needs its punctuation dropped still finds the
// book; one it keeps refusing tells the person so instead of showing an
// empty list or a vague "try again".
func TestBookSearchSurvivesOpenLibrary422(t *testing.T) {
	server, base, client := loginNewServer(t)
	ol := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.ContainsAny(q, ":(\"") || q == "refused" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"error":"invalid"}`)
			return
		}
		fmt.Fprint(w, `{"docs":[{"key":"/works/OL1W","title":"The Hobbit","author_name":["J.R.R. Tolkien"],"first_publish_year":1937,"cover_i":123}]}`)
	}))
	defer ol.Close()
	server.TestSetOpenLibrary(ol.URL)
	postJSONMethod[map[string]any](t, client, http.MethodPut, base+"/api/modules", map[string]any{"ebooks": true}, http.StatusOK)

	found := getJSON[[]map[string]any](t, client, base+"/api/books/search?q="+url.QueryEscape(`the hobbit: (tolkien`))
	if len(found) != 1 || found[0]["key"] != "OL1W" {
		t.Fatalf("search with punctuation: %v", found)
	}

	status, body := doStatus(t, client, http.MethodGet, base+"/api/books/search?q=refused")
	if status != http.StatusUnprocessableEntity || !strings.Contains(fmt.Sprint(body["error"]), "couldn't read that search") {
		t.Fatalf("refused search: %d %v", status, body)
	}
}
