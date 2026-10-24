package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/settings"
	"github.com/rdborg/mediarium/internal/tvmaze"
)

// Air times. TMDB gives an episode a day, not a time, so the calendar could
// only ever put a show on the day of its home country. TVmaze knows the exact
// moment, and the show's refresh asks it for those (see internal/tvmaze). A
// time is only stored when TVmaze's day agrees with TMDB's, and never guessed.

const (
	// tvmazeNotFoundRetry is how long a show TVmaze does not know is left
	// alone before it is looked up again.
	tvmazeNotFoundRetry = 7 * 24 * time.Hour
	// tvmazeDateSlack is how far apart TMDB's and TVmaze's days for an
	// episode may be: the two disagree by a day for shows that air late in
	// the evening far from where TMDB counts the day.
	tvmazeDateSlack = 24 * time.Hour
	// tvmazeBudget is the longest one show's look-up may take in all.
	tvmazeBudget = 45 * time.Second
)

// tvmazeOn is the "Look up air times on TVmaze" switch. On by default; off
// means TVmaze is never contacted.
func (s *Server) tvmazeOn() bool { return s.settingOn(settings.KeyTVmazeAirTimes, true) }

// matchAirTimes pairs TMDB's episodes with TVmaze's by season and number and
// returns the air time of each one TVmaze has a real time for, when the day
// TVmaze gives is the stored day or the one next to it. An episode with no
// stored day, or whose days disagree by more, keeps whatever it had: wrong
// data must never move an episode.
func matchAirTimes(stored []library.Episode, listed []tvmaze.Episode) []library.AirTime {
	type key struct{ season, number int }
	byKey := make(map[key]tvmaze.Episode, len(listed))
	for _, e := range listed {
		if e.Number == 0 {
			continue // a special TVmaze does not number
		}
		k := key{e.Season, e.Number}
		if _, dup := byKey[k]; !dup {
			byKey[k] = e
		}
	}
	var out []library.AirTime
	for _, ep := range stored {
		tv, ok := byKey[key{ep.Season, ep.Episode}]
		if !ok || ep.AirDate == "" {
			continue
		}
		at, ok := tv.AirAt()
		if !ok {
			continue
		}
		have, err1 := time.Parse("2006-01-02", ep.AirDate)
		theirs, err2 := time.Parse("2006-01-02", tv.Airdate)
		if err1 != nil || err2 != nil {
			continue
		}
		if d := have.Sub(theirs); d > tvmazeDateSlack || d < -tvmazeDateSlack {
			continue
		}
		out = append(out, library.AirTime{Season: ep.Season, Episode: ep.Episode, At: at})
	}
	return out
}

// tvmazeNeeded says whether TVmaze should be asked about the show now. A
// show it did not know is asked again after a week. A show that has ended is
// asked once, unless it still has a dated episode ahead that has no time yet.
func tvmazeNeeded(sr library.Series, eps []library.Episode, now time.Time) bool {
	checked, _ := time.Parse(time.RFC3339, sr.TVmazeCheckedAt)
	if sr.TVmazeID < 0 {
		return checked.IsZero() || now.Sub(checked) >= tvmazeNotFoundRetry
	}
	if sr.TVmazeID == 0 || checked.IsZero() || !showEnded(sr.TMDBStatus) {
		return true
	}
	today := now.UTC().Format("2006-01-02")
	for _, e := range eps {
		if e.AirDate >= today && e.AirAt == "" {
			return true
		}
	}
	return false
}

// resolveTVmaze finds the show on TVmaze: by its TheTVDB id, then its IMDb
// id, then by name and year. Only "TVmaze does not know it" moves on to the
// next way; any other failure is returned so nothing is remembered about it.
func (s *Server) resolveTVmaze(ctx context.Context, sr library.Series) (tvmaze.Show, error) {
	c := s.tvmaze
	if sr.TVDBID > 0 {
		show, err := c.LookupTVDB(ctx, sr.TVDBID)
		if !errors.Is(err, tvmaze.ErrNotFound) {
			return show, err
		}
	}
	if sr.IMDBID != "" {
		show, err := c.LookupIMDb(ctx, sr.IMDBID)
		if !errors.Is(err, tvmaze.ErrNotFound) {
			return show, err
		}
	}
	return c.Search(ctx, sr.Title, sr.Year)
}

// syncAirTimes fills in the air times of a show's episodes (and its second
// copies') from TVmaze. It never fails the refresh that calls it: a problem is
// logged and the next refresh tries again.
func (s *Server) syncAirTimes(ctx context.Context, sr library.Series) {
	if s.tvmaze == nil || !s.tvmazeOn() {
		return
	}
	now := time.Now()
	eps, err := s.MovieRepo.ListEpisodes(sr.ID)
	if err != nil {
		slog.Debug("api: air times: list episodes", "show", sr.Title, "error", err)
		return
	}
	if !tvmazeNeeded(sr, eps, now) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, tvmazeBudget)
	defer cancel()

	id := sr.TVmazeID
	if id <= 0 {
		show, err := s.resolveTVmaze(ctx, sr)
		switch {
		case errors.Is(err, tvmaze.ErrNotFound):
			if err := s.MovieRepo.SetSeriesTVmaze(sr.ID, -1, now); err != nil {
				slog.Warn("api: air times: remember a missing show", "show", sr.Title, "error", err)
			}
			return
		case err != nil:
			slog.Warn("api: air times: find the show on TVmaze", "show", sr.Title, "error", err)
			return
		}
		id = show.ID
		if err := s.MovieRepo.SetSeriesTVmaze(sr.ID, id, time.Time{}); err != nil {
			slog.Warn("api: air times: remember the TVmaze id", "show", sr.Title, "error", err)
		}
	}
	listed, err := s.tvmaze.Episodes(ctx, id)
	if err != nil {
		if errors.Is(err, tvmaze.ErrNotFound) {
			// The id went stale: look the show up again next time.
			_ = s.MovieRepo.SetSeriesTVmaze(sr.ID, -1, now)
			return
		}
		slog.Warn("api: air times: read the episodes from TVmaze", "show", sr.Title, "error", err)
		return
	}
	times := matchAirTimes(eps, listed)
	changed, err := s.MovieRepo.SetEpisodeAirTimes(sr.ID, times)
	if err != nil {
		slog.Warn("api: air times: save", "show", sr.Title, "error", err)
		return
	}
	if copies, err := s.MovieRepo.ListSeriesCopies(sr.ID); err == nil {
		for _, c := range copies {
			if _, err := s.MovieRepo.SetEpisodeAirTimes(c.ID, times); err != nil {
				slog.Warn("api: air times: save for a second copy", "show", sr.Title, "error", err)
			}
		}
	}
	if err := s.MovieRepo.SetSeriesTVmaze(sr.ID, id, now); err != nil {
		slog.Warn("api: air times: remember the check", "show", sr.Title, "error", err)
	}
	slog.Debug("api: air times: updated", "show", sr.Title, "matched", len(times), "changed", changed)
}
