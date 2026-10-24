package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/parser"
	"github.com/rdborg/mediarium/internal/quality"
	"github.com/rdborg/mediarium/internal/queue"
)

// History, files, /parse, /rename and /manualimport of the arr-style API (see
// arr.go). Apps such as nzb360 fill their "recently downloaded" cards from
// /history, so it is answered from Mediarium's own download list: every
// download is a "grabbed" line, and one that finished is an import line, one
// that failed a "downloadFailed" line. A movie that is in the library with a
// file but has no finished download of its own (it came in through a library
// import) gets one import line dated by when it was added.

// arrRecentImports is how far back the lines for titles that came in without a
// download of Mediarium's go in a list not about one title (see
// arrHistoryRecords).
const arrRecentImports = 7 * 24 * time.Hour

// arrLibraryLineBase starts the ids of those lines. The apps read an id as a
// 32-bit number (a bigger one makes the whole list unreadable to them), so it
// stays under 2^31, above the ids of the download lines.
const arrLibraryLineBase = 1_500_000_000

// The event ids the apps filter history with (eventType=1,3).
const (
	arrEventGrabbed  = 1
	arrEventImported = 3 // downloadFolderImported
	arrEventFailed   = 4
)

// arrEventTable is the event ids and names of one app. The two apps share 1, 3
// and 4 and differ after that (6 is "movieFolderImported" in Radarr and
// "episodeFileRenamed" in Sonarr).
func arrEventTable(app arrApp) map[int]string {
	if app == arrSonarr {
		return map[int]string{1: "grabbed", 2: "seriesFolderImported", 3: "downloadFolderImported", 4: "downloadFailed", 5: "episodeFileDeleted", 6: "episodeFileRenamed", 7: "downloadIgnored"}
	}
	return map[int]string{1: "grabbed", 3: "downloadFolderImported", 4: "downloadFailed", 5: "movieFileDeleted", 6: "movieFolderImported", 8: "movieFileRenamed", 9: "downloadIgnored"}
}

// arrImportEvents are the ids under which a finished import is answered: an
// app asking for either one gets it (once, under the id it asked for).
func arrImportEvents(app arrApp) []int {
	if app == arrSonarr {
		return []int{3, 2}
	}
	return []int{3, 6}
}

type arrHistoryRecord struct {
	ID                  int64           `json:"id"`
	MovieID             int64           `json:"movieId,omitempty"`
	SeriesID            int64           `json:"seriesId,omitempty"`
	EpisodeID           int64           `json:"episodeId,omitempty"`
	SourceTitle         string          `json:"sourceTitle"`
	Languages           []arrLanguage   `json:"languages"`
	Quality             arrQualityModel `json:"quality"`
	CustomFormats       []any           `json:"customFormats"`
	CustomFormatScore   int             `json:"customFormatScore"`
	QualityCutoffNotMet bool            `json:"qualityCutoffNotMet"`
	Date                string          `json:"date"`
	DownloadID          string          `json:"downloadId"`
	EventType           string          `json:"eventType"`
	Data                map[string]any  `json:"data"`
	Movie               *arrMovie       `json:"movie,omitempty"`
	Series              *arrSeries      `json:"series,omitempty"`
	Episode             *arrEpisode     `json:"episode,omitempty"`

	when      time.Time
	event     int
	sortTitle string
}

// arrQueryValues returns every value of the query keys named (compared without
// case, a trailing "[]" ignored), split at commas: ?eventType=1,3,
// ?eventType=1&eventType=3 and ?eventType[]=1 all give 1 and 3.
func arrQueryValues(r *http.Request, keys ...string) []string {
	var out []string
	for k, vals := range r.URL.Query() {
		name := strings.ToLower(strings.TrimSuffix(k, "[]"))
		match := false
		for _, want := range keys {
			if name == strings.ToLower(want) {
				match = true
			}
		}
		if !match {
			continue
		}
		for _, v := range vals {
			for _, part := range strings.Split(v, ",") {
				if part = strings.TrimSpace(part); part != "" {
					out = append(out, part)
				}
			}
		}
	}
	return out
}

