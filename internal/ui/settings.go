package ui

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/issues"
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
							ui.Select(c.Key("settings-language"), &lang, languageNames).Label(locales.SettingsChooseLanguage()).Width(160).OnChange(func() {
								for i, name := range languageNames {
									if name == lang {
										a.Locale = languageTags[i]
									}
								}
								a.SettingsNotice = "" // says it in the language it was shown in
								a.shownSettingsNotice = ""
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
							ui.Select(c.Key("settings-theme"), &value, values).Label(locales.SettingsChooseTheme()).Width(160).OnChange(func() {
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
						if a.StartHidden || a.DockerPromoHidden {
							settingRow(c.Key("settings-reset-banners-row"), locales.SettingsResetBanners(), locales.SettingsResetBannersHint(), func() {
								ui.Button(c.Key("settings-reset-banners"), locales.SettingsResetBannersAction()).Height(controlHeight).OnClick(func() {
									a.StartHidden = false
									a.DockerPromoHidden = false
									a.savePreferences()
									a.notice = locales.SettingsResetBannersDone()
								})
							})
						}
					})
					card(c, locales.SettingsSystemSection(), "", nil, func() {
						settingRow(c, settingsTrayLabel(), settingsTrayHint(), func() {
							toggle(c, "settings-tray", &a.TrayEnabled).Label(settingsTrayLabel()).OnChange(func() {
								if !a.TrayEnabled {
									a.ShowInDock = true
								}
								a.savePreferences()
							})
						})
						a.settingsDockRow(c)
						settingRow(c, locales.SettingsOpenAtLogin(), locales.SettingsOpenAtLoginHint(), func() {
							toggle(c, "settings-open-at-login", &a.OpenAtLogin).Label(locales.SettingsOpenAtLogin()).
								Disabled(a.OnOpenAtLogin == nil).OnChange(func() {
								a.SettingsError = ""
								a.shownSettingsError = ""
								if err := a.OnOpenAtLogin(a.OpenAtLogin); err != nil {
									a.OpenAtLogin = !a.OpenAtLogin
									a.SettingsError = err.Error()
								}
							})
						})
					})
					a.appsCard(c)
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
							dir := p.Dir
							settingRow(c.Key(dir), filepath.Base(dir), tilde(dir, a.Home), func() {
								iconButton(c.Key("remove"), "trash-2", locales.RemoveFromList()).Disabled(a.RemoveProject == nil).OnClick(func() { a.RemoveProject(dir) })
							})
						}
					})
				}, func() {
					a.dataCard(c)
					a.cleanupSettingsCard(c)
					a.hiddenIssuesCard(c)
					card(c, locales.SettingsAbout(), "", nil, func() {
						v := a.AppVersion
						if v == "" {
							v = locales.SettingsDevelopment()
						}
						versionHint := locales.SettingsFree()
						if a.AppUpdateAvailable != "" {
							versionHint = locales.AppUpdateAvailableShort(a.AppUpdateAvailable)
						}
						settingRow(c.Key("settings-version"), locales.SettingsVersion(), versionHint, func() {
							ui.Row(c).Gap(8).Shrink(0).AlignItems(ui.Center).Children(func() {
								ui.Text(c, v).Font(FontMono).FontSize(11).TextColor(c.Theme().TextMuted)
								if a.CheckAppUpdate != nil {
									if a.AppUpdateAvailable != "" {
										ui.PrimaryButton(c.Key("check-app-update"), locales.AppUpdateInstallButton()).OnClick(a.CheckAppUpdate)
									} else {
										iconButton(c.Key("check-app-update"), "refresh-cw", locales.AppUpdateCheckButton()).OnClick(a.CheckAppUpdate)
									}
								}
							})
						})
						if a.AppUpdateEnabled && a.OnAppUpdateAutoCheck != nil {
							settingRow(c.Key("settings-app-auto-update"), locales.AppUpdateAutoCheck(), locales.AppUpdateAutoCheckHint(), func() {
								toggle(c, "app-auto-update", &a.AppUpdateAutoCheck).Label(locales.AppUpdateAutoCheck()).OnChange(func() {
									a.OnAppUpdateAutoCheck(a.AppUpdateAutoCheck)
								})
							})
						}
						navListRow(c, "settings-licenses", locales.SettingsLicenses(), locales.SettingsLicensesHint(), locales.SettingsOpen(), func() {
							a.licensesOpen = true
						})
					})
					a.trayPanelSettingsCard(c)
				})
			})
		})
	})
	a.licensesDialog(c)
}

