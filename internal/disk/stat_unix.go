//go:build !windows

package disk

import (
	"io/fs"
	"syscall"
)

// stat returns the space a file takes, its blocks as du counts them, and
// its identity when hard links may lead to it.
func stat(_ string, info fs.FileInfo) (int64, fileID, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size(), fileID{}, false
	}
	return int64(st.Blocks) * 512, fileID{uint64(st.Dev), uint64(st.Ino)}, !info.IsDir() && st.Nlink > 1 //nolint:unconvert // Dev and Ino differ by OS
}
