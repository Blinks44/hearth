//go:build !windows

package disk

import (
	"io/fs"
	"syscall"
)

// stat returns the space a file takes, its blocks as du counts them, its
// identity, and how many hard links lead to it; a folder has 1.
func stat(_ string, info fs.FileInfo) (int64, fileID, int) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size(), fileID{}, 1
	}
	if info.IsDir() {
		return st.Blocks * 512, fileID{}, 1
	}
	return int64(st.Blocks) * 512, fileID{uint64(st.Dev), uint64(st.Ino)}, int(st.Nlink) //nolint:unconvert // Dev and Ino differ by OS
}
