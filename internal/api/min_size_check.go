package api

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/quality"
	"github.com/rdborg/mediarium/internal/settings"
)

// The built-in minimum size per quality skips fakes and samples. It is on
// unless someone switches it off (short shows and small encodes are real
// releases far below the table). The pickers are plain functions, so the
// switch is kept here, like the preferred protocol.
var minSizeOff atomic.Bool

func minSizeCheckOn() bool { return !minSizeOff.Load() }

func setMinSizeCheck(on bool) { minSizeOff.Store(!on) }

// loadMinSizeCheck reads the saved switch.
func (s *Server) loadMinSizeCheck() {
	raw, _ := s.Settings.Get(settings.KeyMinSizeCheck)
	setMinSizeCheck(raw != "0")
}

// builtinSizeOK is quality.SizePlausible when the switch is on, and always
// true when it is off. minutes is the episode runtime, 0 when unknown.
func builtinSizeOK(t quality.Tier, size int64, episodes int, minutes float64) bool {
	if !minSizeCheckOn() {
		return true
	}
	return quality.SizePlausibleRuntime(t, size, episodes, minutes)
}

// knownEpisodeMinutes is the episode length of a show when it is already
// known (from TMDB, kept by episodeRuntime), 0 otherwise. It never asks TMDB:
// the picker runs in the middle of a search.
func knownEpisodeMinutes(series library.Series) float64 {
	if series.TMDBID == 0 {
		return 0
	}
	if v, ok := runtimeCache.Load("tv:" + strconv.Itoa(series.TMDBID)); ok {
		if e := v.(runtimeEntry); time.Now().Before(e.until) {
			return e.minutes
		}
	}
	return 0
}

// warmEpisodeRuntime makes sure the runtime of a show is known before its
// releases are picked.
func (s *Server) warmEpisodeRuntime(series library.Series, results []indexers.Result) {
	if !minSizeCheckOn() || len(results) == 0 || series.TMDBID == 0 {
		return
	}
	_ = s.episodeRuntime(context.Background(), series)
}

type minSizePayload struct {
	Enabled bool `json:"enabled"`
}

// handleGetMinSize says whether the built-in minimum size check is on.
func (s *Server) handleGetMinSize(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, minSizePayload{Enabled: minSizeCheckOn()})
}

// handlePutMinSize switches the built-in minimum size check: {"enabled": false} turns it off.
func (s *Server) handlePutMinSize(w http.ResponseWriter, r *http.Request) {
	var in minSizePayload
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "That isn't a valid setting.")
		return
	}
	val := "1"
	if !in.Enabled {
		val = "0"
	}
	if err := s.Settings.Set(settings.KeyMinSizeCheck, val, false); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setMinSizeCheck(in.Enabled)
	writeJSON(w, http.StatusOK, in)
}
