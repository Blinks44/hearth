package sysenv

import "runtime"

// OS describes the operating system Hearth runs on.
type OS struct {
	// Name is "macOS", "Windows" or the Linux distribution's name.
	Name string
	// Version is like "15.2", "10.0.26100" or "24.04"; empty when unknown.
	Version string
	// Arch is runtime.GOARCH: "arm64", "amd64".
	Arch string
}

// ReadOS describes this OS. On an error the fields it could fill are set.
func ReadOS() (OS, error) {
	o, err := readOS()
	o.Arch = runtime.GOARCH
	return o, err
}
