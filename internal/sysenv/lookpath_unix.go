//go:build !windows

package sysenv

import "os"

func candidates(name string, _ Env) []string { return []string{name} }

func executable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}
