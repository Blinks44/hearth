//go:build !linux

package main

// titleBarHeight keeps the system's default: Windows 11's 32 pixels; macOS
// ignores it.
const titleBarHeight = 0
