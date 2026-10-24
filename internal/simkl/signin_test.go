package simkl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	oauth2Refresh  = "fixture-refresh-token-0123456789"
	oauth2NewToken = "fixture-token-after-refresh-0123"
	// What Simkl answered on the PIN address for an OAuth 2.0 app (issue 115).
	oauth2OnlyText = "This client_id is an OAuth 2.0 app: use POST /oauth2/device instead of /oauth/pin"
)

// oauth2Simkl is a strict stand-in for Simkl's sign-in addresses. An OAuth 2.0
// app signs in with the device flow and Simkl refuses the PIN address with the
// exact text real people got; a legacy app is the other way round. polls are
// the answers /oauth2/token gives for the device code, one per call, the last
// one repeating.
type oauth2Simkl struct {
	legacy      bool
	legacyCode  int      // status of /oauth2/device for a legacy app (400 or 404)
	polls       []string // JSON answers; the status is 400 when they hold an error code
	mu          sync.Mutex
	pollCount   int
	pinCalls    int
	deviceCalls int
	tokenBodies []map[string]any
}

func (o *oauth2Simkl) client(t *testing.T) *Client {
	t.Helper()
	bad := func(w http.ResponseWriter, code int, errCode, why string) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"error":"` + errCode + `","message":"` + why + `"}`))
	}
	mux := http.NewServeMux()
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Header.Get("User-Agent") == "" || strings.HasPrefix(r.Header.Get("User-Agent"), "Go-http-client"):
				w.WriteHeader(http.StatusForbidden)
			case r.Header.Get("simkl-api-key") != testClientID:
				w.WriteHeader(http.StatusPreconditionFailed)
			default:
				next(w, r)
			}
		}
	}
	mux.HandleFunc("GET /oauth/pin", guard(func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		o.pinCalls++
		o.mu.Unlock()
		if !o.legacy {
			bad(w, 400, "unauthorized_client", oauth2OnlyText)
			return
		}
		_, _ = w.Write([]byte(`{"result":"OK","user_code":"AB12C","verification_url":"https://simkl.com/pin","expires_in":900,"interval":5}`))
	}))
	mux.HandleFunc("GET /oauth/pin/{code}", guard(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":"OK","access_token":"` + testToken + `"}`))
	}))
	mux.HandleFunc("POST /oauth2/device", guard(func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		o.deviceCalls++
		o.mu.Unlock()
		if o.legacy {
			if o.legacyCode == 404 {
				w.WriteHeader(404)
				return
			}
			bad(w, 400, "unauthorized_client", "This client_id is a legacy PIN app: use GET /oauth/pin instead of /oauth2/device")
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["client_id"] != testClientID {
			bad(w, 400, "invalid_request", "client_id is required in the body")
			return
		}
		if r.URL.Query().Get("app-name") == "" || r.URL.Query().Get("app-version") == "" {
			bad(w, 400, "invalid_request", "app-name and app-version are required")
			return
		}
		_, _ = w.Write([]byte(`{"device_code":"dev-code-0123456789abcdef","user_code":"QZ7-48K","verification_uri":"https://simkl.com/device","verification_uri_complete":"https://simkl.com/device?code=QZ7-48K","expires_in":600,"interval":5}`))
	}))
	mux.HandleFunc("POST /oauth2/token", guard(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["client_id"] != testClientID {
			bad(w, 400, "invalid_request", "client_id is required in the body")
			return
		}
		o.mu.Lock()
		o.tokenBodies = append(o.tokenBodies, body)
		o.mu.Unlock()
		switch body["grant_type"] {
		case "urn:ietf:params:oauth:grant-type:device_code":
			if body["device_code"] != "dev-code-0123456789abcdef" {
				bad(w, 400, "invalid_grant", "unknown device_code")
				return
			}
			o.mu.Lock()
			i := min(o.pollCount, len(o.polls)-1)
			o.pollCount++
			o.mu.Unlock()
			ans := o.polls[i]
			if strings.Contains(ans, `"error"`) {
				w.WriteHeader(400)
			}
			_, _ = w.Write([]byte(ans))
		case "refresh_token":
			if body["refresh_token"] != oauth2Refresh {
				bad(w, 400, "invalid_grant", "refresh token is not valid")
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"` + oauth2NewToken + `","token_type":"Bearer","expires_in":3600}`))
		default:
			bad(w, 400, "unsupported_grant_type", "no")
		}
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	UserAgent = "Mediarium/test (+https://mediarium.app)"
	return NewWithBaseURL(testClientID, srv.URL)
}

func TestPINCallFromAnOAuth2AppGetsTheTextFromTheIssue(t *testing.T) {
	c := (&oauth2Simkl{}).client(t)
	_, err := c.StartPIN(context.Background())
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 400 || !strings.Contains(err.Error(), oauth2OnlyText) {
		t.Fatalf("got %v", err)
	}
}

func TestStartSignInUsesTheDeviceFlowForAnOAuth2App(t *testing.T) {
	o := &oauth2Simkl{}
	c := o.client(t)
	s, err := c.StartSignIn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Flow != FlowDevice || s.UserCode != "QZ7-48K" || s.DeviceCode == "" || s.ExpiresIn != 600*time.Second || s.Interval != 5*time.Second {
		t.Fatalf("sign-in = %+v", s)
	}
	if s.VerificationURL != "https://simkl.com/device" || s.CompleteURL != "https://simkl.com/device?code=QZ7-48K" {
		t.Fatalf("addresses = %q %q", s.VerificationURL, s.CompleteURL)
	}
	if o.pinCalls != 0 {
		t.Errorf("the PIN address was called %d times for an OAuth 2.0 app", o.pinCalls)
	}
}

func TestStartSignInFallsBackToPINForALegacyApp(t *testing.T) {
	for name, code := range map[string]int{"says so": 400, "no such address": 404} {
		t.Run(name, func(t *testing.T) {
			o := &oauth2Simkl{legacy: true, legacyCode: code}
			c := o.client(t)
			s, err := c.StartSignIn(context.Background())
			if err != nil || s.Flow != FlowPIN || s.UserCode != "AB12C" {
				t.Fatalf("got %+v %v", s, err)
			}
			// The flow that worked is remembered: the next start goes straight to it.
			before := o.deviceCalls
			if _, err := c.StartSignIn(context.Background()); err != nil {
				t.Fatal(err)
			}
			if o.deviceCalls != before {
				t.Errorf("a remembered PIN app still tried the device address")
			}
			// And the answer to the PIN code works.
			p, err := c.CheckSignIn(context.Background(), s)
			if err != nil || p.Token == nil || p.Token.Access != testToken {
				t.Fatalf("check: %+v %v", p, err)
			}
		})
	}
}

func TestStartSignInRemembersTheDeviceFlowAndMovesToTheOtherWhenSimklSaysSo(t *testing.T) {
	o := &oauth2Simkl{}
	c := o.client(t)
	c.rememberFlow(FlowPIN) // an old note about this Client ID
	s, err := c.StartSignIn(context.Background())
	if err != nil || s.Flow != FlowDevice {
		t.Fatalf("got %+v %v", s, err)
	}
	if o.pinCalls != 1 {
		t.Errorf("pin calls = %d, want the one that was refused", o.pinCalls)
	}
	if c.rememberedFlow() != FlowDevice {
		t.Errorf("remembered %q", c.rememberedFlow())
	}
}

func TestStartSignInErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("a Client ID Simkl does not know", func(t *testing.T) {
		c := (&oauth2Simkl{}).client(t).WithClientID("other-client-id-000000")
		if _, err := c.StartSignIn(ctx); !errors.Is(err, ErrInvalidClientID) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("an unexpected answer shows Simkl's words", func(t *testing.T) {
		c := fake(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"weird_thing","message":"Something new"}`))
		})
		_, err := c.StartSignIn(ctx)
		if err == nil || !strings.Contains(err.Error(), "weird_thing") || !strings.Contains(err.Error(), "Something new") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a Simkl outage is not retried as another flow", func(t *testing.T) {
		calls := 0
		c := fake(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusBadGateway) })
		if _, err := c.StartSignIn(ctx); err == nil || calls != 1 {
			t.Fatalf("calls %d err %v", calls, err)
		}
	})
	t.Run("a verification page outside simkl.com is ignored", func(t *testing.T) {
		c := fake(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"device_code":"abc","user_code":"AB12C","verification_uri":"https://evil.example/x","verification_uri_complete":"http://simkl.com/x","expires_in":99999,"interval":0}`))
		})
		s, err := c.StartDevice(ctx)
		if err != nil || s.VerificationURL != "https://simkl.com/pin" || s.CompleteURL != "" || s.ExpiresIn != 15*time.Minute || s.Interval != 5*time.Second {
			t.Fatalf("got %+v %v", s, err)
		}
	})
}

func TestDevicePollingHandlesEveryAnswer(t *testing.T) {
	waiting := `{"error":"authorization_pending","error_description":"waiting"}`
	tests := []struct {
		name     string
		answers  []string
		wantPoll Poll
		wantErr  error
		token    string
		refresh  string
	}{
		{"pending", []string{waiting}, Poll{}, nil, "", ""},
		{"slow down", []string{`{"error":"slow_down"}`}, Poll{SlowDown: true}, nil, "", ""},
		{"approved", []string{`{"access_token":"` + testToken + `","token_type":"Bearer","expires_in":7200,"refresh_token":"` + oauth2Refresh + `"}`}, Poll{}, nil, testToken, oauth2Refresh},
		{"approved without a refresh token", []string{`{"access_token":"` + testToken + `"}`}, Poll{}, nil, testToken, ""},
		{"declined", []string{`{"error":"access_denied"}`}, Poll{}, ErrAccessDenied, "", ""},
		{"expired", []string{`{"error":"expired_token"}`}, Poll{}, ErrPINExpired, "", ""},
		{"simkl's own pending wording", []string{`{"result":"KO","message":"Authorization pending"}`}, Poll{}, nil, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := (&oauth2Simkl{polls: tc.answers}).client(t)
			s, err := c.StartDevice(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			p, err := c.CheckSignIn(context.Background(), s)
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if p.SlowDown != tc.wantPoll.SlowDown {
				t.Errorf("slow down = %v", p.SlowDown)
			}
			if tc.token == "" {
				if p.Token != nil {
					t.Fatalf("token %+v", p.Token)
				}
				return
			}
			if p.Token == nil || p.Token.Access != tc.token || p.Token.Refresh != tc.refresh {
				t.Fatalf("token = %+v", p.Token)
			}
		})
	}
}

func TestDevicePollingInOrderAndTheExpiryIsKept(t *testing.T) {
	o := &oauth2Simkl{polls: []string{
		`{"error":"authorization_pending"}`,
		`{"error":"slow_down"}`,
		`{"access_token":"` + testToken + `","expires_in":7200,"refresh_token":"` + oauth2Refresh + `"}`,
	}}
	c := o.client(t)
	ctx := context.Background()
	s, _ := c.StartDevice(ctx)
	var slowed bool
	var got *Token
	for i := 0; i < 5 && got == nil; i++ {
		p, err := c.CheckSignIn(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		slowed = slowed || p.SlowDown
		got = p.Token
	}
	if !slowed || got == nil {
		t.Fatalf("slowed %v token %v", slowed, got)
	}
	if d := time.Until(got.ExpiresAt); d < 7100*time.Second || d > 7200*time.Second {
		t.Errorf("expires in %v", d)
	}
	if Wait(5*time.Second, true) != 10*time.Second || Wait(5*time.Second, false) != 5*time.Second {
		t.Error("slow_down must add 5 seconds")
	}
	// No client secret is sent: the app is a public client.
	for _, b := range o.tokenBodies {
		if _, has := b["client_secret"]; has {
			t.Errorf("a client secret was sent: %v", b)
		}
	}
}

func TestRefresh(t *testing.T) {
	c := (&oauth2Simkl{}).client(t)
	ctx := context.Background()
	tok, err := c.Refresh(ctx, oauth2Refresh)
	if err != nil || tok.Access != oauth2NewToken || tok.Refresh != oauth2Refresh || time.Until(tok.ExpiresAt) < 3500*time.Second {
		t.Fatalf("got %+v %v", tok, err)
	}
	if _, err := c.Refresh(ctx, "revoked-refresh-token"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a refresh token Simkl refuses must mean connect again: %v", err)
	}
	if _, err := c.Refresh(ctx, ""); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no refresh token: %v", err)
	}
}
