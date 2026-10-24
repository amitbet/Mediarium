package api

import "time"

// TestForgetLibraryStats drops the kept answer of the Statistics page.
func (s *Server) TestForgetLibraryStats() {
	s.libStats.mu.Lock()
	s.libStats.at = time.Time{}
	s.libStats.mu.Unlock()
}
