// Package simkl reads a person's own lists from Simkl (simkl.com): Plan to
// Watch, Watching, Completed and the like, for movies, shows and anime.
//
// Unlike a public Trakt list, a Simkl list belongs to an account, so reading
// it needs two things: the Client ID of a Simkl app (free, made on
// simkl.com/settings/developer) and an access token that the person gives by
// entering a short code on Simkl's site (no password ever reaches Mediarium).
//
// Simkl has two ways to sign in, and which one works depends on the app. Apps
// made now are OAuth 2.0 apps and use the device authorization flow
// (POST /oauth2/device, then POST /oauth2/token, RFC 8628). Older apps use
// the PIN flow (GET /oauth/pin). StartSignIn tries the first and falls back to
// the second only when Simkl says the app is an older one.
//
// The calls follow Simkl's API documentation and RFC 8628; see docs/lists.md
// for what could and could not be tested against the real service.
package simkl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rdborg/mediarium/internal/netguard"
)

const apiBaseURL = "https://api.simkl.com"

// UserAgent is sent with every call; the app sets it to include its version.
var UserAgent = "Mediarium (+https://mediarium.app)"

// AppVersion is sent as app-version with every call (Simkl asks every app to
// say who it is with app-name and app-version).
var AppVersion = "dev"

// RedirectURI is the redirect address the guide tells people to enter when
// they make their Simkl app: the "out of band" address for apps that sign in
// with a code and have no web address of their own. The sign-in code flow
// sends it as the redirect, so Simkl sees the same address the app was made
// with.
const RedirectURI = "urn:ietf:wg:oauth:2.0:oob"

// maxAPIResponse is the biggest answer read from Simkl.
const maxAPIResponse = 32 << 20

var (
	// ErrNoClientID means no Simkl app Client ID has been added yet.
	ErrNoClientID = errors.New("no Simkl client ID has been added yet")
	// ErrInvalidClientID means Simkl rejected the Client ID.
	ErrInvalidClientID = errors.New("Simkl rejected this client ID")
	// ErrNotConnected means no Simkl account has been connected yet.
	ErrNotConnected = errors.New("No Simkl account is connected yet")
	// ErrUnauthorized means Simkl no longer accepts the saved access token.
	ErrUnauthorized = errors.New("Simkl no longer accepts the saved sign-in. Connect your account again")
	// ErrPINExpired means the code was not entered in time, or Simkl does not know it.
	ErrPINExpired = errors.New("That Simkl code has expired")
	// ErrAccessDenied means the person said no on Simkl's page.
	ErrAccessDenied = errors.New("The sign-in was declined on Simkl")
)

// StatusError is an answer Simkl gave that is not a success and not one of the
// well-known cases above. Message is Simkl's own explanation when it gave one.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	if e.Status >= 500 {
		return fmt.Sprintf("Simkl isn't answering properly right now (it sent status %d). Try again later", e.Status)
	}
	text := fmt.Sprintf("Simkl answered with status %d", e.Status)
	if e.Message != "" {
		text += " (\"" + e.Message + "\")"
	}
	if e.Status == http.StatusBadRequest {
		text += ". Check that you pasted the Client ID of your Simkl app (not the secret), and that the app's redirect URI is exactly " + RedirectURI
	} else {
		text += ". Try again later"
	}
	return text
}

// errorText pulls the explanation out of a Simkl error answer such as
// {"error":"client_id_failed","message":"..."}; at most 200 characters.
func errorText(body []byte) string {
	var r struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &r) != nil {
		return ""
	}
	text := strings.TrimSpace(r.Message)
	if r.Error != "" {
		if text != "" {
			text = r.Error + ": " + text
		} else {
			text = r.Error
		}
	}
	text = strings.Map(func(c rune) rune {
		if c < ' ' || c == '"' {
			return ' '
		}
		return c
	}, text)
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

// RateLimitError is returned when Simkl asks to slow down.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("Simkl asked to slow down. Try again in %d seconds", int(e.RetryAfter.Seconds()))
	}
	return "Simkl asked to slow down. Try again in a few minutes"
}

