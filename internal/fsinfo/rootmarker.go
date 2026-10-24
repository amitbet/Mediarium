package fsinfo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// RootMarker is the name of the small hidden file Mediarium keeps in a library
// folder once it knows the folder is the real one. When a network drive isn't
// connected, its folder is an empty one on the machine's own disk, and the
// marker is what tells the two apart.
const RootMarker = ".mediarium-root"

// RootReachable reports whether root is a folder that can be read right now.
func RootReachable(root string) bool {
	info, err := os.Stat(root)
	return err == nil && info.IsDir()
}

// HasRootMarker reports whether the marker file is in root.
func HasRootMarker(root string) bool {
	_, err := os.Lstat(filepath.Join(root, RootMarker))
	return err == nil
}

// RootMarkerState looks for the marker. missing is true when the file is not
// there; err is set when the folder could not be read to tell (a network drive
// that stumbled), in which case nothing can be said about the marker. A
// failed read is tried again a few times first.
func RootMarkerState(root string) (missing bool, err error) {
	path := filepath.Join(root, RootMarker)
	for try := 0; try < 3; try++ {
		_, err = os.Lstat(path)
		switch {
		case err == nil:
			return false, nil
		case errors.Is(err, fs.ErrNotExist):
			return true, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false, err
}

// WriteRootMarker puts the marker in root. A folder that can't be written to
// just stays without one.
func WriteRootMarker(root string) error {
	f, err := os.OpenFile(filepath.Join(root, RootMarker), os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// RootHasContent reports whether root holds anything besides the marker.
func RootHasContent(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() != RootMarker {
			return true
		}
	}
	return false
}
