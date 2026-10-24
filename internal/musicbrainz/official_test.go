package musicbrainz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// A fake release browse: each release group has a list of release statuses;
// the status parameter narrows it like the real server does.
func TestHasOfficialRelease(t *testing.T) {
	groups := map[string][]string{
		"official":    {"Official"},
		"bootleg":     {"Bootleg"},
		"mixed":       {"Bootleg", "Promotion", "Official"},
		"promo":       {"Promotion", "Pseudo-Release"},
		"announced":   {},
		"only-pseudo": {"Pseudo-Release"},
	}
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		calls = append(calls, q.Get("release-group")+"/"+q.Get("status"))
		mu.Unlock()
		out := `{"releases":[`
		n := 0
		for _, st := range groups[q.Get("release-group")] {
			if want := q.Get("status"); want != "" && want != "official" || want == "official" && st != "Official" {
				continue
			}
			if n >= 1 { // limit=1
				break
			}
			if n > 0 {
				out += ","
			}
			out += `{"id":"r","status":"` + st + `"}`
			n++
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(out + `]}`))
	}))
	defer srv.Close()
	c := New("test", WithBaseURL(srv.URL), WithRateLimit(0), WithCacheTTL(0))

	cases := []struct {
		group     string
		want      bool
		wantCalls int
	}{
		{"official", true, 1},
		{"mixed", true, 1},
		{"bootleg", false, 2},
		{"promo", false, 2},
		{"only-pseudo", false, 2},
		{"announced", true, 2}, // no release at all: not a bootleg
	}
	for _, tc := range cases {
		mu.Lock()
		calls = nil
		mu.Unlock()
		got, err := c.HasOfficialRelease(context.Background(), tc.group)
		if err != nil {
			t.Errorf("%s: %v", tc.group, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: official = %v, want %v", tc.group, got, tc.want)
		}
		mu.Lock()
		n := len(calls)
		first := ""
		if n > 0 {
			first = calls[0]
		}
		mu.Unlock()
		if n != tc.wantCalls {
			t.Errorf("%s: %d requests, want %d", tc.group, n, tc.wantCalls)
		}
		if first != tc.group+"/official" {
			t.Errorf("%s: the first request was %q, it should ask for official releases only", tc.group, first)
		}
	}
}

func TestHasOfficialReleaseKeepsToTheRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"releases":[{"id":"r","status":"Official"}]}`))
	}))
	defer srv.Close()
	const gap = 60 * time.Millisecond
	c := New("test", WithBaseURL(srv.URL), WithRateLimit(gap), WithCacheTTL(0))
	start := time.Now()
	for _, id := range []string{"a", "b", "c"} {
		if _, err := c.HasOfficialRelease(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	// Three requests need two gaps between them.
	if took := time.Since(start); took < 2*gap {
		t.Errorf("three look-ups took %v, less than the two gaps the rate limit asks for", took)
	}
}

func TestHasOfficialReleaseReportsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := New("test", WithBaseURL(srv.URL), WithRateLimit(0))
	if _, err := c.HasOfficialRelease(context.Background(), "x"); err == nil {
		t.Error("an error from the server must not be taken for a bootleg")
	}
}
