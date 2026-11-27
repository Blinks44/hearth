//go:build windows

package ai

import "strings"

func normalizeRadarPath(p string) string {
	return strings.ToLower(p)
}
