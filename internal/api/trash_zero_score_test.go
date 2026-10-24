package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

const firstTrashID = "820b09bb9acbfde9c35c71e0e565dad8" // the "1080p" format

// A TRaSH format whose score is 0 is still added to the profile, as an entry
// of 0, and stays there through a save; a typed 0 in bulk still takes it out.
func TestZeroScoreFormatsJoinTheProfile(t *testing.T) {
	server, httpSrv, client := newHuntTestServer(t)
	postJSON[map[string]any](t, client, httpSrv.URL+"/api/onboarding/admin", map[string]string{
		"username": "ryan", "password": "correct-horse-battery-staple", "firstName": "Ryan", "lastName": "Tester",
	}, http.StatusCreated)
	guides := newChangingGuides(t)
	guides.edit(t, "1080p.json", `"default": 50`, `"default": 0`)
	server.SetTrashGuidesSource(guides.srv.URL+"/api", guides.srv.URL+"/raw")
	api := httpSrv.URL + "/api"
	created := postJSON[map[string]any](t, client, api+"/quality-profiles", map[string]any{
		"name": "Zero", "allowed": []string{"WEBDL-1080p"}, "cutoff": "WEBDL-1080p",
	}, http.StatusCreated)
	pid := int64(created["id"].(float64))
	profileURL := fmt.Sprintf("%s/quality-profiles/%d", api, pid)

	imp := postJSON[map[string]any](t, client, api+"/trash-guides/radarr/formats", map[string]any{"trashIds": []string{firstTrashID, hdrTrashID}, "profileId": pid}, http.StatusOK)
	var zero float64
	for _, raw := range imp["formats"].([]any) {
		if f := raw.(map[string]any); f["name"] == "1080p" {
			zero = f["id"].(float64)
		}
	}
	p := profileByID(t, client, httpSrv.URL, pid)
	if sc, ok := scoreOf(p, zero); !ok || sc != 0 {
		t.Fatalf("the 0-score format should be in the profile with 0: %v", p["formatScores"])
	}
	if len(p["formatScores"].([]any)) != 2 {
		t.Fatalf("both formats should be in the profile: %v", p["formatScores"])
	}

	// Saving the profile with the entry keeps it; leaving it out removes it.
	putJSONStatus(t, client, profileURL, map[string]any{
		"name": "Zero", "allowed": []string{"WEBDL-1080p"}, "cutoff": "WEBDL-1080p",
		"formatScores": []map[string]any{{"formatId": zero, "score": 0}},
	}, http.StatusOK)
	if sc, ok := scoreOf(profileByID(t, client, httpSrv.URL, pid), zero); !ok || sc != 0 {
		t.Fatal("a saved 0 entry should stay")
	}
	putJSONStatus(t, client, profileURL, map[string]any{
		"name": "Zero", "allowed": []string{"WEBDL-1080p"}, "cutoff": "WEBDL-1080p",
		"formatScores": []map[string]any{},
	}, http.StatusOK)
	if _, ok := scoreOf(profileByID(t, client, httpSrv.URL, pid), zero); ok {
		t.Fatal("left out of the saved list, the entry should go")
	}

	// TRaSH's own 0 through "add to profiles" adds it; a typed 0 removes it.
	postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{"formatIds": []float64{zero}, "profileIds": []int64{pid}, "mode": "trash"}, http.StatusOK)
	if sc, ok := scoreOf(profileByID(t, client, httpSrv.URL, pid), zero); !ok || sc != 0 {
		t.Fatal("assign with TRaSH's 0 should add the format")
	}
	postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{"formatIds": []float64{zero}, "profileIds": []int64{pid}, "mode": "score", "score": 0}, http.StatusOK)
	if _, ok := scoreOf(profileByID(t, client, httpSrv.URL, pid), zero); ok {
		t.Fatal("a typed 0 should take it out")
	}
}
