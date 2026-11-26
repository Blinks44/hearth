package ops

import "testing"

func TestColimaDefaultLimits(t *testing.T) {
	t.Parallel()
	const gib = uint64(1 << 30)
	cases := []struct {
		name     string
		cpus     int
		memBytes uint64
		wantCPU  int
		wantMem  int
	}{
		{"unknown memory", 8, 0, 4, 2},
		{"8GB 4-core Mac", 4, 8 * gib, 2, 2},
		{"16GB 8-core Mac", 8, 16 * gib, 4, 4},
		{"18GB 11-core Mac", 11, 18 * gib, 4, 4},
		{"24GB 10-core Mac", 10, 24 * gib, 4, 6},
		{"32GB 12-core Mac", 12, 32 * gib, 4, 8},
		{"64GB 16-core Mac", 16, 64 * gib, 4, 8},
		{"2-core low spec", 2, 4 * gib, 2, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cpu, mem := colimaDefaultLimits(tc.cpus, tc.memBytes)
			if cpu != tc.wantCPU || mem != tc.wantMem {
				t.Errorf("colimaDefaultLimits(%d, %d) = (%d, %d), want (%d, %d)",
					tc.cpus, tc.memBytes, cpu, mem, tc.wantCPU, tc.wantMem)
			}
		})
	}
}
