package sysenv

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func readOS() (OS, error) {
	v := windows.RtlGetVersion()
	return OS{Name: "Windows", Version: fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)}, nil
}
