//go:build !windows

package ops

import "syscall"

const forwardAddrInUse = syscall.EADDRINUSE
