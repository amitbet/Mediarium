package usenetclient_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/usenetclient"
	"github.com/rdborg/mediarium/internal/usenetclient/fake"
)

func TestNotAClient(t *testing.T) {
	// Something that answers with a web page is not the downloader.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html><body>Hello</body></html>")
	}))
	defer srv.Close()
	for _, kind := range []string{usenetclient.SABnzbd, usenetclient.NZBGet} {
		_, err := usenetclient.NewAPI(kind, srv.URL, "key", "u", "p", srv.Client()).Check(context.Background(), nil)
		if !errors.Is(err, usenetclient.ErrNotThisClient) {
			t.Errorf("%s: err = %v, want ErrNotThisClient", kind, err)
		}
	}
}

func TestNothingListening(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	for _, kind := range []string{usenetclient.SABnzbd, usenetclient.NZBGet} {
		_, err := usenetclient.NewAPI(kind, url, "secretkey", "u", "p", http.DefaultClient).Check(context.Background(), nil)
		if err == nil {
			t.Fatalf("%s: no error", kind)
		}
		if strings.Contains(err.Error(), "secretkey") {
			t.Errorf("%s: the key is in the error: %v", kind, err)
		}
	}
}

// ends runs a stand-in until its job has ended and returns the job.
func ends(t *testing.T, c usenetclient.API, id string) usenetclient.Job {
	t.Helper()
	var seen []usenetclient.State
	for i := 0; i < 20; i++ {
		j, ok, err := c.Get(context.Background(), id)
		if err != nil || !ok {
			t.Fatalf("Get: %v %v", ok, err)
		}
		if len(seen) == 0 || seen[len(seen)-1] != j.State {
			seen = append(seen, j.State)
		}
		if j.State == usenetclient.StateCompleted || j.State == usenetclient.StateFailed {
			want := []usenetclient.State{usenetclient.StateQueued, usenetclient.StateDownloading}
			if len(seen) < 3 || seen[0] != want[0] || seen[1] != want[1] {
				t.Errorf("states %v: expected queued, downloading, then the end", seen)
			}
			return j
		}
	}
	t.Fatal("the job never ended")
	return usenetclient.Job{}
}

