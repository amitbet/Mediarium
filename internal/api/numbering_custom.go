package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/rdborg/mediarium/internal/animemap"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/parser"
)

// A person's own numbering for a show. TMDB sometimes lists a show as a new
// entry (season 1) that releases and files call season 15, or splits a season
// that releases number straight on. The rules saved here say how the show's
// library numbering (TMDB's) is called in release and file names, and the
// extra names are searched for and accepted next to the show's title. Rules win
// over TheXEM and the anime lists; a name no rule covers is read as before.
// The library keeps its own numbering for folders and file names.

// maxCustomQueries is the most extra searches the person's own numbering adds
// for one episode or season.
const maxCustomQueries = 4

// customFor is the person's own numbering for a show, nil when there is none.
func (s *Server) customFor(series library.Series) *animemap.Custom {
	c, err := s.animeService().Repo.GetCustom(numberingKey(series))
	if err != nil {
		slog.Warn("numbering: couldn't read a show's own numbering", "series", series.ID, "err", err)
		return nil
	}
	if c.Empty() {
		return nil
	}
	return &c
}

// mapCustom reads a release name through the person's rules. ok is false when
// no rule covers all of the name's episodes (or its season, for a pack), so
// the name goes on to the other numberings.
func mapCustom(rel parser.Release, c *animemap.Custom) (parser.Release, bool) {
	if c == nil || len(c.Rules) == 0 || rel.Season <= 0 || rel.SeasonTo > rel.Season {
		return rel, false
	}
	if len(rel.Episodes) == 0 {
		ls, ok := c.SeasonToLibrary(rel.Season)
		if !ok {
			return rel, false
		}
		rel.Season = ls
		return rel, true
	}
	season := 0
	eps := make([]int, 0, len(rel.Episodes))
	for _, e := range rel.Episodes {
		s, ep, ok := c.ToLibrary(rel.Season, e)
		if !ok || (season != 0 && s != season) {
			return rel, false
		}
		season = s
		eps = append(eps, ep)
	}
	rel.Season, rel.Episodes, rel.Episode = season, eps, eps[0]
	return rel, true
}

// customMap is the person's rules as the import rules read them, nil when
// there are none.
func customMap(c *animemap.Custom) func(season, episode int) (int, int, bool) {
	if c == nil || len(c.Rules) == 0 {
		return nil
	}
	return c.ToLibrary
}

// customQueries are the searches the person's own numbering and names add for
// a season (episode 0) or an episode: the title under the release numbering,
// then each extra name under it (and under the library numbering when no rule
// covers the episode).
func customQueries(title string, c *animemap.Custom, season, episode int) []string {
	if c.Empty() || season <= 0 {
		return nil
	}
	type pair struct{ s, e int }
	var numberings []pair
	if episode > 0 {
		if rs, re, ok := c.ToRelease(season, episode); ok {
			numberings = append(numberings, pair{rs, re})
		}
	} else {
		for _, rs := range c.ReleaseSeasons(season) {
			numberings = append(numberings, pair{rs, 0})
		}
	}
	covered := len(numberings) > 0
	if !covered {
		numberings = append(numberings, pair{season, episode})
	}
	var out []string
	seen := map[string]bool{}
	add := func(q string) {
		if k := strings.ToLower(q); !seen[k] {
			seen[k] = true
			out = append(out, q)
		}
	}
	if covered {
		for _, n := range numberings {
			add(tvSearchQuery(title, n.s, n.e))
		}
	}
	for _, name := range c.Names {
		for _, n := range numberings {
			add(tvSearchQuery(name, n.s, n.e))
		}
	}
	if len(out) > maxCustomQueries {
		out = out[:maxCustomQueries]
	}
	return out
}

// ---- checking what a person entered

