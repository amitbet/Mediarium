package api_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// The folder browse an app like nzb360 opens: only the library folders and
// the downloads folder, and a link that leaves them stays out.
func TestArrFilesystem(t *testing.T) {
	_, base, admin := loginNewServer(t)
	key := postJSON[map[string]any](t, admin, base+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)

	movies, outside := t.TempDir(), t.TempDir()
	for _, d := range []string{"Heat (1995)", "Alien (1979)"} {
		if err := os.MkdirAll(filepath.Join(movies, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(movies, "Heat (1995)", "Heat.mkv"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movies, "Heat (1995)", "Heat.nfo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := os.Symlink(outside, filepath.Join(movies, "escape")) == nil
	putJSONStatus(t, admin, base+"/api/settings", map[string]any{"moviesPath": movies}, http.StatusOK)

	for _, app := range []string{"radarr", "sonarr"} {
		fs := base + "/" + app + "/api/v3/filesystem"
		q := func(path string, files bool) map[string]any {
			u := fs + "?allowFoldersWithoutTrailingSlashes=true&path=" + url.QueryEscape(path)
			if files {
				u += "&includeFiles=true"
			}
			return obj(arrCall(t, "GET", u, key, nil, http.StatusOK))
		}
		top := q("", false)
		found := false
		for _, d := range top["directories"].([]any) {
			if obj(d)["path"] == movies && obj(d)["type"] == "folder" {
				found = true
			}
		}
		if !found || top["files"] == nil {
			t.Fatalf("%s root listing: %v", app, top)
		}

		in := q(movies, false)
		names := map[string]bool{}
		for _, d := range in["directories"].([]any) {
			names[obj(d)["name"].(string)] = true
		}
		if !names["Heat (1995)"] || !names["Alien (1979)"] || names["escape"] {
			t.Errorf("%s folders: %v", app, names)
		}
		if len(in["files"].([]any)) != 0 {
			t.Errorf("%s files shown without includeFiles: %v", app, in["files"])
		}

		sub := q(filepath.Join(movies, "Heat (1995)"), true)
		if len(sub["files"].([]any)) != 2 || sub["parent"] != movies {
			t.Errorf("%s folder with files: %v", app, sub)
		}
		if f := obj(sub["files"].([]any)[0]); f["type"] != "file" || f["name"] != "Heat.mkv" || f["size"] != float64(5) {
			t.Errorf("%s first file: %v", app, f)
		}

		// Outside the configured folders: an empty list, never the folder.
		for _, p := range []string{outside, filepath.Join(movies, "..")} {
			if out := q(p, true); len(out["directories"].([]any)) != 0 || len(out["files"].([]any)) != 0 {
				t.Errorf("%s listed %s: %v", app, p, out)
			}
		}
		if linked {
			if out := q(filepath.Join(movies, "escape"), true); len(out["files"].([]any)) != 0 {
				t.Errorf("%s followed a link out: %v", app, out)
			}
		}

		if got := obj(arrCall(t, "GET", fs+"/type?path="+url.QueryEscape(movies), key, nil, http.StatusOK))["type"]; got != "folder" {
			t.Errorf("%s type of a folder: %v", app, got)
		}
		if got := obj(arrCall(t, "GET", fs+"/type?path="+url.QueryEscape(filepath.Join(movies, "Heat (1995)", "Heat.mkv")), key, nil, http.StatusOK))["type"]; got != "file" {
			t.Errorf("%s type of a file: %v", app, got)
		}
		if got := obj(arrCall(t, "GET", fs+"/type?path="+url.QueryEscape(outside), key, nil, http.StatusOK))["type"]; got != "unknown" {
			t.Errorf("%s type outside: %v", app, got)
		}
		mf := list(arrCall(t, "GET", fs+"/mediafiles?path="+url.QueryEscape(filepath.Join(movies, "Heat (1995)")), key, nil, http.StatusOK))
		if len(mf) != 1 || obj(mf[0])["name"] != "Heat.mkv" {
			t.Errorf("%s media files: %v", app, mf)
		}
		if mf := list(arrCall(t, "GET", fs+"/mediafiles?path="+url.QueryEscape(outside), key, nil, http.StatusOK)); len(mf) != 0 {
			t.Errorf("%s media files outside: %v", app, mf)
		}

		if st := obj(arrCall(t, "GET", base+"/"+app+"/ping", key, nil, http.StatusOK))["status"]; st != "OK" {
			t.Errorf("%s ping: %v", app, st)
		}
		if _, ok := arrCall(t, "GET", base+"/"+app+"/api/v3/log/file", key, nil, http.StatusOK).([]any); !ok {
			t.Errorf("%s log/file", app)
		}
	}
}

// Taking several queue items off the list, and the grab call, answer even
// when nothing is there.
func TestArrQueueBulkAndGrab(t *testing.T) {
	_, base, admin := loginNewServer(t)
	key := postJSON[map[string]any](t, admin, base+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	arrCall(t, "DELETE", base+"/radarr/api/v3/queue/bulk", key, map[string]any{"ids": []int{999}}, http.StatusOK)
	arrCall(t, "DELETE", base+"/radarr/api/v3/queue/bulk", key, map[string]any{}, http.StatusBadRequest)
	arrCall(t, "POST", base+"/radarr/api/v3/queue/grab/999", key, nil, http.StatusNotFound)
}