func TestSABnzbd(t *testing.T) {
	ctx := context.Background()
	f := &fake.SAB{APIKey: "k3y", Categories: []string{"movies", "Tv"}, Steps: 2, Result: fake.Result{Path: "/complete/movies/Name"}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	good := usenetclient.NewAPI(usenetclient.SABnzbd, srv.URL, "k3y", "", "", srv.Client())

	t.Run("check", func(t *testing.T) {
		cases := []struct {
			name       string
			key        string
			cats       []string
			wantAuth   bool
			wantMissed string
		}{
			{"all good", "k3y", []string{"movies", "tv", ""}, false, ""},
			{"wrong key", "nope", nil, true, ""},
			{"category missing", "k3y", []string{"movies", "music"}, false, "music"},
			{"default category is always there", "k3y", []string{"*"}, false, ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				v, err := usenetclient.NewAPI(usenetclient.SABnzbd, srv.URL, tc.key, "", "", srv.Client()).Check(ctx, tc.cats)
				var ce *usenetclient.CategoryError
				switch {
				case tc.wantAuth:
					if !errors.Is(err, usenetclient.ErrAuth) {
						t.Fatalf("err = %v, want ErrAuth", err)
					}
				case tc.wantMissed != "":
					if !errors.As(err, &ce) || ce.Name != tc.wantMissed {
						t.Fatalf("err = %v, want a missing category %q", err, tc.wantMissed)
					}
				default:
					if err != nil || v != "4.3.2" {
						t.Fatalf("Check = %q, %v", v, err)
					}
				}
			})
		}
	})

	t.Run("upload, progress and finish", func(t *testing.T) {
		id, err := good.Add(ctx, usenetclient.AddOptions{Name: "My.Movie.2020", NZB: []byte("<nzb/>"), Category: "movies", Priority: usenetclient.PriorityHigh})
		if err != nil || id == "" {
			t.Fatalf("Add: %q %v", id, err)
		}
		adds, _ := f.Snapshot()
		if len(adds) != 1 || adds[0].NZB != "<nzb/>" || adds[0].Category != "movies" || adds[0].Priority != "1" || adds[0].Name != "My.Movie.2020" {
			t.Fatalf("what SABnzbd got: %+v", adds)
		}
		j := ends(t, good, id)
		if j.State != usenetclient.StateCompleted || j.Path != "/complete/movies/Name" || j.Progress != 100 {
			t.Errorf("job: %+v", j)
		}
	})

	t.Run("by address", func(t *testing.T) {
		id, err := good.Add(ctx, usenetclient.AddOptions{Name: "By.Link", URL: "https://indexer.example/getnzb/1", Category: "tv"})
		if err != nil {
			t.Fatal(err)
		}
		adds, _ := f.Snapshot()
		if last := adds[len(adds)-1]; last.URL != "https://indexer.example/getnzb/1" || last.NZB != "" || last.Priority != "0" {
			t.Errorf("what SABnzbd got: %+v", last)
		}
		_ = ends(t, good, id)
	})

	t.Run("progress, speed and eta while downloading", func(t *testing.T) {
		hold := &fake.SAB{APIKey: "k3y", Steps: 3, Hold: true}
		hs := httptest.NewServer(hold)
		defer hs.Close()
		c := usenetclient.NewAPI(usenetclient.SABnzbd, hs.URL, "k3y", "", "", hs.Client())
		id, _ := c.Add(ctx, usenetclient.AddOptions{Name: "Slow", NZB: []byte("x")})
		_, _, _ = c.Get(ctx, id) // queued
		j, ok, err := c.Get(ctx, id)
		if err != nil || !ok || j.State != usenetclient.StateDownloading {
			t.Fatalf("job: %+v %v %v", j, ok, err)
		}
		if j.Speed != 2048*1024 || j.ETASeconds != 90 || j.SizeBytes != 100<<20 {
			t.Errorf("speed %d, eta %d, size %d", j.Speed, j.ETASeconds, j.SizeBytes)
		}
	})

	t.Run("pause, resume and remove", func(t *testing.T) {
		hold := &fake.SAB{APIKey: "k3y", Hold: true}
		hs := httptest.NewServer(hold)
		defer hs.Close()
		c := usenetclient.NewAPI(usenetclient.SABnzbd, hs.URL, "k3y", "", "", hs.Client())
		id, _ := c.Add(ctx, usenetclient.AddOptions{Name: "Pausable", NZB: []byte("x")})
		if err := c.Pause(ctx, id); err != nil {
			t.Fatal(err)
		}
		if j, _, _ := c.Get(ctx, id); j.State != usenetclient.StatePaused {
			t.Errorf("state %v", j.State)
		}
		if err := c.Resume(ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := c.Delete(ctx, id, true); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := c.Get(ctx, id); ok {
			t.Error("still there after Delete")
		}
		_, actions := hold.Snapshot()
		want := "queue delete " + id + " files=1"
		found := false
		for _, a := range actions {
			found = found || a == want
		}
		if !found {
			t.Errorf("actions %v lack %q", actions, want)
		}
	})
}

func TestSABnzbdFailure(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, msg string
		release   bool
	}{
		{"missing articles", "Aborted, cannot be completed - https://sabnzbd.org/not-complete", true},
		{"repair failed", "Repair failed, not enough repair blocks (500 short)", true},
		{"disk full", "Disk full while unpacking", false},
		{"cannot move", "Error moving the files: permission denied", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake.SAB{APIKey: "k", Steps: 1, Result: fake.Result{Failed: true, Message: tc.msg}}
			srv := httptest.NewServer(f)
			defer srv.Close()
			c := usenetclient.NewAPI(usenetclient.SABnzbd, srv.URL, "k", "", "", srv.Client())
			id, _ := c.Add(ctx, usenetclient.AddOptions{Name: "Bad", NZB: []byte("x")})
			j := ends(t, c, id)
			if j.State != usenetclient.StateFailed || j.FailMessage != tc.msg || j.ReleaseFault != tc.release {
				t.Errorf("job: %+v", j)
			}
		})
	}
}