func (a *App) trayPanelSettingsCard(c *ui.Context) {
	pal := Colors(c.Theme())
	card(c.Key("settings-tray-panel"), locales.SettingsTrayPanelTitle(), "", nil, func() {
		ui.Column(c.Key("settings-tray-sections")).Padding(10, 14, 12, 14).Gap(8).
			BorderWidth(1, 0, 0, 0).BorderColor(pal.Divider).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, locales.SettingsTraySectionsTitle()).FontSize(13)
				ui.Text(c, locales.SettingsTraySectionsHint()).FontSize(11).TextColor(pal.TextFaint)
			})
			order := a.traySectionOrder()
			ui.Column(c).Radius(10).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
				for i, id := range order {
					secID := id
					idx := i
					vis := !a.isTraySectionHidden(secID)
					label := traySectionDisplayName(secID)
					row := ui.Row(c.Key("tray-sec-"+secID)).MinHeight(40).Padding(4, 10).Gap(8).AlignItems(ui.Center)
					if i > 0 {
						row.BorderWidth(1, 0, 0, 0).BorderColor(pal.Divider)
					}
					row.Children(func() {
						ui.Text(c, strconv.Itoa(idx+1)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Width(14)
						ui.Text(c, label).FontSize(12.5).FontWeight(500).Grow(1).MinWidth(0).SingleLine()
						iconButton(c.Key("up-"+secID), "chevron-up", locales.QuotasMoveUp()+" "+label).
							Disabled(idx == 0).OnClick(func() { a.moveTraySection(secID, -1) })
						iconButton(c.Key("down-"+secID), "chevron-down", locales.QuotasMoveDown()+" "+label).
							Disabled(idx == len(order)-1).OnClick(func() { a.moveTraySection(secID, 1) })
						toggle(c, "tray-sec-sw-"+secID, &vis).Label(label).OnChange(func() {
							a.setTraySectionVisible(secID, vis)
						})
					})
				}
			})
		})
		navListRow(c, "settings-tray-providers", locales.SettingsTrayProvidersTitle(), a.settingsTrayProvidersSummary(), locales.QuotasTitle(), func() {
			a.router.Push(quotasPage)
		})
		settingRow(c.Key("settings-quota-notify-row"), locales.SettingsQuotaNotifyTitle(), locales.SettingsQuotaNotifyHint(), func() {
			toggle(c, "settings-quota-notify", &a.QuotaNotifyLow).Label(locales.SettingsQuotaNotifyTitle()).OnChange(a.savePreferences)
		})
	})
}

func traySectionDisplayName(id string) string {
	switch id {
	case traySectionServices:
		return locales.SettingsTraySectionServices()
	case traySectionQuotas:
		return locales.SettingsTraySectionQuotas()
	case traySectionContexts:
		return locales.SettingsTraySectionContexts()
	default:
		return id
	}
}

func (a *App) settingsTrayProvidersSummary() string {
	if a.Quotas == nil {
		return locales.SettingsTrayProvidersNone()
	}
	ordered := a.Quotas.Ordered(a.QuotaOrder)
	if len(ordered) == 0 {
		return locales.SettingsTrayProvidersNone()
	}
	var shown, hidden []string
	for _, p := range ordered {
		if a.isQuotaTrayHidden(p.Provider) {
			hidden = append(hidden, p.Name)
		} else {
			shown = append(shown, p.Name)
		}
	}
	shownText := "—"
	if len(shown) > 0 {
		shownText = strings.Join(shown, ", ")
	}
	if len(hidden) == 0 {
		return locales.SettingsTrayProvidersSummary(shownText)
	}
	return locales.SettingsTrayProvidersSummaryHidden(shownText, strings.Join(hidden, ", "))
}

func (a *App) canOpenFolder() bool {
	return a.OpenFolder != nil || a.Shell != nil
}

func (a *App) openSettingsFolder(c *ui.Context, path string) {
	if path == "" {
		return
	}
	a.SettingsError = ""
	a.shownSettingsError = ""
	if a.OpenFolder != nil {
		a.OpenFolder(path, func(err error) {
			switch {
			case err == nil:
			case errors.Is(err, os.ErrNotExist):
				a.SettingsError = locales.SettingsFolderMissing(tilde(path, a.Home))
			default:
				a.SettingsError = locales.OpenFailed(tilde(path, a.Home), err.Error())
			}
		})
		return
	}
	if a.Shell != nil {
		if err := a.Shell.OpenPath(path); err != nil {
			a.SettingsError = locales.OpenFailed(tilde(path, a.Home), err.Error())
		}
	}
}

