// Package tvmaze looks up when episodes air on TVmaze (https://www.tvmaze.com),
// a free public TV database that needs no account or key. TMDB only knows the
// day an episode airs; TVmaze also gives the time, as an exact moment
// (`airstamp`), so the calendar can show it in the viewer's own time zone.
//
// Only a show's ids or name and year are sent, never anything about the person
// or the library. TVmaze's data is licensed CC BY-SA, so the app credits it.
// Requests say who Mediarium is in the User-Agent and go through netguard.
//
// The package depends on nothing else in Mediarium except netguard.
package tvmaze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rdborg/mediarium/internal/netguard"
)

// DefaultBaseURL is the public TVmaze API.
const DefaultBaseURL = "https://api.tvmaze.com"

const (
	requestTimeout = 10 * time.Second
	maxBodyBytes   = 8 << 20
	// maxRetryWait is the longest a 429 answer is waited out. A longer
	// Retry-After gives up, and the next refresh tries again.
	maxRetryWait = 15 * time.Second
	// callsPerWindow calls are allowed in each window: TVmaze's own limit is
	// at least 20 every 10 seconds per address.
	callsPerWindow = 20
	window         = 10 * time.Second
)

// ErrNotFound is returned when TVmaze does not know the show.
var ErrNotFound = errors.New("tvmaze does not know that show")

// ErrRateLimited is returned when TVmaze keeps asking to slow down.
var ErrRateLimited = errors.New("tvmaze asked to slow down")

// Limiter keeps calls under a number per window. One is shared by every
// client, so all of Mediarium together stays inside TVmaze's limit.
type Limiter struct {
	mu    sync.Mutex
	times []time.Time
	n     int
	span  time.Duration
}

// NewLimiter allows n calls in every span.
func NewLimiter(n int, span time.Duration) *Limiter {
	return &Limiter{n: n, span: span}
}

var shared = NewLimiter(callsPerWindow, window)

// Wait blocks until a call may be made, or ctx ends.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := time.Now()
		keep := l.times[:0]
		for _, t := range l.times {
			if now.Sub(t) < l.span {
				keep = append(keep, t)
			}
		}
		l.times = keep
		if len(l.times) < l.n {
			l.times = append(l.times, now)
			l.mu.Unlock()
			return nil
		}
		wait := l.span - now.Sub(l.times[0])
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// Client talks to one TVmaze server. It is safe for concurrent use.
type Client struct {
	baseURL   string
	userAgent string
	http      *http.Client
	limiter   *Limiter
}

// Option changes a Client made by New.
type Option func(*Client)

// WithBaseURL points the client at another server (a test fake).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient replaces the HTTP client (tests).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithLimiter replaces the shared rate limiter (tests).
func WithLimiter(l *Limiter) Option { return func(c *Client) { c.limiter = l } }

// UserAgent is the identifying User-Agent sent with every request.
func UserAgent(version string) string {
	if version == "" {
		version = "dev"
	}
	return "Mediarium/" + version
}

// New returns a client for the public server.
func New(version string, opts ...Option) *Client {
	c := &Client{baseURL: DefaultBaseURL, userAgent: UserAgent(version), http: netguard.Client(requestTimeout), limiter: shared}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Show is what Mediarium needs to know about a TVmaze show.
type Show struct {
	ID        int
	Name      string
	Status    string // "Running", "Ended", "To Be Determined"…
	Premiered string // "YYYY-MM-DD", or ""
	Timezone  string // the IANA zone of the network or web channel, or ""
}

// Year is the year the show premiered, 0 when unknown.
func (s Show) Year() int {
	if len(s.Premiered) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(s.Premiered[:4])
	return y
}

// Episode is one episode as TVmaze lists it.
type Episode struct {
	Season  int
	Number  int // 0 for specials TVmaze does not number
	Airdate string
	Airtime string // "21:00", or "" when TVmaze has no time
	// Airstamp is the exact moment, ISO 8601 with an offset. TVmaze fills it
	// even when it has no time (then it is a placeholder), so AirAt only
	// trusts it when Airtime is set.
	Airstamp string
	Runtime  int // minutes, 0 when unknown
}

// AirAt is the moment the episode airs in UTC. ok is false when TVmaze has
// no real time for it: nothing is guessed.
func (e Episode) AirAt() (t time.Time, ok bool) {
	if e.Airtime == "" || e.Airstamp == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, e.Airstamp)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

type apiZone struct {
	Country *struct {
		Timezone string `json:"timezone"`
	} `json:"country"`
}

type apiShow struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Premiered  string   `json:"premiered"`
	Network    *apiZone `json:"network"`
	WebChannel *apiZone `json:"webChannel"`
}

