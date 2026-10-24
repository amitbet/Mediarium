package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/quality"
)

func torrentRes(title string, seeders, min int) indexers.Result {
	return indexers.Result{
		Title: title, DownloadURL: "https://example.org/" + title + ".torrent", Protocol: indexers.ProtocolTorrent,
		Seeders: seeders, SeedersKnown: true, MinSeeders: min,
	}
}

func TestBelowMinSeeders(t *testing.T) {
	unknown := torrentRes("unknown", 0, 5)
	unknown.SeedersKnown = false
	cached := torrentRes("cached", 1, 5)
	cached.Cached = true
	usenet := torrentRes("usenet", 0, 5)
	usenet.Protocol = indexers.ProtocolUsenet
	flagged := torrentRes("flagged", 1, 5)
	flagged.Protocol = indexers.ProtocolUsenet
	flagged.Torrent = true // the feed itself says it is a torrent

	tests := []struct {
		name string
		res  indexers.Result
		want bool
	}{
		{"below the minimum", torrentRes("a", 4, 5), true},
		{"dead torrent", torrentRes("a", 0, 5), true},
		{"exactly the minimum", torrentRes("a", 5, 5), false},
		{"above the minimum", torrentRes("a", 40, 5), false},
		{"minimum off", torrentRes("a", 0, 0), false},
		{"unknown seeders are not skipped", unknown, false},
		{"cached is not skipped", cached, false},
		{"usenet is not affected", usenet, false},
		{"a torrent in a Usenet-typed feed counts", flagged, true},
		{"a different minimum on another indexer", torrentRes("a", 4, 3), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := belowMinSeeders(tc.res); got != tc.want {
				t.Errorf("belowMinSeeders = %v, want %v", got, tc.want)
			}
		})
	}
}

func titles(rs []indexers.Result) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Title)
	}
	return fmt.Sprint(out)
}

// The automatic searches (hunts, the check for new releases, upgrades, season
// packs, books and music all pass through autoCandidates) leave thin torrents
// out, keep everything else, and count what they dropped.
func TestAutoCandidatesSkipThinTorrents(t *testing.T) {
	unknown := torrentRes("unknown", 0, 5)
	unknown.SeedersKnown = false
	nzb := indexers.Result{Title: "nzb", DownloadURL: "https://example.org/n.nzb", Protocol: indexers.ProtocolUsenet}
	in := []indexers.Result{
		torrentRes("thin", 2, 5),
		torrentRes("enough", 5, 5),
		torrentRes("other-indexer-low-minimum", 2, 2),
		torrentRes("off", 0, 0),
		unknown, nzb,
	}
	s := cacheServer(t, newFakeTorBoxCache(t), "", nil)
	out, dropped := s.keepSeeded(context.Background(), in)
	if want := "[enough other-indexer-low-minimum off unknown nzb]"; titles(out) != want || dropped != 1 {
		t.Errorf("kept %s dropped %d, want %s and 1", titles(out), dropped, want)
	}
	if got := titles(s.autoCandidates(context.Background(), in)); got != "[enough other-indexer-low-minimum off unknown nzb]" {
		t.Errorf("autoCandidates kept %s", got)
	}
	if len(in) != 6 {
		t.Error("the slice that was passed in must not change")
	}
}

func TestKeepSeededNothingThin(t *testing.T) {
	s := cacheServer(t, newFakeTorBoxCache(t), "", nil)
	in := []indexers.Result{torrentRes("a", 9, 5), torrentRes("b", 0, 0)}
	out, dropped := s.keepSeeded(context.Background(), in)
	if dropped != 0 || len(out) != 2 {
		t.Errorf("nothing should be dropped, got %s and %d", titles(out), dropped)
	}
}

