//go:build !windows

package detect

import "golang.org/x/sys/unix"

// writable reports that this user may create files in dir.
func writable(dir string) bool { return unix.Access(dir, unix.W_OK) == nil }
