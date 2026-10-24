package api

import (
	"encoding/json"
	"testing"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/quality"
	"github.com/rdborg/mediarium/internal/settings"
)

// Movies and shows have a size table each: a release is judged by the table of
// its own kind, whatever the other one says.
func TestSizeTablesAreCheckedPerKind(t *testing.T) {
	s := newAuditServer(t)
	save := func(key string, enabled bool, tier quality.Tier, l quality.SizeLimit) {
		t.Helper()
		raw, err := json.Marshal(quality.SizeLimits{Enabled: enabled, Limits: map[quality.Tier]quality.SizeLimit{tier: l}})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Settings.Set(key, string(raw), false); err != nil {
			t.Fatal(err)
		}
	}
	// 4 GB for 100 minutes is about 41 MB a minute.
	res := []indexers.Result{{Title: "Example.Movie.2024.1080p.WEB-DL.x264-GRP", SizeBytes: 4 << 30}}
	runtime := func() float64 { return 100 }
	noEpisodes := func(indexers.Result) int { return 0 }
	kept := func(kind string) int {
		out, _ := s.applySizeTable(kind, res, runtime, noEpisodes)
		return len(out)
	}

	// Nothing saved: both tables are off.
	if kept("movie") != 1 || kept("tv") != 1 {
		t.Fatal("an unset table refused a release")
	}
	// Movies capped at 20 MB a minute, shows not switched on.
	save(settings.KeySizeLimits, true, quality.TierWebDL1080p, quality.SizeLimit{Min: 4, Max: 20})
	if kept("movie") != 0 {
		t.Error("the movie table did not refuse a movie release that is too large")
	}
	if kept("tv") != 1 {
		t.Error("the movie table refused a show release")
	}
	// Now shows get their own cap, movies are loosened.
	save(settings.KeySizeLimits, true, quality.TierWebDL1080p, quality.SizeLimit{Min: 4, Max: 100})
	save(settings.KeySizeLimitsTV, true, quality.TierWebDL1080p, quality.SizeLimit{Min: 4, Max: 20})
	if kept("movie") != 1 {
		t.Error("the show table refused a movie release")
	}
	if kept("tv") != 0 {
		t.Error("the show table did not refuse a show release that is too large")
	}
	// The same split in the reasons listed for an interactive search.
	profile := quality.Profile{Name: "Any"}
	if got := s.sizeRejections(s.sizeLimitsFor("movie"), 100, profile, res[0].Title, res[0].SizeBytes, 0); len(got) != 0 {
		t.Errorf("movie reasons = %v", got)
	}
	if got := s.sizeRejections(s.sizeLimitsFor("tv"), 100, profile, res[0].Title, res[0].SizeBytes, 0); len(got) != 1 {
		t.Errorf("show reasons = %v", got)
	}
}
