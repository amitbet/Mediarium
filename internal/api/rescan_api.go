package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
)

// handleRescanMovie looks at a movie's file again, for the Rescan files button.
func (s *Server) handleRescanMovie(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "That isn't a valid movie ID.")
		return
	}
	m, err := s.MovieRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "That movie isn't in your library.")
		return
	}
	if movieWantsAFile(m) {
		rep, err := s.attachMovieFile(m)
		if errors.Is(err, errNoCopiesFolder) {
			writeError(w, http.StatusConflict, "This is a second copy and no folder for second copies is set, so there is nowhere to look. Choose one in Settings first.")
			return
		}
		if err != nil {
			slog.Warn("rescan: couldn't add a movie's file", "movie", m.Title, "err", err)
			writeError(w, http.StatusInternalServerError, "Found a video file but couldn't save it to the library. Check the Mediarium log for details.")
			return
		}
		s.kickMediaInfo() // read what the file that was added holds
		writeJSON(w, http.StatusOK, rep)
		return
	}
	writeJSON(w, http.StatusOK, s.rescanMovieFiles(id, true))
}

// handleRescanSeries looks at every downloaded file of a show again.
func (s *Server) handleRescanSeries(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "That isn't a valid show ID.")
		return
	}
	sr, err := s.MovieRepo.GetSeries(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "That show isn't in your library.")
		return
	}
	rep := s.rescanSeriesFiles(id, true)
	s.attachHandAddedEpisodes(sr, &rep)
	s.kickMediaInfo() // read what the files that were added hold
	writeJSON(w, http.StatusOK, rep)
}
