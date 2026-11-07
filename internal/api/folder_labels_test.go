package api_test

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/api"
)

func TestFolderLabelsAreUniqueAndPathFree(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{"names of the folders", []string{"/data/movies", "/mnt/usb/kids"}, []string{"movies", "kids"}},
		{"two folders with the same name are numbered", []string{"/a/movies", "/b/movies", "/c/movies"}, []string{"movies", "movies 2", "movies 3"}},
		{"a trailing slash does not matter", []string{"/data/movies/", "/x/tv"}, []string{"movies", "tv"}},
		{"the root has a name too", []string{"/"}, []string{"Folder"}},
		{"nothing", nil, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := api.FolderLabelsForTest(tc.paths)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("labels = %q, want %q", got, tc.want)
			}
			for _, l := range got {
				if strings.ContainsAny(l, `/\`) {
					t.Errorf("label %q holds a path", l)
				}
			}
		})
	}
}

// A member sees the folders by name, not by path, and can still pick one.
func TestLibraryFoldersForMembersAreLabels(t *testing.T) {
	server, base, admin, member, _ := familyServer(t)
	server.TestSetTMDBBaseURL("k", newTitleTMDB(t).URL)
	main, usb, kids := t.TempDir(), t.TempDir(), t.TempDir()
	putJSONStatus(t, admin, base+"/api/settings", map[string]any{"moviesPath": main, "moviesExtraPaths": []string{usb, kids}}, http.StatusOK)

	adminFolders := getJSON[folderLists](t, admin, base+"/api/library/folders")
	if len(adminFolders.Movies) != 3 || adminFolders.Movies[0] != main {
		t.Fatalf("administrator should get the full paths: %v", adminFolders)
	}

	got := getJSON[folderLists](t, member, base+"/api/library/folders")
	want := []string{filepath.Base(main), filepath.Base(usb), filepath.Base(kids)}
	if strings.Join(got.Movies, "|") != strings.Join(want, "|") {
		t.Fatalf("member sees %v, want the folder names %v", got.Movies, want)
	}
	for _, l := range got.Movies {
		if strings.Contains(l, "/") || strings.Contains(l, `\`) {
			t.Fatalf("member got a path: %q", l)
		}
	}

	// The label picks the folder; the title then shows the label, not the path.
	added := postJSON[map[string]any](t, member, base+"/api/movies", map[string]any{"tmdbId": 603, "rootPath": got.Movies[1]}, http.StatusCreated)
	if added["rootPath"] != got.Movies[1] {
		t.Fatalf("the member's answer should name the folder by label: %v", added["rootPath"])
	}
	id := int64(added["id"].(float64))
	one := getJSON[map[string]any](t, member, fmt.Sprintf("%s/api/movies/%d", base, id))
	if one["rootPath"] != got.Movies[1] {
		t.Errorf("member sees rootPath %v, want %q", one["rootPath"], got.Movies[1])
	}
	adminOne := getJSON[map[string]any](t, admin, fmt.Sprintf("%s/api/movies/%d", base, id))
	if filepath.Clean(adminOne["rootPath"].(string)) != filepath.Clean(usb) {
		t.Errorf("the title is stored with the real path: %v", adminOne["rootPath"])
	}
	// Changing the folder with a label works, and a label that names nothing is refused.
	putJSONStatus(t, member, fmt.Sprintf("%s/api/movies/%d/folder", base, id), map[string]any{"rootPath": got.Movies[2]}, http.StatusOK)
	putJSONStatus(t, member, fmt.Sprintf("%s/api/movies/%d/folder", base, id), map[string]any{"rootPath": "nowhere"}, http.StatusBadRequest)
	adminOne = getJSON[map[string]any](t, admin, fmt.Sprintf("%s/api/movies/%d", base, id))
	if filepath.Clean(adminOne["rootPath"].(string)) != filepath.Clean(kids) {
		t.Errorf("the label should have moved the title to %s: %v", kids, adminOne["rootPath"])
	}
}
