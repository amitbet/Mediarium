package indexers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rdborg/mediarium/internal/indexers"
)

func TestClampMinSeeders(t *testing.T) {
	for in, want := range map[int]int{-5: 0, 0: 0, 1: 1, 1000: 1000, 1001: 1000, 99999: 1000} {
		if got := indexers.ClampMinSeeders(in); got != want {
			t.Errorf("ClampMinSeeders(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestMinSeedersIsStored(t *testing.T) {
	repo := newTestRepo(t)
	made, err := repo.Create(indexers.Instance{Name: "A", DefinitionID: "torznab", BaseURL: "https://a.test", Protocol: indexers.ProtocolTorrent, Enabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(made.ID); got.MinSeeders != 0 {
		t.Errorf("a new indexer starts with the minimum off, got %d", got.MinSeeders)
	}
	made.MinSeeders = 7
	if err := repo.Update(made, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(made.ID); got.MinSeeders != 7 {
		t.Errorf("stored minimum = %d, want 7", got.MinSeeders)
	}
	made.MinSeeders = 5000
	if err := repo.Update(made, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(made.ID); got.MinSeeders != 1000 {
		t.Errorf("an out-of-range minimum is held at 1000, got %d", got.MinSeeders)
	}
	other, err := repo.Create(indexers.Instance{Name: "B", DefinitionID: "torznab", BaseURL: "https://b.test", Protocol: indexers.ProtocolTorrent, Enabled: true, MinSeeders: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if other.MinSeeders != 3 {
		t.Errorf("created minimum = %d, want 3", other.MinSeeders)
	}
	if got, _ := repo.Get(made.ID); got.MinSeeders != 1000 {
		t.Error("another indexer's minimum must not change this one's")
	}
}

// Results of a torrent carry the minimum of the indexer; a Usenet post does not.
func TestSearchResultsCarryTheIndexersMinimum(t *testing.T) {
	const torrentFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
<item><title>Heat.1995.1080p.BluRay-GROUP</title><enclosure url="http://t.test/heat.torrent" length="1000" type="application/x-bittorrent" />
<torznab:attr name="seeders" value="2"/></item>
</channel></rss>`
	const nzbFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<item><title>Heat.1995.1080p.BluRay-GROUP</title><enclosure url="http://n.test/heat.nzb" length="1000" type="application/x-nzb" /></item>
</channel></rss>`

	tests := []struct {
		name     string
		feed     string
		protocol indexers.Protocol
		min      int
		want     int
	}{
		{"torrent indexer with a minimum", torrentFeed, indexers.ProtocolTorrent, 5, 5},
		{"torrent indexer with the minimum off", torrentFeed, indexers.ProtocolTorrent, 0, 0},
		{"usenet indexer ignores it", nzbFeed, indexers.ProtocolUsenet, 5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(tc.feed))
			}))
			defer srv.Close()
			inst := indexers.Instance{ID: 1, Name: "X", BaseURL: srv.URL, APIKey: "k", Enabled: true, Protocol: tc.protocol, MinSeeders: tc.min}
			var results []indexers.Result
			for _, o := range indexers.SearchAll(context.Background(), []indexers.Instance{inst}, "heat", nil) {
				results = append(results, o.Results...)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			if results[0].MinSeeders != tc.want {
				t.Errorf("MinSeeders = %d, want %d", results[0].MinSeeders, tc.want)
			}
		})
	}
}
