package api_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/api"
	"github.com/rdborg/mediarium/internal/config"
	"github.com/rdborg/mediarium/internal/crypto"
	"github.com/rdborg/mediarium/internal/library"
)

// A torrent release goes to Premiumize, once as a torrent it already has (read
// at once) and once as a transfer that has to run first. Either way the files
// come home over HTTPS and are imported like any other download.
func TestPremiumizeGrabDownloadImport(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	for _, tc := range []struct {
		name   string
		cached bool
	}{{"already cached", true}, {"needs a transfer", false}} {
		t.Run(tc.name, func(t *testing.T) {
			movieContent := []byte("Fixture movie bytes from Premiumize. " + strings.Repeat("padding-", 300))
			var mu sync.Mutex
			polls, transfers, deleted := 0, 0, []string{}
			var base string
			mux := http.NewServeMux()
			authed := func(h http.HandlerFunc) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer pm-key" {
						fmt.Fprint(w, `{"status":"error","message":"customer_id and pin parameter missing or not logged in"}`)
						return
					}
					h(w, r)
				}
			}
			mux.HandleFunc("GET /account/info", authed(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"status":"success","customer_id":"77","premium_until":%d,"limit_used":0.1}`, time.Now().Add(72*time.Hour).Unix())
			}))
			mux.HandleFunc("POST /cache/check", authed(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				resp := []string{}
				for _, h := range r.PostForm["items[]"] {
					resp = append(resp, fmt.Sprint(tc.cached && h == hash))
				}
				fmt.Fprintf(w, `{"status":"success","response":[%s]}`, strings.Join(resp, ","))
			}))
			mux.HandleFunc("POST /transfer/directdl", authed(func(w http.ResponseWriter, r *http.Request) {
				if !tc.cached {
					fmt.Fprint(w, `{"status":"error","message":"Not cached"}`)
					return
				}
				fmt.Fprintf(w, `{"status":"success","content":[{"path":"The.Fixture.Movie.1999.1080p.WEB-DL.x264-FIXTURE/movie.mkv","size":%d,"link":%q}]}`, len(movieContent), base+"/files/movie.mkv")
			}))
			mux.HandleFunc("POST /transfer/create", authed(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseMultipartForm(1 << 20)
				mu.Lock()
				transfers++
				mu.Unlock()
				if !strings.Contains(r.FormValue("src"), hash) {
					fmt.Fprint(w, `{"status":"error","message":"no src"}`)
					return
				}
				fmt.Fprint(w, `{"status":"success","id":"T1","type":"torrent"}`)
			}))
			mux.HandleFunc("GET /transfer/list", authed(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				polls++
				done := polls > 2
				mu.Unlock()
				if done {
					fmt.Fprint(w, `{"status":"success","transfers":[{"id":"T1","name":"Fixture","status":"finished","progress":1,"folder_id":"F1"}]}`)
					return
				}
				fmt.Fprint(w, `{"status":"success","transfers":[{"id":"T1","name":"Fixture","status":"running","progress":0.5,"message":"Downloading"}]}`)
			}))
			mux.HandleFunc("GET /folder/list", authed(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("id") != "F1" {
					fmt.Fprint(w, `{"status":"error","message":"folder not found"}`)
					return
				}
				fmt.Fprintf(w, `{"status":"success","name":"Fixture","content":[{"type":"file","id":"X1","name":"movie.mkv","size":%d,"link":%q}]}`, len(movieContent), base+"/files/movie.mkv")
			}))
			for _, kind := range []string{"transfer", "folder", "item"} {
				mux.HandleFunc("POST /"+kind+"/delete", authed(func(w http.ResponseWriter, r *http.Request) {
					_ = r.ParseForm()
					mu.Lock()
					deleted = append(deleted, kind+":"+r.PostForm.Get("id"))
					mu.Unlock()
					fmt.Fprint(w, `{"status":"success"}`)
				}))
			}
			mux.HandleFunc("GET /files/movie.mkv", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(movieContent)
			})
			pm := httptest.NewServer(mux)
			defer pm.Close()
			base = pm.URL
			defer api.SetDebridURLForTest(pm.URL)()
			defer api.SetDebridPollForTest(20 * time.Millisecond)()

			dir := t.TempDir()
			cfg := config.Config{
				ConfigDir:           filepath.Join(dir, "config"),
				DownloadsDir:        filepath.Join(dir, "downloads"),
				DownloadsIncomplete: filepath.Join(dir, "downloads", "incomplete"),
				DownloadsComplete:   filepath.Join(dir, "downloads", "complete"),
				MoviesDir:           filepath.Join(dir, "movies"),
			}
			for _, d := range []string{cfg.ConfigDir, cfg.DownloadsIncomplete, cfg.MoviesDir} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			db, err := api.OpenTestDB(filepath.Join(cfg.ConfigDir, "app.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			box, err := crypto.LoadOrCreateKey(filepath.Join(cfg.ConfigDir, "secret.key"))
			if err != nil {
				t.Fatal(err)
			}
			server, err := api.New(db, cfg, box, "", "test")
			if err != nil {
				t.Fatal(err)
			}
			defer server.TestWaitBackground()
			httpSrv := httptest.NewServer(server.Routes())
			defer httpSrv.Close()
			jar, _ := cookiejar.New(nil)
			client := &http.Client{Jar: jar}
			postJSON[map[string]any](t, client, httpSrv.URL+"/api/onboarding/admin", map[string]string{
				"username": "ryan", "password": "correct-horse-battery-staple", "firstName": "Ryan", "lastName": "Tester",
			}, http.StatusCreated)
			postJSONMethod[map[string]any](t, client, http.MethodPut, httpSrv.URL+"/api/settings", map[string]string{"moviesPath": cfg.MoviesDir, "namingPreset": "minimal"}, http.StatusOK)

			bad := postJSON[map[string]any](t, client, httpSrv.URL+"/api/debrid/test", map[string]any{"service": "premiumize", "apiKey": "wrong-key"}, http.StatusOK)
			if bad["ok"] != false || !strings.Contains(bad["error"].(string), "refused the API key") {
				t.Fatalf("bad key test = %v", bad)
			}
			good := postJSON[map[string]any](t, client, httpSrv.URL+"/api/debrid/test", map[string]any{"service": "premiumize", "apiKey": "pm-key"}, http.StatusOK)
			acc, _ := good["account"].(map[string]any)
			if good["ok"] != true || acc["name"] != "77" || acc["premium"] != true || acc["fairUse"] != 0.1 {
				t.Fatalf("test = %v", good)
			}
			saved := postJSONMethod[map[string]any](t, client, http.MethodPut, httpSrv.URL+"/api/debrid", map[string]any{"service": "premiumize", "apiKey": "pm-key"}, http.StatusOK)
			if saved["service"] != "premiumize" || saved["hasKey"] != true || saved["canCheck"] != true || saved["apiKey"] != nil {
				t.Fatalf("saved = %v", saved)
			}
			postJSONMethod[map[string]any](t, client, http.MethodPut, httpSrv.URL+"/api/settings", map[string]any{"torrentEnabled": false}, http.StatusOK)

			movie, err := server.MovieRepo.Add(library.Movie{TMDBID: 603, Title: "The Fixture Movie", Year: 1999, Monitored: true})
			if err != nil {
				t.Fatal(err)
			}
			postJSON[map[string]any](t, client, fmt.Sprintf("%s/api/movies/%d/grab", httpSrv.URL, movie.ID), map[string]any{
				"releaseTitle": "The.Fixture.Movie.1999.1080p.WEB-DL.x264-FIXTURE",
				"downloadUrl":  "magnet:?xt=urn:btih:" + hash + "&dn=fixture",
				"sizeBytes":    len(movieContent),
				"protocol":     "torrent",
			}, http.StatusAccepted)

			deadline := time.Now().Add(20 * time.Second)
			status := ""
			for time.Now().Before(deadline) {
				q := getJSON[[]map[string]any](t, client, httpSrv.URL+"/api/queue")
				if len(q) == 1 {
					status, _ = q[0]["status"].(string)
					if status == "failed" {
						t.Fatalf("failed: %v", q[0])
					}
					if status == "completed" {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
			}
			if status != "completed" {
				t.Fatalf("not completed in time, status %q", status)
			}
			final := getJSON[map[string]any](t, client, fmt.Sprintf("%s/api/movies/%d", httpSrv.URL, movie.ID))
			got, err := os.ReadFile(final["filePath"].(string))
			if err != nil || !bytes.Equal(got, movieContent) {
				t.Fatalf("imported file: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if tc.cached {
				if transfers != 0 || len(deleted) != 0 {
					t.Errorf("a cached torrent needs no transfer and leaves nothing to clean up: transfers=%d deleted=%v", transfers, deleted)
				}
			} else if transfers != 1 || strings.Join(deleted, ",") != "transfer:T1,folder:F1" {
				t.Errorf("the finished transfer and its folder should be removed: transfers=%d deleted=%v", transfers, deleted)
			}
		})
	}
}
