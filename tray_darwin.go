//go:build darwin

package main

import "github.com/Blinks44/hearth/resources"

func trayIcon() (png []byte, template bool) {
	return resources.TrayIconMacOS, true
}
