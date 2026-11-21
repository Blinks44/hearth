//go:build !windows && !darwin

package sysenv

import (
	"os"
	"strings"
)

const defaultUnixLoginPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// loginPath is the baseline PATH a new login shell starts with on Linux and
// other Unix systems: PAM's /etc/environment PATH when set, else the standard
// system directories.
func loginPath() string {
	if p := etcEnvironmentPath("/etc/environment"); p != "" {
		return p
	}
	return defaultUnixLoginPath
}

func etcEnvironmentPath(file string) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != "PATH" {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "\"'")
		if v != "" {
			return v
		}
	}
	return ""
}
