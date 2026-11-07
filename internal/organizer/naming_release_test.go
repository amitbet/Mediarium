package organizer_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rdborg/mediarium/internal/organizer"
)

// A file named at random takes the release's name, with the video's own
// extension; a release that is random too gives nothing.
func TestReleaseFileName(t *testing.T) {
	long := strings.Repeat("Very.Long.Release.Name.", 30) + "2024.1080p-GRP"
	cases := []struct {
		name    string
		release string
		file    string
		mode    organizer.SanitizeMode
		repl    string
		want    string
		ok      bool
	}{
		{"a normal release", "Tracker.2024.S04E03.Original.Parts.1080p.DSNP.WEB-DL.DDP5.1.H.264-NTb", "/w/a8f7d6e5c4b3.mkv",
			organizer.SanitizeStrip, "", "Tracker.2024.S04E03.Original.Parts.1080p.DSNP.WEB-DL.DDP5.1.H.264-NTb.mkv", true},
		{"the video's real extension is kept", "Example.Movie.2024.1080p.BluRay-GRP", "/w/wFvDJHhDrbESDBLq.MP4",
			organizer.SanitizeStrip, "", "Example.Movie.2024.1080p.BluRay-GRP.MP4", true},
		{"a release that already ends in an extension", "Example.Movie.2024.1080p.BluRay-GRP.mkv", "/w/abc.mkv",
			organizer.SanitizeStrip, "", "Example.Movie.2024.1080p.BluRay-GRP.mkv", true},
		{"a dot-number ending is not an extension", "Example.Movie.2024.H.264", "/w/abc.mkv",
			organizer.SanitizeStrip, "", "Example.Movie.2024.H.264.mkv", true},
		{"characters the file system refuses are removed", "Example: Movie? 2024 1080p", "/w/abc.mkv",
			organizer.SanitizeStrip, "", "Example Movie 2024 1080p.mkv", true},
		{"or replaced", "Example: Movie? 2024 1080p", "/w/abc.mkv",
			organizer.SanitizeReplace, "-", "Example- Movie- 2024 1080p.mkv", true},
		{"a slash in the release does not make a folder", "Example/Movie\\2024 1080p", "/w/abc.mkv",
			organizer.SanitizeStrip, "", "ExampleMovie2024 1080p.mkv", true},
		{"a random release gives nothing", "a8f7d6e5c4b3", "/w/abc.mkv", organizer.SanitizeStrip, "", "", false},
		{"an empty release gives nothing", "", "/w/abc.mkv", organizer.SanitizeStrip, "", "", false},
		{"only illegal characters gives nothing", "???", "/w/abc.mkv", organizer.SanitizeStrip, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := organizer.ReleaseFileName(tc.release, tc.file, tc.mode, tc.repl)
			if got != tc.want || ok != tc.ok {
				t.Errorf("ReleaseFileName(%q, %q) = %q, %v; want %q, %v", tc.release, tc.file, got, ok, tc.want, tc.ok)
			}
		})
	}

	t.Run("a very long release is cut inside the limit", func(t *testing.T) {
		got, ok := organizer.ReleaseFileName(long, "/w/abc.mkv", organizer.SanitizeStrip, "")
		if !ok || !strings.HasSuffix(got, ".mkv") || len(got) > organizer.MaxNameBytes+len(".mkv") || len(got) > 255 || !utf8.ValidString(got) {
			t.Errorf("got %d bytes %q", len(got), got)
		}
	})
	t.Run("a long name in a wide alphabet is cut on a character", func(t *testing.T) {
		got, ok := organizer.ReleaseFileName(strings.Repeat("日本語 ", 100), "/w/abc.mkv", organizer.SanitizeStrip, "")
		if !ok || len(got) > 255 || !utf8.ValidString(got) {
			t.Errorf("got %d bytes, valid=%v", len(got), utf8.ValidString(got))
		}
	})
}
