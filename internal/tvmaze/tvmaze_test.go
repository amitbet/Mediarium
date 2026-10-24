package tvmaze

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const showJSON = `{"id":82,"name":"Example Show","status":"Running","premiered":"2011-04-17",
 "network":{"name":"HBO","country":{"name":"United States","code":"US","timezone":"America/New_York"}},"webChannel":null}`

const episodesJSON = `[
 {"id":1,"season":1,"number":1,"airdate":"2011-04-17","airtime":"21:00","airstamp":"2011-04-18T01:00:00+00:00","runtime":60},
 {"id":2,"season":1,"number":2,"airdate":"2011-04-24","airtime":"","airstamp":"2011-04-24T12:00:00+00:00","runtime":null},
 {"id":3,"season":1,"number":null,"airdate":"2011-12-01","airtime":"","airstamp":"","runtime":30}
]`

func fake(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("User-Agent") != "Mediarium/9.9.9" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/lookup/shows" && q.Get("thetvdb") == "121361":
			w.Write([]byte(showJSON))
		case r.URL.Path == "/lookup/shows" && q.Get("imdb") == "tt0944947":
			w.Write([]byte(showJSON))
		case r.URL.Path == "/singlesearch/shows" && q.Get("q") == "Example Show":
			w.Write([]byte(showJSON))
		case r.URL.Path == "/shows/82/episodes":
			w.Write([]byte(episodesJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTest(srv *httptest.Server) *Client {
	return New("9.9.9", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(NewLimiter(1000, time.Second)))
}

func TestLookups(t *testing.T) {
	var calls atomic.Int32
	c := newTest(fake(t, &calls))
	ctx := context.Background()
	tests := []struct {
		name    string
		call    func() (Show, error)
		wantID  int
		wantErr error
	}{
		{"by tvdb id", func() (Show, error) { return c.LookupTVDB(ctx, 121361) }, 82, nil},
		{"by imdb id", func() (Show, error) { return c.LookupIMDb(ctx, "tt0944947") }, 82, nil},
		{"unknown tvdb id", func() (Show, error) { return c.LookupTVDB(ctx, 5) }, 0, ErrNotFound},
		{"search with the right year", func() (Show, error) { return c.Search(ctx, "Example Show", 2011) }, 82, nil},
		{"search a year off is fine", func() (Show, error) { return c.Search(ctx, "Example Show", 2012) }, 82, nil},
		{"search with no year known", func() (Show, error) { return c.Search(ctx, "Example Show", 0) }, 82, nil},
		{"search for a different show of that name", func() (Show, error) { return c.Search(ctx, "Example Show", 1999) }, 0, ErrNotFound},
		{"search finds nothing", func() (Show, error) { return c.Search(ctx, "Nothing", 2011) }, 0, ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := tc.call()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if s.ID != tc.wantID {
				t.Fatalf("id = %d, want %d", s.ID, tc.wantID)
			}
			if tc.wantErr == nil && (s.Timezone != "America/New_York" || s.Year() != 2011) {
				t.Fatalf("show = %+v", s)
			}
		})
	}
}

func TestEpisodes(t *testing.T) {
	var calls atomic.Int32
	c := newTest(fake(t, &calls))
	eps, err := c.Episodes(context.Background(), 82)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 3 {
		t.Fatalf("got %d episodes", len(eps))
	}
	tests := []struct {
		name    string
		ep      Episode
		wantAt  string // "" = no time
		runtime int
	}{
		{"with a time", eps[0], "2011-04-18T01:00:00Z", 60},
		{"placeholder stamp but no time", eps[1], "", 0},
		{"unnumbered special with nothing", eps[2], "", 30},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			at, ok := tc.ep.AirAt()
			if tc.wantAt == "" {
				if ok {
					t.Fatalf("AirAt = %v, want none", at)
				}
			} else if !ok || at.Format(time.RFC3339) != tc.wantAt {
				t.Fatalf("AirAt = %v %v, want %s", at, ok, tc.wantAt)
			}
			if tc.ep.Runtime != tc.runtime {
				t.Fatalf("runtime = %d", tc.ep.Runtime)
			}
		})
	}
	if eps[2].Number != 0 {
		t.Fatalf("a null number should read as 0, got %d", eps[2].Number)
	}
}

func TestOffsetStampIsUTC(t *testing.T) {
	e := Episode{Airtime: "21:00", Airstamp: "2024-03-04T21:00:00-08:00"}
	at, ok := e.AirAt()
	if !ok || at.Format(time.RFC3339) != "2024-03-05T05:00:00Z" {
		t.Fatalf("got %v %v", at, ok)
	}
}

func TestRateLimited(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		first429s  int32
		wantErr    error
	}{
		{"one 429 is waited out", "0", 1, nil},
		{"a second 429 gives up", "0", 2, ErrRateLimited},
		{"a long wait gives up at once", "600", 1, ErrRateLimited},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var n atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if n.Add(1) <= tc.first429s {
					w.Header().Set("Retry-After", tc.retryAfter)
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				w.Write([]byte(showJSON))
			}))
			defer srv.Close()
			_, err := newTest(srv).LookupTVDB(context.Background(), 1)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestServerErrorAndBadJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
	}{
		{"server error", 500, ""},
		{"not json", 200, "<html>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			if _, err := newTest(srv).LookupTVDB(context.Background(), 1); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLimiterSpacesCalls(t *testing.T) {
	l := NewLimiter(2, 100*time.Millisecond)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 90*time.Millisecond {
		t.Fatalf("third call came after %v, want it held back", d)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	l2 := NewLimiter(1, time.Hour)
	_ = l2.Wait(context.Background())
	if err := l2.Wait(ctx); err == nil {
		t.Fatal("a cancelled wait should fail")
	}
}
