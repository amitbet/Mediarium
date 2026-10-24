package api_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/settings"
)

// A file that was changed a moment ago may still be being written; the job
// leaves it for a later run, and never reads a title that is not downloaded.
func TestFileDetailsSkipFilesStillChanging(t *testing.T) {
	e := newMCPEnv(t)
	s := e.server
	movies := filepath.Join(t.TempDir(), "movies")
	if err := s.Settings.Set(settings.KeyMoviesPath, movies, false); err != nil {
		t.Fatal(err)
	}
	body := string(tinyMKV(mkvTrack(1, "V_VP9", "eng", videoElement(1280, 720, 0))))
	fresh := filepath.Join(movies, "Fresh (2024)", "Fresh.2024.mkv")
	writeBody(t, fresh, body)
	old := filepath.Join(movies, "Old (2000)", "Old.2000.mkv")
	writeBody(t, old, body)
	waiting := filepath.Join(movies, "Waiting (2025)", "Waiting.2025.mkv")
	writeBody(t, waiting, body)
	longAgo := time.Now().Add(-time.Hour)
	for _, p := range []string{old, waiting} {
		if err := os.Chtimes(p, longAgo, longAgo); err != nil {
			t.Fatal(err)
		}
	}

	for i, p := range []string{fresh, old} {
		m, err := s.MovieRepo.Add(library.Movie{TMDBID: 900 + i, Title: fmt.Sprint("Film ", i), Year: 2000})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, "", p); err != nil {
			t.Fatal(err)
		}
	}
	// A title still waiting for its download is not looked at, whatever lies at its path.
	m, _ := s.MovieRepo.Add(library.Movie{TMDBID: 950, Title: "Waiting", Year: 2025})
	_ = s.MovieRepo.SetStatus(m.ID, library.StatusDownloading, "", waiting)

	s.RunMediaInfoJob(context.Background())

	if _, ok, _ := s.MovieRepo.GetMediaInfo(fresh); ok {
		t.Error("a file changed a moment ago should be left for later")
	}
	if _, ok, _ := s.MovieRepo.GetMediaInfo(old); !ok {
		t.Error("an old file should be read")
	}
	if _, ok, _ := s.MovieRepo.GetMediaInfo(waiting); ok {
		t.Error("a movie that is not downloaded should not be read")
	}
}
