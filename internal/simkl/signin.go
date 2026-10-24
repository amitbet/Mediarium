package simkl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The two ways to sign in. Which one works depends on the Simkl app: apps
// made now are OAuth 2.0 apps (device flow), older ones are PIN apps.
const (
	FlowDevice = "device"
	FlowPIN    = "pin"
)

// SignIn is a sign-in in progress: a code the person enters on Simkl's site.
type SignIn struct {
	Flow            string // FlowDevice or FlowPIN
	UserCode        string
	DeviceCode      string // the device flow's secret half; never shown or sent to the browser
	VerificationURL string
	// CompleteURL is the same page with the code already filled in, when Simkl gives one.
	CompleteURL string
	ExpiresIn   time.Duration
	Interval    time.Duration
}

// Token is what a finished sign-in gives.
type Token struct {
	Access  string
	Refresh string
	// ExpiresAt is zero when Simkl said nothing about the token running out.
	ExpiresAt time.Time
}

// Poll is the answer to "has the code been entered yet?". Token is nil while
// Simkl is still waiting; SlowDown asks the caller to wait longer between
// questions.
type Poll struct {
	Token    *Token
	SlowDown bool
}

const (
	deviceGrant  = "urn:ietf:params:oauth:grant-type:device_code"
	refreshGrant = "refresh_token"
	// slowDownBy is what RFC 8628 says to add to the interval after slow_down.
	slowDownBy = 5 * time.Second
)

