package api

import (
	"testing"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/quality"
)

// GitHub #91: a profile that prefers "x265-FuN = 100" and "x265 = 50" took
// the other release, and listing the terms the other way round seemed to fix
// it. The order of the preferred terms never changes a pick (the score is the
// sum of every term that matches). These tests run the real pickers over both
// orders and a spread of situations, so an order effect shows up as a failure.

const (
	i91Bdrip   = "Some.Movie.2020.BDRip.DL.1080p.x265.AAC-2BA"
	i91Fun     = "Some.Movie.2020.DL.AC3.1080p.BluRay.x265-FuN"
	i91BdripDE = "Some.Movie.2020.German.BDRip.DL.1080p.x265.AAC-2BA"
	i91PlainDE = "Some.Movie.2020.German.1080p.BluRay.x265-FuN"
	i91FunDE   = "Some.Movie.2020.German.DL.AC3.1080p.BluRay.x265-FuN"
	i91Web     = "Some.Movie.2020.DL.1080p.WEB-DL.x265-FuN"
)

func i91Orders() map[string][]quality.Preferred {
	return map[string][]quality.Preferred{
		"fun first":  {{Term: "x265-FuN", Score: 100}, {Term: "x265", Score: 50}},
		"x265 first": {{Term: "x265", Score: 50}, {Term: "x265-FuN", Score: 100}},
	}
}

func TestPreferredTermOrderDoesNotChangeThePick(t *testing.T) {
	torrent := func(title string, seeders int, known bool) indexers.Result {
		return indexers.Result{Title: title, DownloadURL: "http://x/" + title, Protocol: indexers.ProtocolTorrent, Seeders: seeders, SeedersKnown: known, IndexerName: "t"}
	}
	tests := []struct {
		name     string
		language string
		results  []indexers.Result
		want     string
	}{
		{"both alive, no language setting: the better score wins", "", []indexers.Result{torrent(i91Bdrip, 5, true), torrent(i91Fun, 5, true)}, i91Fun},
		{"same, the other way round in the list", "", []indexers.Result{torrent(i91Fun, 5, true), torrent(i91Bdrip, 5, true)}, i91Fun},
		{"German wanted, neither names German: the score decides", "German", []indexers.Result{torrent(i91Bdrip, 5, true), torrent(i91Fun, 5, true)}, i91Fun},
		{"German wanted, the winner names German too", "German", []indexers.Result{torrent(i91Bdrip, 5, true), torrent(i91FunDE, 5, true)}, i91FunDE},
		{"German wanted alone: plain German beats German DL", "German", []indexers.Result{torrent(i91FunDE, 5, true), torrent(i91PlainDE, 5, true)}, i91PlainDE},
		{"German then English: German DL beats plain German", "German,English", []indexers.Result{torrent(i91PlainDE, 5, true), torrent(i91FunDE, 5, true)}, i91FunDE},
		{"German then English: German DL beats plain German, other order", "German,English", []indexers.Result{torrent(i91FunDE, 5, true), torrent(i91PlainDE, 5, true)}, i91FunDE},
		{"German then English: both German DL, the score decides", "German,English", []indexers.Result{torrent(i91BdripDE, 5, true), torrent(i91FunDE, 5, true)}, i91FunDE},
		{"English wanted and the better score is a German DL: language wins, whatever the order", "English", []indexers.Result{torrent(i91Bdrip, 5, true), torrent(i91FunDE, 5, true)}, i91Bdrip},
		{"the better score has no seeders: it is dead", "", []indexers.Result{torrent(i91Bdrip, 5, true), torrent(i91Fun, 0, true)}, i91Bdrip},
		{"seeders unknown is not dead", "", []indexers.Result{torrent(i91Bdrip, 5, true), torrent(i91Fun, 0, false)}, i91Fun},
		{"a higher tier beats the score", "", []indexers.Result{torrent(i91Web, 5, true), torrent(i91Bdrip, 5, true)}, i91Bdrip},
	}
	for _, tt := range tests {
		for orderName, order := range i91Orders() {
			t.Run(tt.name+" ("+orderName+")", func(t *testing.T) {
				p := quality.Presets()[quality.PresetAny]
				p.Language = tt.language
				p.Preferred = order
				if got := pickedTitle(pickBestFor(tt.results, p, 0)); got != tt.want {
					t.Fatalf("automatic pick = %q, want %q", got, tt.want)
				}
				// The interactive list's "Best match" is the same pick.
				payloads := make([]searchResultPayload, len(tt.results))
				markBestMatch(payloads, tt.results, p)
				best := ""
				for i, pl := range payloads {
					if pl.Best {
						best = tt.results[i].Title
					}
				}
				if best != tt.want {
					t.Fatalf("best match in the list = %q, want %q", best, tt.want)
				}
			})
		}
	}
}

func TestPreferredHitsListEveryMatchWhateverTheOrder(t *testing.T) {
	for name, order := range i91Orders() {
		p := quality.Presets()[quality.PresetAny]
		p.Preferred = order
		sum := 0
		for _, h := range p.PreferredHits(i91Fun) {
			sum += h.Score
		}
		if sum != 150 || len(p.PreferredHits(i91Fun)) != 2 || p.Score(i91Fun) != 150 {
			t.Fatalf("%s: hits %+v score %d", name, p.PreferredHits(i91Fun), p.Score(i91Fun))
		}
		if hits := p.PreferredHits(i91Bdrip); len(hits) != 1 || hits[0].Term != "x265" || p.Score(i91Bdrip) != 50 {
			t.Fatalf("%s: hits %+v score %d", name, hits, p.Score(i91Bdrip))
		}
	}
}