func (a apiShow) show() Show {
	s := Show{ID: a.ID, Name: a.Name, Status: a.Status, Premiered: a.Premiered}
	if a.Network != nil && a.Network.Country != nil {
		s.Timezone = a.Network.Country.Timezone
	}
	if s.Timezone == "" && a.WebChannel != nil && a.WebChannel.Country != nil {
		s.Timezone = a.WebChannel.Country.Timezone
	}
	return s
}

type apiEpisode struct {
	Season   int    `json:"season"`
	Number   *int   `json:"number"`
	Airdate  string `json:"airdate"`
	Airtime  string `json:"airtime"`
	Airstamp string `json:"airstamp"`
	Runtime  *int   `json:"runtime"`
}

// LookupTVDB finds a show by its TheTVDB id.
func (c *Client) LookupTVDB(ctx context.Context, id int) (Show, error) {
	return c.show(ctx, "/lookup/shows?thetvdb="+strconv.Itoa(id))
}

// LookupIMDb finds a show by its IMDb id ("tt0903747").
func (c *Client) LookupIMDb(ctx context.Context, id string) (Show, error) {
	return c.show(ctx, "/lookup/shows?imdb="+url.QueryEscape(id))
}

// Search finds the best match for a name. A result is only returned when its
// premiere year is within a year of year (when year is known), so a show with
// a similar name is not taken for this one.
func (c *Client) Search(ctx context.Context, name string, year int) (Show, error) {
	s, err := c.show(ctx, "/singlesearch/shows?q="+url.QueryEscape(name))
	if err != nil {
		return Show{}, err
	}
	if y := s.Year(); year > 0 && (y == 0 || y < year-1 || y > year+1) {
		return Show{}, ErrNotFound
	}
	return s, nil
}

func (c *Client) show(ctx context.Context, path string) (Show, error) {
	var a apiShow
	if err := c.get(ctx, path, &a); err != nil {
		return Show{}, err
	}
	if a.ID == 0 {
		return Show{}, ErrNotFound
	}
	return a.show(), nil
}

// Episodes lists every episode of a show.
func (c *Client) Episodes(ctx context.Context, showID int) ([]Episode, error) {
	var list []apiEpisode
	if err := c.get(ctx, "/shows/"+strconv.Itoa(showID)+"/episodes", &list); err != nil {
		return nil, err
	}
	out := make([]Episode, 0, len(list))
	for _, a := range list {
		e := Episode{Season: a.Season, Airdate: a.Airdate, Airtime: a.Airtime, Airstamp: a.Airstamp}
		if a.Number != nil {
			e.Number = *a.Number
		}
		if a.Runtime != nil {
			e.Runtime = *a.Runtime
		}
		out = append(out, e)
	}
	return out, nil
}

// get makes one request, waiting out a 429 once.
func (c *Client) get(ctx context.Context, path string, into any) error {
	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
		if err != nil {
			return fmt.Errorf("tvmaze request: %w", err)
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("tvmaze request: %w", netguard.CleanError(err))
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound:
			return ErrNotFound
		case resp.StatusCode == http.StatusTooManyRequests:
			wait := retryAfter(resp.Header.Get("Retry-After"))
			if attempt > 0 || wait > maxRetryWait {
				return ErrRateLimited
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		case resp.StatusCode != http.StatusOK:
			return fmt.Errorf("tvmaze request: unexpected status %d", resp.StatusCode)
		}
		if readErr != nil {
			return fmt.Errorf("tvmaze read: %w", readErr)
		}
		if len(body) > maxBodyBytes {
			return errors.New("tvmaze answer is too big")
		}
		if err := json.Unmarshal(body, into); err != nil {
			return fmt.Errorf("tvmaze answer: %w", err)
		}
		return nil
	}
}

// retryAfter reads a Retry-After header in seconds (the form TVmaze uses);
// anything else waits a second.
func retryAfter(h string) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	return time.Second
}
