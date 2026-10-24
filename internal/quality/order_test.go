package quality_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rdborg/mediarium/internal/parser"
	"github.com/rdborg/mediarium/internal/quality"
	"github.com/rdborg/mediarium/internal/store"
)

// A profile saved before the order existed has NULL in its column and reads as
// having no order of its own.
func TestProfileWithoutStoredOrderReadsAsBuiltIn(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	res, err := db.Exec(`INSERT INTO quality_profiles (name, allowed_tiers, cutoff, upgrade_allowed) VALUES ('Old', 'WEBDL-1080p,Bluray-1080p', 'Bluray-1080p', 1)`)
	if err != nil {
		t.Fatalf("a row written without the order column must still be valid: %v", err)
	}
	id, _ := res.LastInsertId()
	p, err := quality.NewRepo(db).Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if p.HasCustomOrder() || len(p.Allowed) != 2 || p.TierRank(quality.TierBluray1080p) != quality.Rank(quality.TierBluray1080p) {
		t.Errorf("an old profile keeps the built-in order, got %+v", p)
	}
}

func item(tiers ...quality.Tier) quality.QualityItem { return quality.QualityItem{Tiers: tiers} }

func group(name string, tiers ...quality.Tier) quality.QualityItem {
	return quality.QualityItem{Name: name, Tiers: tiers}
}

// Every profile without an order ranks exactly like the built-in ladder.
func TestTierRankWithoutOrderIsTheLadder(t *testing.T) {
	for key, p := range quality.Presets() {
		for _, tier := range quality.AllTiers() {
			if got, want := p.TierRank(tier), quality.Rank(tier); got != want {
				t.Errorf("%s: TierRank(%s) = %d, want %d", key, tier, got, want)
			}
		}
		if p.HasCustomOrder() {
			t.Errorf("%s: a built-in preset has no custom order", key)
		}
	}
}

// A custom order that lists the tiers in the built-in order decides everything
// the same way the built-in order does.
func TestDefaultOrderMatchesBuiltIn(t *testing.T) {
	for key, base := range quality.Presets() {
		custom := base
		custom.QualityOrder = quality.DefaultOrder(base.Allowed)
		for _, a := range quality.AllTiers() {
			if base.AllowsTier(a) != custom.AllowsTier(a) {
				t.Errorf("%s: AllowsTier(%s) differs", key, a)
			}
			if base.ReachedCutoffTier(a) != custom.ReachedCutoffTier(a) {
				t.Errorf("%s: ReachedCutoffTier(%s) differs", key, a)
			}
			for _, b := range quality.AllTiers() {
				if (base.TierRank(a) > base.TierRank(b)) != (custom.TierRank(a) > custom.TierRank(b)) {
					t.Errorf("%s: %s against %s ranks differently with the default order written out", key, a, b)
				}
			}
		}
	}
}

func TestTierRankWithOrder(t *testing.T) {
	p := quality.Profile{
		Name: "x", Cutoff: quality.TierBluray1080p,
		QualityOrder: []quality.QualityItem{
			item(quality.TierBluray1080p),
			group("WEB 1080p", quality.TierWebDL1080p, quality.TierHDTV1080p),
			item(quality.TierWebDL720p),
		},
	}
	p, err := quality.Validate(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Allowed, []quality.Tier{quality.TierWebDL720p, quality.TierHDTV1080p, quality.TierWebDL1080p, quality.TierBluray1080p}) {
		t.Fatalf("allowed is the union of the order, got %v", p.Allowed)
	}
	r := p.TierRank
	if r(quality.TierWebDL1080p) != r(quality.TierHDTV1080p) {
		t.Error("the tiers of a group share one rank")
	}
	if !(r(quality.TierBluray1080p) > r(quality.TierWebDL1080p) && r(quality.TierWebDL1080p) > r(quality.TierWebDL720p)) {
		t.Error("rank follows the position in the list, best first")
	}
	// Not in the list: just above the listed tier next down the built-in ladder.
	if got := r(quality.TierBluray720p); !(got > r(quality.TierWebDL720p) && got < r(quality.TierWebDL1080p)) {
		t.Errorf("an unlisted tier ranks next to its neighbour, got %d", got)
	}
	if got := r(quality.TierRemux2160p); got <= r(quality.TierBluray1080p) {
		t.Errorf("a file better than everything allowed stays above the cutoff, got %d", got)
	}
	if got := r(quality.TierSDTV); got >= r(quality.TierWebDL720p) {
		t.Errorf("a tier below everything listed ranks below it, got %d", got)
	}
}

