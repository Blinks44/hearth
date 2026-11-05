package ui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) packages(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.Snapshot.Tool("packages")
		if !ok || len(st.PackageManagers) == 0 {
			a.toolEmpty(c, "packages", p.title())
			return
		}
		selected := 0
		for i, m := range st.PackageManagers {
			if m.Path == a.packageManager {
				selected = i
			}
		}
		m := st.PackageManagers[selected]
		if checked, ok := a.PackageUpdates[m.Path]; ok {
			m = checked
		}
		updateNote := locales.PkgCached()
		if a.PackageChecks[m.Path] {
			updateNote = locales.Checking()
		} else if !m.UpdatesFetched.IsZero() {
			updateNote = locales.VersionsAsOf(m.UpdatesFetched)
		}
		// A list some sources did not answer for counts at least what it
		// holds; an update count not checked differs from none.
		partial := m.Unlisted && !m.UpdatesFetched.IsZero()
		installedCount, countNote := strconv.Itoa(len(m.Packages)), locales.PkgCountNote()
		if m.Unlisted {
			installedCount = atLeast(len(m.Packages))
		}
		if partial {
			countNote = locales.PkgPartialNote()
		}
		updates := 0
		for _, pkg := range m.Packages {
			if pkg.Available != "" {
				updates++
			}
		}
		updateCount := "—"
		if m.UpdatesKnown {
			updateCount = strconv.Itoa(updates)
		} else if partial {
			updateCount = atLeast(updates)
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				pal := Colors(c.Theme())
				ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.SuccessSoft).Center().Children(func() { ui.Icon(c, Icon("package")).Size(24, 24).TextColor(pal.MonoText) })
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, m.Name).FontSize(22).FontWeight(600)
						path := m.Prefix
						if path == "" {
							path = m.Path
						}
						ui.Text(c, tilde(path, a.Home)).Font(FontMono).FontSize(11.5).TextColor(c.Theme().TextMuted).SingleLine()
					})
					version := m.Version
					if version == "" { // Scoop is a PowerShell script Hearth does not run
						version = locales.UnknownVersion()
					}
					stateCard(c, c.Theme().Success, m.Name, version, "")
					stateCard(c, pal.TextFaint, locales.PkgInstalled(), installedCount, countNote)
					stateCard(c, c.Theme().Warning, locales.PkgUpdateSummary(), updateCount, updateNote)
				})
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					segments(c, "package-tabs", &a.packageTab, []segment{{locales.PkgInstalled(), installedCount}, {locales.PkgUpdates(), updateCount}})
					ui.Box(c).Grow(1)
					ui.SearchField(c.Key("package-filter"), &a.packageFilter).Placeholder(locales.PkgFilter()).Width(200)
				})
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
						if m.Unlisted && !partial {
							toolNote(c, locales.PkgWingetLocal())
						} else if !m.UpdatesKnown && !partial {
							toolNote(c, packageUpdatesNote(m.ID, a.VersionLists))
						}
						if m.UpdateError != "" {
							warningRow(c, locales.PkgProbeFailed()+" "+m.UpdateError)
						}
						for _, warning := range m.Warnings {
							warningRow(c, locales.PkgProbeFailed()+" "+warning)
						}
						a.packageTable(c, m)
						toolNote(c, locales.PkgReadOnly())
						listCard(c, locales.PkgStatus(), locales.PkgReadOnlyLabel(), func() {
							listRow(c, m.Name, false, tilde(m.Path, a.Home), func() {
								iconButton(c, "copy", locales.CopyPath()).OnClick(func() { c.WriteClipboard(m.Path); c.Toast(locales.PathCopied()) })
							})
							cardNote(c, locales.PkgCachedNote())
							if !m.UpdatesFetched.IsZero() {
								cardNote(c, locales.VersionsAsOf(m.UpdatesFetched))
							}
						})
					})
					ui.Column(c).Width(342).Shrink(0).Gap(14).Children(func() {
						listCard(c, locales.PkgOtherManagers(), "", func() {
							for _, other := range st.PackageManagers {
								listRow(c, other.Name, false, tilde(other.Path, a.Home), func() {
									if other.Path == m.Path {
										ui.Text(c, locales.PkgSelected()).FontSize(10.5).TextColor(pal.TextFaint)
									} else {
										ui.Button(c, locales.Details()).OnClick(func() { a.packageManager = other.Path })
									}
								})
							}
						})
						if len(m.Settings) > 0 {
							listCard(c, locales.PkgSettings(), m.Name, func() {
								for _, setting := range m.Settings {
									cardNote(c, setting)
								}
							})
						}
						if m.ID == "brew" {
							listCard(c, "Taps", "brew tap", func() {
								for _, tap := range m.Taps {
									listRow(c, tap, false, "", func() {})
								}
								if len(m.Taps) == 0 {
									cardNote(c, locales.PkgNoTaps())
								}
							})
						}
						if services := packageServices(a.Snapshot, m.ID); len(services) > 0 {
							listCard(c, locales.DatabaseServices(), "", func() {
								for _, server := range services {
									status, _ := databaseStatus(c, server.Status)
									listRow(c, server.Service, false, status, func() {})
								}
								ui.Button(c, locales.DatabasesTitle()).OnClick(func() { a.router.Push("/tools/databases") })
							})
						}
					})
				})
			})
		})
	})
}

