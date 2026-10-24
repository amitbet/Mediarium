package api

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/rdborg/mediarium/internal/libimport"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/mediaservers"
	"github.com/rdborg/mediarium/internal/organizer"
	"github.com/rdborg/mediarium/internal/parser"
	"github.com/rdborg/mediarium/internal/quality"
)

// Apps that change a file after the import (Tdarr and the like) tell Radarr
// or Sonarr to look again with a "rescan" or "refresh" command. Mediarium does
// the same: it checks that each downloaded file is still where it was (and
// finds it again when only the extension changed, for example .mp4 to .mkv),
// and asks the media servers to scan the folder, so the new file shows up
// there. It never deletes anything, and only the Rescan files button (never an
// app) marks a file that is gone as missing.

// stillThere returns where the file is now: the same path when it is still
// there, or the one video in its folder that has the same name apart from the
// extension. gone is true only when the file surely isn't there any more; when
// it can't be told (the disk can't be read, or two look-alikes are there) the
// answer is "" and gone is false, and the title is left alone.
func stillThere(path string) (now string, gone bool) {
	if path == "" {
		return "", false
	}
	_, err := os.Stat(path)
	if err == nil {
		return path, false
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	dir := filepath.Dir(path)
	want := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", errors.Is(err, fs.ErrNotExist)
	}
	var found []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !organizer.IsVideoFile(name) || organizer.IsSample(name) {
			continue
		}
		if strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name))) == want {
			found = append(found, filepath.Join(dir, name))
		}
	}
	switch len(found) {
	case 0:
		return "", true
	case 1:
		return found[0], false
	}
	return "", false
}

// rescanReport says what a rescan found.
type rescanReport struct {
	Found   int `json:"found"`   // files that are where they should be
	Renamed int `json:"renamed"` // files found again under another extension
	Missing int `json:"missing"` // files that are gone (now wanted again when markMissing was asked for)
	Skipped int `json:"skipped"` // files left alone because the library folder looks unavailable or it couldn't be told whether they are there
	// The next three are for a movie that had no file when Rescan files was
	// pressed: the name of the file it was given, the folder that was looked in
	// when there was nothing to give, and whether a running download made
	// Mediarium leave the movie alone.
	Added  string `json:"added,omitempty"`
	Looked string `json:"looked,omitempty"`
	Busy   bool   `json:"busy,omitempty"`
	// For a show: how many episodes took a video file put in its folder by
	// hand, and the names (at most ten; NotMatchedTotal says how many there
	// are) of the video files there that matched no episode.
	AddedEpisodes   int      `json:"addedEpisodes,omitempty"`
	NotMatched      []string `json:"notMatched,omitempty"`
	NotMatchedTotal int      `json:"notMatchedTotal,omitempty"`
}

// libraryAvailable says the library folder is there and has something in it. A
// drive that isn't plugged in leaves an empty folder (or none), and a rescan
// must never read that as "every file is gone".
func libraryAvailable(root string) bool {
	if root == "" {
		return false
	}
	entries, err := os.ReadDir(root)
	return err == nil && len(entries) > 0
}

// rescanMovieFiles looks at one movie's file again. With markMissing, a file
// that is gone (and has no look-alike) makes the movie wanted again; the apps
// that ask by themselves (Tdarr) never get that, only a person pressing the button.
func (s *Server) rescanMovieFiles(id int64, markMissing bool) rescanReport {
	var rep rescanReport
	defer s.refreshMovieMediaInfo(id) // a changed file (Tdarr and the like) is read again
	m, err := s.MovieRepo.Get(id)
	if err != nil || m.Status != library.StatusDownloaded || m.FilePath == "" {
		return rep
	}
	now, gone := stillThere(m.FilePath)
	if now == "" {
		if !gone {
			slog.Warn("rescan: couldn't tell whether a movie's file is still there, so it was left alone", "movie", m.Title, "path", m.FilePath)
			if markMissing {
				rep.Skipped++
			}
			return rep
		}
		if !markMissing {
			slog.Warn("rescan: a movie's file is not where it was and no file with the same name was found", "movie", m.Title, "path", m.FilePath)
			return rep
		}
		if !libraryAvailable(s.fileRoot(s.movieRootFor(m), m.FilePath)) {
			rep.Skipped++
			return rep
		}
		if err := s.MovieRepo.ClearMovieFile(id); err == nil {
			rep.Missing++
			_ = s.QueueRepo.LogActivity(id, "removed", fmt.Sprintf("%s: its file %s is not there any more, so the movie is wanted again.", movieLabel(m), filepath.Base(m.FilePath)))
		}
		return rep
	}
	if now != m.FilePath {
		if err := s.MovieRepo.SetStatus(id, m.Status, "", now); err != nil {
			slog.Warn("rescan: couldn't save a movie's new file name", "movie", m.Title, "err", err)
			return rep
		}
		slog.Info("rescan: a movie's file has a new name", "movie", m.Title, "from", filepath.Base(m.FilePath), "to", filepath.Base(now))
		rep.Renamed++
	} else {
		rep.Found++
	}
	s.mediaServersImported(mediaservers.MediaMovie, now)
	return rep
}

// movieWantsAFile is true for a movie that has no file and isn't being
// downloaded: the Rescan files button then looks in the movie's folder for a
// file somebody put there by hand.
func movieWantsAFile(m library.Movie) bool {
	return m.FilePath == "" && m.Status != library.StatusDownloading
}

// errNoCopiesFolder is a second copy of a movie when no folder is set for them.
var errNoCopiesFolder = errors.New("no folder for second copies is set")