// Statuses of a Simkl list.
const (
	StatusPlanToWatch = "plantowatch"
	StatusWatching    = "watching"
	StatusCompleted   = "completed"
	StatusOnHold      = "hold"
	StatusDropped     = "dropped"
)

// ValidStatus reports whether s is one of the Status* names.
func ValidStatus(s string) bool {
	switch s {
	case StatusPlanToWatch, StatusWatching, StatusCompleted, StatusOnHold, StatusDropped:
		return true
	}
	return false
}

// Client talks to the Simkl API.
type Client struct {
	clientID   string
	baseURL    string
	httpClient *http.Client

	mu    sync.Mutex
	flows map[string]string // Client ID -> the sign-in flow Simkl accepted for it
}

// New constructs a client. clientID may be empty; calls return ErrNoClientID
// until SetClientID is used.
func New(clientID string) *Client {
	return &Client{clientID: clientID, baseURL: apiBaseURL, httpClient: netguard.Client(20 * time.Second)}
}

// NewWithBaseURL points a client at another server (tests use a fixture).
func NewWithBaseURL(clientID, base string) *Client {
	c := New(clientID)
	c.baseURL = strings.TrimRight(base, "/")
	return c
}

func (c *Client) SetClientID(id string) { c.clientID = id }
func (c *Client) HasClientID() bool     { return c.clientID != "" }

// WithClientID returns a separate client using another Client ID against the
// same server, to test an ID before saving it.
func (c *Client) WithClientID(id string) *Client {
	n := New(id)
	n.baseURL = c.baseURL
	return n
}

// WrapTransport wraps the HTTP transport, e.g. to count requests. Call it
// before the client is shared between goroutines.
func (c *Client) WrapTransport(wrap func(http.RoundTripper) http.RoundTripper) {
	rt := c.httpClient.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	c.httpClient.Transport = wrap(rt)
}

// do makes one GET and returns the body (at most maxAPIResponse bytes) of a
// 200 answer. token is empty for the sign-in calls.
func (c *Client) do(ctx context.Context, path string, q url.Values, token string) ([]byte, error) {
	return c.request(ctx, http.MethodGet, path, q, token)
}

// request is do for any method. Every call carries the Client ID as the
// simkl-api-key header and as client_id, and says which app is calling with
// app-name and app-version, as Simkl's documentation asks.
func (c *Client) request(ctx context.Context, method, path string, q url.Values, token string) ([]byte, error) {
	status, header, body, err := c.send(ctx, method, path, q, token, nil)
	if err != nil {
		return nil, err
	}
	return mapStatus(status, header, body, token)
}

// mapStatus turns the answer to one call into its body or a clear error.
func mapStatus(status int, header http.Header, body []byte, token string) ([]byte, error) {
	switch {
	case status == http.StatusOK:
		return body, nil
	case status == http.StatusUnauthorized:
		if token != "" {
			return nil, ErrUnauthorized
		}
		return nil, ErrInvalidClientID
	case status == http.StatusForbidden, status == http.StatusPreconditionFailed:
		return nil, ErrInvalidClientID
	case status == http.StatusTooManyRequests:
		secs, _ := strconv.Atoi(strings.TrimSpace(header.Get("Retry-After")))
		return nil, &RateLimitError{RetryAfter: time.Duration(secs) * time.Second}
	}
	if len(body) > 4<<10 {
		body = body[:4<<10]
	}
	return nil, &StatusError{Status: status, Message: errorText(body)}
}

