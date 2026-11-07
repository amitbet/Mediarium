package api_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
)

// The same for shows: a release picked by hand replaces the files of the
// episodes it covers (only those that have a file), and only when the person
// agreed. A season pack replaces the episodes that have files and fills the
// others as usual.

func (e *tvEnv) grabWith(t *testing.T, releaseTitle string, replace bool) {
	t.Helper()
	body := map[string]any{"releaseTitle": releaseTitle, "downloadUrl": e.nzbURL, "sizeBytes": 1000}
	if replace {
		body["replace"] = true
	}
	postJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/series/%d/grab", e.httpSrv.URL, e.seriesID), body, http.StatusAccepted)
}

// giveFile makes the episode downloaded with a file of old content at the
// path the naming preset would give a new one (or at name, in the same folder).
func (e *tvEnv) giveFile(t *testing.T, season, episode int, name string) string {
	t.Helper()
	ep, err := e.server.MovieRepo.GetEpisode(e.seriesID, season, episode)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.tvRoot, "Fixture Show (2011)", fmt.Sprintf("Season %02d", season), name)
	putFile(t, path, hpOldContent+" "+name)
	if err := e.server.MovieRepo.SetEpisodeStatus(ep.ID, library.StatusDownloaded, "HDTV-720p", path); err != nil {
		t.Fatal(err)
	}
	return path
}

func (e *tvEnv) tvBin() map[string]string {
	return binFilesNoT(filepath.Join(e.tvRoot, ".mediarium-trash"))
}