func (a *App) dataCard(c *ui.Context) {
	canOpen := a.canOpenFolder()
	card(c, locales.SettingsData(), "", nil, func() {
		settingRow(c.Key("settings-cache"), locales.SettingsCache(), tilde(a.CachePath, a.Home), func() {
			ui.Row(c).Gap(2).Shrink(0).AlignItems(ui.Center).Children(func() {
				if a.CachePath != "" && canOpen {
					iconButton(c.Key("open"), "folder-open", locales.SettingsOpen()+" "+locales.SettingsCache()).
						OnClick(func() { a.openSettingsFolder(c, a.CachePath) })
				}
				iconButton(c.Key("clear"), "trash-2", locales.SettingsClear()).Disabled(a.ClearCache == nil || a.CacheBusy).OnClick(a.ClearCache)
			})
		})
		if a.DataDir != "" {
			settingRow(c.Key("settings-data-dir"), locales.SettingsDataDir(), tilde(a.DataDir, a.Home), func() {
				iconButton(c.Key("open"), "folder-open", locales.SettingsOpen()+" "+locales.SettingsDataDir()).
					Disabled(!canOpen).OnClick(func() { a.openSettingsFolder(c, a.DataDir) })
			})
		}
		if a.LogsDir != "" {
			settingRow(c.Key("settings-logs-dir"), locales.SettingsLogsDir(), tilde(a.LogsDir, a.Home), func() {
				iconButton(c.Key("open"), "folder-open", locales.SettingsOpen()+" "+locales.SettingsLogsDir()).
					Disabled(!canOpen).OnClick(func() { a.openSettingsFolder(c, a.LogsDir) })
			})
		}
		if a.SkillBackups != "" {
			settingRow(c.Key("settings-backups-dir"), locales.SettingsBackupsDir(), locales.SettingsBackupsHint(tilde(a.SkillBackups, a.Home)), func() {
				iconButton(c.Key("open"), "folder-open", locales.SettingsOpen()+" "+locales.SettingsBackupsDir()).
					Disabled(!canOpen).OnClick(func() { a.openSettingsFolder(c, a.SkillBackups) })
			})
		}
	})
}

var (
	cleanupStaleDayChoices = []int{30, 60, 90, 180}
	cleanupMinMBChoices    = []int{1, 10, 50, 100}
)

