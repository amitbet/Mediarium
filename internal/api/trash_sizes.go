package api

import (
	"net/http"

	"github.com/rdborg/mediarium/internal/quality"
)

// "Apply TRaSH sizes" on the size limits card: the sizes the TRaSH Guides
// recommend per quality, read from their repository the same way the custom
// formats are (and kept for six hours), mapped onto the size table. Nothing is
// saved here; the page fills the table with the answer and the person saves.

// trashSizeLists are the size files the TRaSH Guides publish, by the name the
// page asks for. Radarr's lists belong to the movie size table and Sonarr's to
// the show table (kind says which).
var trashSizeLists = map[string]struct {
	app  quality.FormatApp
	file string
	kind string
}{
	"radarr-movie":         {quality.AppRadarr, "movie.json", "movie"},
	"radarr-anime":         {quality.AppRadarr, "anime.json", "movie"},
	"radarr-sqp-streaming": {quality.AppRadarr, "sqp-streaming.json", "movie"},
	"radarr-sqp-uhd":       {quality.AppRadarr, "sqp-uhd.json", "movie"},
	"sonarr-series":        {quality.AppSonarr, "series.json", "tv"},
	"sonarr-anime":         {quality.AppSonarr, "anime.json", "tv"},
}

type trashSizeRow struct {
	Quality string  `json:"quality"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"` // 0 = no upper limit
}

type trashSizePayload struct {
	List string         `json:"list"`
	Kind string         `json:"kind"` // the size table it is for: "movie" or "tv"
	Rows []trashSizeRow `json:"rows"`
	// Skipped names the TRaSH qualities that have no row in Mediarium.
	Skipped []string `json:"skipped"`
}

// handleTrashSizes is GET /api/quality/size-limits/trash/{list}.
func (s *Server) handleTrashSizes(w http.ResponseWriter, r *http.Request) {
	list := r.PathValue("list")
	src, ok := trashSizeLists[list]
	if !ok {
		writeError(w, http.StatusBadRequest, "That isn't a list of TRaSH sizes. Reload the page and try again.")
		return
	}
	files, err := s.trashGuides().folder(r.Context(), src.app, "quality-size")
	if err != nil {
		writeError(w, http.StatusBadGateway, sentence(err.Error()))
		return
	}
	raw, ok := files[src.file]
	if !ok {
		writeError(w, http.StatusNotFound, "The TRaSH Guides don't have that list of sizes any more.")
		return
	}
	parsed, err := quality.ParseTrashSizes(raw)
	if err != nil {
		writeError(w, http.StatusBadGateway, sentence(err.Error()))
		return
	}
	out := trashSizePayload{List: list, Kind: src.kind, Rows: []trashSizeRow{}, Skipped: parsed.Skipped}
	if out.Skipped == nil {
		out.Skipped = []string{}
	}
	for _, tier := range quality.SizeTiers() {
		if l, ok := parsed.Limits[tier]; ok {
			out.Rows = append(out.Rows, trashSizeRow{Quality: string(tier), Min: l.Min, Max: l.Max})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
