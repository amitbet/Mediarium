package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// getStatus gets url, checks the status and returns the body.
func getStatus(t *testing.T, client *http.Client, url string, want int) string {
	t.Helper()
	status, _, body := getRaw(t, client, url)
	if status != want {
		t.Fatalf("GET %s: status %d, want %d (%s)", url, status, want, body)
	}
	return string(body)
}

const movieSizeFile = `{"trash_id":"x","type":"movie","qualities":[
 {"quality":"WEBDL-1080p","min":12.5,"preferred":1999,"max":2000},
 {"quality":"WEBRip-1080p","min":12.5,"preferred":1999,"max":2000},
 {"quality":"Bluray-1080p","min":50.8,"preferred":100,"max":155},
 {"quality":"Bluray-576p","min":5,"preferred":100,"max":155}]}`

func TestTrashSizesEndpoint(t *testing.T) {
	server, httpSrv, client := newHuntTestServer(t)
	postJSON[map[string]any](t, client, httpSrv.URL+"/api/onboarding/admin", map[string]string{
		"username": "ryan", "password": "correct-horse-battery-staple", "firstName": "Ryan", "lastName": "Tester",
	}, http.StatusCreated)
	down := false
	guides := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down {
			http.Error(w, "gone", http.StatusInternalServerError)
			return
		}
		switch {
		case r.URL.Path == "/api/radarr/quality-size":
			_, _ = w.Write([]byte(`[{"name":"movie.json","type":"file"}]`))
		case strings.HasPrefix(r.URL.Path, "/raw/radarr/quality-size/"):
			_, _ = w.Write([]byte(movieSizeFile))
		default:
			http.NotFound(w, r)
		}
	}))
	defer guides.Close()
	server.SetTrashGuidesSource(guides.URL+"/api", guides.URL+"/raw")
	api := httpSrv.URL + "/api"

	before := getJSON[map[string]any](t, client, api+"/quality/size-limits")

	got := getJSON[map[string]any](t, client, api+"/quality/size-limits/trash/radarr-movie")
	if got["kind"] != "movie" {
		t.Errorf("kind = %v, want movie", got["kind"])
	}
	rows := got["rows"].([]any)
	want := map[string][2]float64{"WEBDL-1080p": {12.5, 0}, "Bluray-1080p": {50.8, 155}}
	if len(rows) != len(want) {
		t.Fatalf("rows: %v", rows)
	}
	for _, raw := range rows {
		r := raw.(map[string]any)
		w, ok := want[r["quality"].(string)]
		if !ok || r["min"] != w[0] || r["max"] != w[1] {
			t.Errorf("row %v, want %v", r, w)
		}
	}
	if sk := got["skipped"].([]any); len(sk) != 1 || sk[0] != "Bluray-576p" {
		t.Errorf("skipped: %v", sk)
	}
	// Reading the list changes nothing by itself.
	if after := getJSON[map[string]any](t, client, api+"/quality/size-limits"); toJSONString(after) != toJSONString(before) {
		t.Errorf("the size table changed: %v", after)
	}

	// A list that doesn't exist, and a name that isn't one.
	getStatus(t, client, api+"/quality/size-limits/trash/radarr-sqp-uhd", http.StatusNotFound)
	getStatus(t, client, api+"/quality/size-limits/trash/nope", http.StatusBadRequest)

	// GitHub being down: the cached copy still answers; with no copy there is a clear error.
	down = true
	getJSON[map[string]any](t, client, api+"/quality/size-limits/trash/radarr-movie")
	server.SetTrashGuidesSource(guides.URL+"/api", guides.URL+"/raw")
	resp := getStatus(t, client, api+"/quality/size-limits/trash/radarr-movie", http.StatusBadGateway)
	if !strings.Contains(resp, "TRaSH Guides") {
		t.Errorf("the error should name the TRaSH Guides: %s", resp)
	}
}
