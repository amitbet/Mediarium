package api

import (
	"context"
	"time"
)

// RunMediaInfoJob runs the background job that reads the library files' details once.
func (s *Server) RunMediaInfoJob(ctx context.Context) { s.mediaInfoJob(ctx) }

// SetMediaInfoSettle sets how old a file must be before the job reads it (so a
// file a test just wrote can be read), and returns a function that puts the old value back.
func SetMediaInfoSettle(d time.Duration) (restore func()) {
	old := mediaInfoSettle
	mediaInfoSettle = d
	return func() { mediaInfoSettle = old }
}
