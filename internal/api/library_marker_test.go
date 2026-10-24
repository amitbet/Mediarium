package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/fsinfo"
	"github.com/rdborg/mediarium/internal/library"
)

func askMarker(t *testing.T, l tempLayout, path string, connected bool) (int, string) {
	t.Helper()
	body, _ := json.Marshal(rootMarkerRequest{Path: path, DriveConnected: connected})
	rec := httptest.NewRecorder()
	l.s.handleCreateRootMarker(rec, httptest.NewRequest(http.MethodPost, "/api/settings/library-marker", strings.NewReader(string(body))))
	return rec.Code, rec.Body.String()
}

// The administrator's "this is the right folder" button: it makes the marker
// only where that is safe, and asks for more when the drive may be missing.
func TestCreateRootMarkerByHand(t *testing.T) {
	old := onContainerDisk
	t.Cleanup(func() { onContainerDisk = old })

	tests := []struct {
		name      string
		onDisk    bool // the folder is on the container's own disk
		titles    int  // movies recorded under the folder
		withDirs  bool // their folders exist
		connected bool
		path      func(l tempLayout) string
		wantCode  int
		wantText  string
		wantMark  bool
	}{
		{name: "an empty library folder", wantCode: 200, wantMark: true},
		{name: "titles recorded and their folders there", titles: 2, withDirs: true, wantCode: 200, wantMark: true},
		{name: "titles recorded, no folder there, drive not confirmed", titles: 2, wantCode: 409, wantText: "drive is not connected"},
		{name: "titles recorded, no folder there, drive confirmed", titles: 2, connected: true, wantCode: 200, wantMark: true},
		{name: "a folder on the container's own disk", onDisk: true, wantCode: 409, wantText: "container's own disk"},
		{name: "not a library folder", path: func(l tempLayout) string { return l.downloads }, wantCode: 400, wantText: "not one of your library folders"},
		{name: "a folder that is not there", path: func(l tempLayout) string { return filepath.Join(l.movies, "gone") }, wantCode: 400, wantText: "not one of your library folders"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := newTempLayout(t)
			onContainerDisk = func(string) bool { return tc.onDisk }
			for i := 0; i < tc.titles; i++ {
				dir := filepath.Join(l.movies, "Film "+string(rune('A'+i))+" (2020)")
				file := filepath.Join(dir, "film.mkv")
				if tc.withDirs {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				m, err := l.s.MovieRepo.Add(library.Movie{TMDBID: 100 + i, Title: "Film " + string(rune('A'+i)), Monitored: true})
				if err != nil {
					t.Fatal(err)
				}
				if err := l.s.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, "WEBDL-1080p", file); err != nil {
					t.Fatal(err)
				}
			}
			path := l.movies
			if tc.path != nil {
				path = tc.path(l)
			}
			code, body := askMarker(t, l, path, tc.connected)
			if code != tc.wantCode || !strings.Contains(body, tc.wantText) {
				t.Fatalf("answer %d %s; want %d containing %q", code, body, tc.wantCode, tc.wantText)
			}
			if got := fsinfo.HasRootMarker(l.movies); got != tc.wantMark {
				t.Fatalf("marker present = %v, want %v", got, tc.wantMark)
			}
			if tc.wantMark {
				// Imports are let through again, and the action is in Activity.
				if err := l.s.checkLibraryRoot(filepath.Join(l.movies, "New", "new.mkv")); err != nil {
					t.Fatalf("import after the marker: %v", err)
				}
				entries, _ := l.s.QueueRepo.ListActivityFor(20, "")
				found := false
				for _, e := range entries {
					found = found || e.EventType == "library" && strings.Contains(e.Message, fsinfo.RootMarker)
				}
				if !found {
					t.Error("the action is not in Activity")
				}
			}
		})
	}
}

// The folder check says what the button needs to know, and only for a library
// folder that lacks its marker.
func TestFolderCheckOffersTheMarker(t *testing.T) {
	old := onContainerDisk
	onContainerDisk = func(string) bool { return false }
	t.Cleanup(func() { onContainerDisk = old })
	l := newTempLayout(t)

	if p := l.s.folderPayloadFor(l.downloads); p.Marker != nil {
		t.Errorf("a folder that is not a library folder offers a marker: %+v", p.Marker)
	}
	p := l.s.folderPayloadFor(l.movies)
	if p.Marker == nil || !p.Marker.CanCreate || p.Marker.Seen || p.Marker.Strong {
		t.Fatalf("a new library folder: %+v", p.Marker)
	}
	if err := fsinfo.WriteRootMarker(l.movies); err != nil {
		t.Fatal(err)
	}
	if p := l.s.folderPayloadFor(l.movies); p.Marker != nil {
		t.Errorf("a folder with its marker offers one: %+v", p.Marker)
	}
	// Seen once, then lost.
	l.s.setRootConfirmed(l.movies, true)
	if err := os.Remove(filepath.Join(l.movies, fsinfo.RootMarker)); err != nil {
		t.Fatal(err)
	}
	if p := l.s.folderPayloadFor(l.movies); p.Marker == nil || !p.Marker.Seen {
		t.Errorf("a folder that lost its marker: %+v", p.Marker)
	}
}

// The marker lookup says present or missing; a read that fails for another reason is
// reported as an error, not as a lost file (not testable here without a broken drive).
func TestMarkerState(t *testing.T) {
	dir := t.TempDir()
	if missing, err := fsinfo.RootMarkerState(dir); !missing || err != nil {
		t.Errorf("no marker: missing=%v err=%v", missing, err)
	}
	if err := fsinfo.WriteRootMarker(dir); err != nil {
		t.Fatal(err)
	}
	if missing, err := fsinfo.RootMarkerState(dir); missing || err != nil {
		t.Errorf("marker there: missing=%v err=%v", missing, err)
	}
}
