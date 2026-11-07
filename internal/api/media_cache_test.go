package api

import (
	"testing"
	"time"
)

// A request that finds the file details lists out of date is handed the old
// ones at once and the lists are renewed in the background; it does not wait
// for a read of every folder (GitHub issue #52). An explicit change (gen)
// still makes the next caller wait for fresh ones.
func TestMediaListsAreRenewedInTheBackground(t *testing.T) {
	s, db, dir := bigServer(t)
	bigLibrary(t, db, dir, 20, 3, 4)

	first := s.mediaSummaryNow(true)
	if len(first.movies) != 20 {
		t.Fatalf("movies read = %d, want 20", len(first.movies))
	}
	firstIncomplete := s.incompleteNow()
	if again := s.mediaSummaryNow(true); again != first {
		t.Fatal("a fresh list was read again")
	}

	// Make it old: it is handed out as it is, and renewed behind the scenes.
	first.at = time.Now().Add(-2 * mediaSummaryTTL)
	firstIncomplete.at = first.at
	if got := s.mediaSummaryNow(true); got != first {
		t.Fatal("an out-of-date list made the caller wait for a new one")
	}
	if got := s.incompleteNow(); got != firstIncomplete {
		t.Fatal("an out-of-date incomplete list made the caller wait for a new one")
	}
	deadline := time.Now().Add(10 * time.Second)
	for s.mediaSummaryNow(true) == first || s.incompleteNow() == firstIncomplete {
		if time.Now().After(deadline) {
			t.Fatal("the lists were not renewed in the background")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// After an explicit change the caller gets fresh ones straight away.
	cur := s.mediaSummaryNow(true)
	s.mediaInfo.gen.Add(1)
	if got := s.mediaSummaryNow(true); got == cur {
		t.Fatal("a list from before an explicit change was handed out")
	}
	// Too old to hand out: wait for a new one.
	cur = s.mediaSummaryNow(true)
	cur.at = time.Now().Add(-2 * mediaSummaryMaxStale)
	if got := s.mediaSummaryNow(true); got == cur {
		t.Fatal("a list older than the limit was handed out")
	}
}

// Saving a file's details (done many times a second by the reading job) no
// longer throws the lists away.
func TestReadingFilesKeepsTheLists(t *testing.T) {
	s, db, dir := bigServer(t)
	bigLibrary(t, db, dir, 5, 1, 2)
	cur := s.mediaSummaryNow(true)
	if out := s.saveFileInfo(dir+"/movies/Movie 1 (2020)/movie.mkv", mediaRoots{}, true); out == miHung {
		t.Fatal("hung")
	}
	if got := s.mediaSummaryNow(true); got != cur {
		t.Fatal("reading a file made the next list load read everything again")
	}
}