func (a *App) cleanupSettingsCard(c *ui.Context) {
	card(c, locales.SettingsCleanup(), "", nil, func() {
		staleDays := a.CleanupStaleDays
		if staleDays <= 0 {
			staleDays = 180
		}
		staleLabels := make([]string, len(cleanupStaleDayChoices))
		staleValue := locales.SettingsCleanupDays(staleDays)
		for i, d := range cleanupStaleDayChoices {
			staleLabels[i] = locales.SettingsCleanupDays(d)
			if d == staleDays {
				staleValue = staleLabels[i]
			}
		}
		if !slices.Contains(staleLabels, staleValue) {
			staleLabels = append(staleLabels, staleValue)
		}
		settingRow(c.Key("settings-cleanup-stale-row"), locales.SettingsCleanupStale(), locales.SettingsCleanupStaleHint(), func() {
			ui.Select(c.Key("settings-cleanup-stale"), &staleValue, staleLabels).Label(locales.SettingsCleanupStale()).Width(116).OnChange(func() {
				if idx := slices.Index(staleLabels, staleValue); idx >= 0 && idx < len(cleanupStaleDayChoices) {
					a.CleanupStaleDays = cleanupStaleDayChoices[idx]
					a.savePreferences()
				}
			})
		})

		minMB := a.CleanupMinMB
		if minMB <= 0 {
			minMB = 1
		}
		minLabels := make([]string, len(cleanupMinMBChoices))
		minValue := formatBytes(int64(minMB) * 1_000_000)
		for i, mb := range cleanupMinMBChoices {
			minLabels[i] = formatBytes(int64(mb) * 1_000_000)
			if mb == minMB {
				minValue = minLabels[i]
			}
		}
		if !slices.Contains(minLabels, minValue) {
			minLabels = append(minLabels, minValue)
		}
		settingRow(c.Key("settings-cleanup-min-row"), locales.SettingsCleanupMinSize(), locales.SettingsCleanupMinSizeHint(), func() {
			ui.Select(c.Key("settings-cleanup-min"), &minValue, minLabels).Label(locales.SettingsCleanupMinSize()).Width(116).OnChange(func() {
				if idx := slices.Index(minLabels, minValue); idx >= 0 && idx < len(cleanupMinMBChoices) {
					a.CleanupMinMB = cleanupMinMBChoices[idx]
					a.savePreferences()
				}
			})
		})

		settingRow(c.Key("settings-cleanup-exclude-head"), locales.SettingsCleanupExclude(), locales.SettingsCleanupExcludeHint(), nil)
		addExclude := func(raw string) {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				return
			}
			expanded := expandHomePath(trimmed, a.Home)
			if slices.Contains(a.CleanupExclude, expanded) {
				a.cleanupExcludeInput = ""
				return
			}
			a.CleanupExclude = append(slices.Clone(a.CleanupExclude), expanded)
			a.cleanupExcludeInput = ""
			a.savePreferences()
		}
		for _, dir := range a.CleanupExclude {
			entry := dir
			ui.Row(c.Key("settings-cleanup-exclude:"+entry)).Padding(4, 14).Gap(8).AlignItems(ui.Center).Children(func() {
				clipped(c, tilde(entry, a.Home)).Grow(1).MinWidth(0).Font(FontMono).FontSize(11.5).TextColor(c.Theme().TextMuted)
				iconButton(c.Key("remove"), "trash-2", locales.RemoveFromList()+" "+tilde(entry, a.Home)).OnClick(func() {
					a.CleanupExclude = slices.DeleteFunc(slices.Clone(a.CleanupExclude), func(v string) bool { return v == entry })
					a.savePreferences()
				})
			})
		}
		ui.Row(c.Key("settings-cleanup-exclude-input-row")).Padding(6, 14, 12, 14).Gap(6).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c.Key("input"), &a.cleanupExcludeInput).Grow(1).MinWidth(0).
				Placeholder(locales.SettingsCleanupExcludePlaceholder()).Label(locales.SettingsCleanupExclude())
			if in.Submitted() {
				addExclude(a.cleanupExcludeInput)
			}
			if a.PickPath != nil {
				iconButton(c.Key("browse"), "folder-open", locales.SettingsCleanupExcludePick()).OnClick(func() {
					a.PickPath(locales.SettingsCleanupExcludePick(), true, func(path string, err error) {
						if err != nil {
							a.SettingsError = err.Error()
							return
						}
						if path != "" {
							addExclude(path)
						}
					})
				})
			}
			ui.Button(c.Key("add"), locales.SettingsCleanupExcludeAdd()).Height(controlHeight).
				Disabled(strings.TrimSpace(a.cleanupExcludeInput) == "").OnClick(func() {
				addExclude(a.cleanupExcludeInput)
			})
		})
	})
}

func (a *App) hiddenIssuesCard(c *ui.Context) {
	count := ""
	if len(a.Hidden) > 0 {
		count = strconv.Itoa(len(a.Hidden))
	}
	byKey := map[string]attentionItem{}
	if a.Snapshot != nil && len(a.Hidden) > 0 {
		_, hiddenItems := a.visibleIssues(a.issues(c))
		for _, it := range hiddenItems {
			byKey[it.key] = it
			if it.hiddenKey != "" {
				byKey[it.hiddenKey] = it
			}
		}
	}
	staleCount := 0
	if a.Snapshot != nil {
		for _, key := range a.Hidden {
			if _, ok := byKey[key]; !ok {
				staleCount++
			}
		}
	}
	var end func()
	if len(a.Hidden) > 0 {
		end = func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if a.Snapshot != nil && staleCount > 0 {
					ui.Button(c.Key("settings-hidden-clear-stale"), locales.SettingsHiddenClearStale(staleCount)).OnClick(func() {
						var kept []string
						for _, key := range a.Hidden {
							if _, ok := byKey[key]; ok {
								kept = append(kept, key)
							}
						}
						if len(kept) == 0 {
							a.problemsShowHidden = false
						}
						a.setHidden(kept)
					})
				}
				ui.Button(c.Key("settings-hidden-restore-all"), locales.ProblemsRestoreAll()).OnClick(func() {
					a.problemsShowHidden = false
					a.setHidden(nil)
				})
			})
		}
	}
	card(c, locales.SettingsHiddenIssues(), count, end, func() {
		if len(a.Hidden) == 0 {
			settingRow(c, locales.SettingsNoHiddenIssues(), "", nil)
			return
		}
		for _, key := range a.Hidden {
			title, hint := staleIssueTitle(key, a.Home), locales.SettingsHiddenInactive()
			if it, ok := byKey[key]; ok {
				title, hint = it.title, it.explain
			}
			settingRow(c.Key("settings-hidden:"+key), title, hint, func() {
				ui.Button(c.Key("restore"), locales.ProblemsRestore()).Label(locales.ProblemsRestore() + " " + title).
					OnClick(func() { a.unhideIssue(key) })
			})
		}
	})
}

