//go:build !windows

package ops

func instructionLinkDenied(error) bool { return false }
