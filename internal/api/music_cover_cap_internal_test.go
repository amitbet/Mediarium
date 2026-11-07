package api

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// "No cover" marks are not pictures: a pile of them must not push real covers
// out of the cache, and thumbnails have a limit of their own.
func TestPruneCoverCacheDoesNotCountMarksAndKeepsThumbnailsApart(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write := func(name string, age time.Duration) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 50; i++ {
		write(fmt.Sprintf("mark-%d.none", i), time.Minute) // fresh marks: kept, and not counted
	}
	for i := 0; i < 3; i++ {
		write(fmt.Sprintf("cover-%d.img", i), time.Duration(10-i)*time.Hour)
	}
	for i := 0; i < 5; i++ {
		write(fmt.Sprintf("%s%d.img", thumbPrefix, i), time.Duration(10-i)*time.Hour)
	}
	pruneCoverCache(dir, 3, 2, 24*time.Hour)

	left := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		left[e.Name()] = true
	}
	for i := 0; i < 50; i++ {
		if !left[fmt.Sprintf("mark-%d.none", i)] {
			t.Fatalf("mark-%d.none was removed, marks are not counted", i)
		}
	}
	for i := 0; i < 3; i++ {
		if !left[fmt.Sprintf("cover-%d.img", i)] {
			t.Errorf("cover-%d.img was pushed out by marks or thumbnails", i)
		}
	}
	for i, want := range []bool{false, false, false, true, true} {
		if left[fmt.Sprintf("%s%d.img", thumbPrefix, i)] != want {
			t.Errorf("thumbnail %d kept=%v, want %v", i, !want, want)
		}
	}
}

// The limits suit a big library, and a cover that was looked at recently is
// not written to again just for that.
func TestCoverCacheLimitsAreSizedForBigLibraries(t *testing.T) {
	if maxCoverFiles < 20000 || maxThumbFiles < maxCoverFiles {
		t.Fatalf("limits %d covers, %d thumbnails", maxCoverFiles, maxThumbFiles)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "a.img")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	two := time.Now().Add(-2 * 24 * time.Hour)
	if err := os.Chtimes(p, two, two); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	touchCover(p, st) // looked at two days ago: not touched again yet
	if st2, _ := os.Stat(p); !st2.ModTime().Equal(st.ModTime()) {
		t.Fatal("touched too often")
	}
}
