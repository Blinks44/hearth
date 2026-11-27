//go:build darwin

package main

import (
	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/resources"
)

func trayIcon() (png []byte, template bool) {
	return resources.TrayIconMacOS, true
}

func (t *trayController) initialTrayMenu() *mygo.Menu {
	// Keep Menu nil on macOS so left-click triggers OnClick (flyout) while
	// right-click pops up the compact system menu on demand.
	return nil
}

func (t *trayController) bindTrayClicks() {
	t.tray.OnClick(func() {
		t.togglePopover()
	})
	t.tray.OnRightClick(func() {
		t.hidePopover()
		t.tray.SetMenu(t.buildCompactSystemMenu())
		t.tray.PopUpMenu()
		t.tray.SetMenu(nil)
	})
}

func (t *trayController) syncTrayMenu() {
	// Menu is attached transiently on right-click so left-click keeps opening the popover.
}
