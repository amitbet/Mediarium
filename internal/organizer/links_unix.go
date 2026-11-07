//go:build !windows

package organizer

import (
	"io/fs"
	"syscall"
)

// LinkCount is how many names a file has on disk: more than 1 means it is
// hard-linked, for example to a torrent that is still seeding.
func LinkCount(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}