func TestSABnzbdList(t *testing.T) {
	ctx := context.Background()
	f := &fake.SAB{APIKey: "k", Steps: 1, Hold: true}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := usenetclient.NewAPI(usenetclient.SABnzbd, srv.URL, "k", "", "", srv.Client())
	_, _ = c.Add(ctx, usenetclient.AddOptions{Name: "One", NZB: []byte("x"), Category: "movies"})
	_, _ = c.Add(ctx, usenetclient.AddOptions{Name: "Two", NZB: []byte("x")})
	list, err := c.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("List: %v %v", list, err)
	}
	if list[0].Category != "movies" || list[1].Category != "" {
		t.Errorf("categories: %q %q (the default category is shown as none)", list[0].Category, list[1].Category)
	}
}

func TestNZBGet(t *testing.T) {
	ctx := context.Background()
	f := &fake.NZBGet{User: "nzbget", Pass: "pw", Categories: []string{"Movies", "TV"}, Steps: 2, Result: fake.Result{Path: "/data/dst/Name"}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	good := usenetclient.NewAPI(usenetclient.NZBGet, srv.URL, "", "nzbget", "pw", srv.Client())

	t.Run("check", func(t *testing.T) {
		cases := []struct {
			name, user, pass string
			cats             []string
			wantAuth         bool
			wantMissed       string
		}{
			{"all good", "nzbget", "pw", []string{"movies", "tv"}, false, ""},
			{"wrong password", "nzbget", "nope", nil, true, ""},
			{"wrong user", "root", "pw", nil, true, ""},
			{"category missing", "nzbget", "pw", []string{"Movies", "Music"}, false, "Music"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				v, err := usenetclient.NewAPI(usenetclient.NZBGet, srv.URL, "", tc.user, tc.pass, srv.Client()).Check(ctx, tc.cats)
				var ce *usenetclient.CategoryError
				switch {
				case tc.wantAuth:
					if !errors.Is(err, usenetclient.ErrAuth) {
						t.Fatalf("err = %v, want ErrAuth", err)
					}
				case tc.wantMissed != "":
					if !errors.As(err, &ce) || ce.Name != tc.wantMissed {
						t.Fatalf("err = %v, want a missing category %q", err, tc.wantMissed)
					}
				default:
					if err != nil || v != "21.1" {
						t.Fatalf("Check = %q, %v", v, err)
					}
				}
			})
		}
	})

	t.Run("upload, progress and finish", func(t *testing.T) {
		id, err := good.Add(ctx, usenetclient.AddOptions{Name: "My.Movie.2020", NZB: []byte("<nzb/>"), Category: "Movies", Priority: usenetclient.PriorityForce})
		if err != nil || id == "" {
			t.Fatalf("Add: %q %v", id, err)
		}
		adds, _ := f.Snapshot()
		if len(adds) != 1 || adds[0].NZB != "<nzb/>" || adds[0].Category != "Movies" || adds[0].Priority != "900" || adds[0].Name != "My.Movie.2020" {
			t.Fatalf("what NZBGet got: %+v", adds)
		}
		j := ends(t, good, id)
		if j.State != usenetclient.StateCompleted || j.Path != "/data/dst/Name" || j.Progress != 100 {
			t.Errorf("job: %+v", j)
		}
	})

	t.Run("by address", func(t *testing.T) {
		id, err := good.Add(ctx, usenetclient.AddOptions{Name: "By.Link", URL: "https://indexer.example/getnzb/1"})
		if err != nil {
			t.Fatal(err)
		}
		adds, _ := f.Snapshot()
		if last := adds[len(adds)-1]; last.URL != "https://indexer.example/getnzb/1" || last.NZB != "" {
			t.Errorf("what NZBGet got: %+v", last)
		}
		_ = ends(t, good, id)
	})

	t.Run("speed and eta", func(t *testing.T) {
		hold := &fake.NZBGet{User: "u", Pass: "p", Steps: 3, Hold: true}
		hs := httptest.NewServer(hold)
		defer hs.Close()
		c := usenetclient.NewAPI(usenetclient.NZBGet, hs.URL, "", "u", "p", hs.Client())
		id, _ := c.Add(ctx, usenetclient.AddOptions{Name: "Slow", NZB: []byte("x")})
		_, _, _ = c.Get(ctx, id)
		j, ok, err := c.Get(ctx, id)
		if err != nil || !ok || j.State != usenetclient.StateDownloading {
			t.Fatalf("job: %+v %v %v", j, ok, err)
		}
		if j.Speed != 2<<20 || j.ETASeconds <= 0 || j.Progress <= 0 {
			t.Errorf("speed %d, eta %d, progress %v", j.Speed, j.ETASeconds, j.Progress)
		}
	})

	t.Run("pause, resume and remove", func(t *testing.T) {
		hold := &fake.NZBGet{User: "u", Pass: "p", Hold: true}
		hs := httptest.NewServer(hold)
		defer hs.Close()
		c := usenetclient.NewAPI(usenetclient.NZBGet, hs.URL, "", "u", "p", hs.Client())
		id, _ := c.Add(ctx, usenetclient.AddOptions{Name: "Pausable", NZB: []byte("x")})
		if err := c.Pause(ctx, id); err != nil {
			t.Fatal(err)
		}
		if j, _, _ := c.Get(ctx, id); j.State != usenetclient.StatePaused {
			t.Errorf("state %v", j.State)
		}
		if err := c.Resume(ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := c.Delete(ctx, id, true); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := c.Get(ctx, id); ok {
			t.Error("still there after Delete")
		}
		_, actions := hold.Snapshot()
		if len(actions) < 4 || actions[2] != "GroupFinalDelete 1" || actions[3] != "HistoryFinalDelete 1" {
			t.Errorf("actions %v", actions)
		}
	})
}

func TestNZBGetFailure(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		status  string
		state   usenetclient.State
		release bool
		removed bool
	}{
		{"FAILURE/HEALTH", usenetclient.StateFailed, true, false},
		{"FAILURE/PAR", usenetclient.StateFailed, true, false},
		{"FAILURE/UNPACK", usenetclient.StateFailed, true, false},
		{"WARNING/PASSWORD", usenetclient.StateFailed, true, false},
		{"FAILURE/MOVE", usenetclient.StateFailed, false, false},
		{"WARNING/SPACE", usenetclient.StateFailed, false, false},
		{"FAILURE/SCRIPT", usenetclient.StateFailed, false, false},
		{"DELETED/MANUAL", usenetclient.StateFailed, false, true},
		{"WARNING/SCRIPT", usenetclient.StateCompleted, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			failed := tc.state == usenetclient.StateFailed || strings.HasPrefix(tc.status, "WARNING")
			f := &fake.NZBGet{User: "u", Pass: "p", Steps: 1, Result: fake.Result{Failed: failed, Message: tc.status}}
			srv := httptest.NewServer(f)
			defer srv.Close()
			c := usenetclient.NewAPI(usenetclient.NZBGet, srv.URL, "", "u", "p", srv.Client())
			id, _ := c.Add(ctx, usenetclient.AddOptions{Name: "Bad", NZB: []byte("x")})
			j := ends(t, c, id)
			if j.State != tc.state || j.ReleaseFault != tc.release || j.Removed != tc.removed {
				t.Errorf("job: %+v", j)
			}
			if tc.state == usenetclient.StateFailed && j.FailMessage == "" {
				t.Error("a failure with no reason")
			}
		})
	}
}
