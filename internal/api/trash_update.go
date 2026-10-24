package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rdborg/mediarium/internal/notify"
	"github.com/rdborg/mediarium/internal/quality"
	"github.com/rdborg/mediarium/internal/settings"
)

// Keeping the custom formats that came from the TRaSH Guides up to date: their
// conditions follow the guides, and so do the scores profiles give them, as
// long as nobody changed those scores by hand. Formats that were pasted or
// made by hand are never touched. It runs every day (or every week) when the
// person switched it on, or on request from the Custom formats card. Every
// check, from the button or the job, is written to the log and the Activity
// page, and the last one is shown on the card.

const (
	trashUpdateEvery = 24 * time.Hour
	trashUpdateRetry = 6 * time.Hour
)

func (s *Server) trashAutoUpdate() bool {
	return s.settingOn(settings.KeyTrashFormatsAuto, false)
}

// trashEveryDays is how many days pass between two reads of the guides by the
// job: 7 when the person chose every week, otherwise 1.
func (s *Server) trashEveryDays() int {
	if v, _ := s.Settings.Get(settings.KeyTrashFormatsEvery); strings.TrimSpace(v) == "7" {
		return 7
	}
	return 1
}

// trashCheck is what one look at the guides found, kept for the card.
type trashCheck struct {
	At time.Time `json:"at"`
	// Error is why the guides could not be read; the counts are then 0.
	Error string `json:"error,omitempty"`
	// Updated formats changed; UpToDate were the same as the guides.
	Updated  int `json:"updated"`
	UpToDate int `json:"upToDate"`
	// Scores is how many profile scores moved with the guides, in Profiles
	// different profiles.
	Scores   int `json:"scores"`
	Profiles int `json:"profiles"`
	// Names are the formats that changed (the first few).
	Names []string `json:"names"`
	// Daily is true when the job made the check, false for the button.
	Daily bool `json:"daily"`
}

// recordTrashCheck keeps the result of a check for the card, and writes it to
// the log and the Activity page.
func (s *Server) recordTrashCheck(res trashUpdateResult, err error, daily bool) trashCheck {
	c := trashCheck{At: time.Now().UTC(), Names: []string{}, Daily: daily}
	if err != nil {
		c.Error = sentence(err.Error())
	} else {
		c.Updated = len(res.Changes)
		c.UpToDate = max(res.Checked-c.Updated, 0)
		seen := map[int64]bool{}
		for _, ch := range res.Changes {
			if len(c.Names) < 10 {
				c.Names = append(c.Names, ch.Name)
			}
			for pid := range ch.scores {
				c.Scores++
				seen[pid] = true
			}
		}
		c.Profiles = len(seen)
	}
	if b, jerr := json.Marshal(c); jerr == nil {
		_ = s.Settings.Set(settings.KeyTrashFormatsCheck, string(b), false)
	}
	switch {
	case err != nil:
		slog.Warn("custom formats: couldn't check the TRaSH Guides", "daily", daily, "err", err)
		_ = s.QueueRepo.LogActivity(0, "update", fmt.Sprintf("Couldn't check the TRaSH Guides: %s", c.Error))
		// A check that failed is always worth a message.
		s.sendNotification(notify.ComposeTrashSyncFailed(c.Error, daily, s.publicLinks(), time.Now()))
	case c.Updated == 0:
		slog.Info("custom formats: checked the TRaSH Guides, nothing to update", "daily", daily, "checked", res.Checked)
		_ = s.QueueRepo.LogActivity(0, "update", fmt.Sprintf("Checked the TRaSH Guides: nothing to update, %d formats are up to date.", c.UpToDate))
		if s.settingOn(settings.KeyTrashSyncAlways, false) {
			s.sendNotification(notify.ComposeTrashSyncNothing(c.UpToDate, daily, s.publicLinks(), time.Now()))
		}
	default:
		slog.Info("custom formats: checked the TRaSH Guides", "daily", daily, "updated", c.Updated, "upToDate", c.UpToDate, "scoresChanged", c.Scores, "profiles", c.Profiles, "formats", strings.Join(c.Names, ", "))
		_ = s.QueueRepo.LogActivity(0, "update", fmt.Sprintf("Checked the TRaSH Guides: %d formats updated, %d already up to date, %d profile scores changed.", c.Updated, c.UpToDate, c.Scores))
		// Only to the notifications that ticked the event.
		if ev, ok := notify.ComposeTrashSync(c.Names, c.Updated, c.Scores, c.Profiles, daily, s.publicLinks(), time.Now()); ok {
			s.sendNotification(ev)
		}
	}
	return c
}

