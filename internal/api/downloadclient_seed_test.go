package api_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/api"
	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/settings"
	"github.com/rdborg/mediarium/internal/torrentopts"
)

func floatPtr(v float64) *float64 { return &v }

// The seeding rule of the indexer a release came from reaches the download
// client, whichever way the release was grabbed. A rule used to be lost because
// the queue item never remembered its indexer.
func TestSeedRuleReachesTheClient(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	cases := []struct {
		name           string
		rule           torrentopts.SeedRule
		globalRatio    string
		globalHours    string
		known          bool // the search that found the release is remembered
		automatic      bool
		wantRatio      string // "-" means the field is not sent
		wantMinutes    string
		wantLimitsCall bool
	}{
		{name: "manual grab, hours only", rule: torrentopts.SeedRule{Hours: floatPtr(2)}, known: true, wantRatio: "-1", wantMinutes: "120", wantLimitsCall: true},
		{name: "feed or scheduled grab, hours only", rule: torrentopts.SeedRule{Hours: floatPtr(2)}, known: true, automatic: true, wantRatio: "-1", wantMinutes: "120", wantLimitsCall: true},
		{name: "ratio only", rule: torrentopts.SeedRule{Ratio: floatPtr(1.5)}, known: true, wantRatio: "1.5", wantMinutes: "-1", wantLimitsCall: true},
		{name: "both", rule: torrentopts.SeedRule{Ratio: floatPtr(2), Hours: floatPtr(36)}, known: true, wantRatio: "2", wantMinutes: "2160", wantLimitsCall: true},
		{name: "a fraction of an hour", rule: torrentopts.SeedRule{Hours: floatPtr(0.5)}, known: true, wantRatio: "-1", wantMinutes: "30", wantLimitsCall: true},
		{name: "an empty box follows the global limit", rule: torrentopts.SeedRule{Hours: floatPtr(2)}, globalRatio: "3", known: true, wantRatio: "3", wantMinutes: "120", wantLimitsCall: true},
		{name: "no rule, no global limit: nothing is sent", known: true, wantRatio: "-", wantMinutes: "-"},
		{name: "no rule: the global limit", globalRatio: "2", globalHours: "48", known: true, wantRatio: "2", wantMinutes: "2880", wantLimitsCall: true},
		{name: "release from no known search: the global limit", rule: torrentopts.SeedRule{Hours: floatPtr(2)}, globalRatio: "2", wantRatio: "2", wantMinutes: "-1", wantLimitsCall: true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newClientEnv(t)
			e.qb.state = "error" // the download ends at once, only the add call matters
			e.save(t, map[string]any{})
			if tc.globalRatio != "" {
				if err := e.server.Settings.Set(settings.KeyTorrentSeedRatioLimit, tc.globalRatio, false); err != nil {
					t.Fatal(err)
				}
			}
			if tc.globalHours != "" {
				if err := e.server.Settings.Set(settings.KeyTorrentSeedTimeLimitH, tc.globalHours, false); err != nil {
					t.Fatal(err)
				}
			}
			inst, err := e.server.IndexerRepo.Create(indexers.Instance{Name: "Private", BaseURL: "https://tracker.example/api", Protocol: indexers.ProtocolTorrent, Kind: indexers.KindTorznab, Enabled: true}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.server.TorrentOpts.SetSeedRule(inst.ID, tc.rule); err != nil {
				t.Fatal(err)
			}
			magnet := "magnet:?xt=urn:btih:" + fixtureHash + "&dn=fixture" + strconv.Itoa(i)
			if tc.known {
				e.server.IndexerRepo.Tracker().RecordSearch(inst.ID, []indexers.Result{{Title: "Fixture", DownloadURL: magnet}}, nil)
			}
			m, err := e.server.MovieRepo.Add(library.Movie{TMDBID: 603, Title: "The Fixture Movie", Year: 1999, Monitored: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.server.TestGrabTorrentAs(m, "The.Fixture.Movie.1999.1080p.WEB-DL.x264-FIXTURE", magnet, tc.automatic); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(20 * time.Second)
			for time.Now().Before(deadline) {
				e.qb.mu.Lock()
				n := len(e.qb.adds)
				e.qb.mu.Unlock()
				if n > 0 {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			e.server.TestWaitBackground()
			e.qb.mu.Lock()
			defer e.qb.mu.Unlock()
			if len(e.qb.adds) != 1 {
				t.Fatalf("add calls = %v", e.qb.adds)
			}
			if got := e.qb.adds[0]; got["ratioLimit"] != tc.wantRatio || got["seedingTimeLimit"] != tc.wantMinutes {
				t.Errorf("add sent ratio %q and time %q, want %q and %q", got["ratioLimit"], got["seedingTimeLimit"], tc.wantRatio, tc.wantMinutes)
			}
			if !tc.wantLimitsCall {
				if len(e.qb.limitCalls) != 0 {
					t.Errorf("setShareLimits called: %v", e.qb.limitCalls)
				}
				return
			}
			if len(e.qb.limitCalls) != 1 {
				t.Fatalf("setShareLimits calls = %v", e.qb.limitCalls)
			}
			want := fmt.Sprintf("%s|%s|%s", fixtureHash, tc.wantRatio, tc.wantMinutes)
			got := e.qb.limitCalls[0]
			if have := fmt.Sprintf("%s|%s|%s", got["hashes"], got["ratioLimit"], got["seedingTimeLimit"]); have != want {
				t.Errorf("setShareLimits = %s, want %s", have, want)
			}
		})
	}
}
