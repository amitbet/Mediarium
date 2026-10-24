package api_test

import (
	"net/http"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
)

// The recommended movies an app like nzb360 asks for on its home screen are
// what's trending, without what the library already has; the settings pages
// next to it answer in their v3 shapes.
func TestArrImportListAndSettingsPages(t *testing.T) {
	server, base, admin, _, _ := familyServer(t)
	server.TestSetTMDBBaseURL("fixture-key", arrTMDB(t).URL)
	key := postJSON[map[string]any](t, admin, base+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	radarr, sonarr := base+"/radarr/api/v3", base+"/sonarr/api/v3"

	recs := list(arrCall(t, "GET", radarr+"/importlist/movie?includeRecommendations=true", key, nil, http.StatusOK))
	if len(recs) != 2 || obj(recs[0])["tmdbId"] != float64(701) || obj(recs[1])["title"] != "Low Tide" {
		t.Fatalf("recommendations: %v", recs)
	}
	if obj(recs[1])["year"] != float64(2022) || obj(recs[1])["images"] == nil || obj(recs[1])["id"] != nil {
		t.Errorf("a recommendation has the lookup shape: %v", recs[1])
	}
	// Owning a movie takes it out, even within the hour it is kept.
	if _, err := server.MovieRepo.Add(library.Movie{TMDBID: 701, Title: "Night Harbour", Year: 2021, Monitored: true}); err != nil {
		t.Fatal(err)
	}
	recs = list(arrCall(t, "GET", radarr+"/importlist/movie", key, nil, http.StatusOK))
	if len(recs) != 1 || obj(recs[0])["tmdbId"] != float64(702) {
		t.Fatalf("after adding 701: %v", recs)
	}

	for _, p := range []string{"/exclusions", "/importexclusion", "/importlistexclusion", "/autotagging", "/releaseprofile", "/system/backup", "/delayprofile", "/importlist", "/metadata"} {
		for _, base := range []string{radarr, sonarr} {
			if _, ok := arrCall(t, "GET", base+p, key, nil, http.StatusOK).([]any); !ok {
				t.Errorf("%s should answer a list", base+p)
			}
		}
	}
	for _, p := range []string{"/exclusions/paged", "/importlistexclusion/paged"} {
		pg := obj(arrCall(t, "GET", radarr+p+"?page=1&pageSize=20", key, nil, http.StatusOK))
		if pg["totalRecords"] != float64(0) || pg["records"] == nil || pg["pageSize"] != float64(20) {
			t.Errorf("%s: %v", p, pg)
		}
	}
	for _, p := range []string{"/config/mediamanagement", "/config/naming", "/config/downloadclient", "/config/indexer"} {
		if o := obj(arrCall(t, "GET", radarr+p, key, nil, http.StatusOK)); o["id"] != float64(1) {
			t.Errorf("%s: %v", p, o)
		}
	}
	// The lookups the home screen makes.
	if l := list(arrCall(t, "GET", radarr+"/movie/lookup?term=night", key, nil, http.StatusOK)); len(l) != 1 {
		t.Errorf("lookup: %v", l)
	}
	if m := obj(arrCall(t, "GET", radarr+"/movie/lookup/tmdb?tmdbId=701", key, nil, http.StatusOK)); m["title"] != "Night Harbour" {
		t.Errorf("lookup by tmdb: %v", m)
	}
}
