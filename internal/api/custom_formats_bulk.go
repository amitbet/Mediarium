package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"github.com/rdborg/mediarium/internal/quality"
)

// Giving many custom formats a score in many quality profiles at once, and
// the TRaSH score a format would get.

// trashDefaultScores is the TRaSH score of each format that came from the
// guides and that the guides score: read from the guides when they can be
// reached, otherwise the score remembered from the last time they were read.
// A format without a TRaSH score isn't in the map.
func (s *Server) trashDefaultScores(ctx context.Context, formats []quality.CustomFormat) map[int64]int {
	live := map[quality.FormatApp]map[string]quality.ImportedFormat{}
	out := map[int64]int{}
	for _, f := range formats {
		if f.TrashID == "" {
			continue
		}
		if _, tried := live[f.App]; !tried {
			all, err := s.trashFormats(ctx, f.App)
			if err != nil {
				all = nil
			}
			live[f.App] = all
		}
		score, known := 0, false
		if lf, ok := live[f.App][f.TrashID]; ok {
			score, known = lf.DefaultScore, true
		} else if f.TrashScore != nil {
			score, known = *f.TrashScore, true
		}
		if known {
			out[f.ID] = score
		}
	}
	return out
}

// formatsByID loads the formats with these ids; ok is false when one is gone.
func (s *Server) formatsByID(ids []int64) ([]quality.CustomFormat, bool, error) {
	all, err := s.FormatRepo.List()
	if err != nil {
		return nil, false, err
	}
	byID := map[int64]quality.CustomFormat{}
	for _, f := range all {
		byID[f.ID] = f
	}
	var out []quality.CustomFormat
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		f, ok := byID[id]
		if !ok {
			return nil, false, nil
		}
		out = append(out, f)
	}
	return out, true, nil
}

type formatIDsRequest struct {
	FormatIDs []int64 `json:"formatIds"`
}

const maxBulkFormats = 1000

// handleTrashDefaultScores says what the TRaSH Guides score each of these
// formats, for the form that adds formats to a profile.
func (s *Server) handleTrashDefaultScores(w http.ResponseWriter, r *http.Request) {
	var req formatIDsRequest
	if err := decodeJSON(r, &req); err != nil || len(req.FormatIDs) == 0 || len(req.FormatIDs) > maxBulkFormats {
		writeError(w, http.StatusBadRequest, "Pick the custom formats first.")
		return
	}
	formats, ok, err := s.formatsByID(req.FormatIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "One of those custom formats no longer exists. Reload the page and try again.")
		return
	}
	scores := []formatScorePayload{}
	for id, sc := range s.trashDefaultScores(r.Context(), formats) {
		scores = append(scores, formatScorePayload{FormatID: id, Score: sc})
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].FormatID < scores[j].FormatID })
	writeJSON(w, http.StatusOK, map[string]any{"scores": scores})
}

type formatAssignRequest struct {
	FormatIDs  []int64 `json:"formatIds"`
	ProfileIDs []int64 `json:"profileIds"`
	// Mode is "trash" (each format gets the score the TRaSH Guides give it) or
	// "score" (every format gets Score; 0 takes the format out of the profile).
	Mode  string `json:"mode"`
	Score int    `json:"score"`
	// ReplaceScores says a format the profile already has gets the new score.
	// Left out, such a format is skipped and keeps the score it has.
	ReplaceScores bool `json:"replaceScores"`
}

// profileScoreChange says what adding formats did in one quality profile.
type profileScoreChange struct {
	ProfileID int64  `json:"profileId"`
	Name      string `json:"name"`
	// Added are formats the profile did not have, now scored.
	Added int `json:"added"`
	// AlreadyThere are formats the profile had and that were left alone.
	AlreadyThere int `json:"alreadyThere"`
	// Replaced are formats the profile had whose score was changed because
	// the person asked for that.
	Replaced int `json:"replaced"`
	// Removed are formats taken out (a typed score of 0).
	Removed int `json:"removed"`
}

