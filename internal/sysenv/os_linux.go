package sysenv

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func readOS() (OS, error) {
	// os-release(5): /etc first, /usr/lib as the fallback.
	data, err := os.ReadFile("/etc/os-release")
	if errors.Is(err, os.ErrNotExist) {
		data, err = os.ReadFile("/usr/lib/os-release")
	}
	if err != nil {
		return OS{Name: "Linux"}, fmt.Errorf("read os-release: %w", err)
	}
	return parseOSRelease(data), nil
}

func totalMemoryBytes() uint64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return info.Totalram * uint64(info.Unit)
}
