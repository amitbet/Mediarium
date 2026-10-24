package api

import (
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/parser"
	"github.com/rdborg/mediarium/internal/settings"
)

// The folder formats are for new titles. With no format set (or any style but
// Custom) the folders are named as they always were; with one set, a new
// title uses it, and a title that already has files keeps its folder.
func TestFolderFormatsDestPath(t *testing.T) {
	s := newAuditServer(t)
	movies, tv := filepath.Join(t.TempDir(), "movies"), filepath.Join(t.TempDir(), "tv")
	for k, v := range map[string]string{settings.KeyMoviesPath: movies, settings.KeyTVPath: tv, settings.KeyIllegalCharMode: "strip"} {
		if err := s.Settings.Set(k, v, false); err != nil {
			t.Fatal(err)
		}
	}
	set := func(preset, movie, series string) {
		t.Helper()
		for k, v := range map[string]string{settings.KeyNamingPreset: preset, settings.KeyMovieFolderFormat: movie, settings.KeySeriesFolderFormat: series} {
			if err := s.Settings.Set(k, v, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	pack := "Example.Show.S01.1080p.WEB-DL-GRP"
	rel := "Example.Movie.2024.1080p.BluRay-GRP"
	movieDest := func() string {
		got, err := s.buildDestPath("", "Example: Movie", 2024, 5, rel, "x.mkv")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	series := library.Series{Title: "Example Show", Year: 2023, TMDBID: 7}
	epDest := func(sr library.Series) string {
		got, err := s.buildTVDestPath(sr, library.Episode{Season: 1, Episode: 2, Title: "Two"}, parser.Parse(pack), pack, "x.mkv")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Run("nothing set: the usual folders", func(t *testing.T) {
		set("custom", "", "")
		if got, want := movieDest(), filepath.Join(movies, "Example Movie (2024)"); filepath.Dir(got) != want {
			t.Errorf("movie folder = %q, want %q", filepath.Dir(got), want)
		}
		if got, want := filepath.Dir(epDest(series)), filepath.Join(tv, "Example Show (2023)", "Season 01"); got != want {
			t.Errorf("season folder = %q, want %q", got, want)
		}
	})
	t.Run("another style ignores a saved format", func(t *testing.T) {
		set("plex", "{Movie CleanTitle} [tmdbid-{TmdbId}]", "{Series TitleYear} [tmdbid-{TmdbId}]")
		if got, want := filepath.Dir(movieDest()), filepath.Join(movies, "Example Movie (2024)"); got != want {
			t.Errorf("movie folder = %q, want %q", got, want)
		}
		if got, want := filepath.Dir(epDest(series)), filepath.Join(tv, "Example Show (2023)", "Season 01"); got != want {
			t.Errorf("season folder = %q, want %q", got, want)
		}
	})
	t.Run("custom formats name a new title", func(t *testing.T) {
		set("custom", "{Movie CleanTitle} ({Release Year}) [tmdbid-{TmdbId}]", "{Series TitleYear} [tmdbid-{TmdbId}]")
		if got, want := filepath.Dir(movieDest()), filepath.Join(movies, "Example Movie (2024) [tmdbid-5]"); got != want {
			t.Errorf("movie folder = %q, want %q", got, want)
		}
		if got, want := filepath.Dir(epDest(series)), filepath.Join(tv, "Example Show (2023) [tmdbid-7]", "Season 01"); got != want {
			t.Errorf("season folder = %q, want %q", got, want)
		}
	})
	t.Run("a movie with a file keeps its folder", func(t *testing.T) {
		set("custom", "{Movie CleanTitle} [tmdbid-{TmdbId}]", "")
		old := filepath.Join(movies, "Example Movie (2024)", "Example Movie (2024).mkv")
		m := library.Movie{Title: "Example: Movie", Year: 2024, TMDBID: 5, FilePath: old}
		got := s.keepMovieFolder(m, movies, movieDest())
		if want := filepath.Join(movies, "Example Movie (2024)", filepath.Base(movieDest())); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		m.FilePath = ""
		if got := s.keepMovieFolder(m, movies, movieDest()); got != movieDest() {
			t.Errorf("a movie with no file moved: %q", got)
		}
		// With the usual folder format nothing is changed, as before.
		set("custom", "", "")
		m.FilePath = filepath.Join(movies, "Somewhere Else", "x.mkv")
		if got := s.keepMovieFolder(m, movies, movieDest()); got != movieDest() {
			t.Errorf("default format changed the path: %q", got)
		}
	})
	t.Run("a show with files keeps its folder", func(t *testing.T) {
		set("custom", "", "{Series TitleYear} [tmdbid-{TmdbId}]")
		sr, err := s.MovieRepo.AddSeries(library.Series{TMDBID: 7, Title: "Example Show", Year: 2023, Monitored: true}, []library.Episode{{Season: 1, Episode: 1}, {Season: 1, Episode: 2}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := filepath.Dir(epDest(sr)), filepath.Join(tv, "Example Show (2023) [tmdbid-7]", "Season 01"); got != want {
			t.Errorf("new show: season folder = %q, want %q", got, want)
		}
		eps, _ := s.MovieRepo.ListEpisodes(sr.ID)
		old := filepath.Join(tv, "Example Show (2023)", "Season 01", "Example Show - S01E01 - One.mkv")
		if err := s.MovieRepo.SetEpisodeStatus(eps[0].ID, library.StatusDownloaded, "1080p", old); err != nil {
			t.Fatal(err)
		}
		if got, want := filepath.Dir(epDest(sr)), filepath.Join(tv, "Example Show (2023)", "Season 01"); got != want {
			t.Errorf("show with files: season folder = %q, want %q", got, want)
		}
	})
}
