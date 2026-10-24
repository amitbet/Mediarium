package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	hdrTrashID  = "493b6d1dbec3c3364c59d7607f7e3405"
	x265TrashID = "dc98083864ea246d05a42df0d05f81cc"
)

// changingGuides serves the fixture formats like fakeTrashGuides, and lets a
// test change what one file says between two reads.
type changingGuides struct {
	srv   *httptest.Server
	mu    sync.Mutex
	cf    map[string]string
	lists int
}

func newChangingGuides(t *testing.T) *changingGuides {
	t.Helper()
	g := &changingGuides{cf: map[string]string{
		"hdr.json":       fixture(t, "radarr-hdr.json"),
		"x265-hd.json":   fixture(t, "radarr-x265-hd.json"),
		"1080p.json":     fixture(t, "radarr-1080p.json"),
		"amzn.json":      fixture(t, "radarr-amzn.json"),
		"imax.json":      fixture(t, "radarr-imax.json"),
		"dts-hd-ma.json": fixture(t, "radarr-dts-hd-ma.json"),
	}}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		switch {
		case r.URL.Path == "/api/radarr/cf":
			g.lists++
			var list []map[string]string
			for name := range g.cf {
				list = append(list, map[string]string{"name": name, "type": "file"})
			}
			_ = json.NewEncoder(w).Encode(list)
		case r.URL.Path == "/api/radarr/quality-profiles":
			_, _ = w.Write([]byte("[]"))
		case strings.HasPrefix(r.URL.Path, "/raw/radarr/cf/"):
			body, ok := g.cf[strings.TrimPrefix(r.URL.Path, "/raw/radarr/cf/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.srv.Close)
	return g
}

// set replaces one file's text with the text after replacing old by new.
func (g *changingGuides) edit(t *testing.T, file, old, new string) {
	t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	if !strings.Contains(g.cf[file], old) {
		t.Fatalf("%s has no %q", file, old)
	}
	g.cf[file] = strings.Replace(g.cf[file], old, new, 1)
}

func formatByID(t *testing.T, formats []any, id float64) map[string]any {
	t.Helper()
	for _, raw := range formats {
		if f := raw.(map[string]any); f["id"].(float64) == id {
			return f
		}
	}
	t.Fatalf("no format %v", id)
	return nil
}

func scoreOf(p map[string]any, formatID float64) (float64, bool) {
	for _, raw := range p["formatScores"].([]any) {
		if s := raw.(map[string]any); s["formatId"].(float64) == formatID {
			return s["score"].(float64), true
		}
	}
	return 0, false
}

func TestBulkAssignAndTrashUpdate(t *testing.T) {
	server, httpSrv, client := newHuntTestServer(t)
	postJSON[map[string]any](t, client, httpSrv.URL+"/api/onboarding/admin", map[string]string{
		"username": "ryan", "password": "correct-horse-battery-staple", "firstName": "Ryan", "lastName": "Tester",
	}, http.StatusCreated)
	guides := newChangingGuides(t)
	server.SetTrashGuidesSource(guides.srv.URL+"/api", guides.srv.URL+"/raw")
	api := httpSrv.URL + "/api"

	// Two formats from the guides, added without a profile: no scores anywhere.
	res := postJSON[map[string]any](t, client, api+"/trash-guides/radarr/formats", map[string]any{"trashIds": []string{hdrTrashID, x265TrashID}}, http.StatusOK)
	if res["added"].(float64) != 2 {
		t.Fatalf("import: %v", res)
	}
	// One of our own as well; the guides never touch it.
	ownJSON := `{"name": "Mine", "specifications": [{"name": "my group", "implementation": "ReleaseGroupSpecification", "negate": false, "required": false, "fields": {"value": "^MYGRP$"}}]}`
	postJSON[map[string]any](t, client, api+"/custom-formats", map[string]any{"json": ownJSON, "app": "radarr"}, http.StatusOK)
	formats := getJSON[map[string]any](t, client, api+"/custom-formats")["formats"].([]any)
	hdr := formatIDByName(t, formats, "HDR")
	x265 := formatIDByName(t, formats, "x265 (HD)")
	plain := formatIDByName(t, formats, "Mine")

	profiles := getJSON[map[string]any](t, client, api+"/quality-profiles")
	p1080 := profileIDByName(t, profiles, "1080p")
	p720 := profileIDByName(t, profiles, "720p")

	t.Run("the score TRaSH gives each format", func(t *testing.T) {
		got := postJSON[map[string]any](t, client, api+"/custom-formats/default-scores", map[string]any{"formatIds": []float64{hdr, x265, plain}}, http.StatusOK)
		scores := map[float64]float64{}
		for _, raw := range got["scores"].([]any) {
			s := raw.(map[string]any)
			scores[s["formatId"].(float64)] = s["score"].(float64)
		}
		if len(scores) != 2 || scores[hdr] != 500 || scores[x265] != -10000 {
			t.Fatalf("scores %v", scores)
		}
		postJSON[map[string]any](t, client, api+"/custom-formats/default-scores", map[string]any{"formatIds": []float64{99999}}, http.StatusNotFound)
		postJSON[map[string]any](t, client, api+"/custom-formats/default-scores", map[string]any{"formatIds": []float64{}}, http.StatusBadRequest)
	})

	t.Run("bad requests change nothing", func(t *testing.T) {
		postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{"formatIds": []float64{hdr}, "profileIds": []int64{}, "mode": "trash"}, http.StatusBadRequest)
		postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{"formatIds": []float64{hdr}, "profileIds": []int64{p1080}, "mode": "nope"}, http.StatusBadRequest)
		postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{"formatIds": []float64{hdr}, "profileIds": []int64{p1080}, "mode": "score", "score": 20000}, http.StatusBadRequest)
		// A profile that is gone: the other one is not changed either.
		postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{"formatIds": []float64{hdr}, "profileIds": []int64{p1080, 99999}, "mode": "trash"}, http.StatusNotFound)
		if _, ok := scoreOf(profileByID(t, client, httpSrv.URL, p1080), hdr); ok {
			t.Fatal("a refused request changed a profile")
		}
	})

	t.Run("TRaSH's own scores in two profiles at once", func(t *testing.T) {
		got := postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{
			"formatIds": []float64{hdr, x265, plain}, "profileIds": []int64{p1080, p720}, "mode": "trash",
		}, http.StatusOK)
		if got["profiles"].(float64) != 2 || got["scored"].(float64) != 4 {
			t.Fatalf("result %v", got)
		}
		if un := got["unscored"].([]any); len(un) != 1 || un[0] != "Mine" {
			t.Fatalf("the format the guides don't score is reported: %v", un)
		}
		for _, pid := range []int64{p1080, p720} {
			p := profileByID(t, client, httpSrv.URL, pid)
			if s, _ := scoreOf(p, hdr); s != 500 {
				t.Fatalf("HDR in %d: %v", pid, s)
			}
			if s, _ := scoreOf(p, x265); s != -10000 {
				t.Fatalf("x265 in %d: %v", pid, s)
			}
			if _, ok := scoreOf(p, plain); ok {
				t.Fatalf("an unscored format must not appear in %d", pid)
			}
		}
	})

	t.Run("a typed score, and 0 takes the format out", func(t *testing.T) {
		postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{
			"formatIds": []float64{hdr, plain}, "profileIds": []int64{p720}, "mode": "score", "score": 75, "replaceScores": true,
		}, http.StatusOK)
		p := profileByID(t, client, httpSrv.URL, p720)
		if s, _ := scoreOf(p, hdr); s != 75 {
			t.Fatalf("HDR: %v", s)
		}
		if s, _ := scoreOf(p, plain); s != 75 {
			t.Fatalf("Mine: %v", s)
		}
		postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{
			"formatIds": []float64{plain}, "profileIds": []int64{p720}, "mode": "score", "score": 0,
		}, http.StatusOK)
		if _, ok := scoreOf(profileByID(t, client, httpSrv.URL, p720), plain); ok {
			t.Fatal("a score of 0 should take the format out of the profile")
		}
	})

	t.Run("update from the guides", func(t *testing.T) {
		// Nothing changed upstream yet.
		pre := getJSON[map[string]any](t, client, api+"/custom-formats/trash-update")
		if len(pre["changes"].([]any)) != 0 {
			t.Fatalf("nothing changed upstream: %v", pre)
		}
		// Someone changes a score in the 720p profile by hand; the 1080p profile still has TRaSH's.
		postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{
			"formatIds": []float64{hdr}, "profileIds": []int64{p720}, "mode": "score", "score": 123, "replaceScores": true,
		}, http.StatusOK)
		// The guides raise HDR to 600 and change its pattern; x265's pattern changes too; a made-up format never appears.
		guides.edit(t, "hdr.json", `"default": 500`, `"default": 600`)
		guides.edit(t, "x265-hd.json", `\bHEVC`, `\bHEVC|\bAV1`)
		// The guides' own cache keeps a read for six hours; a fresh server source gets the new text.
		server.SetTrashGuidesSource(guides.srv.URL+"/api", guides.srv.URL+"/raw")

		preview := getJSON[map[string]any](t, client, api+"/custom-formats/trash-update")
		changes := preview["changes"].([]any)
		if len(changes) != 2 {
			t.Fatalf("want 2 changed formats: %v", preview)
		}
		for _, raw := range changes {
			c := raw.(map[string]any)
			if c["name"] == "HDR" {
				if c["scoreChanged"] != true || c["scoreFrom"].(float64) != 500 || c["scoreTo"].(float64) != 600 {
					t.Fatalf("HDR change: %v", c)
				}
				// Only the profile that still has TRaSH's score follows.
				if ps := c["profiles"].([]any); len(ps) != 1 || ps[0] != "1080p" {
					t.Fatalf("profiles that follow: %v", ps)
				}
			}
		}
		// A preview changes nothing.
		if s, _ := scoreOf(profileByID(t, client, httpSrv.URL, p1080), hdr); s != 500 {
			t.Fatalf("the preview changed a score: %v", s)
		}

		applied := postJSON[map[string]any](t, client, api+"/custom-formats/trash-update", map[string]any{}, http.StatusOK)
		if len(applied["changes"].([]any)) != 2 {
			t.Fatalf("applied: %v", applied)
		}
		if s, _ := scoreOf(profileByID(t, client, httpSrv.URL, p1080), hdr); s != 600 {
			t.Fatalf("1080p should follow the guides: %v", s)
		}
		if s, _ := scoreOf(profileByID(t, client, httpSrv.URL, p720), hdr); s != 123 {
			t.Fatalf("a score changed by hand must stay: %v", s)
		}
		after := getJSON[map[string]any](t, client, api+"/custom-formats")["formats"].([]any)
		if conds := formatByID(t, after, x265)["conditions"].([]any); len(conds) == 0 {
			t.Fatal("the format lost its conditions")
		}
		// The format made by hand is exactly as it was, and a second run has nothing to do.
		if f := formatByID(t, after, plain); f["name"] != "Mine" || len(f["conditions"].([]any)) != 1 {
			t.Fatalf("own format: %v", f)
		}
		again := postJSON[map[string]any](t, client, api+"/custom-formats/trash-update", map[string]any{}, http.StatusOK)
		if len(again["changes"].([]any)) != 0 {
			t.Fatalf("a second update has nothing to do: %v", again)
		}
		act := getJSON[[]any](t, client, api+"/activity")
		if !strings.Contains(toJSONString(act), "Checked the TRaSH Guides: 2 formats updated") {
			t.Fatalf("no Activity line: %v", act)
		}
		if c, _ := applied["check"].(map[string]any); c == nil || c["updated"] != float64(2) {
			t.Fatalf("the result should say what was checked: %v", applied["check"])
		}
		st := getJSON[map[string]any](t, client, api+"/custom-formats/trash-update/status")
		if last, _ := st["last"].(map[string]any); last == nil || last["updated"] != float64(0) {
			t.Fatalf("the last check is the second one, with nothing to update: %v", st)
		}
	})
}

func toJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// listings counts how many times the guides' cf folder was listed.
func (g *changingGuides) listings() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lists
}

func TestTrashUpdateJobRunsOncePerDayAndOnlyWhenOn(t *testing.T) {
	server, httpSrv, client := newHuntTestServer(t)
	postJSON[map[string]any](t, client, httpSrv.URL+"/api/onboarding/admin", map[string]string{
		"username": "ryan", "password": "correct-horse-battery-staple", "firstName": "Ryan", "lastName": "Tester",
	}, http.StatusCreated)
	guides := newChangingGuides(t)
	server.SetTrashGuidesSource(guides.srv.URL+"/api", guides.srv.URL+"/raw")
	api := httpSrv.URL + "/api"
	postJSON[map[string]any](t, client, api+"/trash-guides/radarr/formats", map[string]any{"trashIds": []string{hdrTrashID}}, http.StatusOK)
	before := guides.listings()

	// Off by default: the job reads nothing.
	server.TestTrashFormatsJob(t.Context())
	if guides.listings() != before {
		t.Fatal("the job read the guides while the setting is off")
	}
	if s := getJSON[map[string]any](t, client, api+"/settings"); s["trashFormatsAuto"] != false {
		t.Fatalf("off by default: %v", s["trashFormatsAuto"])
	}

	putJSONStatus(t, client, api+"/settings", map[string]any{"trashFormatsAuto": true}, http.StatusOK)
	if s := getJSON[map[string]any](t, client, api+"/settings"); s["trashFormatsAuto"] != true {
		t.Fatalf("switched on: %v", s["trashFormatsAuto"])
	}
	server.SetTrashGuidesSource(guides.srv.URL+"/api", guides.srv.URL+"/raw") // forget the cached read
	server.TestTrashFormatsJob(t.Context())
	afterFirst := guides.listings()
	if afterFirst == before {
		t.Fatal("the job should read the guides when it is on")
	}
	// A second run the same day reads nothing.
	server.SetTrashGuidesSource(guides.srv.URL+"/api", guides.srv.URL+"/raw")
	server.TestTrashFormatsJob(t.Context())
	if guides.listings() != afterFirst {
		t.Fatal("the job read the guides twice in one day")
	}
}

