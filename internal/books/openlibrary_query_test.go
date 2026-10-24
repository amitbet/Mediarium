package books

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestPlainQuery(t *testing.T) {
	cases := []struct{ in, want string }{
		{"dune", "dune"},
		{"The Hitchhiker's Guide", "The Hitchhiker's Guide"},
		{"title: dune", "title dune"},
		{"dune:", "dune"},
		{`"dune`, "dune"},
		{"harry potter (book", "harry potter book"},
		{"spider-man [1", "spider man 1"},
		{"AND dune OR", "dune"},
		{"rock AND roll", "rock AND roll"},
		{"  a \t b  ", "a b"},
		{"+-:()", ""},
		{"", ""},
		{strings.Repeat("word ", 100), strings.TrimSpace(strings.Repeat("word ", 40))},
	}
	for _, tc := range cases {
		if got := PlainQuery(tc.in); got != tc.want {
			t.Errorf("PlainQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Open Library answers 422 for a search its syntax can't read. The search
// then retries with the plain words, and says so plainly when that fails too.
func TestSearchRetriesWithPlainWordsAfter422(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		mu.Lock()
		asked = append(asked, q)
		mu.Unlock()
		if strings.ContainsAny(q, ":(\"") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"invalid query"}`))
			return
		}
		if q == "nothing" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		_, _ = w.Write([]byte(`{"docs":[{"key":"/works/OL1W","title":"Dune","author_name":["Frank Herbert"],"first_publish_year":1965}]}`))
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, HTTP: srv.Client()}

	found, err := c.Search(context.Background(), "dune: (messiah", 5)
	if err != nil || len(found) != 1 || found[0].Key != "OL1W" {
		t.Fatalf("search after 422: %+v %v", found, err)
	}
	if len(asked) != 2 || asked[0] != "dune: (messiah" || asked[1] != "dune messiah" {
		t.Fatalf("requests: %q", asked)
	}

	// Only punctuation: nothing to look for, so an empty list without asking again.
	asked = nil
	found, err = c.Search(context.Background(), ":(", 5)
	if err != nil || len(found) != 0 || len(asked) != 1 {
		t.Fatalf("punctuation only: %+v %v %q", found, err, asked)
	}

	// The plain words are refused too: the error says the search was refused.
	_, err = c.Search(context.Background(), "nothing", 5)
	if !errors.Is(err, ErrBadQuery) {
		t.Fatalf("a refused plain search: %v", err)
	}
}
