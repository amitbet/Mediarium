package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInIncompleteFolder(t *testing.T) {
	tests := []struct {
		rel  string
		want bool
	}{
		{"Movie.mkv", false},
		{"Movie/Movie.mkv", false},
		{"incomplete/Movie.mkv", true},
		{"Incomplete/Show/S01E01.mkv", true},
		{"INCOMPLETE DOWNLOADS/x/y.mkv", true},
		{"a/incomplete/b/c.mkv", true},
		{"_UNPACK_Movie/Movie.mkv", true},
		{"incompleted/Movie.mkv", false},
		{"complete/Movie.mkv", false},
		{"incomplete.mkv", false}, // a file, not a folder
	}
	for _, tc := range tests {
		if got := inIncompleteFolder(tc.rel); got != tc.want {
			t.Errorf("inIncompleteFolder(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

func TestIsPartialName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Movie.mkv", false},
		{"Movie.mkv.part", true},
		{"Movie.mkv.!qB", true},
		{"Movie.mkv.!QB", true},
		{"Movie.mkv.crdownload", true},
		{"Movie.mkv.tmp", true},
		{".unpack-123", true},
		{"_UNPACK_Movie", true},
		{"Party.Movie.mkv", false},
	}
	for _, tc := range tests {
		if got := isPartialName(tc.name); got != tc.want {
			t.Errorf("isPartialName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsSampleFile(t *testing.T) {
	tests := []struct {
		rel  string
		want bool
	}{
		{"Movie.2020.1080p.mkv", false},
		{"Movie/Sample/movie.mkv", true},
		{"Movie/samples/movie.mkv", true},
		{"Movie/movie-sample.mkv", true},
		{"Movie/sample.mkv", true},
		{"Movie.2020.SAMPLE.mkv", true},
		{"Movie/Movie.Sampler.2020.mkv", false},
		{"Resampled.2020.mkv", false},
		{"Sample.Size.2020.1080p.mkv", true}, // sample set apart at the start of a name is still read as a sample
	}
	for _, tc := range tests {
		if got := isSampleFile(tc.rel); got != tc.want {
			t.Errorf("isSampleFile(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

func TestManualSystemPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/proc", true},
		{"/proc/1/root", true},
		{"/sys/class", true},
		{"/etc", true},
		{"/etc/ssl", true},
		{"/dev/shm", true},
		{"/usr/share", true},
		{"/downloads", false},
		{"/mnt/media/Downloads", false},
		{"/etcetera", false},
	}
	for _, tc := range tests {
		if got := manualSystemPath(tc.path); got != tc.want {
			t.Errorf("manualSystemPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestParseManualMode(t *testing.T) {
	tests := []struct {
		in   string
		want manualMode
		ok   bool
	}{
		{"", manualLink, true}, {"link", manualLink, true}, {" Copy ", manualCopy, true}, {"MOVE", manualMove, true}, {"hardlink", "", false}, {"x", "", false},
	}
	for _, tc := range tests {
		got, ok := parseManualMode(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseManualMode(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// A move never removes the file that is now in the library, even when the
// chosen folder was the library itself and the file only moved onto itself.
func TestManualFinishNeverRemovesTheLibraryFile(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "Heat (1995).mkv")
	other := filepath.Join(dir, "download.mkv")
	for _, p := range []string{lib, other} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name     string
		src      string
		dest     string
		mode     manualMode
		wantKept bool
		wantGone bool
	}{
		{"link leaves the original", other, lib, manualLink, false, false},
		{"copy leaves the original", other, lib, manualCopy, false, false},
		{"move removes the original", other, lib, manualMove, false, true},
		{"move onto itself keeps it", lib, lib, manualMove, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(tc.src, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			if kept := manualFinish(tc.src, tc.dest, tc.mode); kept != tc.wantKept {
				t.Errorf("kept = %v, want %v", kept, tc.wantKept)
			}
			if _, err := os.Stat(tc.src); (err != nil) != tc.wantGone {
				t.Errorf("original gone = %v, want %v", err != nil, tc.wantGone)
			}
			if _, err := os.Stat(tc.dest); err != nil {
				t.Errorf("the library file is gone: %v", err)
			}
		})
	}
}
