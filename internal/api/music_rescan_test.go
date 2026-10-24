package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// importPartialAlbum registers "First Album" from a folder holding only its
// first two tracks, the way a person's own collection (or an incomplete
// release) is found, and returns the album's id and folder.
func importPartialAlbum(t *testing.T, e *musicEnv) (id float64, folder string) {
	t.Helper()
	folder = filepath.Join(e.musicRoot, "Fixture Band", "First Album (1999)")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01 - Opening.flac", "02 - Middle Song.flac"} {
		if err := os.WriteFile(filepath.Join(folder, name), flacFile(44100, 16, 5), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	start := postJSON[map[string]any](t, e.client, e.base+"/api/music/import/scan", nil, http.StatusAccepted)
	waitBackground(t, e.server)
	job := getJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/import/scan/%v", e.base, start["id"]))
	for _, r := range job["results"].([]any) {
		m := r.(map[string]any)
		if m["album"] == "First Album" && m["status"] == "imported" {
			return m["albumId"].(float64), folder
		}
	}
	t.Fatalf("First Album was not imported: %v", job)
	return 0, ""
}

func deleteAlbumFile(t *testing.T, e *musicEnv, id float64, rel string) (int, map[string]any) {
	t.Helper()
	return doStatus(t, e.client, http.MethodDelete, fmt.Sprintf("%s/api/music/albums/%v/files?path=%s", e.base, id, url.QueryEscape(rel)))
}

func TestRescanAlbumPicksUpFilesAddedByHand(t *testing.T) {
	e := newMusicEnv(t)
	id, folder := importPartialAlbum(t, e)

	rescan := func() map[string]any {
		return postJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/albums/%v/rescan", e.base, id), nil, http.StatusOK)
	}
	// Nothing changed yet: two of three tracks, the album stays downloaded.
	rep := rescan()
	if rep["matched"] != float64(2) || rep["tracks"] != float64(3) || rep["complete"] != false || rep["linked"] != float64(0) || rep["status"] != "downloaded" {
		t.Fatalf("first rescan: %v", rep)
	}
	if miss := rep["missingTitles"].([]any); len(miss) != 1 || miss[0] != "Closing Time" {
		t.Fatalf("missing titles: %v", miss)
	}

	// The third track arrives under a name no track number can match, with
	// only its tags to go by.
	tagged := flacFile(44100, 16, 5, "TITLE=Closing Time", "TRACKNUMBER=3")
	if err := os.WriteFile(filepath.Join(folder, "closing.flac"), tagged, 0o644); err != nil {
		t.Fatal(err)
	}
	rep = rescan()
	if rep["matched"] != float64(3) || rep["complete"] != true || rep["linked"] != float64(1) || rep["extra"] != float64(0) || rep["quality"] != "FLAC" {
		t.Fatalf("rescan after adding a file: %v", rep)
	}
	album := getJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/albums/%v", e.base, id))
	tracks := album["tracks"].([]any)
	if tracks[2].(map[string]any)["filePath"] != filepath.Join(folder, "closing.flac") || album["quality"] != "FLAC" || album["status"] != "downloaded" {
		t.Fatalf("album after rescan: %v", album)
	}

	// An extra file that fits no track is reported, not linked.
	if err := os.WriteFile(filepath.Join(folder, "99 - Bonus Nobody Knows.flac"), flacFile(44100, 16, 5), 0o644); err != nil {
		t.Fatal(err)
	}
	if rep = rescan(); rep["extra"] != float64(1) || rep["matched"] != float64(3) {
		t.Fatalf("extra file: %v", rep)
	}

	// A file removed by hand is dropped from its track.
	if err := os.Remove(filepath.Join(folder, "02 - Middle Song.flac")); err != nil {
		t.Fatal(err)
	}
	rep = rescan()
	if rep["matched"] != float64(2) || rep["dropped"] != float64(1) || rep["complete"] != false {
		t.Fatalf("rescan after removing a file: %v", rep)
	}

	// With every audio file gone the album is wanted again.
	for _, n := range []string{"01 - Opening.flac", "closing.flac", "99 - Bonus Nobody Knows.flac"} {
		if err := os.Remove(filepath.Join(folder, n)); err != nil {
			t.Fatal(err)
		}
	}
	rep = rescan()
	if rep["status"] != "missing" || rep["matched"] != float64(0) || rep["folder"] != false {
		t.Fatalf("rescan of an empty folder: %v", rep)
	}
	album = getJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/albums/%v", e.base, id))
	if album["status"] != "missing" || album["monitored"] != true {
		t.Fatalf("album: %v", album)
	}
	var wanted bool
	for _, w := range getJSON[[]map[string]any](t, e.client, e.base+"/api/music/wanted") {
		wanted = wanted || w["id"] == id
	}
	if !wanted {
		t.Fatal("an album with no files left should be wanted")
	}
}

func TestRescanAlbumFindsTheFolderOfAnAlbumNotYetRegisteredWithOne(t *testing.T) {
	e := newMusicEnv(t)
	artist := e.addArtist(t, "none")
	al := albumByTitle(t, artist, "First Album")
	id := al["id"]
	folder := filepath.Join(e.musicRoot, "Fixture Band", "First Album (1999)")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01 - Opening.flac", "02 - Middle Song.flac", "03 - Closing Time.flac"} {
		if err := os.WriteFile(filepath.Join(folder, name), flacFile(44100, 16, 5), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep := postJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/albums/%v/rescan", e.base, id), nil, http.StatusOK)
	if rep["status"] != "downloaded" || rep["complete"] != true || rep["linked"] != float64(3) {
		t.Fatalf("rescan: %v", rep)
	}
	album := getJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/albums/%v", e.base, id))
	if album["path"] != folder {
		t.Fatalf("path: %v", album["path"])
	}
}

func TestRescanAlbumNeedsAdministrator(t *testing.T) {
	e := newMusicEnv(t)
	id, _ := importPartialAlbum(t, e)
	status, _ := doStatus(t, http.DefaultClient, http.MethodPost, fmt.Sprintf("%s/api/music/albums/%v/rescan", e.base, id))
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Fatalf("anonymous rescan: %d", status)
	}
	if status, _ := doStatus(t, e.client, http.MethodPost, e.base+"/api/music/albums/999999/rescan"); status != http.StatusNotFound {
		t.Fatalf("unknown album: %d", status)
	}
}

func TestDeleteAlbumFileUnlinksTheTrackAndUpdatesTheAlbum(t *testing.T) {
	e := newMusicEnv(t)
	id, folder := importPartialAlbum(t, e)

	status, rep := deleteAlbumFile(t, e, id, "02 - Middle Song.flac")
	if status != http.StatusOK {
		t.Fatalf("delete: %d %v", status, rep)
	}
	if _, err := os.Stat(filepath.Join(folder, "02 - Middle Song.flac")); !os.IsNotExist(err) {
		t.Fatalf("the file should be gone from the folder: %v", err)
	}
	if rep["matched"] != float64(1) || rep["complete"] != false || rep["status"] != "downloaded" || rep["deleted"] != "02 - Middle Song.flac" {
		t.Fatalf("answer: %v", rep)
	}
	album := getJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/albums/%v", e.base, id))
	tracks := album["tracks"].([]any)
	if tracks[1].(map[string]any)["hasFile"] != false || tracks[0].(map[string]any)["hasFile"] != true {
		t.Fatalf("tracks: %v", tracks)
	}
	files := getJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/albums/%v/files", e.base, id))
	if n := len(files["files"].([]any)); n != 1 {
		t.Fatalf("files left: %v", files)
	}

	// Deleting the last file makes the album wanted again.
	status, rep = deleteAlbumFile(t, e, id, "01 - Opening.flac")
	if status != http.StatusOK || rep["status"] != "missing" || rep["matched"] != float64(0) {
		t.Fatalf("delete the last file: %d %v", status, rep)
	}
	album = getJSON[map[string]any](t, e.client, fmt.Sprintf("%s/api/music/albums/%v", e.base, id))
	if album["status"] != "missing" || album["monitored"] != true {
		t.Fatalf("album: %v", album)
	}
	if p, _ := album["path"].(string); p != "" {
		t.Fatalf("path should be cleared: %v", p)
	}
}

func TestDeleteAlbumFileNeverLeavesTheAlbumFolder(t *testing.T) {
	e := newMusicEnv(t)
	id, folder := importPartialAlbum(t, e)
	outside := filepath.Join(e.musicRoot, "Fixture Band", "keep.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "secret.flac")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(folder, "link.flac")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	for _, rel := range []string{"../keep.txt", "../../Fixture Band/keep.txt", outside, secret, "link.flac", "", ".", "nope.flac", "sub/../../keep.txt", `..\keep.txt`} {
		status, body := deleteAlbumFile(t, e, id, rel)
		if status < 400 {
			t.Errorf("deleting %q was accepted: %d %v", rel, status, body)
		}
	}
	for _, p := range []string{outside, secret, filepath.Join(folder, "01 - Opening.flac"), filepath.Join(folder, "02 - Middle Song.flac")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s must still exist: %v", p, err)
		}
	}
	if status, _ := doStatus(t, http.DefaultClient, http.MethodDelete, fmt.Sprintf("%s/api/music/albums/%v/files?path=x.flac", e.base, id)); status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Errorf("anonymous delete: %d", status)
	}
}