// lastTrashCheck is the check kept by recordTrashCheck, if there is one.
func (s *Server) lastTrashCheck() *trashCheck {
	v, _ := s.Settings.Get(settings.KeyTrashFormatsCheck)
	if v == "" {
		return nil
	}
	var c trashCheck
	if json.Unmarshal([]byte(v), &c) != nil || c.At.IsZero() {
		return nil
	}
	if c.Names == nil {
		c.Names = []string{}
	}
	return &c
}

// trashFormatChange is one format the guides changed.
type trashFormatChange struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Conditions is true when the conditions (patterns, sizes...) differ.
	Conditions bool `json:"conditions"`
	// ScoreFrom and ScoreTo are TRaSH's score before and now; they differ only
	// when ScoreChanged.
	ScoreChanged bool `json:"scoreChanged"`
	ScoreFrom    int  `json:"scoreFrom"`
	ScoreTo      int  `json:"scoreTo"`
	// Profiles names the quality profiles whose score follows the change: the
	// ones that still have the old TRaSH score.
	Profiles []string `json:"profiles"`

	app    quality.FormatApp
	specs  []quality.FormatSpec
	scores map[int64]int // profile id -> new score
}

type trashUpdateResult struct {
	Changes []trashFormatChange `json:"changes"`
	// Checked is how many formats from the guides were compared.
	Checked int `json:"checked"`

	// Check is what the Custom formats card shows afterwards; it is set when an
	// update ran.
	Check *trashCheck `json:"check,omitempty"`

	// quiet are formats whose conditions are the same and whose remembered
	// TRaSH score moved from "not known" (0): nothing to tell, but the new
	// score is kept for next time.
	quiet []trashFormatChange
}

// trashChanges compares the formats that came from the guides with the guides
// as they are now. It changes nothing.
func (s *Server) trashChanges(ctx context.Context) (trashUpdateResult, error) {
	res := trashUpdateResult{Changes: []trashFormatChange{}}
	formats, err := s.FormatRepo.List()
	if err != nil {
		return res, fmt.Errorf("list custom formats: %w", err)
	}
	profiles, err := s.QualityRepo.List()
	if err != nil {
		return res, fmt.Errorf("list quality profiles: %w", err)
	}
	live := map[quality.FormatApp]map[string]quality.ImportedFormat{}
	for _, f := range formats {
		if f.TrashID == "" || f.TrashScore == nil {
			continue
		}
		if _, ok := live[f.App]; !ok {
			all, err := s.trashFormats(ctx, f.App)
			if err != nil {
				return res, err
			}
			live[f.App] = all
		}
		now, ok := live[f.App][f.TrashID]
		if !ok {
			continue // the guides dropped it: it stays as it is
		}
		res.Checked++
		ch := trashFormatChange{ID: f.ID, Name: f.Name, app: now.Format.App, specs: now.Format.Specs, scores: map[int64]int{}, Profiles: []string{}}
		ch.Conditions = !specsEqual(f.Specs, now.Format.Specs) || f.App != now.Format.App
		ch.ScoreFrom, ch.ScoreTo = *f.TrashScore, now.DefaultScore
		ch.ScoreChanged = ch.ScoreFrom != ch.ScoreTo
		if ch.ScoreChanged {
			for _, p := range profiles {
				if sc, changed := quality.RetargetScore(p.FormatScores, f.ID, ch.ScoreFrom, ch.ScoreTo); changed {
					ch.scores[p.ID] = sc
					ch.Profiles = append(ch.Profiles, p.Name)
				}
			}
		}
		switch {
		case ch.Conditions || (ch.ScoreChanged && ch.ScoreFrom != 0):
			res.Changes = append(res.Changes, ch)
		case ch.ScoreChanged:
			res.quiet = append(res.quiet, ch)
		}
	}
	return res, nil
}

