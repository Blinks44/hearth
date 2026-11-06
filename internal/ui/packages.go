package ui

import (
	"cmp"
	"slices"
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
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.SuccessSoft).Center().Children(func() { ui.Icon(c, Icon("package")).Size(24, 24).TextColor(pal.MonoText) })
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, m.Name).FontSize(22).FontWeight(600)
						path := m.Prefix
						if path == "" {
							path = m.Path
						}
						clipped(c, tilde(path, a.Home)).Font(FontMono).FontSize(11.5).TextColor(c.Theme().TextMuted)
					})
				}, func() {
					version := m.Version
					if version == "" { // Scoop is a PowerShell script Hearth does not run
						version = locales.UnknownVersion()
					}
					stateCard(c, c.Theme().Success, m.Name, version, "")
					stateCard(c, pal.TextFaint, locales.PkgInstalled(), installedCount, countNote)
					stateCard(c, c.Theme().Warning, locales.PkgUpdateSummary(), updateCount, updateNote)
				})
				// A narrow window puts the order and the filter under the tabs.
				toolbar := ui.Row(c).Gap(12).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segs := []segment{{locales.PkgInstalled(), installedCount}, {locales.PkgUpdates(), updateCount}}
					if m.ReasonsKnown {
						asked := 0
						for _, pkg := range m.Packages {
							if !pkg.Dependency {
								asked++
							}
						}
						segs = []segment{{locales.TabAll(), installedCount}, {locales.PkgAsked(), strconv.Itoa(asked)},
							{locales.PkgDependencies(), strconv.Itoa(len(m.Packages) - asked)}, {locales.PkgUpdates(), updateCount}}
					}
					a.packageTab = min(a.packageTab, len(segs)-1)
					segments(c, "package-tabs", &a.packageTab, segs)
					ui.Box(c).Grow(1).Height(1)
					if sorts := packageSorts(m, a.Sizes); len(sorts) > 1 {
						labels := make([]string, len(sorts))
						for i, sort := range sorts {
							labels[i] = sortLabel(sort)
						}
						value := labels[max(slices.Index(sorts, a.packageSort), 0)]
						ui.Select(c.Key("package-sort"), &value, labels).Label(locales.PkgSort()).Width(170).OnChange(func() {
							a.packageSort = sorts[slices.Index(labels, value)]
						})
					}
					ui.SearchField(c.Key("package-filter"), &a.packageFilter).Bind(&a.filterField).Placeholder(locales.PkgFilter()).Width(200)
				})
				columns(c, 12, func() {
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
				}, func() {
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
}

func (a *App) packageTable(c *ui.Context, m inventory.PackageManager) {
	pal := Colors(c.Theme())
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.PkgPackage()).Width(nameWidth(c)).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.PkgKind()).Width(64).FontSize(11.5).TextColor(pal.TextFaint)
			usage := locales.PkgPathUsage()
			if m.ReasonsKnown {
				usage = locales.PkgSizeUsage()
			}
			ui.Text(c, usage).FontSize(11.5).TextColor(pal.TextFaint)
		})
		filter := strings.ToLower(a.packageFilter)
		dependents := packageDependents(m)
		order := make([]int, len(m.Packages))
		for i := range order {
			order[i] = i
		}
		switch sorts := packageSorts(m, a.Sizes); {
		case a.packageSort == sortByDependents && slices.Contains(sorts, sortByDependents):
			slices.SortStableFunc(order, func(x, y int) int {
				return cmp.Compare(dependents[m.Packages[y].Name], dependents[m.Packages[x].Name])
			})
		case a.packageSort == sortBySize && slices.Contains(sorts, sortBySize):
			slices.SortStableFunc(order, func(x, y int) int {
				sx, _ := a.Sizes.Size(m.Packages[x])
				sy, _ := a.Sizes.Size(m.Packages[y])
				return cmp.Compare(sy.Bytes, sx.Bytes)
			})
		}
		updatesTab, askedTab, depsTab := a.packageTab == 1, false, false
		if m.ReasonsKnown {
			updatesTab, askedTab, depsTab = a.packageTab == 3, a.packageTab == 1, a.packageTab == 2
		}
		shown, total := 0, 0
		for _, i := range order {
			pkg := m.Packages[i]
			if updatesTab && pkg.Available == "" || askedTab && pkg.Dependency || depsTab && !pkg.Dependency ||
				!strings.Contains(strings.ToLower(pkg.Name), filter) {
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
				ui.Column(c).Width(nameWidth(c)).Gap(3).Children(func() {
					clipped(c, pkg.Name).Font(FontMono).FontSize(12)
					clipped(c, pkg.Version).FontSize(10.5).TextColor(pal.TextFaint)
				})
				ui.Text(c, pkg.Kind).Width(64).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					if pkg.Reference != "" {
						clipped(c, pkg.Scope+" · "+pkg.Reference).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
					}
					var usage []string
					if size, ok := a.Sizes.Size(pkg); ok {
						usage = append(usage, formatUsage(size))
					}
					if m.ReasonsKnown {
						usage = append(usage, packageReason(m, pkg, dependents[pkg.Name]))
					}
					if len(usage) > 0 {
						ui.Text(c, strings.Join(usage, " · ")).FontSize(10.5).TextColor(pal.TextFaint)
					}
					// Who asked for it takes the path's place; the buttons
					// still copy and show it.
					if pkg.Path != "" && !m.ReasonsKnown {
						clipped(c, tilde(pkg.Path, a.Home)).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
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

// packageDependents counts, by name, the installed packages that depend on
// each; empty where the manager does not list dependencies.
func packageDependents(m inventory.PackageManager) map[string]int {
	out := map[string]int{}
	for _, pkg := range m.Packages {
		for _, d := range pkg.Dependencies {
			out[d]++
		}
	}
	return out
}

// packageReason says who asked for pkg: the user, or other packages, and
// how many of them need it where the manager knows.
func packageReason(m inventory.PackageManager, pkg inventory.SystemPackage, dependents int) string {
	switch {
	case !pkg.Dependency:
		return locales.PkgAskedByYou()
	case dependents > 0:
		return locales.PkgNeededBy(dependents)
	case m.ID == "brew": // brew lists every dependency, so none is none
		return locales.PkgNotNeeded()
	}
	return locales.PkgDependency()
}

// The package table's orders.
const (
	sortByName = iota
	sortByDependents
	sortBySize
)

// packageSorts are the orders m's packages can take: by dependents where
// the manager tells them, by size once any is known.
func packageSorts(m inventory.PackageManager, sizes *inventory.Sizes) []int {
	out := []int{sortByName}
	if m.ReasonsKnown {
		out = append(out, sortByDependents)
	}
	if slices.ContainsFunc(m.Packages, func(p inventory.SystemPackage) bool { _, ok := sizes.Size(p); return ok }) {
		out = append(out, sortBySize)
	}
	return out
}

func sortLabel(sort int) string {
	switch sort {
	case sortByDependents:
		return locales.PkgByDependents()
	case sortBySize:
		return locales.PkgBySize()
	}
	return locales.PkgByName()
}

// formatUsage writes a size in decimal units, as the file managers of
// macOS and Linux do: "86 MB", "2.4 GB"; one partly read is "≥ 86 MB".
func formatUsage(u inventory.Usage) string {
	var s string
	switch b := u.Bytes; {
	case b >= 1e9:
		s = locales.SizeGb(float64(b) / 1e9)
	case b >= 1e6:
		s = locales.SizeMb(int(b / 1e6))
	default:
		s = locales.SizeKb(int((b + 999) / 1000))
	}
	if u.Partial {
		return "≥ " + s
	}
	return s
}