func TestOrderChangesWhatBeatsWhat(t *testing.T) {
	// Web copies preferred over Bluray.
	p, err := quality.Validate(quality.Profile{
		Name: "web first", Cutoff: quality.TierWebDL1080p, UpgradeAllowed: true,
		QualityOrder: []quality.QualityItem{item(quality.TierWebDL1080p), item(quality.TierBluray1080p), item(quality.TierHDTV1080p)},
	})
	if err != nil {
		t.Fatal(err)
	}
	web := parser.Parse("Movie.2024.1080p.WEB-DL.x264-GROUP")
	blu := parser.Parse("Movie.2024.1080p.BluRay.x264-GROUP")
	hdtv := parser.Parse("Movie.2024.1080p.HDTV.x264-GROUP")
	if p.IsUpgrade(web, blu) {
		t.Error("Bluray is no upgrade over a web copy here")
	}
	if !p.IsUpgrade(blu, web) && !p.ReachedCutoff(blu) {
		// blu is below the cutoff (web), so web is an upgrade
		t.Error("web should be an upgrade over Bluray here")
	}
	if !p.IsUpgrade(hdtv, web) {
		t.Error("web is an upgrade over HDTV")
	}
	if !p.ReachedCutoff(web) || p.ReachedCutoff(blu) {
		t.Error("the cutoff is the web quality: a web copy reaches it, a Bluray does not")
	}
}

