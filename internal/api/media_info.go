package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/mediainfo"
	"github.com/rdborg/mediarium/internal/subtitles"
)

// What the library's video files hold (video codec, HDR, audio and subtitle
// tracks), read from the files' headers by internal/mediainfo and saved per
// file. It is read when a file is imported, when a title is rescanned, and by
// a slow background job for the files that have none yet; a file whose size
// and time are the same as last time is never read again.

const (
	// mediaInfoBatch is how many unread files one look at the database fetches.
	mediaInfoBatch = 100
	// mediaInfoRunFor ends one run of the background job, which carries on in
	// the next one: the reading is spread out, never one long burst.
	mediaInfoRunFor = 8 * time.Minute
	// mediaInfoInterval is the wait between runs.
	mediaInfoInterval = 10 * time.Minute
	// mediaInfoPause is the rest between two files, so a share isn't kept busy.
	mediaInfoPause = 40 * time.Millisecond
	// mediaInfoRecheckAfter is how long a saved file is trusted before the job
	// looks at its size and time again.
	mediaInfoRecheckAfter = 7 * 24 * time.Hour
	mediaInfoRecheckBatch = 300
	// mediaInfoKickDelay lets the rest of a season pack finish importing.
	mediaInfoKickDelay = 4 * time.Second
	mediaSummaryTTL    = 15 * time.Second
)

// mediaInfoSettle: a file changed more recently than this may still be being
// written, and is left for a later run.
var mediaInfoSettle = 2 * time.Minute

// mediaInfoState is the reading of file details; its zero value is ready.
type mediaInfoState struct {
	run    sync.Mutex  // one reading pass at a time
	kicked atomic.Bool // a pass after an import is already waiting to start
	gen    atomic.Int64

	mu         sync.Mutex
	summaries  *mediaSummaries
	incomplete *incompleteSet         // the files that look cut off (incomplete_files.go)
	folders    map[string]folderNames // folder listings, for the subtitle files beside the videos
}

// folderNames is the files in one folder, valid while the folder's time is
// the same.
type folderNames struct {
	mtime time.Time
	names []string
}

// mediaSummaries is what the Library lists need, kept for a short time.
type mediaSummaries struct {
	at       time.Time
	gen      int64
	sidecars bool // the subtitle files beside the videos were looked at too
	movies   map[int64]library.MediaSummary
	series   map[int64][]library.MediaSummary
}

type miOutcome int

const (
	miSkipped   miOutcome = iota // not read: gone, still changing, or its drive isn't there
	miUnchanged                  // the same as when it was last read
	miDone                       // read and saved
	miHung                       // the disk didn't answer
)

// mediaRoots remembers, for one pass, which library folders are there.
type mediaRoots map[string]bool

func (s *Server) rootAvailable(known mediaRoots, path string) bool {
	root := s.fileRoot("", path)
	if root == "" {
		return true // a file outside the known folders (a hand-added one): the file itself is the check
	}
	ok, seen := known[root]
	if !seen {
		ok = libraryAvailable(root)
		known[root] = ok
	}
	return ok
}

// saveFileInfo reads the file at path, unless it is the same as when it was
// last read, and saves what it holds. settle leaves out a file that changed
// in the last minutes.
func (s *Server) saveFileInfo(path string, known mediaRoots, settle bool) miOutcome {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return miSkipped
	}
	if settle && time.Since(st.ModTime()) < mediaInfoSettle {
		return miSkipped
	}
	if !s.rootAvailable(known, path) {
		return miSkipped
	}
	now := time.Now().UTC().Format(time.RFC3339)
	row, ok, err := s.MovieRepo.GetMediaInfo(path)
	if err != nil {
		slog.Warn("media info: couldn't look up a file", "err", err)
		return miSkipped
	}
	if ok && row.Size == st.Size() && row.MTime == st.ModTime().UnixNano() {
		_ = s.MovieRepo.TouchMediaInfo(path, time.Now())
		return miUnchanged
	}
	info, err := mediainfo.ReadFile(path)
	if errors.Is(err, mediainfo.ErrTimeout) {
		return miHung
	}
	row = library.MediaInfoRow{Path: path, Size: st.Size(), MTime: st.ModTime().UnixNano(), ScannedAt: now, CheckedAt: now}
	if err != nil {
		row.Failed = true
	} else {
		sum := info.Summary()
		row.Container = info.Container
		row.MediaSummary = library.MediaSummary{VCodec: sum.VCodec, HDR: sum.HDR, Res: sum.Res, ACodecs: sum.ACodecs, ALangs: sum.ALangs, SLangs: sum.SLangs}
		if b, err := json.Marshal(info); err == nil {
			row.Info = string(b)
		}
	}
	if err := s.MovieRepo.SaveMediaInfo(row); err != nil {
		slog.Warn("media info: couldn't save a file's details", "err", err)
		return miSkipped
	}
	s.mediaInfo.gen.Add(1)
	return miDone
}

