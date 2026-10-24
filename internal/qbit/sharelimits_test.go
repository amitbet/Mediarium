package qbit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// shareFake records the add call and the setShareLimits call.
type shareFake struct {
	mu        sync.Mutex
	add       map[string]string
	hasAdd    bool
	limits    map[string]string
	limitCall int
	failLimit bool
}

func (f *shareFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api/v2/auth/login":
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "s", Path: "/"})
		fmt.Fprint(w, "Ok.")
	case "/api/v2/torrents/createCategory":
		w.WriteHeader(http.StatusConflict)
	case "/api/v2/torrents/add":
		_ = r.ParseMultipartForm(1 << 20)
		_, f.hasAdd = r.MultipartForm.Value["ratioLimit"]
		f.add = map[string]string{"ratioLimit": r.FormValue("ratioLimit"), "seedingTimeLimit": r.FormValue("seedingTimeLimit")}
		fmt.Fprint(w, "Ok.")
	case "/api/v2/torrents/setShareLimits":
		f.limitCall++
		if f.failLimit {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = r.ParseForm()
		f.limits = map[string]string{}
		for _, k := range []string{"hashes", "ratioLimit", "seedingTimeLimit", "inactiveSeedingTimeLimit"} {
			f.limits[k] = r.PostForm.Get(k)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestAddSendsShareLimits(t *testing.T) {
	cases := []struct {
		name      string
		share     ShareLimits
		hash      string
		failLimit bool
		wantSent  bool
		ratio     string
		minutes   string
	}{
		{"both", ShareLimits{Ratio: 1.5, Time: 72 * time.Hour}, "abc", false, true, "1.5", "4320"},
		{"ratio only means no time limit", ShareLimits{Ratio: 2}, "abc", false, true, "2", "-1"},
		{"time only means no ratio limit", ShareLimits{Time: 90 * time.Minute}, "abc", false, true, "-1", "90"},
		{"under a minute rounds up", ShareLimits{Time: 10 * time.Second}, "abc", false, true, "-1", "1"},
		{"both zero sends nothing", ShareLimits{}, "abc", false, false, "", ""},
		{"do not seed stops it at ratio 0", ShareLimits{NoSeed: true}, "abc", false, true, "0", "-1"},
		{"no hash: only in the add call", ShareLimits{Ratio: 1}, "", false, true, "1", "-1"},
		{"failed limit call doesn't fail the add", ShareLimits{Ratio: 1}, "abc", true, true, "1", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &shareFake{failLimit: tc.failLimit}
			srv := httptest.NewServer(f)
			defer srv.Close()
			err := New(srv.URL, "u", "p", srv.Client()).Add(context.Background(),
				AddOptions{Magnet: "magnet:?xt=urn:btih:abc", Hash: tc.hash, Share: tc.share})
			if err != nil {
				t.Fatalf("Add: %v", err)
			}
			if f.hasAdd != tc.wantSent {
				t.Fatalf("limits in the add call = %v, want %v", f.hasAdd, tc.wantSent)
			}
			if !tc.wantSent {
				if f.limitCall != 0 {
					t.Fatalf("setShareLimits was called")
				}
				return
			}
			if f.add["ratioLimit"] != tc.ratio || f.add["seedingTimeLimit"] != tc.minutes {
				t.Errorf("add got ratio %q time %q", f.add["ratioLimit"], f.add["seedingTimeLimit"])
			}
			if tc.hash == "" {
				if f.limitCall != 0 {
					t.Errorf("setShareLimits called without a hash")
				}
				return
			}
			if f.limitCall != 1 {
				t.Fatalf("setShareLimits called %d times", f.limitCall)
			}
			if tc.failLimit {
				return
			}
			want := map[string]string{"hashes": "abc", "ratioLimit": tc.ratio, "seedingTimeLimit": tc.minutes, "inactiveSeedingTimeLimit": "-2"}
			for k, v := range want {
				if f.limits[k] != v {
					t.Errorf("%s = %q, want %q", k, f.limits[k], v)
				}
			}
		})
	}
}

// A .torrent file is added with the hash worked out from its contents (lower
// case), and the limits are set on that hash.
func TestAddTorrentFileSetsLimitsOnItsHash(t *testing.T) {
	file := []byte("d4:infod6:lengthi1e4:name1:a12:piece lengthi16384e6:pieces20:" + "01234567890123456789" + "ee")
	hash := InfoHash("", file)
	if len(hash) != 40 || hash != strings.ToLower(hash) {
		t.Fatalf("InfoHash = %q", hash)
	}
	f := &shareFake{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	err := New(srv.URL, "u", "p", srv.Client()).Add(context.Background(),
		AddOptions{TorrentFile: file, Hash: hash, Share: ShareLimits{Time: 2 * time.Hour}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if f.add["seedingTimeLimit"] != "120" || f.add["ratioLimit"] != "-1" {
		t.Errorf("add got ratio %q time %q", f.add["ratioLimit"], f.add["seedingTimeLimit"])
	}
	if f.limitCall != 1 || f.limits["hashes"] != hash || f.limits["seedingTimeLimit"] != "120" {
		t.Errorf("setShareLimits = %v (%d calls)", f.limits, f.limitCall)
	}
}
