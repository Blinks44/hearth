//go:build !windows

package ui

// whichCommand and whichAllCommand are how the terminal shows the first
// copy of a command in PATH and all of them.
const (
	whichCommand    = "which"
	whichAllCommand = "which -a"
)
