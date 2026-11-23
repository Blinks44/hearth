//go:build !darwin

package main

// setAppMenu is a no-op outside macOS: Windows and Linux windows render a
// custom titlebar without a native top menu bar, and Hearth exposes app update
// checks in Settings, the Updates page banner, and the system tray menu.
func setAppMenu(func()) {}
