package api

import "github.com/rdborg/mediarium/internal/library"

// TestImportSwapping runs the careful swap of a file a person agreed to
// replace, for hand_replace_test.go (package api_test). It reports whether the
// old file went to the recycle bin.
func (s *Server) TestImportSwapping(root, src, dest, label string, movieID int64) (bool, error) {
	_, binned, err := s.importSwapping(root, src, dest, label, movieID, nil)
	return binned, err
}

// SwapRenameForTest replaces the rename the last step of a swap uses and
// returns the function that puts the real one back.
func SwapRenameForTest(f func(oldPath, newPath string) error) (restore func()) {
	old := renameFile
	renameFile = f
	return func() { renameFile = old }
}

// TestImportKeepsFor is importKeepsFor, as the numbers of importKeeps
// (0 nothing, 1 the title is left alone, 2 the quality of the file is unknown).
func (s *Server) TestImportKeepsFor(noUpgrade, currentKnown bool) int {
	return int(s.importKeepsFor(noUpgrade, currentKnown))
}

// TestDiscardWorkDirReport removes a download's working folder and says what
// became of the data.
func (s *Server) TestDiscardWorkDirReport(queueID int64) string {
	return s.discardWorkDirReport(s.workDirFor(queueID), "usenet")
}

// TestMovieSearchKeeps is the note a release list gets for the movie's file.
func (s *Server) TestMovieSearchKeeps(m library.Movie) int {
	_, known := existingTier(m.Quality, m.ReleaseTitle, m.FilePath)
	return int(s.importKeepsFor(m.NoUpgrade, known))
}