func TestTrashCheckStatusAndEveryWeek(t *testing.T) {
	server, httpSrv, client := newHuntTestServer(t)
	postJSON[map[string]any](t, client, httpSrv.URL+"/api/onboarding/admin", map[string]string{
		"username": "ryan", "password": "correct-horse-battery-staple", "firstName": "Ryan", "lastName": "Tester",
	}, http.StatusCreated)
	guides := newChangingGuides(t)
	server.SetTrashGuidesSource(guides.srv.URL+"/api", guides.srv.URL+"/raw")
	api := httpSrv.URL + "/api"
	postJSON[map[string]any](t, client, api+"/trash-guides/radarr/formats", map[string]any{"trashIds": []string{hdrTrashID}}, http.StatusOK)

	if st := getJSON[map[string]any](t, client, api+"/custom-formats/trash-update/status"); st["last"] != nil || st["next"] != nil {
		t.Fatalf("nothing checked and the switch off: %v", st)
	}
	if s := getJSON[map[string]any](t, client, api+"/settings"); s["trashFormatsEveryDays"] != float64(1) {
		t.Fatalf("every day by default: %v", s["trashFormatsEveryDays"])
	}
	for _, bad := range []int{0, 2, 30} {
		putJSONStatus(t, client, api+"/settings", map[string]any{"trashFormatsEveryDays": bad}, http.StatusBadRequest)
	}
	putJSONStatus(t, client, api+"/settings", map[string]any{"trashFormatsEveryDays": 7, "trashFormatsAuto": true}, http.StatusOK)
	if s := getJSON[map[string]any](t, client, api+"/settings"); s["trashFormatsEveryDays"] != float64(7) {
		t.Fatalf("every week: %v", s["trashFormatsEveryDays"])
	}

	// The job makes a check, and the card can see it.
	server.TestTrashFormatsJob(t.Context())
	st := getJSON[map[string]any](t, client, api+"/custom-formats/trash-update/status")
	last, _ := st["last"].(map[string]any)
	if last == nil || last["daily"] != true || last["error"] != nil {
		t.Fatalf("the daily check should be kept: %v", st)
	}
	next, err := time.Parse(time.RFC3339, st["next"].(string))
	if err != nil || time.Until(next) < 6*24*time.Hour {
		t.Fatalf("the next check is a week away: %v (%v)", st["next"], err)
	}
	if act := toJSONString(getJSON[[]any](t, client, api+"/activity")); !strings.Contains(act, "Checked the TRaSH Guides") {
		t.Fatalf("the job should write to Activity: %s", act)
	}
}

