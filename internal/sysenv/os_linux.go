package sysenv

import (
	"errors"
	"fmt"
	"os"
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
