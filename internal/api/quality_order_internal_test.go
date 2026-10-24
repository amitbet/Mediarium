package api

import (
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/parser"
	"github.com/rdborg/mediarium/internal/quality"
)

func orderedProfile(t *testing.T, cutoff quality.Tier, order ...quality.QualityItem) quality.Profile {
	t.Helper()
	p, err := quality.Validate(quality.Profile{Name: "custom", Cutoff: cutoff, UpgradeAllowed: true, SkipBadReleases: true, QualityOrder: order})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func pickFrom(profile quality.Profile, titles ...string) string {
	var best pickKey
	bestTitle := ""
	for _, title := range titles {
		res := indexers.Result{Title: title}
		rel := parser.Parse(title)
		if !profile.Accepts(rel) {
			continue
		}
		k := keyFor(profile, rel, &res)
		if bestTitle == "" || k.above(best) {
			best, bestTitle = k, title
		}
	}
	return bestTitle
}

const (
	relWeb  = "Movie.2024.1080p.WEB-DL.x264-GROUP"
	relBlu  = "Movie.2024.1080p.BluRay.x264-GROUP"
	relTV   = "Movie.2024.1080p.HDTV.x264-GROUP"
	relWeb7 = "Movie.2024.720p.WEB-DL.x264-GROUP"
)

func TestPickFollowsTheProfileOrder(t *testing.T) {
	tier := func(ts ...quality.Tier) quality.QualityItem { return quality.QualityItem{Tiers: ts} }
	builtin := quality.Presets()[quality.Preset1080p]
	webFirst := orderedProfile(t, quality.TierWebDL1080p, tier(quality.TierWebDL1080p), tier(quality.TierBluray1080p), tier(quality.TierHDTV1080p))
	grouped := orderedProfile(t, quality.TierBluray1080p,
		tier(quality.TierBluray1080p),
		quality.QualityItem{Name: "Web or TV", Tiers: []quality.Tier{quality.TierHDTV1080p, quality.TierWebDL1080p}})
	cases := []struct {
		name    string
		profile quality.Profile
		titles  []string
		want    string
	}{
		{"built-in: Bluray beats web", builtin, []string{relWeb, relBlu, relTV}, relBlu},
		{"web first: web beats Bluray", webFirst, []string{relBlu, relWeb, relTV}, relWeb},
		{"web first: Bluray beats HDTV", webFirst, []string{relTV, relBlu}, relBlu},
		{"group: the item above the group wins", grouped, []string{relWeb, relTV, relBlu}, relBlu},
		{"group: inside the group the built-in order settles it", grouped, []string{relTV, relWeb}, relWeb},
		{"a quality that isn't listed is not taken", grouped, []string{relWeb7}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickFrom(tc.profile, tc.titles...); got != tc.want {
				t.Errorf("picked %q, want %q", got, tc.want)
			}
		})
	}
}

// A profile with no order of its own decides exactly as it did before.
func TestBuiltInProfilesRankLikeTheLadder(t *testing.T) {
	for key, p := range quality.Presets() {
		for _, a := range quality.AllTiers() {
			for _, b := range quality.AllTiers() {
				ka := pickKey{tier: p.TierRank(a), sub: quality.Rank(a)}
				kb := pickKey{tier: p.TierRank(b), sub: quality.Rank(b)}
				if ka.above(kb) != (quality.Rank(a) > quality.Rank(b)) {
					t.Fatalf("%s: %s against %s decides differently from the ladder", key, a, b)
				}
			}
		}
	}
}

