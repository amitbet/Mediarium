package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/parser"
	"github.com/rdborg/mediarium/internal/quality"
	"github.com/rdborg/mediarium/internal/settings"
)

// "Prefer season packs once a season has ended". While a season is still
// airing its episodes are fetched one by one. Once it has ended, and some of
// its episodes are missing or were fetched at a quality a pack could beat,
// the hunt looks for a season pack of it and takes one that is the same
// quality or better than what is there.

const (
	// seasonPackRetry is how long to wait before looking again for a pack of a
	// season that was searched for already.
	seasonPackRetry = 7 * 24 * time.Hour
	// seasonPackGraceDays is how many days after the last episode aired a
	// season counts as ended: packs usually show up a day or two after the
	// finale, and searching on the day itself would only use up the weekly try.
	seasonPackGraceDays = 2
	// seasonPackMaxTries is how many searches in a row may find no pack before
	// the season is left alone: until an episode is added to it, a pack
	// setting changes, or seasonPackGiveUp has passed since the last search.
	seasonPackMaxTries = 6
	seasonPackGiveUp   = library.SeasonPackGiveUpDays * 24 * time.Hour
)

// packHistory is what is known of the earlier searches for a pack of one
// season: when it was last searched for, how many searches in a row found no
// pack, and how many episodes the season had then.
type packHistory struct {
	at       time.Time
	tries    int
	episodes int
}

// seasonPackDefault is the switch in Settings (off by default).
func (s *Server) seasonPackDefault() bool { return s.settingOn(settings.KeySeasonPacks, false) }

// wantsSeasonPacks is whether this show takes season packs: its own choice,
// or the setting in Settings when it has none.
func (s *Server) wantsSeasonPacks(sr library.Series) bool {
	switch sr.SeasonPack {
	case library.SeasonPackOn:
		return true
	case library.SeasonPackOff:
		return false
	}
	return s.seasonPackDefault()
}

// seasonEnded reports whether every episode of a season has aired, with the
// last one at least seasonPackGraceDays ago. status is TMDB's status of the
// show: for a show that has ended or been cancelled, episodes TMDB gives no
// date are never going to air, so they do not hold the season open (a season
// with no dated episode at all is never ended). A season of one episode is not
// a pack worth looking for.
func seasonEnded(eps []library.Episode, status string, now time.Time) bool {
	if len(eps) < 2 {
		return false
	}
	limit := now.UTC().AddDate(0, 0, -seasonPackGraceDays).Format("2006-01-02")
	dated := 0
	for _, ep := range eps {
		if ep.AirDate == "" {
			if showEnded(status) {
				continue
			}
			return false
		}
		if ep.AirDate > limit {
			return false
		}
		dated++
	}
	return dated > 0
}

// packPlan says what a season pack has to be to be worth taking.
type packPlan struct {
	// floor is the rank of the best quality among the episodes of the season
	// already in: a pack below it is never taken.
	floor int
	// over is set when no episode is missing, so the pack is only wanted if it
	// is an upgrade over the lowest of the episodes that could be upgraded.
	over *upgradeBase
	// episodes is how many episodes the season has, remembered with the search.
	episodes int
}

// planSeasonPack decides whether to look for a pack of one season of a show,
// and what it has to beat. eps are all the episodes of the season. It returns
// false when the season has not ended, when it is already covered by a pack
// (or an episode is being downloaded), when nothing in it would gain from a
// pack, when it was searched for less than seasonPackRetry ago, or when
// seasonPackMaxTries searches in a row found no pack and nothing has changed
// since (no episode added, less than seasonPackGiveUp since the last search).
func planSeasonPack(profile quality.Profile, sr library.Series, eps []library.Episode, status string, last packHistory, now time.Time) (packPlan, bool) {
	plan, ok := planSeasonPackFor(profile, sr, eps, status, last, now)
	plan.episodes = len(eps)
	return plan, ok
}