// Scores given by importing TRaSH formats into a profile, or by adding formats
// to profiles, are what the profile returns afterwards; a form that saves the
// profile later with the scores it holds now keeps them.
func TestAssignedScoresReachTheProfile(t *testing.T) {
	server, httpSrv, client := newHuntTestServer(t)
	postJSON[map[string]any](t, client, httpSrv.URL+"/api/onboarding/admin", map[string]string{
		"username": "ryan", "password": "correct-horse-battery-staple", "firstName": "Ryan", "lastName": "Tester",
	}, http.StatusCreated)
	guides := newChangingGuides(t)
	server.SetTrashGuidesSource(guides.srv.URL+"/api", guides.srv.URL+"/raw")
	api := httpSrv.URL + "/api"
	created := postJSON[map[string]any](t, client, api+"/quality-profiles", map[string]any{
		"name": "Scored", "allowed": []string{"WEBDL-1080p"}, "cutoff": "WEBDL-1080p",
	}, http.StatusCreated)
	pid := int64(created["id"].(float64))
	imp := postJSON[map[string]any](t, client, api+"/trash-guides/radarr/formats", map[string]any{"trashIds": []string{hdrTrashID}, "profileId": pid}, http.StatusOK)
	hdr := imp["formats"].([]any)[0].(map[string]any)["id"].(float64)
	if sc, ok := scoreOf(profileByID(t, client, httpSrv.URL, pid), hdr); !ok || sc == 0 {
		t.Fatalf("the import should score the format in the profile: %v", profileByID(t, client, httpSrv.URL, pid))
	}
}
