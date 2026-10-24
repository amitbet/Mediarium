package api

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/settings"
)

// A site that goes through FlareSolverr gets longer than a plain one, because
// solving a Cloudflare challenge takes 20 to 60 seconds.
func TestIndexerTestBudget(t *testing.T) {
	tests := []struct {
		name  string
		saved string
		inst  indexers.Instance
		min   time.Duration
		max   time.Duration
	}{
		{"plain site", "", indexers.Instance{Kind: indexers.KindCardigann}, 0, 40 * time.Second},
		{"site with FlareSolverr", "http://flaresolverr:8191", indexers.Instance{Kind: indexers.KindCardigann}, 75 * time.Second, 5 * time.Minute},
		{"newznab ignores FlareSolverr", "http://flaresolverr:8191", indexers.Instance{Kind: indexers.KindNewznab}, 0, 40 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := OpenTestDB(filepath.Join(t.TempDir(), "app.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			s := &Server{Settings: settings.New(db, nil)}
			if tc.saved != "" {
				if err := s.Settings.Set(settings.KeyFlareSolverrURL, tc.saved, false); err != nil {
					t.Fatal(err)
				}
			}
			got := s.indexerTestBudget(tc.inst)
			if got < tc.min || got > tc.max {
				t.Fatalf("budget = %v, want between %v and %v", got, tc.min, tc.max)
			}
		})
	}
}

// A check that was given extra time for FlareSolverr says so when it runs out.
func TestRunProbeTimeoutMessage(t *testing.T) {
	old := probeTimeout
	probeTimeout = 30 * time.Millisecond
	defer func() { probeTimeout = old }()
	slow := func() connTestResult {
		time.Sleep(time.Second)
		return connTestResult{OK: true}
	}
	err := runProbe(context.Background(), 100*time.Millisecond, slow)
	if err == nil || !strings.Contains(err.Error(), "FlareSolverr") || !strings.Contains(err.Error(), "try again") {
		t.Fatalf("a long check should name FlareSolverr, got %v", err)
	}
	err = runProbe(context.Background(), probeTimeout, slow)
	if err == nil || err.Error() != "the check timed out" {
		t.Fatalf("a normal check keeps its short message, got %v", err)
	}
}
