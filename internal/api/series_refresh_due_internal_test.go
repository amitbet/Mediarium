package api

import (
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/library"
)

func TestSeriesRefreshDue(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	tests := []struct {
		name string
		sr   library.Series
		want bool
	}{
		{"never read", library.Series{Monitored: true}, true},
		{"status not kept yet", library.Series{TMDBStatus: "", StatusCheckedAt: ago(time.Hour)}, true},
		{"ended, read yesterday", library.Series{TMDBStatus: "Ended", StatusCheckedAt: ago(24 * time.Hour), Monitored: true}, false},
		{"ended, read a week ago", library.Series{TMDBStatus: "Ended", StatusCheckedAt: ago(8 * 24 * time.Hour)}, true},
		{"canceled, read yesterday", library.Series{TMDBStatus: "Canceled", StatusCheckedAt: ago(24 * time.Hour)}, false},
		{"running and monitored", library.Series{TMDBStatus: "Returning Series", StatusCheckedAt: ago(time.Minute), Monitored: true}, true},
		{"running, not monitored, read an hour ago", library.Series{TMDBStatus: "Returning Series", StatusCheckedAt: ago(time.Hour)}, false},
		{"running, not monitored, read a day ago", library.Series{TMDBStatus: "Returning Series", StatusCheckedAt: ago(25 * time.Hour)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := seriesRefreshDue(tt.sr, now); got != tt.want {
				t.Errorf("seriesRefreshDue = %v, want %v", got, tt.want)
			}
		})
	}
}
