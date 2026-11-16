package ui

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/locales"
	"github.com/Blinks44/hearth/resources"
)

// languageTags are the catalogs of locales/, in the order the language
// menu lists them by languageNames, each in its own language.
var (
	languageTags  = []string{"en", "de", "es", "fr", "ru", "zh-Hans"}
	languageNames = []string{"English", "Deutsch", "Español", "Français", "Русский", "中文（简体）"}
	// machineTranslated are the languages an AI translated without a
	// native speaker's review.
	machineTranslated = map[string]bool{"de": true, "es": true, "fr": true, "zh-Hans": true}
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
						hint := locales.SettingsLanguageHint()
						if machineTranslated[a.Locale] {
							hint = locales.SettingsLanguageMachine()
						}
						settingRow(c, locales.SettingsLanguage(), hint, func() {
							lang := languageNames[0]
							for i, tag := range languageTags {
								if tag == a.Locale {
									lang = languageNames[i]
								}
							}
							ui.Select(c.Key("settings-language"), &lang, languageNames).Label(locales.SettingsChooseLanguage()).Width(145).OnChange(func() {
								for i, name := range languageNames {
									if name == lang {
										a.Locale = languageTags[i]
									}
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
						settingRow(c, locales.SettingsLicenses(), locales.SettingsLicensesHint(), func() {
							ui.Button(c, locales.SettingsOpen()).OnClick(func() { a.licensesOpen = true })
						})
					})
				})
			})
		})
	})
	a.licensesDialog(c)
}

// licensesDialog shows Hearth's license and those of the fonts, icons and
// Go modules it ships.
func (a *App) licensesDialog(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	dialogBase(c, &a.licensesOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Children(func() {
			ui.Text(c, locales.SettingsLicenses()).FontSize(18).FontWeight(600).Padding(20, 20, 14, 20)
			ui.Scroll(c).Height(420).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, resources.Notice).Font(FontMono).FontSize(11).TextColor(t.TextMuted).Padding(14, 20).Selectable()
			})
			ui.Row(c).Padding(12, 20).Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Button(c, locales.SettingsClose()).OnClick(func() { a.licensesOpen = false })
			})
		})
	})
}

func (a *App) savePreferences() {
	if a.OnSettings != nil {
		a.OnSettings()
	}
}

func settingRow(c *ui.Context, title, hint string, end func()) {
	ui.Row(c).MinHeight(48).Padding(8, 14).Gap(8).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(Colors(c.Theme()).Divider).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(13)
			if hint != "" {
				ui.Text(c, hint).FontSize(11).TextColor(Colors(c.Theme()).TextFaint)
			}
		})
		if end != nil {
			end()
		}
	})
}
