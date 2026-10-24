package api

// TestStopTorrents stops every torrent the built-in client runs, so a test can
// end without a seeding torrent keeping its folder open.
func (s *Server) TestStopTorrents() int { return s.torrents.stopAll() }

// TestSeedingDone reports whether the torrent of a queue item reached its
// seeding goal.
func (s *Server) TestSeedingDone(id int64) bool { return s.torrents.seedingDone(id) }

// TestTorrentSkipped lists the files a running torrent leaves out.
func (s *Server) TestTorrentSkipped(id int64) []string {
	if ctl := s.torrents.control(id); ctl != nil {
		return ctl.Skipped()
	}
	return nil
}
