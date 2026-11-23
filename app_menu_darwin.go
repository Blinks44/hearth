//go:build darwin

package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"

	"github.com/Blinks44/hearth/locales"
)

// setAppMenu installs the macOS application menu bar with Hearth's About,
// Check for Updates, Settings, Edit, and Window menus. Rebuilt on locale change.
func setAppMenu(openSettings func()) {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{
			Label: "Hearth",
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout},
				updater.MenuItem(),
				{
					ID:          "app-settings",
					Label:       locales.TraySettings(),
					Accelerator: "CmdOrCtrl+,",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						if openSettings != nil {
							openSettings()
						}
					},
				},
				mygo.Separator(),
				{Role: mygo.RoleServices},
				mygo.Separator(),
				{Role: mygo.RoleHide},
				{Role: mygo.RoleHideOthers},
				{Role: mygo.RoleUnhide},
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			},
		},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	}))
}
