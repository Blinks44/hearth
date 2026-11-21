//go:build !windows

package docker

import (
	"io/fs"
	"syscall"
)

func allocatedSize(_ string, info fs.FileInfo) (int64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Blocks * 512, true
}
