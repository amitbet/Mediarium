package quality_test

import (
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/quality"
	"github.com/rdborg/mediarium/internal/store"
)

func newFormatRepo(t *testing.T) *quality.FormatRepo {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return quality.NewFormatRepo(db)
}

func intp(n int) *int { return &n }

func TestFormatRepoRemembersTrashScore(t *testing.T) {
	repo := newFormatRepo(t)
	spec := func(v string) []quality.FormatSpec {
		return []quality.FormatSpec{{Name: "x", Kind: quality.SpecReleaseTitle, Value: v}}
	}
	from, _, err := repo.Save(quality.CustomFormat{Name: "HDR", App: quality.AppRadarr, TrashID: "abc", Specs: spec("hdr"), TrashScore: intp(500)})
	if err != nil {
		t.Fatal(err)
	}
	pasted, _, err := repo.Save(quality.CustomFormat{Name: "Mine", App: quality.AppRadarr, Specs: spec("mine")})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(from.ID)
	if err != nil || got.TrashScore == nil || *got.TrashScore != 500 {
		t.Fatalf("a format from the guides keeps its score: %+v %v", got.TrashScore, err)
	}
	got, _ = repo.Get(pasted.ID)
	if got.TrashScore != nil {
		t.Fatalf("a format the person made has no TRaSH score, got %v", *got.TrashScore)
	}

	// Updating from the guides changes the conditions and the remembered
	// score, never the name the person gave it.
	if err := repo.Rename(from.ID, "My HDR"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateFromTrash(from.ID, quality.AppRadarr, spec("hdr10"), 700); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(from.ID)
	if got.Name != "My HDR" || got.Specs[0].Value != "hdr10" || *got.TrashScore != 700 {
		t.Fatalf("after the update: %q %q %v", got.Name, got.Specs[0].Value, got.TrashScore)
	}
	if err := repo.UpdateFromTrash(pasted.ID, quality.AppRadarr, spec("x"), 1); err == nil {
		t.Fatal("a format the person made must not be updated from the guides")
	}
}

func TestRetargetScore(t *testing.T) {
	tests := []struct {
		name        string
		scores      map[int64]int
		prev, next  int
		want        int
		wantChanged bool
	}{
		{"still the old guide score: follows the guides", map[int64]int{1: 100}, 100, 250, 250, true},
		{"changed by hand: left alone", map[int64]int{1: 90}, 100, 250, 0, false},
		{"not scored in the profile: left alone", map[int64]int{2: 100}, 100, 250, 0, false},
		{"the guides drop the score: it goes", map[int64]int{1: 100}, 100, 0, 0, true},
		{"nothing changed upstream", map[int64]int{1: 100}, 100, 100, 0, false},
		{"no old score known: left alone", map[int64]int{1: 100}, 0, 250, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := quality.RetargetScore(tt.scores, 1, tt.prev, tt.next)
			if changed != tt.wantChanged || (changed && got != tt.want) {
				t.Fatalf("got %d, %v; want %d, %v", got, changed, tt.want, tt.wantChanged)
			}
		})
	}
}
