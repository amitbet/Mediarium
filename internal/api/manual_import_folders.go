package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rdborg/mediarium/internal/cleanup"
	"github.com/rdborg/mediarium/internal/organizer"
)

// The folder a manual import looks in can be any folder the container can
// read, not only the downloads folder. These helpers pick the folder safely
// and tell finished files from ones still being downloaded.

// manualFolder turns what the person chose into the folder to look in: the
// downloads folder when raw is empty, otherwise a full path to a readable
// folder with its links resolved. The system's own folders are refused.
func (s *Server) manualFolder(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = s.downloadsRoot()
	}
	if raw == "" || !filepath.IsAbs(raw) || len(raw) > maxPathLen || strings.ContainsRune(raw, 0) {
		return "", errors.New("Enter the full path of a folder, for example /downloads.")
	}
	clean := filepath.Clean(raw)
	if manualSystemPath(clean) {
		return "", errors.New("That folder can't be used: it belongs to the system.")
	}
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return "", errors.New("Mediarium can't read that folder. Give the user it runs as (its PUID and PGID) read access, or check that the folder is mapped into the container.")
		}
		return "", errors.New("That folder doesn't exist. Check the path, and that the folder is mapped into the container.")
	}
	if manualSystemPath(real) {
		return "", errors.New("That folder can't be used: it belongs to the system.")
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", errors.New("That folder doesn't exist. Check the path, and that the folder is mapped into the container.")
	}
	if !info.IsDir() {
		return "", errors.New("That path isn't a folder.")
	}
	if _, err := os.ReadDir(real); err != nil {
		return "", errors.New("Mediarium can't read that folder. Give the user it runs as (its PUID and PGID) read access, or check that the folder is mapped into the container.")
	}
	return real, nil
}

// manualSystemPath says clean is the top of the filesystem or one of the
// system's own folders (or inside one).
func manualSystemPath(clean string) bool { return !insideWorthChecking(clean) }

// incompleteFolder matches the folder names download programs keep unfinished
// files in (qBittorrent's incomplete folder, SABnzbd's unpack folders).
var incompleteFolder = regexp.MustCompile(`(?i)^(incomplete|incomplete downloads|_unpack_.*|\.unpack.*)$`)

// inIncompleteFolder says a path, relative to the folder being looked at, is
// inside a folder where downloads are still being written.
func inIncompleteFolder(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, dir := range parts[:max(len(parts)-1, 0)] {
		if incompleteFolder.MatchString(dir) {
			return true
		}
	}
	return false
}

// partialSuffixes are what download programs add to a file that isn't finished.
var partialSuffixes = []string{".part", ".!qb", ".!bt", ".crdownload", ".partial", ".tmp", ".temp", ".download", ".bc!", ".mediarium-tmp"}

// isPartialName says a file name is one a download program is still writing.
func isPartialName(name string) bool {
	low := strings.ToLower(name)
	for _, suffix := range partialSuffixes {
		if strings.HasSuffix(low, suffix) {
			return true
		}
	}
	return strings.HasPrefix(low, ".unpack-") || strings.HasPrefix(low, "_unpack_")
}

// sampleFile matches a file called sample, or one with sample set apart in its name.
var sampleFile = regexp.MustCompile(`(?i)(^|[._\-\s\[(])sample([._\-\s\])]|$)`)

// isSampleFile says the path, relative to the folder being looked at, is a
// release's sample clip: a file named like one, or one in a Sample folder.
func isSampleFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	for _, dir := range parts[:max(len(parts)-1, 0)] {
		if strings.EqualFold(dir, "sample") || strings.EqualFold(dir, "samples") {
			return true
		}
	}
	base := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
	return sampleFile.MatchString(base)
}

