//go:build !windows

package ai

func normalizeRadarPath(p string) string {
	return p
}
