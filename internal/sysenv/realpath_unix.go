//go:build !windows

package sysenv

import "path/filepath"

// RealPath is path with symlinks resolved.
func RealPath(path string) (string, error) { return filepath.EvalSymlinks(path) }