func TestGroupCountsAsOneRank(t *testing.T) {
	p, err := quality.Validate(quality.Profile{
		Name: "grouped", Cutoff: quality.TierBluray1080p, UpgradeAllowed: true,
		QualityOrder: []quality.QualityItem{
			item(quality.TierBluray1080p),
			group("Web or TV", quality.TierWebDL1080p, quality.TierHDTV1080p),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	web := parser.Parse("Movie.2024.1080p.WEB-DL.x264-GROUP")
	hdtv := parser.Parse("Movie.2024.1080p.HDTV.x264-GROUP")
	blu := parser.Parse("Movie.2024.1080p.BluRay.x264-GROUP")
	if p.IsUpgrade(hdtv, web) || p.IsUpgrade(web, hdtv) {
		t.Error("two qualities of one group are never an upgrade over each other")
	}
	if !p.IsUpgrade(web, blu) {
		t.Error("the quality above the group is an upgrade")
	}
	if p.IsUpgradeOverTier(quality.TierBluray1080p, web) {
		t.Error("nothing is an upgrade past the cutoff")
	}
}

func TestGroupAsCutoff(t *testing.T) {
	p, err := quality.Validate(quality.Profile{
		Name: "cutoff is a group", Cutoff: quality.TierHDTV1080p, UpgradeAllowed: true,
		QualityOrder: []quality.QualityItem{
			item(quality.TierBluray1080p),
			group("Web or TV", quality.TierWebDL1080p, quality.TierHDTV1080p),
			item(quality.TierWebDL720p),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for tier, want := range map[quality.Tier]bool{
		quality.TierBluray1080p: true, quality.TierWebDL1080p: true, quality.TierHDTV1080p: true, quality.TierWebDL720p: false,
	} {
		if got := p.ReachedCutoffTier(tier); got != want {
			t.Errorf("ReachedCutoffTier(%s) = %v, want %v", tier, got, want)
		}
	}
	it, ok := p.CutoffItem()
	if !ok || it.Name != "Web or TV" {
		t.Errorf("the cutoff item is the group, got %+v", it)
	}
}

func TestAcceptsFollowsTheOrder(t *testing.T) {
	p, err := quality.Validate(quality.Profile{
		Name: "x", Cutoff: quality.TierWebDL1080p,
		QualityOrder: []quality.QualityItem{group("Web", quality.TierWebDL1080p, quality.TierWebDL720p)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for title, want := range map[string]bool{
		"Movie.2024.1080p.WEB-DL.x264-GROUP": true,
		"Movie.2024.720p.WEB-DL.x264-GROUP":  true,
		"Movie.2024.1080p.BluRay.x264-GROUP": false,
	} {
		if got := p.Accepts(parser.Parse(title)); got != want {
			t.Errorf("Accepts(%s) = %v, want %v", title, got, want)
		}
	}
}

func TestValidateOrder(t *testing.T) {
	base := quality.Profile{Name: "x", Cutoff: quality.TierWebDL1080p}
	cases := []struct {
		name    string
		order   []quality.QualityItem
		wantErr bool
	}{
		{"fine", []quality.QualityItem{item(quality.TierWebDL1080p), group("G", quality.TierDVD, quality.TierSDTV)}, false},
		{"tier twice", []quality.QualityItem{item(quality.TierWebDL1080p), item(quality.TierWebDL1080p)}, true},
		{"tier twice across groups", []quality.QualityItem{item(quality.TierWebDL1080p), group("G", quality.TierWebDL1080p, quality.TierDVD)}, true},
		{"made-up tier", []quality.QualityItem{item(quality.TierWebDL1080p), item("Bluray-8K")}, true},
		{"empty item", []quality.QualityItem{item(quality.TierWebDL1080p), item()}, true},
		{"group with no name", []quality.QualityItem{item(quality.TierWebDL1080p, quality.TierDVD)}, true},
		{"same group name twice", []quality.QualityItem{group("G", quality.TierWebDL1080p), group("g", quality.TierDVD)}, true},
		{"cutoff not in the order", []quality.QualityItem{item(quality.TierDVD)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			p.QualityOrder = tc.order
			_, err := quality.Validate(p)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, quality.ErrInvalid) {
				t.Errorf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestOrderIsStoredAndKeptOnUpdate(t *testing.T) {
	repo := newRepo(t)
	created, err := repo.Create(quality.Profile{
		Name: "grouped", Cutoff: quality.TierBluray1080p, UpgradeAllowed: true,
		QualityOrder: []quality.QualityItem{
			item(quality.TierBluray1080p),
			group("Web or TV", quality.TierHDTV1080p, quality.TierWebDL1080p),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []quality.QualityItem{item(quality.TierBluray1080p), group("Web or TV", quality.TierWebDL1080p, quality.TierHDTV1080p)}
	if !reflect.DeepEqual(got.QualityOrder, want) {
		t.Fatalf("order after a round trip = %+v, want %+v", got.QualityOrder, want)
	}
	// Saving it again unchanged keeps it; clearing it goes back to the built-in order.
	if _, err := repo.Update(got); err != nil {
		t.Fatal(err)
	}
	got.QualityOrder = nil
	if _, err := repo.Update(got); err != nil {
		t.Fatal(err)
	}
	again, _ := repo.Get(created.ID)
	if again.HasCustomOrder() {
		t.Errorf("the order should be cleared, got %+v", again.QualityOrder)
	}
	if len(again.Allowed) != 3 {
		t.Errorf("the allowed qualities stay after the order is cleared, got %v", again.Allowed)
	}
}

func TestPresetsHaveNoOrderInTheDatabase(t *testing.T) {
	repo := newRepo(t)
	list, err := repo.SeedPresets(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.HasCustomOrder() {
			t.Errorf("%s has an order after seeding", p.Name)
		}
	}
}

func TestTrashQualityOrder(t *testing.T) {
	cases := []struct {
		name  string
		items []quality.TrashItem
		want  []quality.QualityItem
	}{
		{
			name: "built-in order with a plain WEB group needs no order",
			items: []quality.TrashItem{
				{Name: "Bluray-1080p", Allowed: true},
				{Name: "WEB 1080p", Allowed: true, Items: []string{"WEBRip-1080p", "WEBDL-1080p"}},
				{Name: "Bluray-720p", Allowed: true},
				{Name: "Remux-2160p", Allowed: false},
			},
			want: nil,
		},
		{
			name: "a group of several qualities is kept",
			items: []quality.TrashItem{
				{Name: "Remux 1080p", Allowed: true, Items: []string{"Remux-1080p", "Bluray-1080p"}},
				{Name: "WEB 1080p", Allowed: true, Items: []string{"HDTV-1080p", "WEBRip-1080p", "WEBDL-1080p"}},
				{Name: "Bluray-720p", Allowed: true},
			},
			want: []quality.QualityItem{
				group("Remux 1080p", quality.TierRemux1080p, quality.TierBluray1080p),
				group("WEB 1080p", quality.TierWebDL1080p, quality.TierHDTV1080p),
				item(quality.TierBluray720p),
			},
		},
		{
			name: "a different order is kept",
			items: []quality.TrashItem{
				{Name: "WEBDL-1080p", Allowed: true},
				{Name: "Bluray-1080p", Allowed: true},
			},
			want: []quality.QualityItem{item(quality.TierWebDL1080p), item(quality.TierBluray1080p)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quality.TrashProfile{Name: "x", Items: tc.items}.QualityOrder()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
