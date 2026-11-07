package api

import (
	"context"
	"net/http"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/queue"
)

// ownedData is what Discover needs to mark the titles you already have: the
// movies and shows with their state, and nothing else. Kept like the other
// library caches (dropped as soon as a movie, show or episode changes).
type ownedData struct {
	movies []library.OwnedMovie
	shows  []library.OwnedShow
}

// ownedLibrary returns the slim movie and show lists for Discover. The rails
// call it on every page, so one narrow read is shared instead of each rail
// loading the whole library.
func (s *Server) ownedLibrary(ctx context.Context) (ownedData, error) {
	return s.ownedCache.get(ctx, overviewTTL, func(ctx context.Context) (ownedData, error) {
		var o ownedData
		var err error
		if o.movies, err = s.MovieRepo.OwnedMovies(); err != nil {
			return o, err
		}
		if err = ctx.Err(); err != nil {
			return o, err
		}
		o.shows, err = s.MovieRepo.OwnedSeries()
		return o, err
	})
}

type ownedMoviePayload struct {
	ID        int64  `json:"id"`
	TMDBID    int    `json:"tmdbId"`
	Status    string `json:"status"`
	Monitored bool   `json:"monitored"`
}

type ownedShowPayload struct {
	ID              int64 `json:"id"`
	TMDBID          int   `json:"tmdbId"`
	Monitored       bool  `json:"monitored"`
	EpisodeCount    int   `json:"episodeCount"`
	DownloadedCount int   `json:"downloadedCount"`
}

// ownedQueuePayload is one line of what the queue says about a title: just
// enough for the "downloading 40%", "paused" or "failed" mark.
type ownedQueuePayload struct {
	MovieID     int64   `json:"movieId"`
	SeriesID    int64   `json:"seriesId"`
	Status      string  `json:"status"`
	ProgressPct float64 `json:"progressPct"`
}

type ownedPayload struct {
	MovieCount int                 `json:"movieCount"`
	ShowCount  int                 `json:"showCount"`
	Movies     []ownedMoviePayload `json:"movies"`
	Shows      []ownedShowPayload  `json:"shows"`
	Queue      []ownedQueuePayload `json:"queue"`
}

// handleLibraryOwned answers which TMDB titles are in the library and how
// each is doing, in a form far smaller than the movie list, the show list and
// the queue together (which Discover used to read every 15 seconds just for
// its marks). It is sent with an ETag, so a poll that finds nothing changed
// costs a 304 and no body. Every signed-in account sees the same as in the
// library lists; it carries no paths, names or account details.
func (s *Server) handleLibraryOwned(w http.ResponseWriter, r *http.Request) {
	o, err := s.ownedLibrary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read the library: "+err.Error())
		return
	}
	progress, err := s.QueueRepo.TitleProgress()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read the downloads: "+err.Error())
		return
	}
	out := ownedPayload{
		MovieCount: len(o.movies), ShowCount: len(o.shows),
		Movies: make([]ownedMoviePayload, len(o.movies)),
		Shows:  make([]ownedShowPayload, len(o.shows)),
		Queue:  make([]ownedQueuePayload, len(progress)),
	}
	for i, m := range o.movies {
		out.Movies[i] = ownedMoviePayload{ID: m.ID, TMDBID: m.TMDBID, Status: string(m.Status), Monitored: m.Monitored}
	}
	for i, sh := range o.shows {
		out.Shows[i] = ownedShowPayload{ID: sh.ID, TMDBID: sh.TMDBID, Monitored: sh.Monitored, EpisodeCount: sh.EpisodeCount, DownloadedCount: sh.DownloadedCount}
	}
	for i, p := range progress {
		out.Queue[i] = ownedQueuePayload{MovieID: p.MovieID, SeriesID: p.SeriesID, Status: string(p.Status), ProgressPct: p.ProgressPct}
	}
	writeJSON(w, http.StatusOK, out)
}

type queueCountPayload struct {
	Active      int `json:"active"`
	Queued      int `json:"queued"`
	Downloading int `json:"downloading"`
	Importing   int `json:"importing"`
}

// handleQueueCount answers how many downloads are waiting, downloading or
// importing: the number beside Activity in the menu, which used to need the
// whole queue answer (names, posters, disk checks) every few seconds. The
// "active" number is the same as the length of GET /api/queue?active=1.
func (s *Server) handleQueueCount(w http.ResponseWriter, r *http.Request) {
	counts, err := s.QueueRepo.CountByStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "count the downloads: "+err.Error())
		return
	}
	out := queueCountPayload{
		Queued:      counts[queue.StatusQueued],
		Downloading: counts[queue.StatusDownloading],
		Importing:   counts[queue.StatusImporting],
	}
	out.Active = out.Queued + out.Downloading + out.Importing
	writeJSON(w, http.StatusOK, out)
}