func checkCustom(c animemap.Custom, seasons map[int]bool) error {
	if len(c.Rules) > animemap.MaxCustomRules {
		return fmt.Errorf("That is more than %d rules. Remove some.", animemap.MaxCustomRules)
	}
	if len(c.Names) > animemap.MaxCustomNames {
		return fmt.Errorf("That is more than %d extra names. Remove some.", animemap.MaxCustomNames)
	}
	for _, n := range c.Names {
		if len([]rune(n)) > animemap.MaxCustomNameLen {
			return fmt.Errorf("An extra name can be at most %d letters long.", animemap.MaxCustomNameLen)
		}
		if len([]rune(n)) < 3 {
			return fmt.Errorf("The extra name %q is too short to search for. Use at least 3 letters.", n)
		}
	}
	for i, r := range c.Rules {
		n := i + 1
		switch {
		case r.LibrarySeason < 1 || r.LibrarySeason > animemap.MaxCustomSeason:
			return fmt.Errorf("Rule %d: the library season must be a number from 1 to %d.", n, animemap.MaxCustomSeason)
		case r.ReleaseSeason < 1 || r.ReleaseSeason > animemap.MaxCustomSeason:
			return fmt.Errorf("Rule %d: the release season must be a number from 1 to %d.", n, animemap.MaxCustomSeason)
		case r.Offset < -animemap.MaxCustomOffset || r.Offset > animemap.MaxCustomOffset:
			return fmt.Errorf("Rule %d: the episode offset must be between -%d and %d.", n, animemap.MaxCustomOffset, animemap.MaxCustomOffset)
		case r.From < 0 || r.From > animemap.MaxCustomEpisode || r.To < 0 || r.To > animemap.MaxCustomEpisode:
			return fmt.Errorf("Rule %d: episode numbers must be between 1 and %d.", n, animemap.MaxCustomEpisode)
		case r.To > 0 && r.From > r.To:
			return fmt.Errorf("Rule %d: the first episode comes after the last one.", n)
		}
		if first, _ := r.ReleaseRange(); first < 1 {
			return fmt.Errorf("Rule %d: this offset would give episode %d in the releases. Episodes start at 1.", n, first)
		}
		if seasons != nil && !seasons[r.LibrarySeason] {
			return fmt.Errorf("Rule %d: the show has no season %d in the library.", n, r.LibrarySeason)
		}
	}
	for i, a := range c.Rules {
		for j := i + 1; j < len(c.Rules); j++ {
			b := c.Rules[j]
			if a.LibrarySeason == b.LibrarySeason && animemap.RangesOverlap(a.From, a.To, b.From, b.To) {
				return fmt.Errorf("Rules %d and %d cover some of the same episodes of library season %d.", i+1, j+1, a.LibrarySeason)
			}
			if a.ReleaseSeason == b.ReleaseSeason {
				af, al := a.ReleaseRange()
				bf, bl := b.ReleaseRange()
				if animemap.RangesOverlap(af, al, bf, bl) {
					return fmt.Errorf("Rules %d and %d use some of the same episode numbers in release season %d, so a release could mean two episodes.", i+1, j+1, a.ReleaseSeason)
				}
			}
		}
	}
	return nil
}

// ---- the show's page

// GET /api/series/{id}/numbering/custom
func (s *Server) handleGetCustomNumbering(w http.ResponseWriter, r *http.Request) {
	series, ok := s.seriesForNumbering(w, r)
	if !ok {
		return
	}
	c, err := s.animeService().Repo.GetCustom(numberingKey(series))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// PUT /api/series/{id}/numbering/custom {"rules": [...], "names": [...]}
// replaces the show's own numbering; an empty body clears it.
func (s *Server) handlePutCustomNumbering(w http.ResponseWriter, r *http.Request) {
	series, ok := s.seriesForNumbering(w, r)
	if !ok {
		return
	}
	var req animemap.Custom
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "That isn't a valid request.")
		return
	}
	c := req.Clean()
	var seasons map[int]bool
	if eps, err := s.MovieRepo.ListEpisodes(series.ID); err == nil && len(eps) > 0 {
		seasons = map[int]bool{}
		for _, e := range eps {
			seasons[e.Season] = true
		}
	}
	if err := checkCustom(c, seasons); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.animeService().Repo.SaveCustom(numberingKey(series), c); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.forgetNumbering(numberingKey(series))
	writeJSON(w, http.StatusOK, c)
}
