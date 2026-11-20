package docker

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseSize reads a size as Docker writes it, in decimal units:
// "0B", "769.4MB", "11.66GB".
func ParseSize(s string) (int64, error) {
	units := []struct {
		suffix string
		scale  float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}}
	for _, u := range units {
		if num, ok := strings.CutSuffix(s, u.suffix); ok {
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, fmt.Errorf("size %q: %w", s, err)
			}
			return int64(math.Round(f * u.scale)), nil
		}
	}
	return 0, fmt.Errorf("size %q: unknown unit", s)
}
