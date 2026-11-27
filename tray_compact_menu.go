//go:build darwin || windows

package main

import (
	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/locales"
)

// buildCompactSystemMenu builds the concise right-click system menu
// (Open Hearth, Check for updates, Settings, Quit) used on macOS and Windows
// so live controls stay in the left-click flyout without duplicating complex menus.
func (t *trayController) buildCompactSystemMenu() *mygo.Menu {
	appUpdateLabel := locales.TrayCheckUpdates()
	if t.app.AppUpdateAvailable != "" {
		appUpdateLabel = locales.AppUpdateAvailable(t.app.AppUpdateAvailable)
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{
			ID:          "open",
			Label:       locales.TrayOpenHearth(),
			Accelerator: "CmdOrCtrl+O",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.showWindow()
			},
		},
		{
			ID:    "app-update",
			Label: appUpdateLabel,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if t.app.CheckAppUpdate != nil {
					t.app.CheckAppUpdate()
				}
			},
		},
		{
			ID:          "settings",
			Label:       locales.TraySettings(),
			Accelerator: "CmdOrCtrl+,",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				t.app.OpenSettings()
				t.showWindow()
			},
		},
		mygo.Separator(),
		{
			ID:          "quit",
			Label:       locales.TrayQuit(),
			Accelerator: "CmdOrCtrl+Q",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if t.onQuit != nil {
					t.onQuit()
				}
			},
		},
	})
}
