//go:build !windows

package organizer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Opt-in: set MEDIARIUM_TWO_DRIVES_A to a folder on one drive and
// MEDIARIUM_FULL_DRIVE to a folder on a tiny one (for example a 2 MB tmpfs).
// A folder that does not fit runs the target out of room halfway through the
// copy; the original must be untouched and nothing may be left on the tiny drive.
func TestMoveToADriveThatFillsUpHalfway(t *testing.T) {
	a, small := os.Getenv("MEDIARIUM_TWO_DRIVES_A"), os.Getenv("MEDIARIUM_FULL_DRIVE")
	if a == "" || small == "" {
		t.Skip("set MEDIARIUM_TWO_DRIVES_A and MEDIARIUM_FULL_DRIVE")
	}
	src := filepath.Join(a, "full-drive-test", "Heat (1995)")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(a, "full-drive-test")) })
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	// 5 files of 700 kB: the third or fourth cannot be written.
	chunk := make([]byte, 700<<10)
	for i := range chunk {
		chunk[i] = byte(i * 7)
	}
	for _, n := range []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv", "e.mkv"} {
		if err := os.WriteFile(filepath.Join(src, n), chunk, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before := tree(t, src)
	dst := filepath.Join(small, "Heat (1995)")
	_, err := StartMove(context.Background(), []MoveUnit{{Src: src, Dst: dst, Dir: true}}, MoveOptions{})
	if err == nil {
		t.Fatal("the move claims to have fitted on a 2 MB drive")
	}
	t.Logf("failed as it should: %v", err)
	if tree(t, src) != before {
		t.Fatal("the original changed")
	}
	entries, _ := os.ReadDir(small)
	for _, e := range entries {
		if e.Name() != "lost+found" {
			t.Fatalf("left on the full drive: %s", e.Name())
		}
	}
}