// send makes one call and returns whatever Simkl answered, whatever the
// status (at most maxAPIResponse bytes of body). A payload is sent as JSON.
func (c *Client) send(ctx context.Context, method, path string, q url.Values, token string, payload map[string]any) (int, http.Header, []byte, error) {
	if c.clientID == "" {
		return 0, nil, nil, ErrNoClientID
	}
	full := url.Values{}
	for k, v := range q {
		full[k] = v
	}
	full.Set("client_id", c.clientID)
	full.Set("app-name", "mediarium")
	full.Set("app-version", AppVersion)
	target := c.baseURL + path + "?" + full.Encode()
	var rdr io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("build simkl request: %w", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rdr)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("build simkl request: %w", netguard.CleanError(err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("simkl-api-key", c.clientID)
	req.Header.Set("User-Agent", UserAgent)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// net/http puts the whole address, query included, in its errors.
		return 0, nil, nil, fmt.Errorf("simkl request: %w", netguard.CleanError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponse+1))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("read simkl answer: %w", netguard.CleanError(err))
	}
	if len(body) > maxAPIResponse {
		return 0, nil, nil, errors.New("Simkl's answer is too big to read")
	}
	return resp.StatusCode, resp.Header, body, nil
}

// PIN is a sign-in started with the older PIN flow (a code entered at
// simkl.com/pin).
type PIN = SignIn

var codeRe = regexp.MustCompile(`^[A-Za-z0-9]{3,32}$`)

// StartPIN asks Simkl for a new code with the older PIN flow, which only apps
// made before OAuth 2.0 apps existed can use. It also proves the Client ID
// works.
func (c *Client) StartPIN(ctx context.Context) (PIN, error) {
	body, err := c.do(ctx, "/oauth/pin", url.Values{"redirect": {RedirectURI}}, "")
	if err != nil {
		return PIN{}, err
	}
	var r struct {
		Result          string `json:"result"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return PIN{}, fmt.Errorf("decode simkl code: %w", err)
	}
	if r.Result != "OK" || !codeRe.MatchString(r.UserCode) {
		// Simkl answers 200 with result KO for a Client ID it does not know.
		return PIN{}, ErrInvalidClientID
	}
	p := PIN{Flow: FlowPIN, UserCode: r.UserCode, VerificationURL: "https://simkl.com/pin", ExpiresIn: time.Duration(r.ExpiresIn) * time.Second, Interval: time.Duration(r.Interval) * time.Second}
	// Only ever point people at Simkl itself, whatever the answer says.
	if u, err := url.Parse(r.VerificationURL); err == nil && u.Scheme == "https" && (u.Hostname() == "simkl.com" || u.Hostname() == "www.simkl.com") {
		p.VerificationURL = strings.TrimRight(r.VerificationURL, "/")
	}
	if p.ExpiresIn <= 0 || p.ExpiresIn > time.Hour {
		p.ExpiresIn = 15 * time.Minute
	}
	if p.Interval < time.Second || p.Interval > time.Minute {
		p.Interval = 5 * time.Second
	}
	return p, nil
}

// CheckPIN asks whether the code was entered yet. It returns the access
// token when it was, "" while Simkl is still waiting, and ErrPINExpired when
// the code is no longer good.
func (c *Client) CheckPIN(ctx context.Context, userCode string) (string, error) {
	if !codeRe.MatchString(userCode) {
		return "", ErrPINExpired
	}
	body, err := c.do(ctx, "/oauth/pin/"+userCode, nil, "")
	if err != nil {
		return "", err
	}
	var r struct {
		Result      string `json:"result"`
		Message     string `json:"message"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("decode simkl sign-in: %w", err)
	}
	if r.Result == "OK" && r.AccessToken != "" {
		return r.AccessToken, nil
	}
	if strings.Contains(strings.ToLower(r.Message), "pending") {
		return "", nil
	}
	return "", ErrPINExpired
}

// CheckToken proves the saved sign-in works by asking Simkl for the account's
// activity times, the call the list sync starts with. It is the fallback for
// UserName, which some accounts get refused even though the lists sync fine.
func (c *Client) CheckToken(ctx context.Context, token string) error {
	if token == "" {
		return ErrNotConnected
	}
	_, err := c.request(ctx, http.MethodGet, "/sync/activities", nil, token)
	return err
}

