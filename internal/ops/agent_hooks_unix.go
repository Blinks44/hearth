//go:build !windows

package ops

import "strings"

// quoteHookExe safely quotes an executable path for a POSIX /bin/sh -c hook
// command without expanding $, `, or other shell metacharacters.
func quoteHookExe(exe string) string {
	if exe == "" || strings.ContainsAny(exe, " \t\n\"'$\\;&|<>*?()[]{}`!#~") {
		return "'" + strings.ReplaceAll(exe, "'", `'\''`) + "'"
	}
	return exe
}
