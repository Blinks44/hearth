//go:build !windows

package ops

import "github.com/Blinks44/hearth/internal/sysenv"

func gitFallbackPaths(sysenv.Env) []string {
	return []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/home/linuxbrew/.linuxbrew/bin",
		"/usr/bin",
	}
}