// readPendingMediaInfo reads files that have no details yet until there are
// none, the time is up, or the disk stops answering. It returns how many it
// read.
func (s *Server) readPendingMediaInfo(ctx context.Context, until time.Time, settle bool, pause time.Duration) int {
	known := mediaRoots{}
	done := 0
	after := "" // a file that can't be read now is passed over, and tried again in the next pass
	for time.Now().Before(until) {
		paths, err := s.MovieRepo.MediaInfoPending(after, mediaInfoBatch)
		if err != nil {
			slog.Warn("media info: couldn't list the files to read", "err", err)
			return done
		}
		if len(paths) == 0 {
			return done
		}
		for _, p := range paths {
			after = p
			if ctx.Err() != nil || time.Now().After(until) {
				return done
			}
			switch s.saveFileInfo(p, known, settle) {
			case miDone:
				done++
			case miHung:
				slog.Warn("media info: the disk didn't answer, so reading stops for now")
				return done
			}
			if pause > 0 {
				select {
				case <-ctx.Done():
					return done
				case <-time.After(pause):
				}
			}
		}
	}
	return done
}

// mediaInfoJob is the background job: it reads the files that have no details
// yet (a few minutes at a time), checks a batch of saved files for changes,
// and forgets the details of files that are gone from the library. It waits
// out the quiet time like the other heavy jobs.
func (s *Server) mediaInfoJob(ctx context.Context) {
	if !s.mediaInfo.run.TryLock() {
		return
	}
	defer s.mediaInfo.run.Unlock()
	until := time.Now().Add(mediaInfoRunFor)
	n := s.readPendingMediaInfo(ctx, until, true, mediaInfoPause)
	s.fillRuntimes(ctx, time.Now().Add(2*time.Minute)) // what the Library needs to tell a cut-off file from a real one

	known := mediaRoots{}
	changed := 0
	stale, err := s.MovieRepo.MediaInfoStale(time.Now().Add(-mediaInfoRecheckAfter), mediaInfoRecheckBatch)
	if err != nil {
		slog.Warn("media info: couldn't list the files to check", "err", err)
	}
	for _, row := range stale {
		if ctx.Err() != nil || time.Now().After(until) {
			break
		}
		out := s.saveFileInfo(row.Path, known, true)
		if out == miHung {
			break
		}
		if out == miDone {
			changed++
		}
		time.Sleep(mediaInfoPause)
	}
	if gone, err := s.MovieRepo.PruneMediaInfo(); err == nil && gone > 0 {
		s.mediaInfo.gen.Add(1)
	}
	if n > 0 || changed > 0 {
		slog.Info("media info: read the details of files", "new", n, "changed", changed)
	}
}

// kickMediaInfo reads the details of files that were just imported, a few
// seconds from now (so the files of a season pack are read in one pass). It
// does nothing when a pass is already waiting.
func (s *Server) kickMediaInfo() {
	if s.MovieRepo == nil || !s.mediaInfo.kicked.CompareAndSwap(false, true) {
		return
	}
	go func() {
		time.Sleep(mediaInfoKickDelay)
		s.mediaInfo.kicked.Store(false)
		if !s.mediaInfo.run.TryLock() {
			return // the background job is running, and it takes new files too
		}
		defer s.mediaInfo.run.Unlock()
		s.readPendingMediaInfo(context.Background(), time.Now().Add(2*time.Minute), false, 0)
	}()
}

