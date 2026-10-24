package quality_test

import (
	"testing"

	"github.com/rdborg/mediarium/internal/quality"
)

// Preferred terms come back from the database as saved: the same terms, the
// same scores, in the same order, even when one term is part of another
// (GitHub #91). Saving them the other way round and reading them again must
// not change either score.
func TestPreferredTermsRoundTrip(t *testing.T) {
	orders := map[string][]quality.Preferred{
		"fun first":  {{Term: "x265-FuN", Score: 100}, {Term: "x265", Score: 50}},
		"x265 first": {{Term: "x265", Score: 50}, {Term: "x265-FuN", Score: 100}},
		"odd terms":  {{Term: "a=b", Score: -20}, {Term: "WEB-DL", Score: 5}, {Term: "x265", Score: 0}},
	}
	repo := newRepo(t)
	for name, order := range orders {
		created, err := repo.Create(quality.Profile{Name: name, Allowed: []quality.Tier{quality.TierBluray1080p}, Cutoff: quality.TierBluray1080p, Preferred: order})
		if err != nil {
			t.Fatalf("%s: create: %v", name, err)
		}
		got, err := repo.Get(created.ID)
		if err != nil || len(got.Preferred) != len(order) {
			t.Fatalf("%s: get: %v %+v", name, err, got.Preferred)
		}
		for i := range order {
			if got.Preferred[i] != order[i] {
				t.Fatalf("%s: term %d = %+v, want %+v", name, i, got.Preferred[i], order[i])
			}
		}
		if name != "odd terms" {
			if s := got.Score("Some.Movie.2020.DL.AC3.1080p.BluRay.x265-FuN"); s != 150 {
				t.Fatalf("%s: score %d, want 150", name, s)
			}
			if s := got.Score("Some.Movie.2020.BDRip.DL.1080p.x265.AAC-2BA"); s != 50 {
				t.Fatalf("%s: score %d, want 50", name, s)
			}
		}
	}
}
