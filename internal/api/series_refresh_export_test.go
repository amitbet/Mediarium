package api

import (
	"context"
	"time"
)

// TestRefreshAllSeries runs the scheduled episode-list refresh now, for
// series_refresh_stop_test.go (package api_test).
func (s *Server) TestRefreshAllSeries() { s.refreshAllSeries(context.Background()) }

// TestSetSeriesRefreshPause sets the wait between two shows in the refresh.
func (s *Server) TestSetSeriesRefreshPause(d time.Duration) { seriesRefreshPause = d }
