//go:build !windows

package sysenv

func sameKey(a, b string) bool { return a == b }