// UserName asks Simkl who the saved sign-in belongs to (the account's name),
// which proves the Client ID and the sign-in work together.
func (c *Client) UserName(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrNotConnected
	}
	// Simkl documents this call as a POST; an old gateway may only know GET.
	body, err := c.request(ctx, http.MethodPost, "/users/settings", nil, token)
	var se *StatusError
	if errors.As(err, &se) && (se.Status == http.StatusMethodNotAllowed || se.Status == http.StatusNotFound) {
		body, err = c.request(ctx, http.MethodGet, "/users/settings", nil, token)
	}
	if err != nil {
		return "", err
	}
	var r struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("decode simkl account: %w", err)
	}
	return strings.TrimSpace(r.User.Name), nil
}

// Media is one movie, show or anime on a Simkl list.
type Media struct {
	Kind   string // "movie" or "tv"
	Title  string
	Year   int
	TMDBID int
	IMDBID string
	TVDBID int
}

// flexInt reads an id that Simkl may send as a number or as text.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		*f = 0 // an id that is not a plain number is no use here, but is no reason to fail the list
		return nil
	}
	*f = flexInt(n)
	return nil
}

type mediaBody struct {
	Title string  `json:"title"`
	Year  flexInt `json:"year"`
	IDs   struct {
		TMDB flexInt `json:"tmdb"`
		IMDB string  `json:"imdb"`
		TVDB flexInt `json:"tvdb"`
	} `json:"ids"`
}

var imdbRe = regexp.MustCompile(`^tt[0-9]{5,10}$`)

func (b *mediaBody) media(kind string) (Media, bool) {
	if b == nil {
		return Media{}, false
	}
	m := Media{Kind: kind, Title: strings.TrimSpace(b.Title), Year: int(b.Year), TMDBID: int(b.IDs.TMDB), TVDBID: int(b.IDs.TVDB)}
	if imdbRe.MatchString(b.IDs.IMDB) {
		m.IMDBID = b.IDs.IMDB
	}
	if m.TMDBID == 0 && m.IMDBID == "" && m.TVDBID == 0 && m.Title == "" {
		return Media{}, false
	}
	return m, true
}

type entry struct {
	Movie     *mediaBody `json:"movie"`
	Show      *mediaBody `json:"show"`
	AnimeType string     `json:"anime_type"`
}

// Items fetches everything with the given status in the three kinds of list
// Simkl keeps (movies, shows, anime). Anime that is a film counts as a movie,
// all other anime as a show. The calls are spaced by gap, to be gentle.
func (c *Client) Items(ctx context.Context, token, status string, gap time.Duration) ([]Media, error) {
	if token == "" {
		return nil, ErrNotConnected
	}
	if !ValidStatus(status) {
		return nil, fmt.Errorf("simkl status %q is not one Mediarium knows", status)
	}
	var out []Media
	for i, kind := range []string{"movies", "shows", "anime"} {
		if i > 0 && gap > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(gap):
			}
		}
		body, err := c.do(ctx, "/sync/all-items/"+kind+"/"+status, url.Values{"extended": {"full"}}, token)
		if err != nil {
			return nil, err
		}
		part, err := parseItems(body, kind)
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

// parseItems reads one /sync/all-items answer. Simkl keeps the list under the
// kind's name ({"movies":[...]}, {"shows":[...]}, {"anime":[...]}) and answers
// null when nothing is on it.
func parseItems(body []byte, kind string) ([]Media, error) {
	var r map[string][]entry
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("decode simkl list: %w", err)
	}
	var out []Media
	for _, e := range r[kind] {
		var m Media
		var ok bool
		switch {
		case kind == "movies" && e.Movie != nil:
			m, ok = e.Movie.media("movie")
		case kind == "anime":
			b := e.Show
			if b == nil {
				b = e.Movie
			}
			k := "tv"
			if strings.EqualFold(e.AnimeType, "movie") {
				k = "movie"
			}
			m, ok = b.media(k)
		case e.Show != nil:
			m, ok = e.Show.media("tv")
		}
		if ok {
			out = append(out, m)
		}
	}
	return out, nil
}
