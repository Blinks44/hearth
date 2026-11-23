//go:build darwin

package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/locales"
)

func settingsTrayLabel() string { return locales.SettingsTrayLabelMac() }
func settingsTrayHint() string  { return locales.SettingsTrayHintMac() }

func (a *App) settingsDockRow(c *ui.Context) {
	hint := locales.SettingsDockHint()
	if !a.TrayEnabled {
		hint = locales.SettingsDockRequiresTray()
	}
	settingRow(c, locales.SettingsDockLabel(), hint, func() {
		toggle(c, "settings-dock", &a.ShowInDock).Label(locales.SettingsDockLabel()).
			Disabled(!a.TrayEnabled).OnChange(a.savePreferences)
	})
}
