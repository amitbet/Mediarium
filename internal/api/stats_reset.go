package api

import (
	"log/slog"
	"net/http"
	"time"
)

// handleResetStats answers POST /api/stats/reset: the download totals on the
// Statistics page and the dashboard start again from zero. Only the counted
// totals go (grabs, finished and failed downloads, their sizes, the Usenet
// share, the downloads by month and by day). The library, the download
// history, the library size snapshots and every file stay as they are.
func (s *Server) handleResetStats(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't reset the statistics. Try again.")
		return
	}
	defer tx.Rollback() //nolint:errcheck // a no-op once committed
	if _, err := tx.Exec(`DELETE FROM download_stats`); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't reset the statistics. Try again.")
		return
	}
	if _, err := tx.Exec(`UPDATE stats_daily SET completed = 0, failed = 0, download_bytes = 0`); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't reset the statistics. Try again.")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "Couldn't reset the statistics. Try again.")
		return
	}
	// The page's answer is kept for a few minutes: forget it, so the next
	// look shows the zeros.
	s.libStats.mu.Lock()
	s.libStats.at = time.Time{}
	s.libStats.mu.Unlock()
	slog.Info("statistics: download totals reset")
	writeJSON(w, http.StatusOK, map[string]bool{"reset": true})
}