// attachMovieFile gives a movie that has no file the video file lying in its
// folder: the folder it would be imported into, or the one folder in the
// library that is named like the movie. The biggest video file there (extras,
// samples and trailers don't count, and neither does a file another movie
// already has) becomes the movie's file where it lies. Nothing is moved,
// renamed or deleted. The error is only for a file that was found but couldn't
// be saved to the library.
func (s *Server) attachMovieFile(m library.Movie) (rescanReport, error) {
	var rep rescanReport
	var busy alreadyDownloadingError
	if err := s.checkMovieNotDownloading(m.ID); errors.As(err, &busy) {
		rep.Busy = true
		return rep, nil
	}
	if m.CopyOf != 0 && s.movieCopiesRoot() == "" {
		return rep, errNoCopiesFolder // there is nowhere to look: not the main title's folder
	}
	folder := s.movieFolderFor(m)
	if folder == "" {
		rep.Looked = s.movieRootFor(m)
		return rep, nil
	}
	return s.attachBiggestVideo(m, folder, rep)
}

// attachBiggestVideo is the end of attachMovieFile: it gives the movie the
// biggest video in folder that no other title has, and says in rep where it
// looked when there was none.
func (s *Server) attachBiggestVideo(m library.Movie, folder string, rep rescanReport) (rescanReport, error) {
	rep.Looked = folder
	files, err := libimport.ScanMovieFolder(folder)
	if err != nil {
		slog.Info("rescan: couldn't read a movie's folder", "movie", m.Title, "folder", folder, "err", err)
		return rep, nil
	}
	paths := make([]string, len(files))
	for i := range files {
		paths[i] = files[i].Path
	}
	// not a file another movie has, nor one that is an episode of a show
	taken := s.filesOfOtherTitles(paths, 0, m.ID)
	var best *libimport.File
	for i := range files {
		if taken[filepath.Clean(files[i].Path)] || !s.insideLibrary(files[i].Path) {
			continue
		}
		if best == nil || files[i].SizeBytes > best.SizeBytes {
			best = &files[i]
		}
	}
	if best == nil {
		return rep, nil
	}
	// Another file is read again: its own name says its quality, and when it
	// says nothing the movie's quality is Not stated, not the old file's.
	tier := best.Quality
	if tier == string(quality.TierUnknown) && sameFile(best.Path, m.FilePath) {
		tier = ""
	}
	if err := s.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, tier, best.Path); err != nil {
		return rep, fmt.Errorf("add the file %s to %s: %w", filepath.Base(best.Path), m.Title, err)
	}
	slog.Info("rescan: a movie got the file that was in its folder", "movie", m.Title, "file", filepath.Base(best.Path))
	_ = s.QueueRepo.LogActivity(m.ID, "imported", fmt.Sprintf("%s: found the file %s in its folder and added it.", movieLabel(m), filepath.Base(best.Path)))
	s.mediaServersImported(mediaservers.MediaMovie, best.Path)
	rep.Added, rep.Looked = filepath.Base(best.Path), ""
	return rep, nil
}

// similarMovieFolder is the one folder directly in root whose name reads as
// the movie ("The Matrix (1999)", "the.matrix.1999"), or "" when there is none
// or there are two, which is a guess Mediarium doesn't make. A movie that has
// a year only takes a folder that names the same year: "Dune" alone could be
// either film.
func similarMovieFolder(root string, m library.Movie) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	found := ""
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || !matchesMovie(e.Name(), m) {
			continue
		}
		if m.Year != 0 && parser.Parse(e.Name()).Year != m.Year {
			continue
		}
		if found != "" {
			return ""
		}
		found = filepath.Join(root, e.Name())
	}
	return found
}

// rescanSeriesFiles does the same for every downloaded episode of a show.
func (s *Server) rescanSeriesFiles(id int64, markMissing bool) rescanReport {
	var rep rescanReport
	defer s.refreshSeriesMediaInfo(id)
	eps, err := s.MovieRepo.ListEpisodes(id)
	if err != nil {
		return rep
	}
	series, serr := s.MovieRepo.GetSeries(id)
	var files []string
	for _, e := range eps {
		if e.Status != library.StatusDownloaded || e.FilePath == "" {
			continue
		}
		now, gone := stillThere(e.FilePath)
		if now == "" {
			if !gone {
				slog.Warn("rescan: couldn't tell whether an episode's file is still there, so it was left alone", "series", id, "season", e.Season, "episode", e.Episode, "path", e.FilePath)
				if markMissing {
					rep.Skipped++
				}
				continue
			}
			if !markMissing {
				slog.Warn("rescan: an episode's file is not where it was and no file with the same name was found", "series", id, "season", e.Season, "episode", e.Episode, "path", e.FilePath)
				continue
			}
			if serr != nil || !libraryAvailable(s.fileRoot(s.seriesRootFor(series), e.FilePath)) {
				rep.Skipped++
				continue
			}
			if err := s.MovieRepo.ClearEpisodeFile(e.ID); err == nil {
				rep.Missing++
			}
			continue
		}
		if now != e.FilePath {
			if err := s.MovieRepo.SetEpisodeStatus(e.ID, e.Status, "", now); err != nil {
				slog.Warn("rescan: couldn't save an episode's new file name", "series", id, "err", err)
				continue
			}
			slog.Info("rescan: an episode's file has a new name", "series", id, "season", e.Season, "episode", e.Episode, "from", filepath.Base(e.FilePath), "to", filepath.Base(now))
			rep.Renamed++
		} else {
			rep.Found++
		}
		files = append(files, now)
	}
	if rep.Missing > 0 && serr == nil {
		_ = s.QueueRepo.LogSeriesActivity(id, "removed", fmt.Sprintf("%s: %d episode file(s) are not on the disk any more, so those episodes are wanted again.", seriesLabel(series), rep.Missing))
	}
	if len(files) > 0 {
		s.mediaServersImported(mediaservers.MediaTV, files...)
	}
	return rep
}
