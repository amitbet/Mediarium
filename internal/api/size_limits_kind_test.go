package api_test

import (
	"net/http"
	"testing"
)

// The movie and show size tables are separate: each is read, saved and reset
// on its own through ?kind=, and the movie table stays the default.
func TestSizeLimitTablesAreSeparate(t *testing.T) {
	_, base, client := loginNewServer(t)
	url := func(kind string) string {
		if kind == "" {
			return base + "/api/quality/size-limits"
		}
		return base + "/api/quality/size-limits?kind=" + kind
	}
	row := func(tbl map[string]any, q string) map[string]any {
		for _, r := range tbl["rows"].([]any) {
			if r.(map[string]any)["quality"] == q {
				return r.(map[string]any)
			}
		}
		t.Fatalf("no row for %s", q)
		return nil
	}
	body := func(tbl map[string]any, enabled bool, q string, min, max float64) map[string]any {
		rows := tbl["rows"].([]any)
		row(tbl, q)["min"], row(tbl, q)["max"] = min, max
		return map[string]any{"enabled": enabled, "rows": rows}
	}

	movies := getJSON[map[string]any](t, client, url(""))
	shows := getJSON[map[string]any](t, client, url("tv"))
	if movies["enabled"] != false || shows["enabled"] != false {
		t.Fatal("size limits must be off on a fresh install")
	}

	// Saving the show table leaves the movie table alone.
	saved := postJSONMethod[map[string]any](t, client, http.MethodPut, url("tv"), body(shows, true, "WEBDL-1080p", 7, 60), http.StatusOK)
	if saved["enabled"] != true || row(saved, "WEBDL-1080p")["max"] != float64(60) {
		t.Fatalf("saved show table = %v", row(saved, "WEBDL-1080p"))
	}
	again := getJSON[map[string]any](t, client, url(""))
	if again["enabled"] != false || row(again, "WEBDL-1080p")["min"] != row(again, "WEBDL-1080p")["defaultMin"] {
		t.Fatalf("the movie table changed: %v", row(again, "WEBDL-1080p"))
	}
	if kept := getJSON[map[string]any](t, client, url("tv")); kept["enabled"] != true || row(kept, "WEBDL-1080p")["min"] != float64(7) {
		t.Fatalf("show table read back = %v", row(kept, "WEBDL-1080p"))
	}

	// And the other way round.
	postJSONMethod[map[string]any](t, client, http.MethodPut, url("movie"), body(again, true, "Bluray-1080p", 9, 90), http.StatusOK)
	if got := getJSON[map[string]any](t, client, url("tv")); row(got, "Bluray-1080p")["min"] != row(got, "Bluray-1080p")["defaultMin"] {
		t.Fatalf("the show table changed with the movie table: %v", row(got, "Bluray-1080p"))
	}

	// Reset only touches the table named; its switch keeps its position.
	reset := postJSON[map[string]any](t, client, base+"/api/quality/size-limits/reset?kind=tv", nil, http.StatusOK)
	if reset["enabled"] != true || row(reset, "WEBDL-1080p")["min"] != row(reset, "WEBDL-1080p")["defaultMin"] {
		t.Fatalf("reset show table = %v", row(reset, "WEBDL-1080p"))
	}
	if got := getJSON[map[string]any](t, client, url("movie")); row(got, "Bluray-1080p")["min"] != float64(9) {
		t.Fatalf("resetting shows reset the movie table: %v", row(got, "Bluray-1080p"))
	}

	getStatus(t, client, url("music"), http.StatusBadRequest)
}
