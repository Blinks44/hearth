//go:build windows

package main

import (
	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/resources"
)

func trayIcon() (png []byte, template bool) {
	return resources.TrayIconColor, false
}

func (t *trayController) initialTrayMenu() *mygo.Menu {
	return t.buildCompactSystemMenu()
}

func (t *trayController) bindTrayClicks() {
	// On Windows, left-click fires OnClick (flyout) while right-click opens
	// the attached context menu natively.
	t.tray.OnClick(func() {
		t.togglePopover()
	})
}

func (t *trayController) syncTrayMenu() {
	t.tray.SetMenu(t.buildCompactSystemMenu())
}
