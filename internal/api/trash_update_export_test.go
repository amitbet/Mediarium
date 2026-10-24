package api

import "context"

// TestTrashFormatsJob runs the daily TRaSH update job once (package api_test
// cannot reach it).
func (s *Server) TestTrashFormatsJob(ctx context.Context) { s.trashFormatsJob(ctx) }
