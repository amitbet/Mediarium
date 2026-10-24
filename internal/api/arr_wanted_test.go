package api_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
)

const (
	movieHDR    = "Night.Harbour.2021.1080p.BluRay.HDR.x264-GRP"
	movieRepack = "Night.Harbour.2021.Extended.1080p.BluRay.REPACK.x264-GRP"
)

// What an app like nzb360 shows next to each release: the custom formats the
// profile gave a score to, their sum, the revision, the edition, the reasons
// and the order (best first).
func TestArrReleaseCarriesFormatsAndScore(t *testing.T) {
	env := newTVAutoEnv(t, []string{movieBlu, movieHDR, movieRepack}, nil)
	pid := profileIDByName(t, getJSON[map[string]any](t, env.client, env.baseURL+"/api/quality-profiles"), "1080p")
	movie, err := env.server.MovieRepo.Add(library.Movie{TMDBID: 701, Title: "Night Harbour", Year: 2021, Monitored: true, ProfileID: pid})
	if err != nil {
		t.Fatal(err)
	}
	postJSON[map[string]any](t, env.client, env.baseURL+"/api/custom-formats", map[string]any{"json": fixture(t, "radarr-hdr.json"), "app": "radarr"}, http.StatusOK)
	formats := getJSON[map[string]any](t, env.client, env.baseURL+"/api/custom-formats")["formats"].([]any)
	hdr := formatIDByName(t, formats, "HDR")
	putJSONStatus(t, env.client, env.baseURL+"/api/quality-profiles/"+itoa(pid), map[string]any{
		"name": "1080p", "allowed": []string{"WEBDL-1080p", "Bluray-1080p"}, "cutoff": "Bluray-1080p", "upgradeAllowed": true,
		"formatScores": []map[string]any{{"formatId": hdr, "score": 500}}, "minFormatScore": 0, "cutoffFormatScore": 1000,
	}, http.StatusOK)
	key := postJSON[map[string]any](t, env.client, env.baseURL+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	radarr := env.baseURL + "/radarr/api/v3"

	rels := list(arrCall(t, "GET", fmt.Sprintf("%s/release?movieId=%d", radarr, movie.ID), key, nil, http.StatusOK))
	by := map[string]map[string]any{}
	for _, r := range rels {
		by[obj(r)["title"].(string)] = obj(r)
	}
	if len(rels) != 3 || by[movieHDR] == nil {
		t.Fatalf("releases: %v", rels)
	}
	h := by[movieHDR]
	cf := list(h["customFormats"])
	if len(cf) != 1 || obj(cf[0])["name"] != "HDR" || obj(cf[0])["id"] != hdr || h["customFormatScore"] != float64(500) {
		t.Errorf("custom formats: %v score %v", h["customFormats"], h["customFormatScore"])
	}
	if by[movieBlu]["customFormatScore"] != float64(0) || len(list(by[movieBlu]["customFormats"])) != 0 {
		t.Errorf("a release without a matched format: %v", by[movieBlu])
	}
	rp := by[movieRepack]
	if rv := obj(obj(rp["quality"])["revision"]); rv["version"] != float64(2) || rv["isRepack"] != true {
		t.Errorf("revision: %v", rp["quality"])
	}
	if rp["edition"] != "Extended" {
		t.Errorf("edition: %v", rp["edition"])
	}
	if h["approved"] == h["rejected"] || (h["rejected"] == true) != (len(list(h["rejections"])) > 0) {
		t.Errorf("approved, rejected and rejections disagree: %v", h)
	}
	for _, f := range []string{"qualityWeight", "ageHours", "ageMinutes", "age", "languages", "indexerFlags", "seeders", "leechers"} {
		if _, ok := h[f]; !ok {
			t.Errorf("no %q: %v", f, h)
		}
	}
	// The best match first: the HDR one scores 500 against 0.
	if obj(rels[0])["title"] != movieHDR {
		t.Errorf("best first: %v", obj(rels[0])["title"])
	}
	env.finish(t)
}

// The paged lists and settings pages the apps ask for next to the releases
// answer in the v3 shape.
func TestArrWantedCutoffAndSettingsShapes(t *testing.T) {
	env := newTVAutoEnv(t, []string{s1e1WEB}, []episodeSpec{{1, 1, "2020-01-01", library.StatusDownloaded, "SDTV"}, {1, 2, "2020-01-08", library.StatusMissing, ""}})
	file := filepath.Join(t.TempDir(), "Night.Harbour.2021.mkv")
	if err := os.WriteFile(file, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := env.server.MovieRepo.Add(library.Movie{TMDBID: 701, Title: "Night Harbour", Year: 2021, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.server.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, "SDTV", file); err != nil {
		t.Fatal(err)
	}
	key := postJSON[map[string]any](t, env.client, env.baseURL+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	radarr, sonarr := env.baseURL+"/radarr/api/v3", env.baseURL+"/sonarr/api/v3"

	envelope := func(url string) map[string]any {
		p := obj(arrCall(t, "GET", url, key, nil, http.StatusOK))
		for _, f := range []string{"page", "pageSize", "sortKey", "sortDirection", "totalRecords", "records"} {
			if _, ok := p[f]; !ok {
				t.Errorf("%s: no %q in %v", url, f, p)
			}
		}
		if _, ok := p["records"].([]any); !ok {
			t.Errorf("%s: records is not a list: %v", url, p["records"])
		}
		return p
	}
	cut := envelope(radarr + "/wanted/cutoff?sortDirection=ascending&monitored=true&page=1&pageSize=20&sortKey=movieMetadata.sortTitle")
	if cut["totalRecords"] != float64(1) || obj(list(cut["records"])[0])["title"] != "Night Harbour" || cut["sortKey"] != "movieMetadata.sortTitle" {
		t.Errorf("radarr cutoff: %v", cut)
	}
	scut := envelope(sonarr + "/wanted/cutoff?page=1&pageSize=20")
	if scut["totalRecords"] != float64(1) || obj(obj(list(scut["records"])[0])["series"])["title"] == nil {
		t.Errorf("sonarr cutoff: %v", scut)
	}
	if miss := envelope(radarr + "/wanted/missing?page=1&pageSize=20"); miss["totalRecords"] != float64(0) {
		t.Errorf("radarr missing: %v", miss)
	}
	envelope(sonarr + "/wanted/missing?page=1&pageSize=20")
	envelope(radarr + "/log?page=1&pageSize=20")

	for _, p := range []string{"/customfilter", "/remotepathmapping", "/notification", "/importlist", "/metadata", "/update", "/system/task", "/tag/detail", "/customformat", "/health", "/tag", "/language", "/languageprofile", "/qualitydefinition", "/diskspace"} {
		if _, ok := arrCall(t, "GET", radarr+p, key, nil, http.StatusOK).([]any); !ok {
			t.Errorf("%s should answer a list", p)
		}
	}
	for _, p := range []string{"/ui/config", "/config/ui", "/config/host"} {
		if o := obj(arrCall(t, "GET", sonarr+p, key, nil, http.StatusOK)); o["id"] == nil {
			t.Errorf("%s: %v", p, o)
		}
	}
	if o := obj(arrCall(t, "GET", radarr+"/config/host", key, nil, http.StatusOK)); o["apiKey"] != "" {
		t.Errorf("the host settings must not hand out a key: %v", o["apiKey"])
	}
	push := list(arrCall(t, "POST", radarr+"/release/push", key, map[string]any{"title": movieBlu, "downloadUrl": "http://x/y.nzb", "protocol": "usenet"}, http.StatusOK))
	if len(push) != 1 || obj(push[0])["rejected"] != true {
		t.Errorf("push: %v", push)
	}
	env.finish(t)
}
