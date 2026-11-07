package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A cover that is looked at counts as recently used, so the clean-up keeps it
// and removes one-off covers first.
func TestTouchCoverMarksUse(t *testing.T) {
	dir := t.TempDir()
	used := filepath.Join(dir, "used.img")
	oneOff := filepath.Join(dir, "oneoff.img")
	for _, p := range []string{used, oneOff} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-10 * 24 * time.Hour)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	st, _ := os.Stat(used)
	touchCover(used, st)

	pruneCoverCache(dir, 1, 1, time.Hour)
	if _, err := os.Stat(used); err != nil {
		t.Fatalf("the cover that was looked at was removed: %v", err)
	}
	if _, err := os.Stat(oneOff); !os.IsNotExist(err) {
		t.Fatalf("the unused cover should have gone first: %v", err)
	}
}