func planSeasonPackFor(profile quality.Profile, sr library.Series, eps []library.Episode, status string, last packHistory, now time.Time) (packPlan, bool) {
	if !seasonEnded(eps, status, now) {
		return packPlan{}, false
	}
	if !last.at.IsZero() {
		since := now.Sub(last.at)
		if since < seasonPackRetry {
			return packPlan{}, false
		}
		if last.tries >= seasonPackMaxTries && last.episodes == len(eps) && since < seasonPackGiveUp {
			return packPlan{}, false
		}
	}
	today := now.UTC().Format("2006-01-02")
	missing := 0
	floor := 0
	var candidates []library.Episode
	for _, ep := range eps {
		if ep.Status == library.StatusDownloading {
			return packPlan{}, false // something is on its way already, maybe the pack itself
		}
		if !ep.Monitored {
			continue
		}
		switch ep.Status {
		case library.StatusMissing:
			if ep.AirDate != "" && ep.AirDate <= today && !ep.AirsLater(now) {
				missing++
			}
		case library.StatusDownloaded:
			if r := profile.TierRank(quality.Tier(ep.Quality)); r > floor {
				floor = r
			}
			if releaseIsPack(ep.ReleaseTitle) {
				return packPlan{}, false // a pack was taken for this season already
			}
			if !sr.NoUpgrade && wantsUpgrade(profile, ep.Quality, ep.ReleaseTitle) {
				candidates = append(candidates, ep)
			}
		}
	}
	if missing > 0 {
		return packPlan{floor: floor}, true
	}
	if len(candidates) == 0 {
		return packPlan{}, false
	}
	lowest := candidates[0]
	for _, ep := range candidates[1:] {
		if profile.TierRank(quality.Tier(ep.Quality)) < profile.TierRank(quality.Tier(lowest.Quality)) {
			lowest = ep
		}
	}
	return packPlan{floor: floor, over: &upgradeBase{Tier: quality.Tier(lowest.Quality), Release: lowest.ReleaseTitle}}, true
}

// releaseIsPack reports whether a release name is a whole season.
func releaseIsPack(title string) bool {
	if title == "" {
		return false
	}
	r := parser.Parse(title)
	return r.Season > 0 && len(r.Episodes) == 0
}

// packCandidates keeps the releases that are not below the quality already in.
func packCandidates(results []indexers.Result, profile quality.Profile, floor int, mapper func(parser.Release) parser.Release) []indexers.Result {
	var out []indexers.Result
	for _, res := range results {
		if profile.TierRank(quality.Classify(parseFor(mapper, res.Title))) >= floor {
			out = append(out, res)
		}
	}
	return out
}

// pickSeasonPack chooses the season pack to take for a plan from the search
// results (already filtered for the blocklist, sources, sizes and delay).
func pickSeasonPack(results []indexers.Result, series library.Series, season int, profile quality.Profile, plan packPlan, mapper func(parser.Release) parser.Release) *indexers.Result {
	cands := packCandidates(results, profile, plan.floor, mapper)
	if plan.over != nil {
		return pickTVResult(cands, series, season, 0, profile, plan.over, mapper)
	}
	return pickTVResult(cands, series, season, 0, profile, nil, mapper)
}

type setSeasonPackRequest struct {
	SeasonPack string `json:"seasonPack"` // "" = follow the setting, "on" or "off"
}

// handleSetSeriesSeasonPack saves a show's own choice about season packs.
func (s *Server) handleSetSeriesSeasonPack(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "That isn't a valid show ID.")
		return
	}
	var req setSeasonPackRequest
	if err := decodeJSON(r, &req); err != nil || !library.ValidSeasonPack(req.SeasonPack) {
		writeError(w, http.StatusBadRequest, `Choose "on" or "off" for season packs. Leave it blank to use the default.`)
		return
	}
	if err := s.MovieRepo.SetSeriesSeasonPack(id, req.SeasonPack); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

// seasonPackPlan is planSeasonPack for a season of a show, with the time of the
// last search read from searches (when each season was last searched for a
// pack). It is called by tvWants, which has the episodes at hand.
func (s *Server) seasonPackPlan(profile quality.Profile, sr library.Series, season int, eps []library.Episode, searches map[int]library.PackSearch) (packPlan, bool) {
	var last packHistory
	if p, ok := searches[season]; ok {
		if at, err := time.Parse(time.RFC3339, p.At); err == nil {
			last = packHistory{at: at, tries: p.Tries, episodes: p.Episodes}
		}
	}
	return planSeasonPack(profile, sr, eps, sr.TMDBStatus, last, time.Now())
}

// trySeasonPack looks for a pack of the season of w, and grabs it when one is
// good enough. searched is whether the indexers were asked at all (they are
// not once this run is out of searches); the try is remembered then, so the
// season is not asked about again for a week. grabbed is whether a pack was
// queued, which settles the season.
func (s *Server) trySeasonPack(w tvWant, search func() ([]indexers.Result, bool), grab func(best *indexers.Result) bool) (grabbed bool) {
	results, searched := search()
	if !searched {
		return false
	}
	_ = s.QueueRepo.LogSeriesActivity(w.series.ID, "searching", fmt.Sprintf("Season %d has ended: looking for a season pack", w.season))
	// The try is remembered whatever comes of it; searches in a row that
	// found no pack are counted, and a season that keeps finding none is
	// given up on (see planSeasonPack).
	defer func() {
		if err := s.MovieRepo.NoteSeasonPackSearch(w.series.ID, w.season, time.Now().UTC().Format(time.RFC3339), w.pack.episodes, grabbed); err != nil {
			log.Printf("automation: season pack: %v", err)
		}
	}()
	best := pickSeasonPack(results, w.series, w.season, w.profile, *w.pack, s.releaseMapper(w.series))
	if best == nil {
		return false
	}
	grabbed = grab(best)
	return grabbed
}
