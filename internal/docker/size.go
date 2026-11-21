package docker

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseSize reads a size as Docker writes it in system df or stats:
// "0B", "769.4MB", "11.66GB".
func ParseSize(s string) (int64, error) {
	units := []struct {
		suffix string
		scale  float64
	}{{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}}
	for _, u := range units {
		if num, ok := strings.CutSuffix(s, u.suffix); ok {
			f, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
			if err != nil {
				return 0, fmt.Errorf("size %q: %w", s, err)
			}
			if f < 0 || math.IsNaN(f) || f*u.scale >= math.MaxInt64 {
				return 0, fmt.Errorf("size %q: out of range", s)
			}
			return int64(math.Round(f * u.scale)), nil
		}
	}
	return 0, fmt.Errorf("size %q: unknown unit", s)
}