// userCodeRe allows what a code on a verification page may contain (letters,
// digits, and a dash or space between groups).
var userCodeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 -]{1,30}[A-Za-z0-9]$`)

func (c *Client) rememberedFlow() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flows[c.clientID]
}

func (c *Client) rememberFlow(flow string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.flows == nil {
		c.flows = map[string]string{}
	}
	c.flows[c.clientID] = flow
}

// StartSignIn asks Simkl for a sign-in code. It tries the OAuth 2.0 device
// flow first (what apps made now use) and the older PIN flow when Simkl says
// the app is an older one; the one that worked is remembered for the Client ID
// and tried first next time. It also proves the Client ID works.
func (c *Client) StartSignIn(ctx context.Context) (SignIn, error) {
	first, second := c.StartDevice, c.StartPIN
	firstFlow := FlowDevice
	if c.rememberedFlow() == FlowPIN {
		first, second = c.StartPIN, c.StartDevice
		firstFlow = FlowPIN
	}
	s, err := first(ctx)
	if err == nil {
		c.rememberFlow(s.Flow)
		return s, nil
	}
	if !canTryOtherFlow(err) {
		return SignIn{}, err
	}
	s2, err2 := second(ctx)
	if err2 == nil {
		c.rememberFlow(s2.Flow)
		return s2, nil
	}
	// Both failed. Simkl's explanation from the first call is the more useful
	// one, unless it was only pointing at the other flow.
	if pointsAtOtherFlow(err, firstFlow) {
		return SignIn{}, err2
	}
	return SignIn{}, err
}

// canTryOtherFlow reports whether a failed start could be down to using the
// wrong flow for this app (so not a network problem, a rate limit or a Simkl
// outage).
func canTryOtherFlow(err error) bool {
	if errors.Is(err, ErrInvalidClientID) {
		return true
	}
	var se *StatusError
	return errors.As(err, &se) && se.Status < 500
}

// pointsAtOtherFlow reports whether the error is Simkl saying "use the other
// sign-in address" (or that this one doesn't exist).
func pointsAtOtherFlow(err error, flow string) bool {
	var se *StatusError
	if !errors.As(err, &se) {
		return false
	}
	text := strings.ToLower(se.Message)
	if flow == FlowDevice {
		return se.Status == http.StatusNotFound || se.Status == http.StatusMethodNotAllowed ||
			strings.Contains(text, "oauth/pin") || strings.Contains(text, "legacy") || strings.Contains(text, "pin app")
	}
	return strings.Contains(text, "oauth2/device") || strings.Contains(text, "oauth 2.0 app") || strings.Contains(text, "device")
}

// StartDevice asks for a code with the OAuth 2.0 device authorization flow
// (RFC 8628): POST /oauth2/device.
func (c *Client) StartDevice(ctx context.Context) (SignIn, error) {
	status, header, raw, err := c.send(ctx, http.MethodPost, "/oauth2/device", nil, "", map[string]any{
		"client_id":   c.clientID,
		"app-name":    "mediarium",
		"app-version": AppVersion,
	})
	if err != nil {
		return SignIn{}, err
	}
	body, err := mapStatus(status, header, raw, "")
	if err != nil {
		return SignIn{}, err
	}
	var r struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURL         string `json:"verification_url"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
		Error                   string `json:"error"`
		Message                 string `json:"message"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return SignIn{}, fmt.Errorf("decode simkl code: %w", err)
	}
	if r.Error != "" {
		return SignIn{}, &StatusError{Status: http.StatusBadRequest, Message: errorText(body)}
	}
	if r.DeviceCode == "" || len(r.DeviceCode) > 512 || !userCodeRe.MatchString(r.UserCode) {
		return SignIn{}, &StatusError{Status: http.StatusBadGateway, Message: "the answer had no usable code"}
	}
	s := SignIn{
		Flow: FlowDevice, UserCode: r.UserCode, DeviceCode: r.DeviceCode,
		VerificationURL: "https://simkl.com/pin",
		ExpiresIn:       time.Duration(r.ExpiresIn) * time.Second, Interval: time.Duration(r.Interval) * time.Second,
	}
	// Only ever point people at Simkl itself, whatever the answer says.
	for _, cand := range []string{r.VerificationURI, r.VerificationURL} {
		if u, ok := simklPage(cand); ok {
			s.VerificationURL = u
			break
		}
	}
	if u, ok := simklPage(r.VerificationURIComplete); ok {
		s.CompleteURL = u
	}
	s.ExpiresIn, s.Interval = tidyTimes(s.ExpiresIn, s.Interval)
	return s, nil
}

// simklPage returns raw when it is an https page on simkl.com.
func simklPage(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil {
		return "", false
	}
	h := strings.ToLower(u.Hostname())
	if h != "simkl.com" && !strings.HasSuffix(h, ".simkl.com") {
		return "", false
	}
	return u.String(), true
}

// tidyTimes replaces a missing or silly lifetime and poll interval.
func tidyTimes(expires, interval time.Duration) (time.Duration, time.Duration) {
	if expires <= 0 || expires > time.Hour {
		expires = 15 * time.Minute
	}
	if interval < time.Second || interval > time.Minute {
		interval = 5 * time.Second
	}
	return expires, interval
}

// CheckSignIn asks whether the code was entered yet, with whichever flow the
// sign-in was started with. ErrPINExpired means the code is no longer good and
// ErrAccessDenied that the person said no.
func (c *Client) CheckSignIn(ctx context.Context, s SignIn) (Poll, error) {
	if s.Flow == FlowPIN {
		tok, err := c.CheckPIN(ctx, s.UserCode)
		if err != nil || tok == "" {
			return Poll{}, err
		}
		return Poll{Token: &Token{Access: tok}}, nil
	}
	status, header, raw, err := c.send(ctx, http.MethodPost, "/oauth2/token", nil, "", map[string]any{
		"grant_type":  deviceGrant,
		"device_code": s.DeviceCode,
		"client_id":   c.clientID,
	})
	if err != nil {
		return Poll{}, err
	}
	return parseTokenAnswer(status, header, raw)
}

// parseTokenAnswer reads an answer from /oauth2/token. RFC 8628 sends the
// "keep waiting" and "no" answers as a 400 with an error code; Simkl's own
// style is a 200 with result KO and a message, so both are understood.
func parseTokenAnswer(status int, header http.Header, raw []byte) (Poll, error) {
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
		Message      string `json:"message"`
	}
	_ = json.Unmarshal(raw, &r)
	if r.AccessToken != "" && status >= 200 && status < 300 {
		t := &Token{Access: r.AccessToken, Refresh: r.RefreshToken}
		if r.ExpiresIn > 0 {
			t.ExpiresAt = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
		}
		return Poll{Token: t}, nil
	}
	code := strings.ToLower(strings.TrimSpace(r.Error))
	msg := strings.ToLower(r.Message)
	switch {
	case code == "authorization_pending" || strings.Contains(msg, "pending"):
		return Poll{}, nil
	case code == "slow_down" || strings.Contains(msg, "slow down"):
		return Poll{SlowDown: true}, nil
	case code == "access_denied" || strings.Contains(msg, "denied"):
		return Poll{}, ErrAccessDenied
	case code == "expired_token" || code == "invalid_grant" || strings.Contains(msg, "expired"):
		return Poll{}, ErrPINExpired
	}
	_, err := mapStatus(status, header, raw, "")
	if err == nil {
		// A 200 that is neither a token nor a known wait.
		return Poll{}, &StatusError{Status: status, Message: errorText(raw)}
	}
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return Poll{SlowDown: true}, nil
	}
	return Poll{}, err
}

// Refresh trades a refresh token for a new access token
// (POST /oauth2/token, grant_type refresh_token). A refresh token Simkl no
// longer accepts gives ErrUnauthorized: the person has to connect again.
func (c *Client) Refresh(ctx context.Context, refresh string) (Token, error) {
	if refresh == "" {
		return Token{}, ErrNotConnected
	}
	status, header, raw, err := c.send(ctx, http.MethodPost, "/oauth2/token", nil, "", map[string]any{
		"grant_type":    refreshGrant,
		"refresh_token": refresh,
		"client_id":     c.clientID,
	})
	if err != nil {
		return Token{}, err
	}
	p, err := parseTokenAnswer(status, header, raw)
	switch {
	case errors.Is(err, ErrPINExpired), errors.Is(err, ErrAccessDenied), errors.Is(err, ErrInvalidClientID):
		return Token{}, ErrUnauthorized
	case err != nil:
		return Token{}, err
	case p.Token == nil:
		return Token{}, &StatusError{Status: status, Message: errorText(raw)}
	}
	t := *p.Token
	if t.Refresh == "" {
		t.Refresh = refresh // Simkl may keep the same refresh token
	}
	return t, nil
}

// Wait is how long to wait between questions, given Simkl's interval and
// whether it asked to slow down.
func Wait(interval time.Duration, slowDown bool) time.Duration {
	if slowDown {
		interval += slowDownBy
	}
	return interval
}
