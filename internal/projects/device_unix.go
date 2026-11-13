//go:build !windows

package projects

import (
	"io/fs"
	"syscall"
)

// device is the file system a file is on, from its info; false when not
// known.
func device(info fs.FileInfo) (uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true //nolint:unconvert,nolintlint // Dev is int32 on macOS, uint64 on Linux
}
