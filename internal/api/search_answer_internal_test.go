package api

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/indexers"
)

// searchedFrom says, for every indexer of a search, what became of it: it had
// releases, it answered with nothing, it did not answer, or it was not asked.
func TestSearchedFrom(t *testing.T) {
	all := []indexers.Instance{
		{ID: 1, Name: "Has Releases", Protocol: indexers.ProtocolTorrent, Enabled: true},
		{ID: 2, Name: "Nothing Here", Protocol: indexers.ProtocolUsenet, Enabled: true},
		{ID: 3, Name: "Broken", Protocol: indexers.ProtocolUsenet, Enabled: true},
		{ID: 4, Name: "Movies Only", Protocol: indexers.ProtocolTorrent, Enabled: true},
		{ID: 5, Name: "Switched Off", Protocol: indexers.ProtocolTorrent},
		{ID: 6, Name: "Resting", Protocol: indexers.ProtocolUsenet, Enabled: true},
		{ID: 7, Name: "Out Of Requests", Protocol: indexers.ProtocolUsenet, Enabled: true},
		{ID: 8, Name: "Slow", Protocol: indexers.ProtocolTorrent, Enabled: true},
	}
	outcomes := []indexers.Outcome{
		{IndexerName: "Has Releases", IndexerID: 1, Protocol: indexers.ProtocolTorrent, Elapsed: 1500 * time.Millisecond, Results: []indexers.Result{{}}},
		{IndexerName: "Nothing Here", IndexerID: 2, Elapsed: 400 * time.Millisecond},
		{IndexerName: "Broken", IndexerID: 3, Err: errors.New("connection refused")},
		{IndexerName: "Resting", Skipped: true, Health: indexers.Health{Failures: 3, Reason: "it timed out", PausedUntil: time.Now().Add(time.Hour)}},
		{IndexerName: "Out Of Requests", Skipped: true, LimitNote: "its daily limit is used up"},
		{IndexerName: "Slow", Err: fmt.Errorf("search: %w", context.DeadlineExceeded), Elapsed: 20 * time.Second},
	}
	counts := map[string]int{"Has Releases": 12}
	got := searchedFrom(all, outcomes, counts, []string{"Movies Only"})

	want := []struct {
		name, kind, status, code string
		count                    int
		ms                       int64
	}{
		{"Has Releases", "torrent", searchFound, "", 12, 1500},
		{"Nothing Here", "usenet", searchEmpty, "", 0, 400},
		{"Broken", "usenet", searchFailed, "unreachable", 0, 0},
		{"Movies Only", "torrent", searchSkipped, "no_category", 0, 0},
		{"Switched Off", "torrent", searchSkipped, "disabled", 0, 0},
		{"Resting", "usenet", searchSkipped, "paused", 0, 0},
		{"Out Of Requests", "usenet", searchSkipped, "daily_limit", 0, 0},
		{"Slow", "torrent", searchFailed, "timeout", 0, 20000},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Name != w.name || g.Kind != w.kind || g.Status != w.status || g.Code != w.code || g.Count != w.count || g.Ms != w.ms {
			t.Errorf("line %d = %+v, want %+v", i, g, w)
		}
		if (w.status == searchSkipped || w.status == searchFailed) && g.Reason == "" {
			t.Errorf("%s: a line that did not answer needs a reason", w.name)
		}
		if (w.status == searchFound || w.status == searchEmpty) && g.Reason != "" {
			t.Errorf("%s: unexpected reason %q", w.name, g.Reason)
		}
	}
	if got[0].IndexerID != 1 {
		t.Errorf("indexer id = %d, want 1", got[0].IndexerID)
	}
}

// An indexer asked several times (a movie's other titles) is one line, and it
// counts as answered when any of its asks was.
func TestSearchedFromSeveralQueriesForOneIndexer(t *testing.T) {
	outcomes := []indexers.Outcome{
		{IndexerName: "Site", Err: errors.New("boom"), Elapsed: time.Second},
		{IndexerName: "Site", Elapsed: time.Second},
	}
	got := searchedFrom(nil, outcomes, nil, nil)
	if len(got) != 1 || got[0].Status != searchEmpty || got[0].Ms != 2000 {
		t.Fatalf("got %+v, want one empty line of 2000 ms", got)
	}
	got = searchedFrom(nil, outcomes[:1], nil, nil)
	if len(got) != 1 || got[0].Status != searchFailed {
		t.Fatalf("got %+v, want one failed line", got)
	}
}
