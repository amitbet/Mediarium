package fsinfo

import (
	"runtime"
	"strconv"
)

// SameVolumeKeys turns what is known about the filesystem holding a folder
// into a list of keys. Two folders sharing any key are on the same volume.
//
// idKeys are the identities the system gave (device number, filesystem id, or
// a drive letter). Each is tied to the volume's size: Docker Desktop gives
// every folder shared from the host the same filesystem id, whichever drive
// it is on, so an identity alone would make one disk out of all of them. Two
// volumes of different sizes are never the same one. When bySpace is true the
// folder's space figures are added as one more key: paths on one volume get
// identical total and free space from the system, even when they are
// separate mounts or subvolumes with different device numbers (a Synology or
// Btrfs volume is the usual case).
func SameVolumeKeys(idKeys []string, u Usage, bySpace bool) []string {
	keys := make([]string, 0, len(idKeys)+1)
	total := strconv.FormatUint(u.TotalBytes, 10)
	for _, id := range idKeys {
		if u.TotalBytes > 0 {
			id += "|" + total
		}
		keys = append(keys, id)
	}
	if bySpace && u.TotalBytes > 0 {
		keys = append(keys, "space:"+total+"/"+strconv.FormatUint(u.FreeBytes, 10))
	}
	return keys
}

// VolumeKeys returns the keys that identify the volume holding path (see
// SameVolumeKeys). On Windows the drive letter alone is used.
func VolumeKeys(path string, u Usage) []string {
	var ids []string
	if id, ok := FilesystemID(path); ok {
		ids = append(ids, id)
	}
	if id, ok := statfsID(path); ok {
		ids = append(ids, id)
	}
	return SameVolumeKeys(ids, u, runtime.GOOS != "windows")
}