// manualSource turns a path relative to the chosen folder into the file it
// names, refusing anything outside that folder, a link, or not a video file.
// A file in an incomplete downloads folder needs incomplete to be true.
func (s *Server) manualSource(folder, rel string, incomplete bool) (string, error) {
	root, err := s.manualFolder(folder)
	if err != nil {
		return "", err
	}
	rel = strings.TrimSpace(rel)
	if rel == "" || filepath.IsAbs(rel) {
		return "", errors.New("Choose a file from the list.")
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	real, err := cleanup.InsideRoot(root, path)
	if err != nil {
		return "", errors.New("That file isn't in the chosen folder.")
	}
	info, err := os.Lstat(real)
	if err != nil || !info.Mode().IsRegular() || !organizer.IsVideoFile(real) || isPartialName(filepath.Base(real)) {
		return "", errors.New("That isn't a video file in the chosen folder any more.")
	}
	if !incomplete && inIncompleteFolder(rel) {
		return "", errors.New("That file is in an incomplete downloads folder, so it may not be finished. Turn on Show incomplete downloads to import it anyway.")
	}
	return real, nil
}

// manualMode is how a file comes into the library by hand.
type manualMode string

const (
	manualLink manualMode = "link" // hardlink when the drives allow it, else a copy (what every import does)
	manualCopy manualMode = "copy" // always a copy of its own
	manualMove manualMode = "move" // like link, then the original goes away
)

// parseManualMode reads the mode of a request; empty means the usual way.
func parseManualMode(s string) (manualMode, bool) {
	switch manualMode(strings.ToLower(strings.TrimSpace(s))) {
	case "", manualLink:
		return manualLink, true
	case manualCopy:
		return manualCopy, true
	case manualMove:
		return manualMove, true
	}
	return "", false
}

// manualPlace puts src at dest the way mode says. The original stays where it
// is; manualFinish removes it for a move.
func manualPlace(src, dest string, mode manualMode) (organizer.ImportResult, error) {
	if mode == manualCopy {
		return organizer.ImportCopy(src, dest, organizer.ConflictSkip, nil)
	}
	return organizer.Import(src, dest, organizer.ConflictSkip, nil)
}

// manualFinish removes the original after a move, once the file is in the
// library. It reports whether the original is still there (it could not be
// removed, or it is the very file now in the library).
func manualFinish(src, dest string, mode manualMode) (kept bool) {
	if mode != manualMove {
		return false
	}
	if a, err := filepath.EvalSymlinks(src); err == nil {
		if b, err := filepath.EvalSymlinks(dest); err == nil && a == b {
			return true // the file already was in the library: never remove it
		}
	}
	return os.Remove(src) != nil
}

// folderEntry is one folder in the folder chooser.
type folderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type manualFoldersPayload struct {
	Path      string        `json:"path"`
	Parent    string        `json:"parent,omitempty"`
	Folders   []folderEntry `json:"folders"`
	Shortcuts []folderEntry `json:"shortcuts,omitempty"` // the downloads and library folders, to start from
}

// handleManualImportFolders lists the folders inside a folder (the top of
// the filesystem when none is given), for choosing where a manual import
// looks. Only folders are listed, and the system's own folders are left out.
func (s *Server) handleManualImportFolders(w http.ResponseWriter, r *http.Request) {
	out := manualFoldersPayload{Folders: []folderEntry{}}
	seen := map[string]bool{}
	for _, root := range append([]string{s.downloadsRoot()}, s.videoRoots()...) {
		if root = strings.TrimSpace(root); root != "" && filepath.IsAbs(root) && !seen[root] {
			seen[root] = true
			out.Shortcuts = append(out.Shortcuts, folderEntry{Name: root, Path: root})
		}
	}
	want := strings.TrimSpace(r.URL.Query().Get("path"))
	if want == "" {
		want = string(filepath.Separator)
	}
	var dir string
	if want == string(filepath.Separator) {
		dir = want // the top may be browsed; its system folders are left out below
	} else {
		var err error
		if dir, err = s.manualFolder(want); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Mediarium can't read that folder. Give the user it runs as (its PUID and PGID) read access, or check that the folder is mapped into the container.")
		return
	}
	out.Path = dir
	if parent := filepath.Dir(dir); parent != dir {
		out.Parent = parent
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "@") {
			continue
		}
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err != nil || !info.IsDir() || manualSystemPath(full) {
			continue
		}
		out.Folders = append(out.Folders, folderEntry{Name: name, Path: full})
	}
	sort.SliceStable(out.Folders, func(i, j int) bool {
		return strings.ToLower(out.Folders[i].Name) < strings.ToLower(out.Folders[j].Name)
	})
	writeJSON(w, http.StatusOK, out)
}