// arrEventFilter reads eventType: a number, a name ("downloadFolderImported",
// without case), several of them separated by commas or repeated. It returns
// nil when nothing was asked for, or when nothing that was asked for can be
// read (an app that sends something odd gets everything rather than nothing).
func arrEventFilter(app arrApp, r *http.Request) map[int]bool {
	table := arrEventTable(app)
	byName := map[string]int{}
	for id, name := range table {
		byName[strings.ToLower(name)] = id
	}
	out := map[int]bool{}
	for _, v := range arrQueryValues(r, "eventType", "eventTypes") {
		if n, err := strconv.Atoi(v); err == nil {
			out[n] = true
		} else if id, ok := byName[strings.ToLower(v)]; ok {
			out[id] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// arrIDSet reads ids from numbers separated by commas or repeated keys.
func arrIDSet(r *http.Request, keys ...string) map[int64]bool {
	out := map[int64]bool{}
	for _, v := range arrQueryValues(r, keys...) {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			out[n] = true
		}
	}
	return out
}

func arrIDParam(r *http.Request, keys ...string) int64 {
	for _, k := range keys {
		if n, err := strconv.ParseInt(r.URL.Query().Get(k), 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func parseArrStored(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z", "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// arrHistoryRecords builds every history line of one app, newest first (or as
// sortKey and sortDirection say), narrowed by eventType, movieId(s),
// seriesId(s), episodeId, downloadId and (for /history/since) date. The movie,
// show and episode are always included: the apps need the poster and the
// title, and a few forget to ask.
func (s *Server) arrHistoryRecords(app arrApp, r *http.Request) ([]arrHistoryRecord, error) {
	items, err := s.QueueRepo.List()
	if err != nil {
		return nil, err
	}
	events := arrEventFilter(app, r)
	wants := func(id int) bool { return events == nil || events[id] }
	names := arrEventTable(app)
	wantMovies := arrIDSet(r, "movieId", "movieIds")
	wantSeries := arrIDSet(r, "seriesId", "seriesIds")
	wantEpisode := arrIDParam(r, "episodeId", "episodeid")
	wantSeason := -1
	if v := r.URL.Query().Get("seasonNumber"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			wantSeason = n
		}
	}
	wantDownload := r.URL.Query().Get("downloadId")
	var since, until time.Time
	for _, v := range arrQueryValues(r, "date", "since", "start", "startDate", "from") {
		if t := parseArrStored(v); !t.IsZero() && t.After(since) {
			since = t
		}
	}
	for _, v := range arrQueryValues(r, "end", "endDate", "until") {
		if t := parseArrStored(v); !t.IsZero() {
			until = t
		}
	}
	// A library import is not a download: in a list that is neither about one
	// title nor limited to a time, only the ones from the last week are lines
	// of its history, so a "recently downloaded" card is not filled with the
	// whole library.
	recentOnly := len(wantMovies) == 0 && len(wantSeries) == 0 && wantEpisode == 0 && since.IsZero()
	recentFrom := time.Now().Add(-arrRecentImports)

	// The ids an import is answered under: 3 when nothing was asked for, else
	// each of the import ids that was.
	importIDs := []int{arrEventImported}
	if events != nil {
		importIDs = nil
		for _, id := range arrImportEvents(app) {
			if events[id] {
				importIDs = append(importIDs, id)
			}
		}
	}

	var (
		mc     arrMovieContext
		sc     arrSeriesContext
		movies = map[int64]*arrMovie{}
		shows  = map[int64]*arrSeries{}
	)
	if app == arrRadarr {
		mc = s.newArrMovieContext()
	} else {
		sc = s.newArrSeriesContext()
	}
	movieOf := func(id int64) *arrMovie {
		if movies[id] == nil {
			if m, err := s.MovieRepo.Get(id); err == nil {
				a := s.toArrMovie(m, mc)
				movies[id] = &a
			}
		}
		return movies[id]
	}
	out := []arrHistoryRecord{}
	imported := map[int64]bool{}    // movies that have a finished download of their own
	importedEps := map[int64]bool{} // episodes that have one
	for _, it := range items {
		switch {
		case app == arrRadarr && it.MovieID == 0, app == arrSonarr && it.SeriesID == 0, app == arrLidarr:
			continue
		}
		if len(wantMovies) > 0 && !wantMovies[it.MovieID] || len(wantSeries) > 0 && !wantSeries[it.SeriesID] {
			continue
		}
		if wantSeason >= 0 && it.Season != wantSeason {
			continue
		}
		downloadID := fmt.Sprintf("mediarium-%d", it.ID)
		if wantDownload != "" && !strings.EqualFold(wantDownload, downloadID) {
			continue
		}
		rel := parser.Parse(it.ReleaseTitle)
		q := arrQualityModelOf(app, quality.Classify(rel))
		added := parseArrStored(it.AddedAt)
		done := parseArrStored(it.CompletedAt)
		if done.IsZero() {
			done = added
		}
		protocol := "1"
		if it.Protocol == queue.ProtocolTorrent {
			protocol = "2"
		}
		type line struct {
			kind int   // 1, 3 or 4: what happened
			ids  []int // the event ids it is answered under
		}
		lines := []line{{arrEventGrabbed, []int{arrEventGrabbed}}}
		switch {
		case it.Status == queue.StatusCompleted && it.Error == "":
			lines = append(lines, line{arrEventImported, importIDs})
			imported[it.MovieID] = true
		case it.Status == queue.StatusFailed:
			lines = append(lines, line{arrEventFailed, []int{arrEventFailed}})
		}
		eps := []library.Episode{{}}
		if app == arrSonarr {
			if found := s.queueEpisodes(it); len(found) > 0 {
				eps = found // a season pack is one line per episode, as in the queue
			}
		}
		for _, ln := range lines {
			when := added
			data := map[string]any{"indexer": "", "releaseGroup": rel.Group, "size": strconv.FormatInt(it.SizeBytes, 10),
				"downloadClient": "Mediarium", "downloadClientName": "Mediarium", "protocol": protocol, "customFormatScore": "0"}
			if ln.kind != arrEventGrabbed {
				when = done
				data = map[string]any{"downloadClient": "Mediarium", "downloadClientName": "Mediarium", "indexer": "", "releaseGroup": rel.Group,
					"size": strconv.FormatInt(it.SizeBytes, 10), "protocol": protocol}
				if ln.kind == arrEventImported && it.DestPath != "" {
					data["droppedPath"] = it.DestPath
					data["importedPath"] = it.DestPath
				}
				if ln.kind == arrEventFailed {
					data["message"] = it.Error
				}
			}
			if !since.IsZero() && !when.After(since) || !until.IsZero() && when.After(until) {
				continue
			}
			for _, id := range ln.ids {
				if !wants(id) {
					continue
				}
				for j, ep := range eps {
					if wantEpisode > 0 && ep.ID != wantEpisode {
						continue
					}
					if ln.kind == arrEventImported {
						importedEps[ep.ID] = true
					}
					rec := arrHistoryRecord{
						ID: (it.ID*1000+int64(j))*10 + int64(id), SourceTitle: it.ReleaseTitle, Languages: []arrLanguage{arrEnglish},
						Quality: q, CustomFormats: []any{}, Date: when.Format("2006-01-02T15:04:05Z"), DownloadID: downloadID,
						EventType: names[id], Data: copyArrData(data), when: when, event: id, sortTitle: arrSortTitle(it.ReleaseTitle),
					}
					if app == arrRadarr {
						rec.MovieID = it.MovieID
						rec.Movie = movieOf(it.MovieID)
						if rec.Movie != nil {
							rec.sortTitle = rec.Movie.SortTitle
						}
					} else {
						rec.SeriesID, rec.EpisodeID = it.SeriesID, ep.ID
						if shows[it.SeriesID] == nil {
							if sr, err := s.MovieRepo.GetSeries(it.SeriesID); err == nil {
								a := s.toArrSeries(sr, sc)
								shows[it.SeriesID] = &a
							}
						}
						rec.Series = shows[it.SeriesID]
						if rec.Series != nil {
							rec.sortTitle = rec.Series.SortTitle
						}
						if ep.ID > 0 {
							e := toArrEpisode(ep)
							rec.Episode = &e
						}
					}
					out = append(out, rec)
				}
			}
		}
	}

	// A movie that is in the library with its file but never came through a
	// download of Mediarium's (it was imported from a folder) has no line yet:
	// it gets one import line, dated by when it was added.
	if app == arrRadarr && len(importIDs) > 0 && wantDownload == "" && wantEpisode == 0 {
		list, err := s.MovieRepo.List()
		if err != nil {
			return nil, err
		}
		for _, m := range list {
			if m.Status != library.StatusDownloaded || m.FilePath == "" || m.CopyOf != 0 || imported[m.ID] {
				continue
			}
			if len(wantMovies) > 0 && !wantMovies[m.ID] {
				continue
			}
			when := parseArrStored(mc.added[m.ID])
			if when.IsZero() {
				when = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			if !since.IsZero() && !when.After(since) || !until.IsZero() && when.After(until) || recentOnly && when.Before(recentFrom) {
				continue
			}
			ids := importIDs
			if events == nil {
				ids = []int{6} // a library import: movieFolderImported
			}
			title := m.ReleaseTitle
			if title == "" {
				title = strings.TrimSuffix(filepath.Base(m.FilePath), filepath.Ext(m.FilePath))
			}
			rel := parser.Parse(title)
			for _, id := range ids {
				rec := arrHistoryRecord{
					ID: arrLibraryLineBase + m.ID*10 + int64(id), MovieID: m.ID, SourceTitle: title, Languages: []arrLanguage{arrEnglish},
					Quality: arrQualityModelOf(app, quality.Tier(m.Quality)), CustomFormats: []any{}, Date: when.Format("2006-01-02T15:04:05Z"),
					DownloadID: fmt.Sprintf("mediarium-movie-%d", m.ID), EventType: names[id], when: when, event: id,
					Data: map[string]any{"droppedPath": m.FilePath, "importedPath": m.FilePath, "indexer": "", "releaseGroup": rel.Group,
						"downloadClient": "Mediarium", "downloadClientName": "Mediarium"},
				}
				rec.Movie = movieOf(m.ID)
				if rec.Movie != nil {
					rec.sortTitle = rec.Movie.SortTitle
				}
				out = append(out, rec)
			}
		}
	}

	// The same for an episode that has its file but never came through a
	// download of Mediarium's: one import line, dated by when its file arrived.
	if app == arrSonarr && len(importIDs) > 0 && wantDownload == "" {
		all, err := s.allShowEpisodes()
		if err != nil {
			return nil, err
		}
		for _, se := range all {
			ep := se.ep
			if ep.Status != library.StatusDownloaded || ep.FilePath == "" || se.series.CopyOf != 0 || importedEps[ep.ID] {
				continue
			}
			if len(wantSeries) > 0 && !wantSeries[se.series.ID] || wantEpisode > 0 && ep.ID != wantEpisode {
				continue
			}
			if wantSeason >= 0 && ep.Season != wantSeason {
				continue
			}
			when := time.Time{}
			if fi, err := os.Stat(ep.FilePath); err == nil {
				when = fi.ModTime().UTC()
			}
			if when.IsZero() {
				when = parseArrStored(sc.added[se.series.ID])
			}
			if when.IsZero() {
				when = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			if !since.IsZero() && !when.After(since) || !until.IsZero() && when.After(until) || recentOnly && when.Before(recentFrom) {
				continue
			}
			ids := importIDs
			if events == nil {
				ids = []int{arrEventImported}
			}
			title := ep.ReleaseTitle
			if title == "" {
				title = strings.TrimSuffix(filepath.Base(ep.FilePath), filepath.Ext(ep.FilePath))
			}
			rel := parser.Parse(title)
			if shows[se.series.ID] == nil {
				a := s.toArrSeries(se.series, sc)
				shows[se.series.ID] = &a
			}
			e := toArrEpisode(ep)
			for _, id := range ids {
				out = append(out, arrHistoryRecord{
					ID: arrLibraryLineBase + ep.ID*10 + int64(id), SeriesID: se.series.ID, EpisodeID: ep.ID, SourceTitle: title,
					Languages: []arrLanguage{arrEnglish}, Quality: arrQualityModelOf(app, quality.Tier(ep.Quality)), CustomFormats: []any{},
					Date: when.Format("2006-01-02T15:04:05Z"), DownloadID: fmt.Sprintf("mediarium-episode-%d", ep.ID), EventType: names[id],
					Data: map[string]any{"droppedPath": ep.FilePath, "importedPath": ep.FilePath, "indexer": "", "releaseGroup": rel.Group,
						"downloadClient": "Mediarium", "downloadClientName": "Mediarium"},
					Series: shows[se.series.ID], Episode: &e, when: when, event: id, sortTitle: shows[se.series.ID].SortTitle,
				})
			}
		}
	}

	desc := !strings.EqualFold(r.URL.Query().Get("sortDirection"), "ascending")
	key := strings.ToLower(r.URL.Query().Get("sortKey"))
	less := func(a, b arrHistoryRecord) int { // -1 when a sorts first in ascending order
		switch {
		case strings.Contains(key, "sorttitle") || strings.HasSuffix(key, "title") && !strings.Contains(key, "source"):
			if c := strings.Compare(a.sortTitle, b.sortTitle); c != 0 {
				return c
			}
		case strings.Contains(key, "sourcetitle"):
			if c := strings.Compare(strings.ToLower(a.SourceTitle), strings.ToLower(b.SourceTitle)); c != 0 {
				return c
			}
		case strings.Contains(key, "eventtype"):
			if c := strings.Compare(a.EventType, b.EventType); c != 0 {
				return c
			}
		}
		// Everything else, and ties: by date, then by id.
		switch {
		case a.when.Before(b.when):
			return -1
		case a.when.After(b.when):
			return 1
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool {
		c := less(out[i], out[j])
		if desc {
			return c > 0
		}
		return c < 0
	})
	return out, nil
}

func copyArrData(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// logArrAsked writes what an app asked the history for (the names of
// the query values, and the values of the harmless ones; never an API key) at
// debug level, so a report of an empty card shows what the app wanted.
func logArrAsked(app arrApp, r *http.Request, found int) {
	if !slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		return
	}
	shown := map[string]bool{"eventtype": true, "eventtypes": true, "sortkey": true, "sortdirection": true, "page": true, "pagesize": true,
		"includemovie": true, "includeseries": true, "includeepisode": true, "movieid": true, "movieids": true, "seriesid": true,
		"seriesids": true, "episodeid": true, "seasonnumber": true, "date": true, "downloadid": true, "monitored": true, "unmonitored": true,
		"start": true, "end": true, "since": true, "includeepisodefile": true, "limit": true}
	var parts []string
	for k, vals := range r.URL.Query() {
		lk := strings.ToLower(strings.TrimSuffix(k, "[]"))
		switch {
		case lk == "apikey":
		case shown[lk]:
			parts = append(parts, k+"="+strings.Join(vals, "|"))
		default:
			parts = append(parts, k)
		}
	}
	sort.Strings(parts)
	slog.Debug("arr: history asked", "app", string(app), "path", r.URL.Path, "query", strings.Join(parts, "&"), "records", found)
}

// arrHistory answers GET /history with the paged envelope.
func (s *Server) arrHistory(app arrApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recs, err := s.arrHistoryRecords(app, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		logArrAsked(app, r, len(recs))
		page, size := paging(r, 10)
		from, to := pageOf(len(recs), page, size)
		dir := "descending"
		if strings.EqualFold(r.URL.Query().Get("sortDirection"), "ascending") {
			dir = "ascending"
		}
		sortKey := r.URL.Query().Get("sortKey")
		if sortKey == "" {
			sortKey = "date"
		}
		writeJSON(w, http.StatusOK, arrPage{Page: page, PageSize: size, SortKey: sortKey, SortDirection: dir,
			TotalRecords: len(recs), Records: recs[from:to]})
	}
}

// arrListLimit is how many lines an app wants out of a plain list: pageSize,
// limit or take, when it sends one (0 when not).
func arrListLimit(r *http.Request) int {
	for _, v := range arrQueryValues(r, "pageSize", "limit", "take") {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// arrHistoryList answers /history/since, /history/movie and /history/series:
// the same lines as a plain list.
func (s *Server) arrHistoryList(app arrApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recs, err := s.arrHistoryRecords(app, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		logArrAsked(app, r, len(recs))
		if n := arrListLimit(r); n > 0 && len(recs) > n {
			recs = recs[:n] // newest first (or as sortKey says), so the app's card gets the latest
		}
		writeJSON(w, http.StatusOK, recs)
	}
}

// --- episode files ---

type arrEpisodeFile struct {
	ID                  int64           `json:"id"`
	SeriesID            int64           `json:"seriesId"`
	SeasonNumber        int             `json:"seasonNumber"`
	RelativePath        string          `json:"relativePath"`
	Path                string          `json:"path"`
	Size                int64           `json:"size"`
	DateAdded           string          `json:"dateAdded"`
	SceneName           string          `json:"sceneName,omitempty"`
	ReleaseGroup        string          `json:"releaseGroup,omitempty"`
	Languages           []arrLanguage   `json:"languages"`
	Quality             arrQualityModel `json:"quality"`
	CustomFormats       []any           `json:"customFormats"`
	CustomFormatScore   int             `json:"customFormatScore"`
	MediaInfo           any             `json:"mediaInfo"`
	QualityCutoffNotMet bool            `json:"qualityCutoffNotMet"`
}

// toArrEpisodeFile is the file of a downloaded episode (nil when it has none).
func (s *Server) toArrEpisodeFile(ep library.Episode) *arrEpisodeFile {
	if ep.Status != library.StatusDownloaded || ep.FilePath == "" {
		return nil
	}
	f := &arrEpisodeFile{
		ID: ep.ID, SeriesID: ep.SeriesID, SeasonNumber: ep.Season, RelativePath: filepath.Base(ep.FilePath), Path: ep.FilePath,
		DateAdded: "2000-01-01T00:00:00Z", Languages: []arrLanguage{arrEnglish}, CustomFormats: []any{}, MediaInfo: map[string]any{},
		Quality: arrQualityModelOf(arrSonarr, quality.Tier(ep.Quality)),
	}
	if fi, err := os.Stat(ep.FilePath); err == nil {
		f.Size = fi.Size()
		f.DateAdded = fi.ModTime().UTC().Format("2006-01-02T15:04:05Z")
	}
	if mi := s.arrMediaInfoFor(ep.FilePath); mi != nil {
		f.MediaInfo = mi
	}
	if ep.ReleaseTitle != "" {
		f.SceneName = ep.ReleaseTitle
		f.ReleaseGroup = parser.Parse(ep.ReleaseTitle).Group
	}
	return f
}

// arrEpisodeFiles answers GET /episodefile?seriesId= or ?episodeFileIds=.
func (s *Server) arrEpisodeFiles(w http.ResponseWriter, r *http.Request) {
	out := []*arrEpisodeFile{}
	want := map[int64]bool{}
	for _, key := range []string{"episodeFileIds", "episodefileids"} {
		for _, v := range r.URL.Query()[key] {
			for _, part := range strings.Split(v, ",") {
				if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && n > 0 {
					want[n] = true
				}
			}
		}
	}
	if sid := arrIDParam(r, "seriesId", "seriesid"); sid > 0 {
		eps, err := s.MovieRepo.ListEpisodes(sid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, ep := range eps {
			if f := s.toArrEpisodeFile(ep); f != nil && (len(want) == 0 || want[f.ID]) {
				out = append(out, f)
			}
		}
	} else {
		for id := range want {
			if ep, err := s.MovieRepo.GetEpisodeByID(id); err == nil {
				if f := s.toArrEpisodeFile(ep); f != nil {
					out = append(out, f)
				}
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) arrGetEpisodeFile(w http.ResponseWriter, r *http.Request) {
	id, ok := arrPathID(w, r)
	if !ok {
		return
	}
	ep, err := s.MovieRepo.GetEpisodeByID(id)
	if err != nil {
		arrNotFound(w)
		return
	}
	f := s.toArrEpisodeFile(ep)
	if f == nil {
		arrNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) arrGetMovieFile(w http.ResponseWriter, r *http.Request) {
	id, ok := arrPathID(w, r)
	if !ok {
		return
	}
	m, err := s.MovieRepo.Get(id)
	if err != nil {
		arrNotFound(w)
		return
	}
	f := s.toArrMovie(m, s.newArrMovieContext()).MovieFile
	if f == nil {
		arrNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// --- parse ---

// arrParse answers GET /parse?title=: what the release name says, and the
// library title it belongs to when there is one.
func (s *Server) arrParse(app arrApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := r.URL.Query().Get("title")
		if strings.TrimSpace(title) == "" {
			arrBadRequest(w, "Title", "Give a release name in title.")
			return
		}
		rel := parser.Parse(title)
		q := arrQualityModelOf(app, quality.Classify(rel))
		langs := []arrLanguage{arrEnglish}
		clean := parseClean(rel.Title)
		if app == arrRadarr {
			info := map[string]any{
				"releaseTitle": title, "movieTitle": rel.Title, "movieTitles": []string{rel.Title}, "year": rel.Year, "quality": q,
				"languages": langs, "releaseGroup": rel.Group, "edition": rel.Edition, "simpleReleaseTitle": rel.Title,
				"imdbId": "", "tmdbId": 0, "hardcodedSubs": "", "movieTitleInfo": map[string]any{"title": rel.Title, "titleWithoutYear": rel.Title, "year": rel.Year},
			}
			var movie *arrMovie
			if movies, err := s.MovieRepo.List(); err == nil && clean != "" {
				c := s.newArrMovieContext()
				for _, m := range movies {
					if parseClean(m.Title) == clean && (rel.Year == 0 || m.Year == rel.Year) {
						a := s.toArrMovie(m, c)
						movie = &a
						break
					}
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": 0, "title": title, "parsedMovieInfo": info, "movie": movie})
			return
		}
		eps := rel.Episodes
		if len(eps) == 0 && rel.Episode > 0 {
			eps = []int{rel.Episode}
		}
		if eps == nil {
			eps = []int{}
		}
		abs := rel.Absolute
		if abs == nil {
			abs = []int{}
		}
		info := map[string]any{
			"releaseTitle": title, "seriesTitle": rel.Title, "seriesTitleInfo": map[string]any{"title": rel.Title, "titleWithoutYear": rel.Title, "year": rel.Year},
			"quality": q, "seasonNumber": rel.Season, "episodeNumbers": eps, "absoluteEpisodeNumbers": abs, "specialAbsoluteEpisodeNumbers": []int{},
			"languages": langs, "fullSeason": rel.Season > 0 && len(eps) == 0, "isPartialSeason": false, "isMultiSeason": rel.SeasonTo > rel.Season,
			"isSeasonExtra": false, "special": rel.Season == 0 && len(eps) > 0, "releaseGroup": rel.Group, "releaseHash": "", "seasonPart": 0,
			"releaseTokens": "", "isDaily": rel.AirDate != "", "isAbsoluteNumbering": len(abs) > 0, "isPossibleSpecialEpisode": false,
			"isPossibleSceneSeasonSpecial": false,
		}
		var (
			series   *arrSeries
			episodes = []arrEpisode{}
		)
		if list, err := s.MovieRepo.ListSeries(); err == nil && clean != "" {
			c := s.newArrSeriesContext()
			for _, sr := range list {
				if parseClean(sr.Title) != clean {
					continue
				}
				a := s.toArrSeries(sr, c)
				series = &a
				if all, err := s.MovieRepo.ListEpisodes(sr.ID); err == nil {
					for _, ep := range all {
						for _, n := range eps {
							if ep.Season == rel.Season && ep.Episode == n {
								episodes = append(episodes, toArrEpisode(ep))
							}
						}
					}
				}
				break
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": 0, "title": title, "parsedEpisodeInfo": info, "series": series, "episodes": episodes})
	}
}

// parseClean is a title for comparing: "&" and "and" are the same word.
func parseClean(title string) string {
	return arrCleanTitle(strings.ReplaceAll(title, "&", " and "))
}
