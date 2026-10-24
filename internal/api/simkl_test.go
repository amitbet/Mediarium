package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rdborg/mediarium/internal/api"
	"github.com/rdborg/mediarium/internal/crypto"
	"github.com/rdborg/mediarium/internal/settings"
)

const (
	simklTestClient = "fixture-simkl-client-id-0123456789"
	simklTestToken  = "fixture-simkl-token-abcdef0123456789"
	// What a refresh gives instead of simklTestToken.
	simklTestFresh = "fixture-simkl-token-refreshed-0123456"
	simklTestRenew = "fixture-simkl-refresh-token-0123456789"
)

// fakeSimkl is a Simkl with the sign-in code flow and a person's lists. entered
// says whether the code AB12C has been entered at simkl.com/pin yet; listCode
// is the status the lists answer with (200, or an error to test).
type fakeSimkl struct {
	srv      *httptest.Server
	entered  atomic.Bool
	listCode atomic.Int32
	pinCode  atomic.Int32 // status of the sign-in code calls; 0 means normal
	setCode  atomic.Int32 // status of the account call (users/settings); 0 means normal
	calls    atomic.Int32

	// An OAuth 2.0 app signs in with the device flow; the PIN address refuses it.
	oauth2    atomic.Bool
	slowOnce  atomic.Bool  // the next poll answers slow_down
	denied    atomic.Bool  // the person said no
	expired   atomic.Bool  // the code ran out
	expiresIn atomic.Int32 // expires_in the token carries (0 = none)
	refreshes atomic.Int32
	fresh     atomic.Bool // the refreshed token is the one in force
	// refuseRefresh makes Simkl refuse every refresh token.
	refuseRefresh atomic.Bool
}

func (f *fakeSimkl) goodToken() string {
	if f.fresh.Load() {
		return simklTestFresh
	}
	return simklTestToken
}