func TestWantsUpgradeWithGroups(t *testing.T) {
	grouped := orderedProfile(t, quality.TierHDTV1080p,
		quality.QualityItem{Tiers: []quality.Tier{quality.TierBluray1080p}},
		quality.QualityItem{Name: "Web or TV", Tiers: []quality.Tier{quality.TierHDTV1080p, quality.TierWebDL1080p}},
		quality.QualityItem{Tiers: []quality.Tier{quality.TierWebDL720p}})
	cases := []struct {
		quality string
		want    bool
	}{
		{string(quality.TierWebDL720p), true},    // below the cutoff group
		{string(quality.TierHDTV1080p), false},   // in the cutoff group
		{string(quality.TierWebDL1080p), false},  // the other quality of the group
		{string(quality.TierBluray1080p), false}, // above it
		{"", false},
	}
	for _, tc := range cases {
		if got := wantsUpgrade(grouped, tc.quality, ""); got != tc.want {
			t.Errorf("wantsUpgrade(%q) = %v, want %v", tc.quality, got, tc.want)
		}
	}
}

func TestRejectionForUpgradeInsideAGroup(t *testing.T) {
	grouped := orderedProfile(t, quality.TierBluray1080p,
		quality.QualityItem{Tiers: []quality.Tier{quality.TierBluray1080p}},
		quality.QualityItem{Name: "Web or TV", Tiers: []quality.Tier{quality.TierHDTV1080p, quality.TierWebDL1080p}})
	reasons := rejectionReasonsFor(relWeb, grouped, true, string(quality.TierHDTV1080p), "")
	found := false
	for _, r := range reasons {
		found = found || (r.Kind == "upgrade" && strings.Contains(r.Text, "not an upgrade"))
	}
	if !found {
		t.Errorf("a quality of the same group is not an upgrade, got %+v", reasons)
	}
}

func TestWhyNotBestInsideAGroup(t *testing.T) {
	grouped := orderedProfile(t, quality.TierBluray1080p,
		quality.QualityItem{Tiers: []quality.Tier{quality.TierBluray1080p}},
		quality.QualityItem{Name: "Web or TV", Tiers: []quality.Tier{quality.TierHDTV1080p, quality.TierWebDL1080p}})
	best, other := indexers.Result{Title: relWeb}, indexers.Result{Title: relTV}
	why := whyNotBest(grouped, &best, parser.Parse(relWeb), &other, parser.Parse(relTV))
	if !strings.Contains(why.Text, "same quality group") {
		t.Errorf("got %q", why.Text)
	}
}

func TestArrProfileShowsGroupsAndTheCutoff(t *testing.T) {
	p := orderedProfile(t, quality.TierHDTV1080p,
		quality.QualityItem{Tiers: []quality.Tier{quality.TierBluray1080p}},
		quality.QualityItem{Name: "Web or TV", Tiers: []quality.Tier{quality.TierHDTV1080p, quality.TierWebDL1080p}})
	out := toArrProfile(arrRadarr, p)
	var group *arrProfileItem
	allowedSingles := 0
	for i, it := range out.Items {
		if it.Name == "Web or TV" {
			group = &out.Items[i]
		} else if it.Allowed {
			allowedSingles++
		}
	}
	if group == nil || group.ID == 0 || len(group.Items) != 2 || group.Quality != nil || !group.Allowed {
		t.Fatalf("expected the group with its two qualities, got %+v", out.Items)
	}
	if out.Cutoff != group.ID {
		t.Errorf("the cutoff is the group id %d, got %d", group.ID, out.Cutoff)
	}
	if allowedSingles != 1 {
		t.Errorf("one allowed single quality (Bluray-1080p), got %d", allowedSingles)
	}
	// Worst first, like Radarr: the group comes before the Bluray.
	last := out.Items[len(out.Items)-1]
	if last.Quality == nil || last.Quality.Name != "Bluray-1080p" {
		t.Errorf("the best item is last, got %+v", last)
	}
	// A profile without an order lists every quality, as before.
	plain := toArrProfile(arrRadarr, quality.Presets()[quality.Preset1080p])
	if len(plain.Items) != len(quality.AllTiers()) {
		t.Errorf("a built-in profile lists %d qualities, got %d", len(quality.AllTiers()), len(plain.Items))
	}
	for _, it := range plain.Items {
		if it.Quality == nil || it.Name != "" {
			t.Errorf("a plain item is a quality, got %+v", it)
		}
	}
}
