package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
)

const movieBlu = "Night.Harbour.2021.1080p.BluRay.x264-GRP"

// The interactive search of the Radarr and Sonarr apps (nzb360 and others):
// the list is an array in the v3 shape, a grab by guid starts the download,
// and what Mediarium does not answer is a JSON "not found".
func TestArrInteractiveReleaseSearch(t *testing.T) {
	env := newTVAutoEnv(t, []string{s1e1WEB, s1e2WEB, sPack, movieBlu}, []episodeSpec{
		{1, 1, "2020-01-01", library.StatusMissing, ""},
		{1, 2, "2020-01-08", library.StatusMissing, ""},
	})
	movie, err := env.server.MovieRepo.Add(library.Movie{TMDBID: 701, Title: "Night Harbour", Year: 2021, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	key := postJSON[map[string]any](t, env.client, env.baseURL+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	sonarr, radarr := env.baseURL+"/sonarr/api/v3", env.baseURL+"/radarr/api/v3"
	eps, err := env.server.MovieRepo.ListEpisodes(env.seriesID)
	if err != nil || len(eps) != 2 {
		t.Fatalf("episodes: %v %v", eps, err)
	}

	// Sonarr, one episode: both the episode's own release and the season pack.
	rels := list(arrCall(t, "GET", fmt.Sprintf("%s/release?episodeId=%d", sonarr, eps[0].ID), key, nil, http.StatusOK))
	byTitle := map[string]map[string]any{}
	for _, r := range rels {
		byTitle[obj(r)["title"].(string)] = obj(r)
	}
	if len(rels) != 2 || byTitle[s1e1WEB] == nil || byTitle[sPack] == nil {
		t.Fatalf("episode releases: %v", rels)
	}
	// The v3 field names and types the apps read.
	rel := byTitle[s1e1WEB]
	for _, f := range []string{"guid", "quality", "qualityWeight", "age", "ageHours", "ageMinutes", "size", "indexerId", "indexer",
		"releaseGroup", "title", "languages", "languageWeight", "approved", "temporarilyRejected", "rejected", "rejections",
		"publishDate", "downloadUrl", "downloadAllowed", "releaseWeight", "protocol", "fullSeason", "seasonNumber", "episodeNumbers", "customFormats"} {
		if _, ok := rel[f]; !ok {
			t.Errorf("release has no %q: %v", f, rel)
		}
	}
	q := obj(obj(rel["quality"])["quality"])
	if q["id"] != float64(3) || q["name"] != "WEBDL-1080p" || obj(rel["quality"])["revision"] == nil {
		t.Errorf("quality: %v", rel["quality"])
	}
	if l := list(rel["languages"]); len(l) != 1 || obj(l[0])["name"] != "English" {
		t.Errorf("languages: %v", rel["languages"])
	}
	if rel["protocol"] != "usenet" || rel["fullSeason"] != false || rel["seasonNumber"] != float64(1) || rel["indexer"] != "TV Indexer" || rel["indexerId"] == float64(0) {
		t.Errorf("release: %v", rel)
	}
	if rel["rejections"] == nil || rel["guid"] == "" || rel["publishDate"] == "" {
		t.Errorf("release: %v", rel)
	}
	if byTitle[sPack]["fullSeason"] != true {
		t.Errorf("the pack should be a full season: %v", byTitle[sPack])
	}

	// A season: only the pack.
	season := list(arrCall(t, "GET", sonarr+"/release?seriesId="+fmt.Sprint(env.seriesID)+"&seasonNumber=1", key, nil, http.StatusOK))
	if len(season) != 1 || obj(season[0])["title"] != sPack {
		t.Fatalf("season releases: %v", season)
	}
	// Nothing named: an empty array, never null; an unknown episode is not found.
	if none := arrCall(t, "GET", sonarr+"/release", key, nil, http.StatusOK); none == nil || len(list(none)) != 0 {
		t.Fatalf("no target: %#v", none)
	}
	arrCall(t, "GET", sonarr+"/release?episodeId=999999", key, nil, http.StatusNotFound)

	// Radarr, one movie.
	mrels := list(arrCall(t, "GET", fmt.Sprintf("%s/release?movieId=%d", radarr, movie.ID), key, nil, http.StatusOK))
	if len(mrels) != 1 || obj(mrels[0])["title"] != movieBlu {
		t.Fatalf("movie releases: %v", mrels)
	}
	m := obj(mrels[0])
	if mq := obj(obj(m["quality"])["quality"]); mq["id"] != float64(7) || mq["name"] != "Bluray-1080p" || m["indexerFlags"] == nil {
		t.Errorf("movie release: %v", m)
	}
	if _, has := m["fullSeason"]; has {
		t.Errorf("a movie release should not carry TV fields: %v", m)
	}

	// Grab by guid: the episode release goes to the queue.
	grabbed := obj(arrCall(t, "POST", sonarr+"/release", key, map[string]any{"guid": rel["guid"], "indexerId": rel["indexerId"]}, http.StatusOK))
	if grabbed["title"] != s1e1WEB {
		t.Fatalf("grab answer: %v", grabbed)
	}
	if got := env.grabbedTitles(t); len(got) != 1 || got[0] != s1e1WEB {
		t.Fatalf("queued: %v", got)
	}
	// A movie's release the same way (the answer is the release, as in Radarr).
	arrCall(t, "POST", radarr+"/release", key, map[string]any{"guid": m["guid"], "indexerId": m["indexerId"]}, http.StatusOK)
	if got := env.grabbedTitles(t); len(got) != 2 || got[1] != movieBlu {
		t.Fatalf("queued after the movie: %v", got)
	}
	// A guid nobody listed is not found.
	arrCall(t, "POST", sonarr+"/release", key, map[string]any{"guid": "rel_nope", "indexerId": 1}, http.StatusNotFound)
	arrCall(t, "POST", sonarr+"/release", key, map[string]any{}, http.StatusBadRequest)

	// The lists the apps load next to it, and an unknown address.
	if ix := list(arrCall(t, "GET", radarr+"/indexer", key, nil, http.StatusOK)); len(ix) != 1 || obj(ix[0])["name"] != "TV Indexer" || obj(ix[0])["protocol"] != "usenet" {
		t.Fatalf("indexers: %v", ix)
	}
	if dc := list(arrCall(t, "GET", radarr+"/downloadclient", key, nil, http.StatusOK)); len(dc) != 1 {
		t.Fatalf("download clients: %v", dc)
	}
	if nf := obj(arrCall(t, "GET", radarr+"/somethingelse", key, nil, http.StatusNotFound)); nf["message"] == nil {
		t.Fatalf("unknown address should answer in JSON: %v", nf)
	}
	env.finish(t)
}
