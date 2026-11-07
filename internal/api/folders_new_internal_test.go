package api

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/fsinfo"
	"github.com/rdborg/mediarium/internal/settings"
)

func fakeUsage(free map[string]uint64) func(string) (fsinfo.Usage, error) {
	return func(p string) (fsinfo.Usage, error) {
		normalized := make(map[string]uint64, len(free))
		for path, value := range free {
			normalized[filepath.Clean(path)] = value
		}
		f, ok := normalized[filepath.Clean(p)]
		if !ok {
			return fsinfo.Usage{}, errors.New("not connected")
		}
		return fsinfo.Usage{TotalBytes: 100 * f, FreeBytes: f}, nil
	}
}

func TestMostRoomPicksTheEmptiestFolder(t *testing.T) {
	offered := []string{"/movies", "/movies2", "/movies3"}
	tests := []struct {
		name string
		free map[string]uint64
		want string
	}{
		{"the second drive has the most room", map[string]uint64{"/movies": 10, "/movies2": 60, "/movies3": 40}, "/movies2"},
		{"a tie goes to the main folder", map[string]uint64{"/movies": 60, "/movies2": 60}, "/movies"},
		{"a drive that isn't connected is passed over", map[string]uint64{"/movies": 10, "/movies3": 20}, "/movies3"},
		{"nothing readable gives nothing", map[string]uint64{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mostRoom(offered, fakeUsage(tc.free)); got != tc.want {
				t.Fatalf("mostRoom = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckNewTitleRule(t *testing.T) {
	extra := []string{"/movies2", "/kids"}
	for _, tc := range []struct{ in, want, problem string }{
		{"", "", ""},
		{"  ", "", ""},
		{mostRoomFolder, mostRoomFolder, ""},
		{"/movies2", "/movies2", ""},
		{"/kids/", "/kids", ""},
		{"/movies", "", "New movies can only go to"}, // the main folder is "" here, never its path
		{"/elsewhere", "", "New movies can only go to"},
	} {
		got, problem := checkNewTitleRule(tc.in, extra, "movies")
		if got != filepath.FromSlash(tc.want) || (tc.problem == "" && problem != "") || (tc.problem != "" && !strings.Contains(problem, tc.problem)) {
			t.Fatalf("checkNewTitleRule(%q) = %q, %q; want %q, %q", tc.in, got, problem, tc.want, tc.problem)
		}
	}
}

// The stored rule turns into what goes on a title: nothing for the main
// folder, the path of an extra folder, and for "most room" whichever has the
// most free space when asked. A rule naming a folder that was taken out of
// the list falls back to the main one.
func TestNewTitleFolderFollowsTheRule(t *testing.T) {
	s, _ := optionsServer(t)
	set := func(key, v string) {
		t.Helper()
		if err := s.Settings.Set(key, v, false); err != nil {
			t.Fatal(err)
		}
	}
	set(settings.KeyMoviesPath, "/movies")
	set(settings.KeyMoviesExtraPaths, `["/movies2","/kids"]`)

	old := folderUsage
	t.Cleanup(func() { folderUsage = old })
	free := map[string]uint64{"/movies": 10, "/movies2": 60, "/kids": 20}
	folderUsage = fakeUsage(free)

	if got := s.newMovieFolder(); got != "" {
		t.Fatalf("no rule: the main folder, got %q", got)
	}
	set(settings.KeyMoviesNewFolder, "/kids")
	if got := s.newMovieFolder(); got != filepath.FromSlash("/kids") {
		t.Fatalf("a chosen extra folder, got %q", got)
	}
	set(settings.KeyMoviesNewFolder, mostRoomFolder)
	if got := s.newMovieFolder(); got != filepath.FromSlash("/movies2") {
		t.Fatalf("the folder with the most room, got %q", got)
	}
	free["/movies"] = 99
	if got := s.newMovieFolder(); got != "" {
		t.Fatalf("when the main folder has the most room nothing is stored, got %q", got)
	}
	set(settings.KeyMoviesNewFolder, "/gone")
	if got := s.newMovieFolder(); got != "" {
		t.Fatalf("a folder no longer in the list reads as the main one, got %q", got)
	}
	if got := s.newTitleRule(settings.KeyMoviesNewFolder, s.movieFolders()); got != "" {
		t.Fatalf("the settings show the main folder for a stale rule, got %q", got)
	}

	// The answer for the dialogs: the resolved folder, as shown to the person.
	set(settings.KeyMoviesNewFolder, mostRoomFolder)
	free["/movies"] = 10
	offered := s.movieFolders()
	p := s.newTitlePayloadFor(settings.KeyMoviesNewFolder, offered, folderLabels(offered))
	if p.Folder != "movies2" || !p.MostRoom {
		t.Fatalf("payload for a member = %+v", p)
	}
	p = s.newTitlePayloadFor(settings.KeyMoviesNewFolder, offered, offered)
	if p.Folder != filepath.FromSlash("/movies2") || !p.MostRoom {
		t.Fatalf("payload for an administrator = %+v", p)
	}

	// With a single folder there is nothing to choose, whatever the rule says.
	set(settings.KeyMoviesExtraPaths, "")
	if got := s.newMovieFolder(); got != "" {
		t.Fatalf("one folder only, got %q", got)
	}
}