// refreshMovieMediaInfo and refreshSeriesMediaInfo read the files of a title
// again if they changed, after a rescan.
func (s *Server) refreshMovieMediaInfo(id int64) {
	if m, err := s.MovieRepo.Get(id); err == nil && m.FilePath != "" && m.Status == library.StatusDownloaded {
		s.refreshMediaInfo([]string{m.FilePath})
	}
}

func (s *Server) refreshSeriesMediaInfo(id int64) {
	eps, err := s.MovieRepo.ListEpisodes(id)
	if err != nil {
		return
	}
	seen := map[string]bool{}
	var paths []string
	for _, e := range eps {
		if e.Status == library.StatusDownloaded && e.FilePath != "" && !seen[e.FilePath] {
			seen[e.FilePath] = true
			paths = append(paths, e.FilePath)
		}
	}
	s.refreshMediaInfo(paths)
}

// refreshMediaInfo reads the given files again if they changed, for a rescan.
func (s *Server) refreshMediaInfo(paths []string) {
	if len(paths) == 0 || s.MovieRepo == nil {
		return
	}
	go func() {
		s.mediaInfo.run.Lock()
		defer s.mediaInfo.run.Unlock()
		known := mediaRoots{}
		for _, p := range paths {
			if s.saveFileInfo(p, known, false) == miHung {
				return
			}
		}
	}()
}

// ---- what the Library lists and detail pages show ----

// mediaSummaryPayload is one title's file details for the Library lists:
// short lists, a show having the different ones its episodes have.
type mediaSummaryPayload struct {
	VideoCodecs []string `json:"videoCodecs,omitempty"`
	HDR         []string `json:"hdr,omitempty"`
	Resolutions []string `json:"resolutions,omitempty"`
	AudioCodecs []string `json:"audioCodecs,omitempty"`
	AudioLangs  []string `json:"audioLangs,omitempty"`
	SubLangs    []string `json:"subLangs,omitempty"`
}

func mediaSummaryOf(list ...library.MediaSummary) *mediaSummaryPayload {
	if len(list) == 0 {
		return nil
	}
	var p mediaSummaryPayload
	add := func(dst *[]string, vals ...string) {
		for _, v := range vals {
			if v == "" {
				continue
			}
			dup := false
			for _, have := range *dst {
				if have == v {
					dup = true
					break
				}
			}
			if !dup {
				*dst = append(*dst, v)
			}
		}
	}
	for _, m := range list {
		add(&p.VideoCodecs, m.VCodec)
		add(&p.HDR, m.HDR)
		add(&p.Resolutions, m.Res)
		add(&p.AudioCodecs, mediainfo.Split(m.ACodecs)...)
		add(&p.AudioLangs, mediainfo.Split(m.ALangs)...)
		add(&p.SubLangs, mediainfo.Split(m.SLangs)...)
	}
	return &p
}

// mediaSummaryNow returns the file details of every movie and show, kept for
// a few seconds and renewed when a file has been read since. Looking for
// subtitle files beside the videos touches every folder on the disks, which
// is slow on a big library or a network share, so sidecars says whether the
// caller needs it.
func (s *Server) mediaSummaryNow(sidecars bool) *mediaSummaries {
	st := &s.mediaInfo
	st.mu.Lock()
	defer st.mu.Unlock()
	gen := st.gen.Load()
	if st.summaries != nil && st.summaries.gen == gen && time.Since(st.summaries.at) < mediaSummaryTTL && (st.summaries.sidecars || !sidecars) {
		return st.summaries
	}
	out := &mediaSummaries{at: time.Now(), gen: gen, sidecars: sidecars}
	var err error
	if out.movies, err = s.MovieRepo.MovieMediaSummaries(); err != nil {
		slog.Warn("media info: couldn't read the movies' file details", "err", err)
	}
	if out.series, err = s.MovieRepo.SeriesMediaSummaries(); err != nil {
		slog.Warn("media info: couldn't read the shows' file details", "err", err)
	}
	if sidecars {
		s.addSidecarSubtitles(out)
	}
	st.summaries = out
	return out
}

