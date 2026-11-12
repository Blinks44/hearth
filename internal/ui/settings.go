package ui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/locales"
)

func (a *App) settingsPage(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, locales.SettingsTitle(), "Hearth")
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(22, 28, 24, 28).Gap(18).Children(func() {
				ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(Colors(c.Theme()).SuccessSoft).Center().Children(func() { ui.Icon(c, Icon("settings")).Size(26, 26).TextColor(c.Theme().TextMuted) })
					ui.Column(c).Gap(3).Children(func() {
						ui.Text(c, locales.SettingsTitle()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.SettingsLocal()).FontSize(12.5).TextColor(c.Theme().TextMuted)
					})
				})
				if a.SettingsNotice != "" {
					ui.Text(c, a.SettingsNotice).FontSize(12).TextColor(c.Theme().Success)
				}
				if a.SettingsError != "" {
					ui.Text(c, a.SettingsError).TextColor(c.Theme().Danger)
				}
				columns(c, 12, func() {
					card(c, locales.SettingsGeneral(), "", nil, func() {
						settingRow(c, locales.SettingsLanguage(), locales.SettingsLanguageHint(), func() {
							lang := "English"
							if a.Locale == "ru" {
								lang = "Русский"
							}
							ui.Select(c.Key("settings-language"), &lang, []string{"English", "Русский"}).Label(locales.SettingsChooseLanguage()).Width(145).OnChange(func() {
								a.Locale = "en"
								if lang == "Русский" {
									a.Locale = "ru"
								}
								a.SettingsNotice = "" // says it in the language it was shown in
								if a.OnLocale != nil {
									a.OnLocale(a.Locale)
								}
							})
						})
						settingRow(c, locales.SettingsTheme(), locales.SettingsThemeHint(), func() {
							values := []string{locales.SettingsSystem(), locales.SettingsLight(), locales.SettingsDark()}
							value := values[0]
							if a.ThemeMode == "light" {
								value = values[1]
							}
							if a.ThemeMode == "dark" {
								value = values[2]
							}
							ui.Select(c.Key("settings-theme"), &value, values).Label(locales.SettingsChooseTheme()).Width(145).OnChange(func() {
								a.ThemeMode = "system"
								if value == values[1] {
									a.ThemeMode = "light"
								}
								if value == values[2] {
									a.ThemeMode = "dark"
								}
								a.savePreferences()
							})
						})
					})
					card(c, locales.SettingsNetwork(), "", nil, func() {
						settingRow(c, locales.SettingsLists(), locales.SettingsListsHint(), func() {
							toggle(c, "settings-lists", &a.VersionLists).Label(locales.SettingsAllowLists()).OnChange(a.savePreferences)
						})
					})
					card(c, locales.SettingsProjects(), "", func() {
						if a.FindProjects != nil {
							ui.Button(c, locales.FindProjects()).OnClick(a.FindProjects)
						}
						ui.Button(c, locales.AddFolder()).Disabled(a.ChooseFolder == nil).OnClick(a.ChooseFolder)
					}, func() {
						if len(a.Projects) == 0 {
							settingRow(c, locales.SettingsProjectHint(), "", nil)
						}
						for _, p := range a.Projects {
							settingRow(c.Key(p.Dir), filepath.Base(p.Dir), tilde(p.Dir, a.Home), func() {
								ui.Button(c, locales.RemoveFromList()).Disabled(a.RemoveProject == nil).OnClick(func() { a.RemoveProject(p.Dir) })
							})
						}
					})
				}, func() {
					card(c, locales.SettingsData(), "", nil, func() {
						settingRow(c, locales.SettingsCache(), tilde(a.CachePath, a.Home), func() {
							ui.Button(c, locales.SettingsClear()).Disabled(a.ClearCache == nil || a.CacheBusy).OnClick(a.ClearCache)
						})
					})
					card(c, locales.SettingsAbout(), "", nil, func() {
						v := a.AppVersion
						if v == "" {
							v = locales.SettingsDevelopment()
						}
						settingRow(c, locales.SettingsVersion(), locales.SettingsFree(), func() { ui.Text(c, v).Font(FontMono).FontSize(11).TextColor(c.Theme().TextMuted) })
					})
				})
			})
		})
	})
	a.addDialog(c)
}

func (a *App) savePreferences() {
	if a.OnSettings != nil {
		a.OnSettings()
	}
}

func settingRow(c *ui.Context, title, hint string, end func()) {
	ui.Row(c).MinHeight(48).Padding(8, 14).Gap(8).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(Colors(c.Theme()).Divider).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(13).SingleLine()
			if hint != "" {
				ui.Text(c, hint).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
			}
		})
		if end != nil {
			end()
		}
	})
}
