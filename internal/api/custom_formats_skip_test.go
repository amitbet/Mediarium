package api_test

import (
	"net/http"
	"testing"
)

// Formats a profile already has are not added a second time: they keep their
// score, are counted as "already there", and only change when the person asks
// for the score to be replaced. The same for the TRaSH import with a profile.
func TestAddingFormatsAlreadyInTheProfileSkipsThem(t *testing.T) {
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
	other := postJSON[map[string]any](t, client, api+"/quality-profiles", map[string]any{
		"name": "Other", "allowed": []string{"WEBDL-1080p"}, "cutoff": "WEBDL-1080p",
	}, http.StatusCreated)
	oid := int64(other["id"].(float64))

	// First import with a profile: both are new there.
	imp := postJSON[map[string]any](t, client, api+"/trash-guides/radarr/formats", map[string]any{"trashIds": []string{hdrTrashID, x265TrashID}, "profileId": pid}, http.StatusOK)
	if p := imp["profile"].(map[string]any); p["added"] != float64(2) || p["alreadyThere"] != float64(0) || p["name"] != "Scored" {
		t.Fatalf("first import: %v", imp["profile"])
	}
	formats := getJSON[map[string]any](t, client, api+"/custom-formats")["formats"].([]any)
	hdr := formatIDByName(t, formats, "HDR")
	x265 := formatIDByName(t, formats, "x265 (HD)")
	// The person changes HDR's score by hand.
	postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{
		"formatIds": []float64{hdr}, "profileIds": []int64{pid}, "mode": "score", "score": 42, "replaceScores": true,
	}, http.StatusOK)

	// The same import again: both skipped, the hand-made score stays, no extra rows.
	imp = postJSON[map[string]any](t, client, api+"/trash-guides/radarr/formats", map[string]any{"trashIds": []string{hdrTrashID, x265TrashID}, "profileId": pid}, http.StatusOK)
	if p := imp["profile"].(map[string]any); p["added"] != float64(0) || p["alreadyThere"] != float64(2) || p["replaced"] != float64(0) {
		t.Fatalf("second import: %v", imp["profile"])
	}
	prof := profileByID(t, client, httpSrv.URL, pid)
	if n := len(prof["formatScores"].([]any)); n != 2 {
		t.Fatalf("the profile has %d format rows, want 2", n)
	}
	if s, _ := scoreOf(prof, hdr); s != 42 {
		t.Fatalf("HDR lost its score: %v", s)
	}

	// Asked to replace: TRaSH's score comes back for HDR; x265 already has it.
	imp = postJSON[map[string]any](t, client, api+"/trash-guides/radarr/formats", map[string]any{"trashIds": []string{hdrTrashID, x265TrashID}, "profileId": pid, "replaceScores": true}, http.StatusOK)
	if p := imp["profile"].(map[string]any); p["replaced"] != float64(1) || p["alreadyThere"] != float64(1) {
		t.Fatalf("replacing import: %v", imp["profile"])
	}
	if s, _ := scoreOf(profileByID(t, client, httpSrv.URL, pid), hdr); s != 500 {
		t.Fatalf("HDR should be back to TRaSH's 500, got %v", s)
	}

	// Add to profiles: one profile has both, the other has neither; counted per profile.
	res := postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{
		"formatIds": []float64{hdr, x265}, "profileIds": []int64{pid, oid}, "mode": "trash",
	}, http.StatusOK)
	changes := res["changes"].([]any)
	if len(changes) != 2 || res["scored"] != float64(2) {
		t.Fatalf("assign: %v", res)
	}
	if c := changes[0].(map[string]any); c["name"] != "Scored" || c["added"] != float64(0) || c["alreadyThere"] != float64(2) {
		t.Errorf("first profile: %v", c)
	}
	if c := changes[1].(map[string]any); c["name"] != "Other" || c["added"] != float64(2) || c["alreadyThere"] != float64(0) {
		t.Errorf("second profile: %v", c)
	}
	// A typed score does not touch what is there either, and 0 still takes a format out.
	res = postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{
		"formatIds": []float64{hdr}, "profileIds": []int64{oid}, "mode": "score", "score": 7,
	}, http.StatusOK)
	if c := res["changes"].([]any)[0].(map[string]any); c["alreadyThere"] != float64(1) || c["added"] != float64(0) {
		t.Errorf("typed score on an existing format: %v", c)
	}
	if s, _ := scoreOf(profileByID(t, client, httpSrv.URL, oid), hdr); s != 500 {
		t.Errorf("HDR in Other: %v", s)
	}
	res = postJSON[map[string]any](t, client, api+"/custom-formats/assign", map[string]any{
		"formatIds": []float64{hdr}, "profileIds": []int64{oid}, "mode": "score", "score": 0,
	}, http.StatusOK)
	if c := res["changes"].([]any)[0].(map[string]any); c["removed"] != float64(1) {
		t.Errorf("taking a format out: %v", c)
	}
}
