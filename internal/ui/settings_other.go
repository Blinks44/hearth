//go:build !darwin

package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/locales"
)

func settingsTrayLabel() string { return locales.SettingsTrayLabelOther() }
func settingsTrayHint() string  { return locales.SettingsTrayHintOther() }

func (a *App) settingsDockRow(*ui.Context) {}
