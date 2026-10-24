package simkl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// strictSimkl follows Simkl's documented rules and refuses everything that
// breaks one, the way the real service does: a missing simkl-api-key or
// client_id, a missing app-name or app-version, a missing User-Agent, a sign-in
// code asked for without the redirect the app was made with, a list asked for
// without the bearer token, a wrong method.
func strictSimkl(t *testing.T) *Client {
	t.Helper()
	bad := func(w http.ResponseWriter, why string) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad_request","message":"` + why + `"}`))
	}
	mux := http.NewServeMux()
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			switch {
			case r.Header.Get("User-Agent") == "" || strings.HasPrefix(r.Header.Get("User-Agent"), "Go-http-client"):
				w.WriteHeader(http.StatusForbidden)
			case r.Header.Get("simkl-api-key") != testClientID || q.Get("client_id") != testClientID:
				w.WriteHeader(http.StatusPreconditionFailed)
			case q.Get("app-name") == "" || q.Get("app-version") == "":
				bad(w, "app-name and app-version are required")
			default:
				next(w, r)
			}
		}
	}
	mux.HandleFunc("GET /oauth/pin", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("redirect") != "urn:ietf:wg:oauth:2.0:oob" {
			bad(w, "redirect does not match the app")
			return
		}
		_, _ = w.Write([]byte(`{"result":"OK","user_code":"AB12C","verification_url":"https://simkl.com/pin","expires_in":900,"interval":5}`))
	}))
	mux.HandleFunc("GET /oauth/pin/{code}", guard(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":"OK","access_token":"` + testToken + `"}`))
	}))
	mux.HandleFunc("GET /sync/all-items/{kind}/{status}", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`null`))
	}))
	mux.HandleFunc("POST /users/settings", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"user":{"name":"Casey"},"account":{"id":7}}`))
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	UserAgent = "Mediarium/test (+https://mediarium.app)"
	return NewWithBaseURL(testClientID, srv.URL)
}

func TestStrictSimklAcceptsTheWholeFlow(t *testing.T) {
	c := strictSimkl(t)
	ctx := context.Background()
	pin, err := c.StartPIN(ctx)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	token, err := c.CheckPIN(ctx, pin.UserCode)
	if err != nil || token != testToken {
		t.Fatalf("check: %q %v", token, err)
	}
	if _, err := c.Items(ctx, token, StatusPlanToWatch, 0); err != nil {
		t.Fatalf("items: %v", err)
	}
	name, err := c.UserName(ctx, token)
	if err != nil || name != "Casey" {
		t.Fatalf("user name: %q %v", name, err)
	}
}

func TestStrictSimklRefusals(t *testing.T) {
	c := strictSimkl(t)
	ctx := context.Background()
	tests := []struct {
		name string
		run  func() error
		is   error
		text string
	}{
		{"wrong client id", func() error { _, err := c.WithClientID("other-client-id-000000").StartPIN(ctx); return err }, ErrInvalidClientID, ""},
		{"no token for the list", func() error { _, err := c.Items(ctx, "wrong-token", StatusPlanToWatch, 0); return err }, ErrUnauthorized, ""},
		{"no token for the account", func() error { _, err := c.UserName(ctx, "wrong-token"); return err }, ErrUnauthorized, ""},
		{"no account at all", func() error { _, err := c.UserName(ctx, ""); return err }, ErrNotConnected, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, tc.is) {
				t.Fatalf("got %v, want %v", err, tc.is)
			}
		})
	}
}

func TestBadRequestShowsSimklsOwnWords(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"client_id_failed","message":"Client ID is not valid"}`))
	})
	_, err := c.StartPIN(context.Background())
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 400 {
		t.Fatalf("got %v", err)
	}
	for _, want := range []string{"status 400", "client_id_failed", "Client ID is not valid", "not the secret", "urn:ietf:wg:oauth:2.0:oob"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message lacks %q: %s", want, err)
		}
	}
	if strings.Contains(err.Error(), testClientID) {
		t.Errorf("the client id is in the message")
	}
}

func TestUserNameFallsBackToGet(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"user":{"name":"Casey"}}`))
	})
	if name, err := c.UserName(context.Background(), testToken); err != nil || name != "Casey" {
		t.Fatalf("%q %v", name, err)
	}
}

func TestErrorTextIsTidied(t *testing.T) {
	tests := []struct{ in, want string }{
		{`{"error":"x","message":"y"}`, "x: y"},
		{`{"error":"only_code"}`, "only_code"},
		{`{"message":"has \"quotes\"\nand a newline"}`, "has  quotes  and a newline"},
		{`<html>`, ""},
		{`{"message":"` + strings.Repeat("a", 500) + `"}`, strings.Repeat("a", 200)},
	}
	for _, tc := range tests {
		if got := errorText([]byte(tc.in)); got != tc.want {
			t.Errorf("errorText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