// applyFormatScores gives the formats their scores in the profile and counts
// what happened. A format the profile already has is only touched when
// replace is set. takeOutOnZero makes a score of 0 take the format out of the
// profile (always, for a format that is in it): that is the "typed 0" case.
func applyFormatScores(p *quality.Profile, scores map[int64]int, replace, takeOutOnZero bool) profileScoreChange {
	ch := profileScoreChange{ProfileID: p.ID, Name: p.Name}
	if p.FormatScores == nil {
		p.FormatScores = map[int64]int{}
	}
	for id, sc := range scores {
		_, has := p.FormatScores[id]
		switch {
		case takeOutOnZero && sc == 0:
			if has {
				delete(p.FormatScores, id)
				ch.Removed++
			}
		case !has:
			p.FormatScores[id] = sc
			ch.Added++
		case replace:
			if p.FormatScores[id] != sc {
				p.FormatScores[id] = sc
				ch.Replaced++
			} else {
				ch.AlreadyThere++
			}
		default:
			ch.AlreadyThere++
		}
	}
	return ch
}

type formatAssignResult struct {
	Profiles int `json:"profiles"`
	// Scored is how many scores were written in total: formats added or
	// replaced, over all the profiles.
	Scored int `json:"scored"`
	// Changes says per profile what was added, skipped, replaced or removed.
	Changes []profileScoreChange `json:"changes"`
	// Unscored names the formats the TRaSH Guides give no score; with mode
	// "trash" they are left as they are.
	Unscored []string `json:"unscored"`
}

// handleAssignCustomFormats gives the chosen formats a score in each of the
// chosen quality profiles: TRaSH's own score for each, or one score typed in.
// A format a profile already has is skipped and keeps its score, unless
// replaceScores is set.
func (s *Server) handleAssignCustomFormats(w http.ResponseWriter, r *http.Request) {
	var req formatAssignRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request. Reload the page and try again.")
		return
	}
	if len(req.FormatIDs) == 0 || len(req.FormatIDs) > maxBulkFormats {
		writeError(w, http.StatusBadRequest, "Pick the custom formats first.")
		return
	}
	if len(req.ProfileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "Pick at least one quality profile.")
		return
	}
	if req.Mode != "trash" && req.Mode != "score" {
		writeError(w, http.StatusBadRequest, "Choose TRaSH's score or type a score.")
		return
	}
	if req.Mode == "score" && (req.Score < -10000 || req.Score > 10000) {
		writeError(w, http.StatusBadRequest, "A score is a whole number between -10,000 and 10,000.")
		return
	}
	formats, ok, err := s.formatsByID(req.FormatIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "One of those custom formats no longer exists. Reload the page and try again.")
		return
	}
	scores := map[int64]int{}
	res := formatAssignResult{Unscored: []string{}, Changes: []profileScoreChange{}}
	if req.Mode == "trash" {
		scores = s.trashDefaultScores(r.Context(), formats)
		for _, f := range formats {
			if _, ok := scores[f.ID]; !ok {
				res.Unscored = append(res.Unscored, f.Name)
			}
		}
	} else {
		for _, f := range formats {
			scores[f.ID] = req.Score
		}
	}

	// Check every profile before changing any, so a stale page changes nothing.
	var profiles []quality.Profile
	seen := map[int64]bool{}
	for _, id := range req.ProfileIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		p, err := s.QualityRepo.Get(id)
		if err != nil {
			writeProfileError(w, err)
			return
		}
		profiles = append(profiles, p)
	}
	for _, p := range profiles {
		// A typed 0 takes the format out; TRaSH's own 0 adds it with a
		// score of 0.
		ch := applyFormatScores(&p, scores, req.ReplaceScores, req.Mode == "score")
		res.Scored += ch.Added + ch.Replaced
		res.Changes = append(res.Changes, ch)
		if ch.Added+ch.Replaced+ch.Removed == 0 {
			res.Profiles++
			continue
		}
		if _, err := s.QualityRepo.Update(p); err != nil {
			writeProfileError(w, err)
			return
		}
		res.Profiles++
	}
	writeJSON(w, http.StatusOK, res)
}

// specsEqual reports whether two formats have the same conditions.
func specsEqual(a, b []quality.FormatSpec) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
