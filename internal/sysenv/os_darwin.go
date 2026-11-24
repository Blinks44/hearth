package sysenv

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func readOS() (OS, error) {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return OS{Name: "macOS"}, fmt.Errorf("read macOS version: %w", err)
	}
	return OS{Name: "macOS", Version: v}, nil
}

func totalMemoryBytes() uint64 {
	v, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return v
}