// staleIssueTitle turns a structured issueKey ("kind:tool:project:...") or
// grouped kind key into a readable fallback label when the issue is no longer
// active; plain keys are returned as-is.
func staleIssueTitle(key, home string) string {
	switch issues.Kind(key) {
	case issues.SkillCopies:
		return locales.SettingsHiddenGroupSkillCopies()
	case issues.SkillDiffers:
		return locales.SettingsHiddenGroupSkillDiffers()
	case issues.MCPMissing:
		return locales.SettingsHiddenGroupMcpMissing()
	}
	raw := strings.Split(key, ":")
	if len(raw) < 3 {
		return key
	}
	parts := make([]string, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if i+1 < len(raw) && len(raw[i]) == 1 && ((raw[i][0] >= 'A' && raw[i][0] <= 'Z') || (raw[i][0] >= 'a' && raw[i][0] <= 'z')) && (strings.HasPrefix(raw[i+1], `\`) || strings.HasPrefix(raw[i+1], "/")) {
			parts = append(parts, raw[i]+":"+raw[i+1])
			i++
			continue
		}
		parts = append(parts, raw[i])
	}
	if len(parts) < 3 {
		return key
	}
	kind := issues.Kind(parts[0])
	switch kind {
	case issues.InstructionDiffers,
		issues.DockerUnavailable, issues.DockerDenied, issues.DockerSignIn,
		issues.DatabasePortConflict,
		issues.Duplicate, issues.GUIDiffers,
		issues.ProjectMissing, issues.ProjectTerminal,
		issues.Unsupported, issues.PathCurrentDir, issues.PathMissing, issues.PathDuplicate,
		issues.SkillCopies, issues.SkillDiffers, issues.MCPMissing, issues.PlainSecret, issues.SettingsSecret, issues.AgentUnasked,
		issues.StubOnPath, issues.ExtraVersion, issues.SlowStartup:
	default:
		return key
	}
	var label []string
	if tool := parts[1]; tool != "" {
		label = append(label, toolName(tool))
	}
	if proj := parts[2]; proj != "" {
		label = append(label, filepath.Base(proj))
	}
	for _, extra := range parts[3:] {
		if extra == "" {
			continue
		}
		if filepath.IsAbs(extra) {
			extra = tilde(extra, home)
		}
		label = append(label, extra)
	}
	if len(label) == 0 {
		return key
	}
	return strings.Join(label, " · ")
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

// appsCard chooses the apps Hearth opens things in: the terminal for a
// container's shell, the editor for files. Each starts as the system's and
// also accepts a custom application or executable path.
func (a *App) appsCard(c *ui.Context) {
	editors := a.editorChoices()
	customLabel := locales.SettingsCustomPath()
	card(c, locales.SettingsApps(), "", nil, func() {
		termNames := append([]string{locales.SettingsSystem()}, a.Terminals...)
		termNames = append(termNames, customLabel)
		termIsCustom := a.Terminal != "" && !slices.Contains(a.Terminals, a.Terminal)
		termShowCustom := a.terminalCustomOpen || termIsCustom
		termValue := termNames[0]
		switch {
		case slices.Contains(a.Terminals, a.Terminal):
			termValue = a.Terminal
		case termShowCustom:
			termValue = customLabel
		}
		termHint := locales.SettingsTerminalHint()
		if termIsCustom {
			termHint = tilde(a.Terminal, a.Home)
		}
		settingRow(c.Key("settings-terminal-row"), locales.SettingsTerminal(), termHint, func() {
			ui.Select(c.Key("settings-terminal"), &termValue, termNames).Label(locales.SettingsPickTerminal()).Width(160).OnChange(func() {
				switch termValue {
				case termNames[0]:
					a.Terminal = ""
					a.terminalCustomOpen = false
					a.savePreferences()
				case customLabel:
					a.terminalCustomOpen = true
					if termIsCustom {
						a.terminalCustomInput = a.Terminal
					} else {
						a.terminalCustomInput = ""
					}
				default:
					a.Terminal = termValue
					a.terminalCustomOpen = false
					a.savePreferences()
				}
			})
		})
		if termShowCustom {
			if !a.terminalCustomOpen && termIsCustom && a.terminalCustomInput == "" {
				a.terminalCustomInput = a.Terminal
			}
			a.customAppPathRow(c, "settings-terminal-custom", locales.SettingsTerminal(), &a.terminalCustomInput,
				locales.SettingsCustomTerminalPlaceholder(), locales.SettingsPickTerminal(),
				func(saved string) {
					a.Terminal = saved
					a.terminalCustomOpen = saved != ""
					a.savePreferences()
				})
		}

		editorNames := []string{locales.SettingsSystem()}
		editorKnown := false
		for _, e := range editors {
			editorNames = append(editorNames, e.Name)
			if e.Name == a.Editor {
				editorKnown = true
			}
		}
		editorLooksCustom := isCustomAppPath(a.Editor)
		if a.Editor != "" && !editorKnown && a.Snapshot == nil && !editorLooksCustom {
			editorNames = append(editorNames, a.Editor)
			editorKnown = true
		}
		editorNames = append(editorNames, customLabel)
		editorIsCustom := a.Editor != "" && !editorKnown
		editorShowCustom := a.editorCustomOpen || editorIsCustom
		editorValue := editorNames[0]
		switch {
		case editorKnown:
			editorValue = a.Editor
		case editorShowCustom:
			editorValue = customLabel
		}
		editorHint := locales.SettingsEditorHint()
		if editorIsCustom {
			editorHint = tilde(a.Editor, a.Home)
		}
		settingRow(c.Key("settings-editor-row"), locales.SettingsEditor(), editorHint, func() {
			ui.Select(c.Key("settings-editor"), &editorValue, editorNames).Label(locales.SettingsPickEditor()).Width(160).OnChange(func() {
				switch editorValue {
				case editorNames[0]:
					a.Editor = ""
					a.editorCustomOpen = false
					a.savePreferences()
				case customLabel:
					a.editorCustomOpen = true
					if editorIsCustom {
						a.editorCustomInput = a.Editor
					} else {
						a.editorCustomInput = ""
					}
				default:
					a.Editor = editorValue
					a.editorCustomOpen = false
					a.savePreferences()
				}
			})
		})
		if editorShowCustom {
			if !a.editorCustomOpen && editorIsCustom && a.editorCustomInput == "" {
				a.editorCustomInput = a.Editor
			}
			a.customAppPathRow(c, "settings-editor-custom", locales.SettingsEditor(), &a.editorCustomInput,
				locales.SettingsCustomEditorPlaceholder(), locales.SettingsPickEditor(),
				func(saved string) {
					a.Editor = saved
					a.editorCustomOpen = saved != ""
					a.savePreferences()
				})
		}
	})
}

func (a *App) customAppPathRow(c *ui.Context, key, targetLabel string, input *string, placeholder, pickTitle string, onSave func(string)) {
	save := func() {
		trimmed := expandHomePath(strings.TrimSpace(*input), a.Home)
		if trimmed == "" {
			return
		}
		if !validCustomApp(trimmed) {
			a.shownSettingsError = ""
			a.SettingsError = locales.SettingsCustomAppInvalid()
			return
		}
		*input = trimmed
		a.SettingsError = ""
		a.shownSettingsError = ""
		onSave(trimmed)
	}
	ui.Row(c.Key(key)).Padding(0, 14, 12, 14).Gap(8).AlignItems(ui.Center).Children(func() {
		in := ui.TextInput(c.Key("input"), input).Grow(1).Placeholder(placeholder).Label(pickTitle)
		if in.Submitted() {
			save()
		}
		if a.PickPath != nil {
			ui.Button(c.Key("browse"), locales.SettingsBrowse()).Label(locales.SettingsBrowse() + " " + targetLabel).Height(controlHeight).OnClick(func() {
				a.PickPath(pickTitle, false, func(path string, err error) {
					if err != nil {
						a.SettingsError = err.Error()
						return
					}
					if path != "" {
						*input = path
						a.SettingsError = ""
						onSave(path)
					}
				})
			})
		}
		ui.Button(c.Key("save"), locales.SettingsSave()).Label(locales.SettingsSave() + " " + targetLabel).Height(controlHeight).Disabled(strings.TrimSpace(*input) == "").OnClick(save)
	})
}

func expandHomePath(p, home string) string {
	if home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
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