func (a *App) packageTable(c *ui.Context, m inventory.PackageManager) {
	pal := Colors(c.Theme())
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.PkgPackage()).Width(130).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.PkgKind()).Width(64).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.PkgPathUsage()).FontSize(11.5).TextColor(pal.TextFaint)
		})
		filter := strings.ToLower(a.packageFilter)
		shown, total := 0, 0
		for i, pkg := range m.Packages {
			if a.packageTab == 1 && pkg.Available == "" || !strings.Contains(strings.ToLower(pkg.Name), filter) {
				continue
			}
			total++
			// A system's packages run to thousands; the view builds every
			// row each frame.
			if shown == maxPackageRows {
				continue
			}
			shown++
			ui.Row(c.Key(strconv.Itoa(i))).Gap(12).Padding(12, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Width(130).Gap(3).Children(func() {
					ui.Text(c, pkg.Name).Font(FontMono).FontSize(12).SingleLine()
					ui.Text(c, pkg.Version).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
				})
				ui.Text(c, pkg.Kind).Width(64).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					if pkg.Reference != "" {
						ui.Text(c, pkg.Scope+" · "+pkg.Reference).Font(FontMono).FontSize(10.5).SingleLine().TextColor(pal.MonoText)
					}
					if pkg.Path != "" {
						ui.Text(c, tilde(pkg.Path, a.Home)).Font(FontMono).FontSize(10.5).SingleLine().TextColor(pal.MonoText)
					}
					if pkg.Available != "" {
						ui.Row(c).Children(func() { chip(c, c.Theme().Accent, pal.Muted, pal.MonoText, locales.PkgAvailable()+" "+pkg.Available) })
					}
					for _, tool := range packageTools(a.Snapshot, m.ID, pkg) {
						ui.Text(c, tool).FontSize(10.5).TextColor(pal.TextFaint)
					}
				})
				if pkg.Path != "" {
					iconButton(c, "copy", locales.CopyPath()).OnClick(func() { c.WriteClipboard(pkg.Path); c.Toast(locales.PathCopied()) })
					if a.Shell != nil {
						iconButton(c, "folder-open", showInFolder()).OnClick(func() { a.Shell.ShowItemInFolder(pkg.Path) })
					}
				}
			})
		}
		if shown == 0 {
			cardNote(c, locales.PkgNone())
		} else if shown < total {
			cardNote(c, locales.PkgShown(shown, total))
		}
	})
}

const maxPackageRows = 300

// packageServices are the database servers that run as the manager's
// services.
func packageServices(snap *inventory.Snapshot, manager string) []inventory.Database {
	db, ok := snap.Tool("databases")
	if !ok || db.Databases == nil {
		return nil
	}
	var out []inventory.Database
	for _, server := range db.Databases.Servers {
		if server.Source == manager && server.Service != "" {
			out = append(out, server)
		}
	}
	return out
}

func packageTools(snap *inventory.Snapshot, manager string, pkg inventory.SystemPackage) []string {
	var out []string
	if snap == nil || pkg.Path == "" {
		return out
	}
	for _, tool := range snap.Tools {
		for _, in := range tool.Installations {
			if in.Source.Manager == manager && (in.RealPath == pkg.Path || strings.HasPrefix(in.RealPath, pkg.Path+"/") || strings.HasPrefix(in.RealPath, pkg.Path+`\`)) {
				out = append(out, toolTitle(tool.Tool))
				break
			}
		}
	}
	return out
}

func packageUpdatesNote(manager string, online bool) string {
	switch manager {
	case "snap", "choco", "flatpak":
		if online {
			return locales.PkgUpdatesUnknown()
		}
		return locales.PkgNetworkUpdates()
	case "nix":
		return locales.PkgNixUpdates()
	case "scoop":
		return locales.PkgScoopUpdates()
	default:
		return locales.PkgUpdatesUnknown()
	}
}

// atLeast is a count of an incomplete list, "—" when it holds nothing.
func atLeast(n int) string {
	if n == 0 {
		return "—"
	}
	return locales.PkgAtLeast(n)
}