// A release the debrid service already has is not skipped, however few
// seeders it has; the thin one it does not have is.
func TestKeepSeededSparesCached(t *testing.T) {
	fake := newFakeTorBoxCache(t, hashA)
	s := cacheServer(t, fake, "torbox", nil)
	in := []indexers.Result{
		torrentRes("cached-thin", 0, 5),
		torrentRes("uncached-thin", 1, 5),
		torrentRes("healthy", 50, 5),
	}
	in[0].InfoHash = hashA
	in[1].InfoHash = hashB
	out, dropped := s.keepSeeded(context.Background(), in)
	if want := "[cached-thin healthy]"; titles(out) != want || dropped != 1 {
		t.Errorf("kept %s dropped %d, want %s and 1", titles(out), dropped, want)
	}
	if len(fake.asked) != 1 || fmt.Sprint(fake.asked[0]) != fmt.Sprint([]string{hashA, hashB}) {
		t.Errorf("only the thin torrents should be asked about, got %v", fake.asked)
	}
}

// With the cache check unavailable the thin torrent is skipped, not let through.
func TestKeepSeededWhenTheServiceIsDown(t *testing.T) {
	fake := newFakeTorBoxCache(t, hashA)
	fake.failing.Store(true)
	s := cacheServer(t, fake, "torbox", nil)
	thin := torrentRes("thin", 1, 5)
	thin.InfoHash = hashA
	out, dropped := s.keepSeeded(context.Background(), []indexers.Result{thin})
	if dropped != 1 || len(out) != 0 {
		t.Errorf("got %s and %d", titles(out), dropped)
	}
}

// A search by hand lists every release and only marks the thin ones; the badge
// goes away on a cached one, and the best match is never a thin one.
func TestManualListMarksThinTorrents(t *testing.T) {
	s := cacheServer(t, newFakeTorBoxCache(t, hashA), "torbox", nil)
	s.offered = offeredURLs{}
	found := []indexers.Result{
		torrentRes("Movie.2024.1080p.WEB-DL.x264-THIN", 1, 5),
		torrentRes("Movie.2024.1080p.WEB-DL.x264-CACHED", 1, 5),
		torrentRes("Movie.2024.720p.WEB-DL.x264-OK", 30, 5),
		{Title: "Movie.2024.1080p.WEB-DL.x264-NZB", DownloadURL: "https://example.org/n.nzb", Protocol: indexers.ProtocolUsenet},
	}
	found[1].InfoHash = hashA
	rows := make([]searchResultPayload, len(found))
	for i, r := range found {
		rows[i] = s.toSearchResultPayload(r)
	}
	if len(rows) != 4 {
		t.Fatal("every release stays in the list")
	}
	want := []bool{true, true, false, false}
	for i := range rows {
		if rows[i].BelowMinSeeders != want[i] {
			t.Errorf("%s: BelowMinSeeders = %v, want %v", rows[i].Title, rows[i].BelowMinSeeders, want[i])
		}
	}
	if rows[0].MinSeeders != 5 {
		t.Errorf("MinSeeders = %d, want 5", rows[0].MinSeeders)
	}

	profile := quality.Presets()[quality.Preset1080p]
	markBestMatch(rows, found, profile)
	if rows[0].Best || rows[1].Best {
		t.Error("a release below the minimum must not be the best match")
	}

	s.markCachedPayloads(context.Background(), rows)
	if !rows[1].Cached || rows[1].BelowMinSeeders {
		t.Errorf("a cached release is not below the minimum: %+v", rows[1])
	}
	if !rows[0].BelowMinSeeders {
		t.Error("an uncached thin release stays marked")
	}
}

func TestCheckMinSeeders(t *testing.T) {
	for _, tc := range []struct {
		n    int
		fail bool
	}{{0, false}, {1, false}, {1000, false}, {-1, true}, {1001, true}} {
		if got := checkMinSeeders(tc.n) != ""; got != tc.fail {
			t.Errorf("checkMinSeeders(%d) refused = %v, want %v", tc.n, got, tc.fail)
		}
	}
}

func TestVerdictCountsThinTorrents(t *testing.T) {
	var v searchVerdict
	v.addThin(0)
	if v.total != 0 || len(v.reasons) != 0 {
		t.Fatal("nothing dropped, nothing noted")
	}
	v.addThin(3)
	if v.total != 3 || v.reasonList() != "3 with fewer seeders than the indexer's minimum" {
		t.Errorf("total %d, reasons %q", v.total, v.reasonList())
	}
}
