package api

import (
	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/library"
)

// TestGrabTorrentAs grabs a torrent release for a movie the way a person
// pressing a button does (automatic false) or the scheduled searches and the
// feed check do (automatic true).
func (s *Server) TestGrabTorrentAs(m library.Movie, releaseTitle, downloadURL string, automatic bool) (int64, error) {
	return s.grabMovie(m, releaseTitle, downloadURL, 1000, indexers.ProtocolTorrent, searchKind(!automatic))
}

// TestRemoveFromClient takes the torrent of a queue item out of the download
// client as stopping the item would (hash "" means look it up by its tag).
func (s *Server) TestRemoveFromClient(queueID int64, hash string, force bool) {
	s.removeClientTorrent(clientRef{queueID: queueID, hash: hash}, force)
}