func binFilesNoT(bin string) map[string]string {
	out := map[string]string{}
	_ = filepath.Walk(bin, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() != "entry.json" {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(bin, p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

func (e *tvEnv) queueItem(t *testing.T) map[string]any {
	t.Helper()
	list := getJSON[[]map[string]any](t, e.client, e.httpSrv.URL+"/api/queue")
	if len(list) != 1 {
		t.Fatalf("expected one queue item, got %d", len(list))
	}
	return list[0]
}

func TestHandPickedSingleEpisodeReplacesThatEpisodesFile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		old     string // the old file's name; the new one is "Fixture Show - S01E01 - Pilot.mkv"
		replace bool
	}{
		{"same name, picked by hand", "Fixture Show - S01E01 - Pilot.mkv", true},
		{"other name, picked by hand", "Fixture Show - S01E01 - Pilot [720p].mp4", true},
		{"not asked: the file you have is kept", "Fixture Show - S01E01 - Pilot.mkv", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := []byte("episode one, the new version " + strings.Repeat("n", 400))
			env := newTVEnv(t, map[string]nntpArticle{"e1@example": {fileName: "release.mkv", content: content}})
			if err := env.server.MovieRepo.SetSeriesNoUpgrade(env.seriesID, true); err != nil {
				t.Fatal(err)
			}
			old := env.giveFile(t, 1, 1, tc.old)
			other := env.giveFile(t, 1, 2, "Fixture Show - S01E02 - Second.mkv") // another episode's file
			newPath := filepath.Join(env.tvRoot, "Fixture Show (2011)", "Season 01", "Fixture Show - S01E01 - Pilot.mkv")

			env.grabWith(t, "Fixture.Show.S01E01.1080p.WEB-DL.x264-GRP", tc.replace)
			status, errMsg := env.waitForTerminal(t)
			if status != "completed" {
				t.Fatalf("the grab ended %s: %s", status, errMsg)
			}
			if got, _ := os.ReadFile(other); !strings.HasPrefix(string(got), hpOldContent) {
				t.Fatalf("the file of another episode changed: %q", got)
			}
			if tc.replace {
				if errMsg != "" {
					t.Errorf("a replaced file is a normal import: %q", errMsg)
				}
				if got, _ := os.ReadFile(newPath); string(got) != string(content) {
					t.Fatalf("the new file is not in place: %q", got)
				}
				if tc.old != filepath.Base(newPath) && exists(old) {
					t.Errorf("the old file under its other name is still there")
				}
				found := false
				for _, c := range env.tvBin() {
					found = found || strings.HasPrefix(c, hpOldContent)
				}
				if !found {
					t.Errorf("the old file should be in the recycle bin: %v", env.tvBin())
				}
				return
			}
			if errMsg != "Kept the file you already have." {
				t.Errorf("expected the file to be kept, got %q", errMsg)
			}
			it := env.queueItem(t)
			if it["keptFile"] != old || it["keptData"] != "deleted" {
				t.Errorf("the row should name the kept file and say the data was deleted: %v / %v", it["keptFile"], it["keptData"])
			}
			if got, _ := os.ReadFile(old); !strings.HasPrefix(string(got), hpOldContent) {
				t.Errorf("the file you have changed: %q", got)
			}
			if len(env.tvBin()) != 0 {
				t.Errorf("nothing should be binned: %v", env.tvBin())
			}
		})
	}
}

func TestHandPickedSeasonPackReplacesEpisodesThatHaveFilesAndFillsTheRest(t *testing.T) {
	for _, replace := range []bool{true, false} {
		t.Run(fmt.Sprintf("replace=%v", replace), func(t *testing.T) {
			e1 := []byte("episode one, new " + strings.Repeat("a", 400))
			e2 := []byte("episode two, new " + strings.Repeat("b", 400))
			env := newTVEnv(t, map[string]nntpArticle{
				"e1@example": {fileName: "Fixture.Show.S01E01.1080p.BluRay.x264-GRP.mkv", content: e1},
				"e2@example": {fileName: "Fixture.Show.S01E02.1080p.BluRay.x264-GRP.mkv", content: e2},
			})
			if err := env.server.MovieRepo.SetSeriesNoUpgrade(env.seriesID, true); err != nil {
				t.Fatal(err)
			}
			// Episode one has a file (under another name); episode two has none.
			old := env.giveFile(t, 1, 1, "Fixture Show - S01E01 - Pilot [720p].mp4")
			newOne := filepath.Join(env.tvRoot, "Fixture Show (2011)", "Season 01", "Fixture Show - S01E01 - Pilot.mkv")
			newTwo := filepath.Join(env.tvRoot, "Fixture Show (2011)", "Season 01", "Fixture Show - S01E02 - Second.mkv")

			env.grabWith(t, "Fixture.Show.S01.1080p.BluRay.x264-GRP", replace)
			status, errMsg := env.waitForTerminal(t)
			if status != "completed" || errMsg != "" {
				t.Fatalf("the pack ended %s: %q", status, errMsg)
			}
			if got, _ := os.ReadFile(newTwo); string(got) != string(e2) {
				t.Fatalf("the episode without a file is filled as usual: %q", got)
			}
			if replace {
				if got, _ := os.ReadFile(newOne); string(got) != string(e1) {
					t.Fatalf("episode one is replaced: %q", got)
				}
				if exists(old) {
					t.Errorf("the old file should be gone from the library")
				}
				found := false
				for _, c := range env.tvBin() {
					found = found || strings.HasPrefix(c, hpOldContent)
				}
				if !found {
					t.Errorf("the old file should be in the recycle bin: %v", env.tvBin())
				}
				return
			}
			if exists(newOne) {
				t.Errorf("without asking, episode one's release is not imported next to the file that stays")
			}
			if got, _ := os.ReadFile(old); !strings.HasPrefix(string(got), hpOldContent) {
				t.Errorf("the file you have changed: %q", got)
			}
			ep, _ := env.server.MovieRepo.GetEpisode(env.seriesID, 1, 1)
			if ep.FilePath != old {
				t.Errorf("episode one should keep pointing at %s, got %s", old, ep.FilePath)
			}
		})
	}
}

// A file that another episode has down is not replaced by a pack.
func TestHandPickedSeasonPackNeverReplacesAnotherEpisodesFile(t *testing.T) {
	e1 := []byte("episode one, new " + strings.Repeat("a", 400))
	env := newTVEnv(t, map[string]nntpArticle{
		"e1@example": {fileName: "Fixture.Show.S01E01.1080p.BluRay.x264-GRP.mkv", content: e1},
	})
	// Episode two's record points at the file episode one would be written to.
	target := env.giveFile(t, 1, 2, "Fixture Show - S01E01 - Pilot.mkv")
	env.grabWith(t, "Fixture.Show.S01.1080p.BluRay.x264-GRP", true)
	env.waitForTerminal(t)
	if got, _ := os.ReadFile(target); !strings.HasPrefix(string(got), hpOldContent) {
		t.Fatalf("the file of another episode was replaced: %q", got)
	}
	if len(env.tvBin()) != 0 {
		t.Errorf("nothing should be binned: %v", env.tvBin())
	}
}
