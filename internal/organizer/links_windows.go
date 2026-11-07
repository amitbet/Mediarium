//go:build windows

package organizer

import "io/fs"

// LinkCount is how many names a file has on disk. It is not read on Windows,
// so a file counts as having one.
func LinkCount(fs.FileInfo) uint64 { return 1 }