// addSidecarSubtitles adds the languages of the subtitle files that sit next
// to the videos (movie.en.srt) to what each title's files hold, so a title
// with a subtitle file in a language counts as having it, like one with the
// track inside the video. The caller holds st.mu.
func (s *Server) addSidecarSubtitles(out *mediaSummaries) {
	st := &s.mediaInfo
	moviePaths, seriesPaths, err := s.MovieRepo.MediaFilePaths()
	if err != nil {
		slog.Warn("media info: couldn't list the files to look beside for subtitles", "err", err)
		return
	}
	seen := map[string]folderNames{}
	langsOf := func(video string) []string {
		dir := filepath.Dir(video)
		f, ok := seen[dir]
		if !ok {
			f = st.folders[dir]
			fi, err := os.Stat(dir)
			if err != nil {
				return nil
			}
			if f.names == nil || !f.mtime.Equal(fi.ModTime()) {
				f = folderNames{mtime: fi.ModTime()}
				if entries, err := os.ReadDir(dir); err == nil {
					f.names = make([]string, 0, len(entries))
					for _, e := range entries {
						if !e.IsDir() {
							f.names = append(f.names, e.Name())
						}
					}
				}
			}
			seen[dir] = f
		}
		var langs []string
		for lang := range subtitles.DiskHaveFromNames(video, f.names) {
			if c := mediainfo.LanguageCode(lang); c != "" {
				langs = append(langs, c)
			}
		}
		return langs
	}
	join := func(have string, extra []string) string {
		all := mediainfo.Split(have)
		for _, l := range extra {
			if !slices.Contains(all, l) {
				all = append(all, l)
			}
		}
		return strings.Join(all, ",")
	}
	for id, p := range moviePaths {
		if ms, ok := out.movies[id]; ok {
			ms.SLangs = join(ms.SLangs, langsOf(p))
			out.movies[id] = ms
		}
	}
	for id, paths := range seriesPaths {
		var extra []string
		for _, p := range paths {
			for _, l := range langsOf(p) {
				if !slices.Contains(extra, l) {
					extra = append(extra, l)
				}
			}
		}
		if len(extra) > 0 && out.series != nil {
			sort.Strings(extra)
			out.series[id] = append(out.series[id], library.MediaSummary{SLangs: strings.Join(extra, ",")})
		}
	}
	st.folders = seen // folders no title uses any more are dropped
}

// fileInfoPayload is the full details of one file for a detail page.
type fileInfoPayload struct {
	mediainfo.Info
	Resolution string `json:"resolution,omitempty"`
	ScannedAt  string `json:"scannedAt,omitempty"`
}

// fileInfoOf turns a saved row into its payload; nil when the file has not
// been read or could not be.
func fileInfoOf(row library.MediaInfoRow) *fileInfoPayload {
	if row.Failed || row.Info == "" {
		return nil
	}
	var info mediainfo.Info
	if json.Unmarshal([]byte(row.Info), &info) != nil {
		return nil
	}
	p := &fileInfoPayload{Info: info, ScannedAt: row.ScannedAt}
	if info.Video != nil {
		p.Resolution = info.Video.Resolution()
	}
	return p
}

// movieFileInfo is the saved details of a movie's file (nil when there are none).
func (s *Server) movieFileInfo(path string) *fileInfoPayload {
	if path == "" {
		return nil
	}
	row, ok, err := s.MovieRepo.GetMediaInfo(path)
	if err != nil || !ok {
		return nil
	}
	return fileInfoOf(row)
}

// handleMediaInfoStatus says how far the reading of the files has got, for the
// note in the Library.
func (s *Server) handleMediaInfoStatus(w http.ResponseWriter, r *http.Request) {
	done, total, err := s.MovieRepo.MediaInfoCounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"done": done, "total": total})
}