func newFakeSimkl(t *testing.T) *fakeSimkl {
	t.Helper()
	f := &fakeSimkl{}
	f.listCode.Store(200)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/device", func(w http.ResponseWriter, r *http.Request) {
		if !f.oauth2.Load() {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"unauthorized_client","message":"This client_id is a legacy PIN app: use GET /oauth/pin instead of /oauth2/device"}`))
			return
		}
		_, _ = w.Write([]byte(`{"device_code":"dev-code-0123456789abcdef","user_code":"QZ7-48K","verification_uri":"https://simkl.com/device","expires_in":600,"interval":5}`))
	})
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		answer := func(code int, text string) { w.WriteHeader(code); _, _ = w.Write([]byte(text)) }
		switch body["grant_type"] {
		case "refresh_token":
			if body["refresh_token"] != simklTestRenew || f.refuseRefresh.Load() {
				answer(400, `{"error":"invalid_grant"}`)
				return
			}
			f.refreshes.Add(1)
			f.fresh.Store(true)
			_, _ = w.Write([]byte(`{"access_token":"` + simklTestFresh + `","refresh_token":"` + simklTestRenew + `","expires_in":3600}`))
		case "urn:ietf:params:oauth:grant-type:device_code":
			switch {
			case body["device_code"] != "dev-code-0123456789abcdef":
				answer(400, `{"error":"invalid_grant"}`)
			case f.denied.Load():
				answer(400, `{"error":"access_denied"}`)
			case f.expired.Load():
				answer(400, `{"error":"expired_token"}`)
			case f.slowOnce.CompareAndSwap(true, false):
				answer(400, `{"error":"slow_down"}`)
			case !f.entered.Load():
				answer(400, `{"error":"authorization_pending"}`)
			default:
				text := `{"access_token":"` + simklTestToken + `","token_type":"Bearer","refresh_token":"` + simklTestRenew + `"`
				if n := f.expiresIn.Load(); n > 0 {
					text += `,"expires_in":` + strconv.Itoa(int(n))
				}
				_, _ = w.Write([]byte(text + `}`))
			}
		default:
			answer(400, `{"error":"unsupported_grant_type"}`)
		}
	})
	mux.HandleFunc("/oauth/pin", func(w http.ResponseWriter, r *http.Request) {
		if f.oauth2.Load() {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"unauthorized_client","message":"This client_id is an OAuth 2.0 app: use POST /oauth2/device instead of /oauth/pin"}`))
			return
		}
		if code := int(f.pinCode.Load()); code != 0 {
			w.WriteHeader(code)
			return
		}
		if r.URL.Query().Get("client_id") != simklTestClient {
			_, _ = w.Write([]byte(`{"result":"KO","message":"unknown client"}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":"OK","user_code":"AB12C","verification_url":"https://simkl.com/pin","expires_in":900,"interval":5}`))
	})
	mux.HandleFunc("/oauth/pin/{code}", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.PathValue("code") != "AB12C":
			_, _ = w.Write([]byte(`{"result":"KO","message":"Code expired"}`))
		case f.entered.Load():
			_, _ = w.Write([]byte(`{"result":"OK","access_token":"` + simklTestToken + `"}`))
		default:
			_, _ = w.Write([]byte(`{"result":"KO","message":"Authorization pending"}`))
		}
	})
	mux.HandleFunc("GET /sync/activities", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+f.goodToken() || r.Header.Get("simkl-api-key") != simklTestClient {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"all":"2026-10-22T10:00:00Z"}`))
	})
	mux.HandleFunc("POST /users/settings", func(w http.ResponseWriter, r *http.Request) {
		if code := int(f.setCode.Load()); code != 0 {
			w.WriteHeader(code)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+f.goodToken() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"user":{"name":"Casey"}}`))
	})
	mux.HandleFunc("/sync/all-items/{kind}/{status}", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+f.goodToken() || r.Header.Get("simkl-api-key") != simklTestClient {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if code := int(f.listCode.Load()); code != 200 {
			w.WriteHeader(code)
			return
		}
		if r.PathValue("status") != "plantowatch" {
			_, _ = w.Write([]byte(`null`))
			return
		}
		switch r.PathValue("kind") {
		case "movies":
			// Foxtrot by its TMDB id, Alpha by its IMDb id only.
			_, _ = w.Write([]byte(`{"movies":[
				{"movie":{"title":"Foxtrot","year":2021,"ids":{"simkl":1,"tmdb":105}}},
				{"movie":{"title":"Alpha","year":2010,"ids":{"simkl":2,"imdb":"tt0000100"}}}]}`))
		case "shows":
			// Show One by its TheTVDB id only.
			_, _ = w.Write([]byte(`{"shows":[{"show":{"title":"Show One","year":2015,"ids":{"simkl":3,"tvdb":5000}}}]}`))
		default:
			_, _ = w.Write([]byte(`{"anime":[]}`))
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func simklServer(t *testing.T) (*api.Server, string, *http.Client, *fakeSimkl) {
	t.Helper()
	server, base, admin := listsServer(t)
	f := newFakeSimkl(t)
	server.TestSetSimklBaseURL(simklTestClient, f.srv.URL)
	return server, base, admin, f
}

func connectSimkl(t *testing.T, base string, admin *http.Client, f *fakeSimkl) {
	t.Helper()
	pin := postJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin", nil, http.StatusOK)
	if pin["code"] != "AB12C" || pin["url"] != "https://simkl.com/pin" {
		t.Fatalf("pin = %v", pin)
	}
	if got := getJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin/AB12C"); got["done"] != false {
		t.Fatalf("pending: %v", got)
	}
	f.entered.Store(true)
	got := getJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin/AB12C")
	if got["done"] != true {
		t.Fatalf("done: %v", got)
	}
	if st, _ := got["status"].(map[string]any); st["connected"] != true {
		t.Fatalf("status after connecting: %v", got)
	}
}

func TestSimklConnectAndImportAList(t *testing.T) {
	server, base, admin, f := simklServer(t)
	connectSimkl(t, base, admin, f)

	// The token never comes back from the server.
	for _, p := range []string{"/api/settings", "/api/settings/simkl", "/api/import-lists"} {
		resp, err := admin.Get(base + p)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(raw), simklTestToken) || strings.Contains(string(raw), simklTestClient) {
			t.Fatalf("a Simkl secret came back from %s", p)
		}
	}
	if st := getJSON[map[string]any](t, admin, base+"/api/settings/simkl"); st["connected"] != true || st["hasClientId"] != true {
		t.Fatalf("status: %v", st)
	}

	list := postJSON[map[string]any](t, admin, base+"/api/import-lists", map[string]any{
		"source": "simkl", "media": "both", "config": map[string]any{"status": "plantowatch"}, "intervalHours": 24, "searchNow": false,
	}, http.StatusCreated)
	waitBackground(t, server)
	l := readList(t, admin, base, list["id"].(float64))
	if l["lastStatus"] != "ok" || l["lastFound"] != float64(3) || l["lastAdded"] != float64(3) {
		t.Fatalf("simkl list after the first read: %v", l)
	}
	movies := libraryTMDBIDs(t, admin, base, "/api/movies")
	if !movies[105] || !movies[100] {
		t.Fatalf("movies in the library: %v", movies)
	}
	if shows := libraryTMDBIDs(t, admin, base, "/api/series"); !shows[200] {
		t.Fatalf("the show found by its TheTVDB id is missing: %v", shows)
	}

	// Another status is another list: empty here, and nothing is removed.
	other := postJSON[map[string]any](t, admin, base+"/api/import-lists", map[string]any{
		"source": "simkl", "media": "both", "config": map[string]any{"status": "completed"}, "intervalHours": 24, "searchNow": false,
	}, http.StatusCreated)
	waitBackground(t, server)
	if l := readList(t, admin, base, other["id"].(float64)); l["lastStatus"] != "ok" || l["lastFound"] != float64(0) {
		t.Fatalf("empty list: %v", l)
	}
	if movies := libraryTMDBIDs(t, admin, base, "/api/movies"); !movies[105] {
		t.Fatal("reading an empty list must not remove anything")
	}
}

func TestSimklListNeedsTheAccountAndSaysSo(t *testing.T) {
	server, base, admin, _ := simklServer(t)
	list := postJSON[map[string]any](t, admin, base+"/api/import-lists", map[string]any{
		"source": "simkl", "media": "both", "config": map[string]any{}, "intervalHours": 24, "searchNow": false,
	}, http.StatusCreated)
	waitBackground(t, server)
	l := readList(t, admin, base, list["id"].(float64))
	if l["lastStatus"] != "error" || !strings.Contains(l["lastMessage"].(string), "Connect your Simkl account") {
		t.Fatalf("no account connected: %v", l)
	}
	if h := healthByID(t, admin, base); h["simkl-off"] == nil {
		t.Fatalf("Health should say Simkl isn't connected: %v", keysOf(h))
	}
}

func TestSimklErrorsAreShownPlainly(t *testing.T) {
	server, base, admin, f := simklServer(t)
	connectSimkl(t, base, admin, f)
	list := postJSON[map[string]any](t, admin, base+"/api/import-lists", map[string]any{
		"source": "simkl", "media": "both", "config": map[string]any{}, "intervalHours": 24, "searchNow": false, "enabled": false,
	}, http.StatusCreated)
	id := list["id"].(float64)
	for _, tc := range []struct {
		code int
		want string
	}{
		{401, "no longer accepts the saved sign-in"},
		{429, "asked to slow down"},
		{503, "isn't answering properly"},
	} {
		f.listCode.Store(int32(tc.code))
		l := runList(t, server, admin, base, id)
		msg, _ := l["lastMessage"].(string)
		if l["lastStatus"] != "error" || !strings.Contains(msg, tc.want) {
			t.Errorf("status %d: %v", tc.code, l)
		}
		if strings.Contains(msg, simklTestToken) || strings.Contains(msg, simklTestClient) {
			t.Errorf("status %d: a secret is in the message %q", tc.code, msg)
		}
	}
}

func TestSimklTestConnectionAndBadRequestText(t *testing.T) {
	_, base, admin, f := simklServer(t)
	res := postJSON[map[string]any](t, admin, base+"/api/settings/simkl/test", nil, http.StatusOK)
	if res["ok"] != false || !strings.Contains(res["message"].(string), "No Simkl account is connected") {
		t.Fatalf("before connecting: %v", res)
	}
	connectSimkl(t, base, admin, f)
	res = postJSON[map[string]any](t, admin, base+"/api/settings/simkl/test", nil, http.StatusOK)
	if res["ok"] != true || !strings.Contains(res["message"].(string), "Casey") {
		t.Fatalf("after connecting: %v", res)
	}

	// A 400 when starting the sign-in shows what Simkl said and what to check.
	f.pinCode.Store(400)
	resp := postJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin", nil, http.StatusBadGateway)
	msg, _ := resp["error"].(string)
	if !strings.Contains(msg, "status 400") || !strings.Contains(msg, "redirect URI") {
		t.Fatalf("a 400 must say what to check, got %q", msg)
	}
}

func TestSimklPinStatesAndDisconnect(t *testing.T) {
	_, base, admin, f := simklServer(t)
	// A code Mediarium never handed out is refused without asking Simkl.
	if r, err := admin.Get(base + "/api/settings/simkl/pin/ZZ99Z"); err != nil || r.StatusCode != http.StatusGone {
		t.Fatalf("an unknown code: %v %v", r, err)
	}
	connectSimkl(t, base, admin, f)
	if code := deleteReq(t, admin, base+"/api/settings/simkl"); code != http.StatusNoContent {
		t.Fatalf("disconnect: %d", code)
	}
	if st := getJSON[map[string]any](t, admin, base+"/api/settings/simkl"); st["connected"] != false {
		t.Fatalf("still connected: %v", st)
	}
}

func TestSimklClientIDIsCheckedAndNeverShown(t *testing.T) {
	_, base, admin, _ := simklServer(t)
	res := postJSON[map[string]any](t, admin, base+"/api/settings/test-service", map[string]any{"service": "simkl", "key": "someone-elses-id"}, http.StatusOK)
	if res["ok"] == true {
		t.Fatalf("a client ID Simkl does not know was accepted: %v", res)
	}
	res = postJSON[map[string]any](t, admin, base+"/api/settings/test-service", map[string]any{"service": "simkl", "key": simklTestClient}, http.StatusOK)
	if res["ok"] != true {
		t.Fatalf("the right client ID was refused: %v", res)
	}
	list := postJSON[map[string]any](t, admin, base+"/api/import-lists", map[string]any{
		"source": "simkl", "media": "both", "config": map[string]any{"status": "someday"}, "intervalHours": 24,
	}, http.StatusBadRequest)
	if msg, _ := list["error"].(string); !strings.Contains(msg, "Simkl list") {
		t.Fatalf("a made-up status: %v", list)
	}
}

func TestSimklSignInThatCannotBeReadIsFlaggedNotLost(t *testing.T) {
	server, base, admin, _ := simklServer(t)
	db, err := api.OpenTestDB(filepath.Join(server.TestConfigDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other, err := crypto.LoadOrCreateKey(filepath.Join(t.TempDir(), "other.key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.New(db, other).Set(settings.KeySimklToken, simklTestToken, true); err != nil {
		t.Fatal(err)
	}
	if st := getJSON[map[string]any](t, admin, base+"/api/settings/simkl"); st["unreadable"] != true || st["connected"] != false {
		t.Fatalf("status with an unreadable sign-in: %v", st)
	}
	list := postJSON[map[string]any](t, admin, base+"/api/import-lists", map[string]any{
		"source": "simkl", "media": "both", "config": map[string]any{}, "intervalHours": 24, "searchNow": false,
	}, http.StatusCreated)
	waitBackground(t, server)
	l := readList(t, admin, base, list["id"].(float64))
	if l["lastStatus"] != "error" || !strings.Contains(l["lastMessage"].(string), "can't be read") {
		t.Fatalf("list with an unreadable sign-in: %v", l)
	}
	h := healthByID(t, admin, base)
	if h["settings-secrets-unreadable"] == nil {
		t.Fatalf("Health should name the saved secret that can't be read: %v", keysOf(h))
	}
	if item := h["settings-secrets-unreadable"]; !strings.Contains(item["impact"].(string), "Simkl sign-in") {
		t.Fatalf("health item: %v", item)
	}
}

// A shared Simkl key that Simkl refuses shows on Health, with the way out.
func TestSharedSimklKeyAtItsLimitIsOnHealth(t *testing.T) {
	server, base, admin, f := simklServer(t)
	f.pinCode.Store(http.StatusTooManyRequests)
	server.TestSetSimklBaseURL("", f.srv.URL)
	server.SetBuiltinKeys(api.BuiltinKeys{SimklClientID: simklTestClient})
	if h := healthByID(t, admin, base); h["simkl-limit"] != nil {
		t.Fatal("no warning before any request was refused")
	}
	resp, err := admin.Post(base+"/api/settings/simkl/pin", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("start of the sign-in while Simkl refuses: %d", resp.StatusCode)
	}
	item := healthByID(t, admin, base)["simkl-limit"]
	if item == nil || item["level"] != "warn" || item["title"] != "The shared Simkl key is at its limit" {
		t.Fatalf("expected the simkl-limit warning, got %v", item)
	}
}

func rawSetting(t *testing.T, server *api.Server, key string) string {
	t.Helper()
	db, err := api.OpenTestDB(filepath.Join(server.TestConfigDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return ""
	}
	return v
}

func TestSimklDeviceFlowConnectsEncryptedAndImports(t *testing.T) {
	server, base, admin, f := simklServer(t)
	f.oauth2.Store(true)
	f.expiresIn.Store(0)
	start := postJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin", nil, http.StatusOK)
	if start["code"] != "QZ7-48K" || start["flow"] != "device" || start["url"] != "https://simkl.com/device" || start["expiresIn"] != float64(600) {
		t.Fatalf("start = %v", start)
	}
	if strings.Contains(toJSON(t, start), "dev-code") {
		t.Fatal("the device code must stay on the server")
	}
	if got := getJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin/QZ7-48K"); got["done"] != false {
		t.Fatalf("pending: %v", got)
	}
	f.entered.Store(true)
	got := getJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin/QZ7-48K")
	if got["done"] != true || got["name"] != "Casey" {
		t.Fatalf("done: %v", got)
	}
	for _, key := range []string{settings.KeySimklToken, settings.KeySimklRefresh} {
		raw := rawSetting(t, server, key)
		if raw == "" || strings.Contains(raw, simklTestToken) || strings.Contains(raw, simklTestRenew) {
			t.Fatalf("%s is not stored encrypted: %q", key, raw)
		}
	}
	if raw := rawSetting(t, server, settings.KeySimklExpires); raw != "" {
		t.Fatalf("a token without an expiry must not get one: %q", raw)
	}
	// The importer reads the token the device flow stored.
	list := postJSON[map[string]any](t, admin, base+"/api/import-lists", map[string]any{
		"source": "simkl", "media": "both", "config": map[string]any{"status": "plantowatch"}, "intervalHours": 24, "searchNow": false,
	}, http.StatusCreated)
	waitBackground(t, server)
	if l := readList(t, admin, base, list["id"].(float64)); l["lastStatus"] != "ok" || l["lastFound"] != float64(3) {
		t.Fatalf("list after a device sign-in: %v", l)
	}
	// Disconnecting forgets the sign-in.
	if code := deleteReq(t, admin, base+"/api/settings/simkl"); code != http.StatusNoContent {
		t.Fatalf("disconnect: %d", code)
	}
	if st := getJSON[map[string]any](t, admin, base+"/api/settings/simkl"); st["connected"] != false {
		t.Fatalf("still connected: %v", st)
	}
}

func TestSimklDeviceFlowSlowDownDeniedAndExpired(t *testing.T) {
	_, base, admin, f := simklServer(t)
	f.oauth2.Store(true)
	postJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin", nil, http.StatusOK)

	f.slowOnce.Store(true)
	got := getJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin/QZ7-48K")
	if got["done"] != false || got["interval"] != float64(10) {
		t.Fatalf("slow_down must add 5 seconds to the 5 Simkl asked for: %v", got)
	}
	if got := getJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin/QZ7-48K"); got["done"] != false || got["interval"] != float64(10) {
		t.Fatalf("the longer wait must stay: %v", got)
	}

	f.denied.Store(true)
	resp, err := admin.Get(base + "/api/settings/simkl/pin/QZ7-48K")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone || !strings.Contains(string(body), "declined") {
		t.Fatalf("denied: %d %s", resp.StatusCode, body)
	}

	f.denied.Store(false)
	f.expired.Store(true)
	postJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin", nil, http.StatusOK)
	resp, err = admin.Get(base + "/api/settings/simkl/pin/QZ7-48K")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone || !strings.Contains(string(body), "expired") {
		t.Fatalf("expired: %d %s", resp.StatusCode, body)
	}
}

func TestSimklRefreshesAnExpiredSignInBeforeReadingLists(t *testing.T) {
	server, base, admin, f := simklServer(t)
	f.oauth2.Store(true)
	f.expiresIn.Store(1) // runs out at once, as far as Mediarium can tell
	postJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin", nil, http.StatusOK)
	f.entered.Store(true)
	if got := getJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin/QZ7-48K"); got["done"] != true {
		t.Fatalf("done: %v", got)
	}
	// Simkl now only accepts the refreshed token.
	f.fresh.Store(false)
	f.refreshes.Store(0)
	list := postJSON[map[string]any](t, admin, base+"/api/import-lists", map[string]any{
		"source": "simkl", "media": "both", "config": map[string]any{"status": "plantowatch"}, "intervalHours": 24, "searchNow": false,
	}, http.StatusCreated)
	waitBackground(t, server)
	if f.refreshes.Load() != 1 {
		t.Fatalf("refreshes = %d, want 1", f.refreshes.Load())
	}
	if l := readList(t, admin, base, list["id"].(float64)); l["lastStatus"] != "ok" || l["lastFound"] != float64(3) {
		t.Fatalf("list after the refresh: %v", l)
	}
	// The refreshed token is what is saved now, with a new expiry an hour out.
	if raw := rawSetting(t, server, settings.KeySimklExpires); raw == "" {
		t.Fatal("the new expiry was not saved")
	}
	res := postJSON[map[string]any](t, admin, base+"/api/settings/simkl/test", nil, http.StatusOK)
	if res["ok"] != true {
		t.Fatalf("test connection after the refresh: %v", res)
	}
	if f.refreshes.Load() != 1 {
		t.Fatalf("a token that is still good was refreshed again: %d", f.refreshes.Load())
	}
}

func TestSimklRefreshTokenSimklRefusesMeansConnectAgain(t *testing.T) {
	_, base, admin, f := simklServer(t)
	f.oauth2.Store(true)
	f.expiresIn.Store(1)
	postJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin", nil, http.StatusOK)
	f.entered.Store(true)
	getJSON[map[string]any](t, admin, base+"/api/settings/simkl/pin/QZ7-48K")
	// A refresh token Simkl no longer knows: the next /oauth2/token refresh answers invalid_grant.
	f.entered.Store(false)
	f.refuseRefresh.Store(true)
	res := postJSON[map[string]any](t, admin, base+"/api/settings/simkl/test", nil, http.StatusOK)
	if res["ok"] != false || !strings.Contains(res["message"].(string), "Connect your Simkl account again") {
		t.Fatalf("got %v", res)
	}
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Some accounts get the account call refused although their lists sync fine:
// the connection test then proves the sign-in with the activity call instead
// of telling the person their client ID is wrong.
func TestSimklTestConnectionWhenTheAccountCallIsRefused(t *testing.T) {
	_, base, admin, f := simklServer(t)
	connectSimkl(t, base, admin, f)
	f.setCode.Store(http.StatusForbidden)
	res := postJSON[map[string]any](t, admin, base+"/api/settings/simkl/test", nil, http.StatusOK)
	if res["ok"] != true || strings.Contains(res["message"].(string), "rejected the client ID") {
		t.Fatalf("account call refused but the sign-in works: %v", res)
	}
}

// The Client ID test under the field goes through the same sign-in choice as
// connecting: an OAuth 2.0 app is accepted through the device flow, an older
// app through the PIN flow, and neither leaves a sign-in to finish (issue 115).
func TestSimklClientIDTestFollowsTheSignInFlow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		oauth2 bool
	}{{"an OAuth 2.0 app", true}, {"a legacy PIN app", false}} {
		t.Run(tc.name, func(t *testing.T) {
			_, base, admin, f := simklServer(t)
			f.oauth2.Store(tc.oauth2)
			res := postJSON[map[string]any](t, admin, base+"/api/settings/test-service", map[string]any{"service": "simkl", "key": simklTestClient}, http.StatusOK)
			if res["ok"] != true || res["message"] != "Simkl accepted this Client ID." {
				t.Fatalf("test result: %v", res)
			}
			// The code the test asked for is not kept, so there is nothing to finish.
			for _, code := range []string{"AB12C", "QZ7-48K"} {
				resp, err := admin.Get(base + "/api/settings/simkl/pin/" + code)
				if err != nil {
					t.Fatal(err)
				}
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusGone {
					t.Fatalf("code %s: status %d, want 410", code, resp.StatusCode)
				}
			}
		})
	}
}

func TestSimklClientIDTestKeepsSimklsOwnWordsAndLogsAWarning(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	_, base, admin, f := simklServer(t)
	f.pinCode.Store(http.StatusInternalServerError) // a legacy app and Simkl is failing
	res := postJSON[map[string]any](t, admin, base+"/api/settings/test-service", map[string]any{"service": "simkl", "key": simklTestClient}, http.StatusOK)
	msg, _ := res["message"].(string)
	if res["ok"] == true || !strings.Contains(msg, "500") {
		t.Fatalf("a refused test: %v", res)
	}
	out := logs.String()
	if !strings.Contains(out, "simkl refused") || !strings.Contains(out, "client_id_start=fixt...") {
		t.Fatalf("the refusal was not logged: %s", out)
	}
	if strings.Contains(out, simklTestClient) {
		t.Fatalf("the whole Client ID was logged: %s", out)
	}
}
