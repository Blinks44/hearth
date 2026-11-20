package ops

import "runtime"

func colimaFirstStart() []string {
	flags := []string{"--vm-type", "vz"}
	if runtime.GOARCH == "arm64" {
		flags = append(flags, "--vz-rosetta")
	}
	return append(flags, "--mount-type", "virtiofs")
}