// applyTrashChanges re-reads the guides and writes what changed.
func (s *Server) applyTrashChanges(ctx context.Context) (trashUpdateResult, error) {
	res, err := s.trashChanges(ctx)
	if err != nil {
		return res, err
	}
	for _, ch := range append(append([]trashFormatChange{}, res.Changes...), res.quiet...) {
		if err := s.FormatRepo.UpdateFromTrash(ch.ID, ch.app, ch.specs, ch.ScoreTo); err != nil {
			return res, fmt.Errorf("update custom format %q: %w", ch.Name, err)
		}
	}
	// One write per profile, whatever the number of formats.
	byProfile := map[int64]map[int64]int{}
	for _, ch := range res.Changes {
		for pid, sc := range ch.scores {
			if byProfile[pid] == nil {
				byProfile[pid] = map[int64]int{}
			}
			byProfile[pid][ch.ID] = sc
		}
	}
	for pid, scores := range byProfile {
		p, err := s.QualityRepo.Get(pid)
		if err != nil {
			continue // deleted while this ran
		}
		for fid, sc := range scores {
			if sc == 0 {
				delete(p.FormatScores, fid)
			} else {
				p.FormatScores[fid] = sc
			}
		}
		if _, err := s.QualityRepo.Update(p); err != nil {
			return res, fmt.Errorf("update quality profile %q: %w", p.Name, err)
		}
	}
	return res, nil
}

func writeTrashUpdateError(w http.ResponseWriter, err error) {
	writeError(w, http.StatusBadGateway, sentence(err.Error()))
}

// handleTrashUpdatePreview lists what an update from the guides would change.
func (s *Server) handleTrashUpdatePreview(w http.ResponseWriter, r *http.Request) {
	res, err := s.trashChanges(r.Context())
	if err != nil {
		writeTrashUpdateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleTrashUpdate updates the formats that came from the guides.
func (s *Server) handleTrashUpdate(w http.ResponseWriter, r *http.Request) {
	res, err := s.applyTrashChanges(r.Context())
	check := s.recordTrashCheck(res, err, false)
	if err != nil {
		writeTrashUpdateError(w, err)
		return
	}
	res.Check = &check
	_ = s.Settings.Set(settings.KeyTrashFormatsLast, time.Now().UTC().Format(time.RFC3339), false)
	writeJSON(w, http.StatusOK, res)
}

// trashUpdateStatus is what the card shows under the switch.
type trashUpdateStatus struct {
	Last *trashCheck `json:"last"`
	// Next is about when the job reads the guides again; empty while the
	// switch is off.
	Next *time.Time `json:"next,omitempty"`
}

// handleTrashUpdateStatus tells when the guides were last checked and when the
// next check is due.
func (s *Server) handleTrashUpdateStatus(w http.ResponseWriter, r *http.Request) {
	st := trashUpdateStatus{Last: s.lastTrashCheck()}
	if s.trashAutoUpdate() {
		next := time.Now().UTC()
		if v, _ := s.Settings.Get(settings.KeyTrashFormatsLast); v != "" {
			if last, err := time.Parse(time.RFC3339, v); err == nil {
				next = last.Add(time.Duration(s.trashEveryDays()) * trashUpdateEvery)
			}
		}
		st.Next = &next
	}
	writeJSON(w, http.StatusOK, st)
}

// trashFormatsJob reads the guides every day (or every week, as chosen) when
// the person asked for it. The job wakes up every hour; the stored time of the
// last read keeps it to one read per period, and a failed read is tried again
// after a few hours.
func (s *Server) trashFormatsJob(ctx context.Context) {
	if !s.trashAutoUpdate() {
		return
	}
	now := time.Now()
	every := time.Duration(s.trashEveryDays()) * trashUpdateEvery
	if v, _ := s.Settings.Get(settings.KeyTrashFormatsLast); v != "" {
		if last, err := time.Parse(time.RFC3339, v); err == nil && now.Sub(last) < every {
			return
		}
	}
	stamp := now
	res, err := s.applyTrashChanges(ctx)
	s.recordTrashCheck(res, err, true)
	if err != nil {
		stamp = now.Add(-(every - trashUpdateRetry))
	}
	_ = s.Settings.Set(settings.KeyTrashFormatsLast, stamp.UTC().Format(time.RFC3339), false)
}
