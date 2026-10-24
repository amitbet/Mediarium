package organizer_test

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/organizer"
)

// Archives in a folder of the download are found too, with their full path;
// hidden and sample folders are left out.
func TestFindPrimaryArchivesDeep(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"top.rar",
		filepath.Join("Torrent Name", "release.part1.rar"),
		filepath.Join("Torrent Name", "release.part2.rar"),
		filepath.Join("Torrent Name", "old.rar"),
		filepath.Join("Torrent Name", "old.r00"),
		filepath.Join("Torrent Name", "CD2", "disc.zip"),
		filepath.Join("Torrent Name", "Sample", "sample.zip"),
		filepath.Join(".unpack-7z-123", "inner.zip"),
		filepath.Join("Torrent Name", "notes.txt"),
		filepath.Join("Split", "Movie.7z.001"),
		filepath.Join("Split", "Movie.7z.002"),
		filepath.Join("Split", "Movie.7z.011"),
		filepath.Join("SplitPadded", "x.7z.0001"),
		filepath.Join("SplitPadded", "x.7z.0002"),
	} {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := organizer.FindPrimaryArchivesDeep(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		filepath.Join(dir, "top.rar"):                           true,
		filepath.Join(dir, "Torrent Name", "release.part1.rar"): true,
		filepath.Join(dir, "Torrent Name", "old.rar"):           true,
		filepath.Join(dir, "Torrent Name", "CD2", "disc.zip"):   true,
		filepath.Join(dir, "Split", "Movie.7z.001"):             true,
		filepath.Join(dir, "SplitPadded", "x.7z.0001"):          true,
	}
	if len(got) != len(want) {
		t.Fatalf("found %v, want %d archives", got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected archive %s", p)
		}
	}
	if _, err := organizer.FindPrimaryArchivesDeep(filepath.Join(dir, "nope")); err == nil {
		t.Error("a folder that does not exist must be an error")
	}
}

// A release cannot write the files the downloader keeps its own notes in.
func TestExtractRefusesTheAppsOwnFileNames(t *testing.T) {
	for _, name := range []string{".mediarium-client-hash", ".mediarium-client-content", "sub/.mediarium-downloaded", ".debrid-id", ".MEDIARIUM-skipped"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "release.zip")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(f)
			w, _ := zw.Create(name)
			w.Write([]byte("0123456789abcdef0123456789abcdef01234567"))
			zw.Close()
			f.Close()

			dest := filepath.Join(dir, "out")
			err = organizer.NewExtractor().Extract(archive, dest)
			if !errors.Is(err, organizer.ErrUnsafeArchive) {
				t.Fatalf("extract = %v, want ErrUnsafeArchive", err)
			}
			if _, statErr := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); statErr == nil {
				t.Fatal("the file must not be written")
			}
		})
	}
}
