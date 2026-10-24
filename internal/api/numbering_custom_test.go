package api_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
)

func TestCustomNumberingSavesAndValidates(t *testing.T) {
	env := newTVAutoEnv(t, nil, nil)
	id := addTitanShow(t, env)
	url := fmt.Sprintf("%s/api/series/%d/numbering/custom", env.baseURL, id)

	empty := getJSON[map[string]any](t, env.client, url)
	if len(empty["rules"].([]any)) != 0 || len(empty["names"].([]any)) != 0 {
		t.Fatalf("nothing set yet: %v", empty)
	}

	good := map[string]any{
		"rules": []map[string]any{{"librarySeason": 2, "releaseSeason": 15, "offset": 0}, {"librarySeason": 1, "releaseSeason": 14}},
		"names": []string{"  Shingeki  no Kyojin ", "shingeki no kyojin", "Attack Titan"},
	}
	saved := putJSONStatus(t, env.client, url, good, http.StatusOK)
	rules := saved["rules"].([]any)
	if len(rules) != 2 || rules[0].(map[string]any)["librarySeason"] != float64(1) {
		t.Fatalf("rules should come back in order: %v", rules)
	}
	if names := saved["names"].([]any); len(names) != 2 || names[0] != "Shingeki no Kyojin" {
		t.Fatalf("names should be tidied: %v", names)
	}
	if again := getJSON[map[string]any](t, env.client, url); len(again["rules"].([]any)) != 2 {
		t.Fatalf("not stored: %v", again)
	}

	for name, body := range map[string]map[string]any{
		"a season the show lacks":  {"rules": []map[string]any{{"librarySeason": 9, "releaseSeason": 1}}},
		"overlapping rules":        {"rules": []map[string]any{{"librarySeason": 1, "releaseSeason": 5}, {"librarySeason": 1, "releaseSeason": 6}}},
		"a name that is too short": {"names": []string{"ab"}},
	} {
		got := putJSONStatus(t, env.client, url, body, http.StatusBadRequest)
		if msg, _ := got["error"].(string); msg == "" {
			t.Fatalf("%s: no reason given: %v", name, got)
		}
	}
	// A refused request changes nothing.
	if again := getJSON[map[string]any](t, env.client, url); len(again["rules"].([]any)) != 2 {
		t.Fatalf("a refused request changed the rules: %v", again)
	}

	// An empty body clears.
	putJSONStatus(t, env.client, url, map[string]any{"rules": []any{}, "names": []any{}}, http.StatusOK)
	if cleared := getJSON[map[string]any](t, env.client, url); len(cleared["rules"].([]any)) != 0 || len(cleared["names"].([]any)) != 0 {
		t.Fatalf("not cleared: %v", cleared)
	}
	putJSONStatus(t, env.client, fmt.Sprintf("%s/api/series/9999/numbering/custom", env.baseURL), good, http.StatusNotFound)
}

// A show TMDB lists as season 1 while the releases call it season 15 under
// another name: nothing is found until the rule and the name are saved.
func TestSeriesSearchUsesTheOwnNumberingAndNames(t *testing.T) {
	titles := []string{
		"The.Bachelor.Ukraine.S15E01.1080p.WEB-DL.x264-GRP",
		"The.Bachelor.Ukraine.S15E02.1080p.WEB-DL.x264-GRP",
		"The.Bachelor.Ukraine.S15.1080p.WEB-DL.x264-GRP",
		"Another.Show.S15E01.1080p.WEB-DL.x264-GRP",
	}
	env := newTVAutoEnv(t, titles, nil)
	var eps []library.Episode
	for e := 1; e <= 8; e++ {
		eps = append(eps, library.Episode{Season: 1, Episode: e, AirDate: "2026-01-01"})
	}
	sr, err := env.server.MovieRepo.AddSeries(library.Series{TMDBID: 777, Title: "Холостяк", Year: 2026, Monitored: true}, eps)
	if err != nil {
		t.Fatal(err)
	}
	search := func(season, episode int) map[string]bool {
		u := fmt.Sprintf("%s/api/series/%d/search?season=%d", env.baseURL, sr.ID, season)
		if episode > 0 {
			u += fmt.Sprintf("&episode=%d", episode)
		}
		return resultTitles(getJSON[[]map[string]any](t, env.client, u))
	}
	if got := search(1, 1); len(got) != 0 {
		t.Fatalf("before anything is set: %v", got)
	}

	putJSONStatus(t, env.client, fmt.Sprintf("%s/api/series/%d/numbering/custom", env.baseURL, sr.ID), map[string]any{
		"rules": []map[string]any{{"librarySeason": 1, "releaseSeason": 15}},
	}, http.StatusOK)
	if got := search(1, 1); len(got) != 0 {
		t.Fatalf("the rule alone does not make the title match: %v", got)
	}

	putJSONStatus(t, env.client, fmt.Sprintf("%s/api/series/%d/numbering/custom", env.baseURL, sr.ID), map[string]any{
		"rules": []map[string]any{{"librarySeason": 1, "releaseSeason": 15}},
		"names": []string{"The Bachelor Ukraine"},
	}, http.StatusOK)
	got := search(1, 1)
	if !got[titles[0]] || got[titles[1]] || got[titles[3]] || !got[titles[2]] {
		t.Fatalf("library S01E01 is release S15E01 (and the season pack covers it): %v", got)
	}
	if got := search(1, 0); !got[titles[2]] || got[titles[0]] {
		t.Fatalf("the season search finds the S15 pack: %v", got)
	}
}

// Rescan files finds an S15E01 file in the show's folder through the rule.
func TestRescanFindsFilesNamedByTheOwnNumbering(t *testing.T) {
	l := newMetaLibrary(t)
	showID := int64(getJSON[[]map[string]any](t, l.client, l.base+"/api/series")[0]["id"].(float64))
	if err := l.server.TestExec(`UPDATE episodes SET status = 'missing', file_path = NULL, monitored = 1 WHERE series_id = ? AND season = 2 AND episode = 1`, showID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(l.ep2); err != nil {
		t.Fatal(err)
	}
	show := filepath.Join(l.tv, "Fixture Show")
	putFile(t, filepath.Join(show, "S15E01.mkv"), "aaa")
	rescan := func() map[string]any {
		return postJSON[map[string]any](t, l.client, fmt.Sprintf("%s/api/series/%d/rescan", l.base, showID), nil, http.StatusOK)
	}
	got := rescan()
	if added, _ := got["addedEpisodes"].(float64); added != 0 {
		t.Fatalf("without a rule the file is nobody's: %v", got)
	}
	if nm, _ := got["notMatched"].([]any); len(nm) != 1 || nm[0] != "S15E01.mkv" {
		t.Fatalf("notMatched: %v", got["notMatched"])
	}

	putJSONStatus(t, l.client, fmt.Sprintf("%s/api/series/%d/numbering/custom", l.base, showID), map[string]any{
		"rules": []map[string]any{{"librarySeason": 2, "releaseSeason": 15}},
	}, http.StatusOK)
	got = rescan()
	if added, _ := got["addedEpisodes"].(float64); added != 1 {
		t.Fatalf("with the rule the file is library S02E01: %v", got)
	}
	ep, err := l.server.MovieRepo.GetEpisode(showID, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(show, "S15E01.mkv"); ep.FilePath != want || ep.Status != library.StatusDownloaded {
		t.Fatalf("S02E01 = %q (%s), want %q", ep.FilePath, ep.Status, want)
	}
}
