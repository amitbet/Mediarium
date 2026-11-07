package library_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/library/librarytest"
	"github.com/rdborg/mediarium/internal/store"
)

// ListSeriesNames must give the same ids, titles, years and countries, in the
// same order, as the full ListSeries it stands in for on the show page.
func TestListSeriesNamesMatchesListSeries(t *testing.T) {
	repo := newRepo(t)
	for i, sr := range []library.Series{
		{TMDBID: 1, Title: "Shameless", Year: 2004, OriginCountry: []string{"GB"}},
		{TMDBID: 2, Title: "Shameless", Year: 2011, OriginCountry: []string{"US"}},
		{TMDBID: 3, Title: "No Year"},
	} {
		if _, err := repo.AddSeries(sr, []library.Episode{{Season: 1, Episode: 1, Title: "x", AirDate: "2011-04-17"}}); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}

	full, err := repo.ListSeries()
	if err != nil {
		t.Fatal(err)
	}
	slim, err := repo.ListSeriesNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != len(slim) || len(full) != 3 {
		t.Fatalf("full %d, slim %d, want 3 each", len(full), len(slim))
	}
	for i := range full {
		a, b := full[i], slim[i]
		if a.ID != b.ID || a.Title != b.Title || a.Year != b.Year || a.CopyOf != b.CopyOf || fmt.Sprint(a.OriginCountry) != fmt.Sprint(b.OriginCountry) {
			t.Errorf("row %d differs: full %+v, slim %+v", i, a, b)
		}
	}
}

// TestListSeriesNamesIsCheaper logs what the slim read saves on a library of
// a few hundred shows, and fails only if it is not faster.
func TestListSeriesNamesIsCheaper(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	sz := librarytest.Small
	sz.Shows, sz.EpisodesPerShow = 300, 100
	if err := librarytest.Fill(db, sz); err != nil {
		t.Fatal(err)
	}
	repo := library.NewRepo(db)
	timeIt := func(f func() ([]library.Series, error)) time.Duration {
		best := time.Hour
		for i := 0; i < 5; i++ {
			start := time.Now()
			if _, err := f(); err != nil {
				t.Fatal(err)
			}
			if d := time.Since(start); d < best {
				best = d
			}
		}
		return best
	}
	full, slim := timeIt(repo.ListSeries), timeIt(repo.ListSeriesNames)
	t.Logf("300 shows x 100 episodes: ListSeries %v, ListSeriesNames %v", full, slim)
	if slim >= full {
		t.Errorf("the slim read (%v) is not faster than the full one (%v)", slim, full)
	}
}
