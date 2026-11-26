package ops

import (
	"runtime"
	"strconv"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// colimaDefaultLimits picks first-start VM limits for a new Colima profile on
// macOS: half the host CPUs clamped to 2..4, and ~25% of host RAM clamped to
// 2..8 GiB (Colima's default 2 GiB OOMs quickly on multi-container stacks,
// while Apple VZ retains high-water RAM until VM restart so 25% capped at
// 8 GiB avoids starving macOS).
func colimaDefaultLimits(cpus int, memBytes uint64) (cpu, memoryGiB int) {
	cpu = max(2, min(4, cpus/2))
	const gib = 1 << 30
	if memBytes == 0 {
		return cpu, 2
	}
	memoryGiB = max(2, min(8, int(memBytes/gib)/4))
	return cpu, memoryGiB
}

func colimaFirstStart() []string {
	cpu, mem := colimaDefaultLimits(runtime.NumCPU(), sysenv.TotalMemoryBytes())
	flags := []string{
		"--cpu", strconv.Itoa(cpu),
		"--memory", strconv.Itoa(mem),
		"--vm-type", "vz",
	}
	if runtime.GOARCH == "arm64" {
		flags = append(flags, "--vz-rosetta")
	}
	return append(flags, "--mount-type", "virtiofs")
}
