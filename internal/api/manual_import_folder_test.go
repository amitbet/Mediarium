package api_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeVideo makes a file big enough to be listed (sparse, so it costs no disk).
func writeVideo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(21 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

type manualList struct {
	Folder           string `json:"folder"`
	HiddenIncomplete int    `json:"hiddenIncomplete"`
	Files            []struct {
		Path       string `json:"path"`
		Incomplete bool   `json:"incomplete"`
	} `json:"files"`
}

func manualPaths(l manualList) string {
	var out []string
	for _, f := range l.Files {
		out = append(out, f.Path)
	}
	return strings.Join(out, ",")
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the folder rules are about Linux paths")
	}
}

func TestManualImportListsAnyFolderAndSkipsWhatIsNotFinished(t *testing.T) {
	skipOnWindows(t)
	_, base, client := loginNewServer(t)
	dir := t.TempDir()
	writeVideo(t, filepath.Join(dir, "Heat.1995.1080p.BluRay.x264-GRP", "Heat.1995.1080p.BluRay.x264-GRP.mkv"))
	writeVideo(t, filepath.Join(dir, "Heat.1995.1080p.BluRay.x264-GRP", "Sample", "heat-sample.mkv"))
	writeVideo(t, filepath.Join(dir, "Heat.1995.1080p.BluRay.x264-GRP", "heat.sample.mkv"))
	writeVideo(t, filepath.Join(dir, "incomplete", "Alien.1979.1080p.BluRay.x264-GRP", "Alien.1979.1080p.BluRay.x264-GRP.mkv"))
	writeVideo(t, filepath.Join(dir, "Incomplete", "Brazil.1985.1080p.BluRay.x264-GRP.mkv"))
	writeVideo(t, filepath.Join(dir, "Seven.1995.1080p.BluRay.x264-GRP.mkv.part"))
	writeVideo(t, filepath.Join(dir, "Se7en.1995.1080p.BluRay.x264-GRP.mkv.!qB"))
	writeVideo(t, filepath.Join(dir, ".unpack-abc", "Hidden.2000.1080p.mkv"))
	writeVideo(t, filepath.Join(dir, "_UNPACK_Wip.2001.1080p.BluRay.x264-GRP", "Wip.2001.1080p.BluRay.x264-GRP.mkv"))
	// A file smaller than 20 MB is an extra, not a title.
	if err := os.WriteFile(filepath.Join(dir, "tiny.mkv"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}

	q := func(extra string) string { return base + "/api/manual-import?folder=" + url.QueryEscape(dir) + extra }

	got := getJSON[manualList](t, client, q(""))
	if want := "Heat.1995.1080p.BluRay.x264-GRP/Heat.1995.1080p.BluRay.x264-GRP.mkv"; manualPaths(got) != want {
		t.Fatalf("finished files: %q, want %q", manualPaths(got), want)
	}
	if got.HiddenIncomplete != 3 {
		t.Errorf("hiddenIncomplete = %d, want 3 (two incomplete folders and an unpack folder)", got.HiddenIncomplete)
	}
	if got.Folder != resolved(t, dir) {
		t.Errorf("folder = %q, want %q", got.Folder, resolved(t, dir))
	}

	all := getJSON[manualList](t, client, q("&incomplete=1"))
	if len(all.Files) != 4 || all.HiddenIncomplete != 0 {
		t.Fatalf("with incomplete: %q (hidden %d)", manualPaths(all), all.HiddenIncomplete)
	}
	marked := 0
	for _, f := range all.Files {
		if f.Incomplete {
			marked++
		}
	}
	if marked != 3 {
		t.Errorf("%d files marked incomplete, want 3", marked)
	}
}

func resolved(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestManualImportModes(t *testing.T) {
	skipOnWindows(t)
	tests := []struct {
		name       string
		mode       string
		wantSource bool // the original is still there afterwards
		wantLinked bool // the library file is the same file as the original
	}{
		{"default links like every import", "", true, true},
		{"link", "link", true, true},
		{"copy is a file of its own", "copy", true, false},
		{"move removes the original", "move", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server, base, client := loginNewServer(t)
			dir := t.TempDir()
			src := filepath.Join(dir, "Heat.1995.1080p.BluRay.x264-GRP", "Heat.1995.1080p.BluRay.x264-GRP.mkv")
			writeVideo(t, src)
			id := seedMovie(t, server.MovieRepo, 949, "Heat", 1995, "")
			body := map[string]any{"folder": dir, "path": "Heat.1995.1080p.BluRay.x264-GRP/Heat.1995.1080p.BluRay.x264-GRP.mkv", "movieId": id, "mode": tc.mode}
			res := postJSON[map[string]any](t, client, base+"/api/manual-import", body, http.StatusOK)
			dest, _ := res["path"].(string)
			if !strings.HasPrefix(dest, server.TestMoviesRoot()) || !exists(dest) {
				t.Fatalf("imported to %q", dest)
			}
			if exists(src) != tc.wantSource {
				t.Errorf("original present = %v, want %v", exists(src), tc.wantSource)
			}
			if tc.wantSource {
				a, _ := os.Stat(src)
				b, _ := os.Stat(dest)
				if linked := os.SameFile(a, b); linked != tc.wantLinked {
					// Linking needs both folders on one drive; the test folders are.
					t.Errorf("same file as the original = %v, want %v", linked, tc.wantLinked)
				}
			}
			if kept, _ := res["sourceKept"].(bool); kept {
				t.Error("sourceKept is set although nothing went wrong")
			}
		})
	}
}

func TestManualImportRefusesWhatItShould(t *testing.T) {
	skipOnWindows(t)
	server, base, client := loginNewServer(t)
	dir := t.TempDir()
	good := "Heat.1995.1080p.BluRay.x264-GRP.mkv"
	writeVideo(t, filepath.Join(dir, good))
	writeVideo(t, filepath.Join(dir, "incomplete", good))
	outside := t.TempDir()
	writeVideo(t, filepath.Join(outside, "Secret.2000.1080p.mkv"))
	if err := os.Symlink(filepath.Join(outside, "Secret.2000.1080p.mkv"), filepath.Join(dir, "link.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	etcLink := filepath.Join(t.TempDir(), "etc-link")
	if err := os.Symlink("/etc", etcLink); err != nil {
		t.Fatal(err)
	}
	id := seedMovie(t, server.MovieRepo, 949, "Heat", 1995, "")

	tests := []struct {
		name string
		body map[string]any
	}{
		{"parent traversal", map[string]any{"folder": dir, "path": "../" + filepath.Base(outside) + "/Secret.2000.1080p.mkv"}},
		{"absolute path", map[string]any{"folder": dir, "path": "/etc/passwd"}},
		{"empty path", map[string]any{"folder": dir, "path": ""}},
		{"file link that leaves the folder", map[string]any{"folder": dir, "path": "link.mkv"}},
		{"folder link that leaves the folder", map[string]any{"folder": dir, "path": "out/Secret.2000.1080p.mkv"}},
		{"incomplete without the switch", map[string]any{"folder": dir, "path": "incomplete/" + good}},
		{"proc", map[string]any{"folder": "/proc", "path": "version"}},
		{"sys", map[string]any{"folder": "/sys", "path": "x.mkv"}},
		{"etc", map[string]any{"folder": "/etc", "path": "x.mkv"}},
		{"the top of the filesystem", map[string]any{"folder": "/", "path": "x.mkv"}},
		{"a link to a system folder", map[string]any{"folder": etcLink, "path": "x.mkv"}},
		{"relative folder", map[string]any{"folder": "downloads", "path": good}},
		{"folder that does not exist", map[string]any{"folder": filepath.Join(dir, "nope"), "path": good}},
		{"a file as the folder", map[string]any{"folder": filepath.Join(dir, good), "path": good}},
		{"unknown mode", map[string]any{"folder": dir, "path": good, "mode": "teleport"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.body["movieId"] = id
			if code, body := doJSONStatus(t, client, http.MethodPost, base+"/api/manual-import", tc.body); code != http.StatusBadRequest {
				t.Errorf("status %d (%v), want 400", code, body)
			}
		})
	}
	// Everything above left the files alone.
	for _, p := range []string{filepath.Join(dir, good), filepath.Join(outside, "Secret.2000.1080p.mkv")} {
		if !exists(p) {
			t.Errorf("%s was touched", p)
		}
	}

	// With the switch on, the incomplete file is accepted.
	postJSON[map[string]any](t, client, base+"/api/manual-import", map[string]any{"folder": dir, "path": "incomplete/" + good, "movieId": id, "incomplete": true}, http.StatusOK)
}

func TestManualImportListRefusesSystemFolders(t *testing.T) {
	skipOnWindows(t)
	_, base, client := loginNewServer(t)
	for _, folder := range []string{"/proc", "/sys", "/etc", "/", "relative/path", "/definitely/not/here"} {
		code, body := doJSONStatus(t, client, http.MethodGet, base+"/api/manual-import?folder="+url.QueryEscape(folder), nil)
		if code != http.StatusBadRequest {
			t.Errorf("%q: status %d (%v), want 400", folder, code, body)
		}
	}
}

func TestManualImportUnreadableFolderSaysSo(t *testing.T) {
	skipOnWindows(t)
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	_, base, client := loginNewServer(t)
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	code, body := doJSONStatus(t, client, http.MethodGet, base+"/api/manual-import?folder="+url.QueryEscape(dir), nil)
	if code != http.StatusBadRequest || !strings.Contains(body["error"].(string), "can't read") {
		t.Errorf("status %d, body %v", code, body)
	}
}

func TestManualImportFolderChooser(t *testing.T) {
	skipOnWindows(t)
	_, base, client := loginNewServer(t)
	dir := t.TempDir()
	for _, d := range []string{"b-films", "A-shows", ".hidden", "@eaDir"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeVideo(t, filepath.Join(dir, "loose.mkv"))
	type answer struct {
		Path    string `json:"path"`
		Parent  string `json:"parent"`
		Folders []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"folders"`
		Shortcuts []struct{ Path string } `json:"shortcuts"`
	}
	got := getJSON[answer](t, client, base+"/api/manual-import/folders?path="+url.QueryEscape(dir))
	var names []string
	for _, f := range got.Folders {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "A-shows,b-films" {
		t.Errorf("folders = %v, want only the visible folders, sorted", names)
	}
	if got.Parent != filepath.Dir(resolved(t, dir)) || len(got.Shortcuts) == 0 {
		t.Errorf("parent %q, shortcuts %v", got.Parent, got.Shortcuts)
	}

	top := getJSON[answer](t, client, base+"/api/manual-import/folders")
	for _, f := range top.Folders {
		switch f.Name {
		case "proc", "sys", "dev", "etc", "usr", "bin", "sbin", "lib", "boot", "run", "root":
			t.Errorf("the system folder %s is offered", f.Name)
		}
	}
	if code, _ := doJSONStatus(t, client, http.MethodGet, base+"/api/manual-import/folders?path=%2Fproc", nil); code != http.StatusBadRequest {
		t.Errorf("/proc: status %d, want 400", code)
	}
}
