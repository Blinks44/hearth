package sysenv

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func readOS() (OS, error) {
	v := windows.RtlGetVersion()
	// Windows 11 still reports kernel version 10.0 through RtlGetVersion.
	if v.ProductType == 1 && v.MajorVersion == 10 {
		release := 10
		if v.BuildNumber >= 22000 {
			release = 11
		}
		return OS{Name: "Windows", Version: fmt.Sprintf("%d (%d)", release, v.BuildNumber)}, nil
	}
	return OS{Name: "Windows", Version: fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)}, nil
}
