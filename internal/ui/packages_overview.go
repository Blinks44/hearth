package ui

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) packagesOverviewView(c *ui.Context, m inventory.PackageManager, orphans []inventory.SystemPackage) {
	pal := Colors(c.Theme())
	partial := m.Unlisted && !m.UpdatesFetched.IsZero()
	installedCount := strconv.Itoa(len(m.Packages))
	if m.Unlisted {
		installedCount = atLeast(len(m.Packages))
	}
	updates := packageUpdateCount(m)
	updateCount := "—"
	if m.UpdatesKnown {
		updateCount = strconv.Itoa(updates)
	} else if partial {
		updateCount = atLeast(updates)
	}
	updateCaption := locales.PkgCached()
	if a.PackageChecks[m.Path] {
		updateCaption = locales.Checking()
	} else if !m.UpdatesFetched.IsZero() {
		updateCaption = locales.VersionsAsOf(m.UpdatesFetched)
	}
	disk, hasDisk := packageDiskUsage(m, a.Sizes)
	diskVal, diskCaption := "—", locales.PkgDiskNoSize()
	if hasDisk {
		diskVal, diskCaption = formatUsage(disk), locales.PkgDiskPackagesCaption()
	}
	asked := 0
	for _, pkg := range m.Packages {
		if !pkg.Dependency {
			asked++
		}
	}
	pkgCaption := locales.PkgCountNote()
	if m.ReasonsKnown {
		pkgCaption = locales.PkgAsked() + ": " + strconv.Itoa(asked) + " · " + locales.PkgDependencies() + ": " + strconv.Itoa(len(m.Packages)-asked)
	} else if partial {
		pkgCaption = locales.PkgPartialNote()
	}
	unusedVal := "—"
	unusedCaption := locales.PkgUnusedDepsNone()
	if m.ReasonsKnown || len(orphans) > 0 {
		unusedVal = strconv.Itoa(len(orphans))
		if len(orphans) > 0 {
			unusedCaption = locales.PkgUnusedDepsCount(len(orphans))
		}
	}
	ui.Row(c).Gap(12).AlignItems(ui.Stretch).Children(func() {
		overviewStat(c, locales.PkgViewPackages(), installedCount, func() {
			dockerOverviewCaption(c, pkgCaption, pal.TextFaint)
		})
		overviewStat(c, locales.PkgViewUpdates(), updateCount, func() {
			color := pal.TextFaint
			if updates > 0 {
				color = pal.WarningText
			}
			dockerOverviewCaption(c, updateCaption, color)
		})
		overviewStat(c, locales.PkgUnusedDeps(), unusedVal, func() {
			dockerOverviewCaption(c, unusedCaption, pal.TextFaint)
		})
		overviewStat(c, locales.PkgDiskCard(), diskVal, func() {
			dockerOverviewCaption(c, diskCaption, pal.TextFaint)
		})
	})
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
	columns(c, 14, func() {
		a.packagesOverviewAttention(c, m, updates, orphans)
		a.packagesOverviewDisk(c, m, disk, hasDisk)
	}, func() {
		a.packagesStatusCard(c, m, updates)
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
					listRow(c.Key("tap:"+tap), tap, false, "", func() {})
				}
				if len(m.Taps) == 0 {
					cardNote(c, locales.PkgNoTaps())
				}
			})
		}
		if services := packageServices(a.Snapshot, m.ID); len(services) > 0 {
			listCard(c, locales.DatabaseServices(), strconv.Itoa(len(services)), func() {
				for _, server := range services {
					status, _ := databaseStatus(c, server.Status)
					listRow(c.Key("ov-svc:"+server.Service), server.Service, false, status, func() {})
				}
				navListRow(c, "pkg-ov-databases", locales.DatabasesTitle(), locales.PkgMaintenanceDatabasesCaption(), locales.DatabasesTitle(), func() {
					a.router.Push("/tools/databases")
				})
			})
		}
	})
}

func (a *App) packagesOverviewAttention(c *ui.Context, m inventory.PackageManager, updates int, orphans []inventory.SystemPackage) {
	pal := Colors(c.Theme())
	listCard(c, locales.AttentionTitle(), "", func() {
		any := false
		if updates > 0 {
			any = true
			if p, ok := a.packagePlan(ops.PackageUpgrade, m, upgradable(m)); m.ID == "pacman" && ok {
				listRow(c.Key("ov-attn-updates"), locales.PkgAttentionUpdates(updates), false, locales.PkgAttentionUpdatesCaption(), func() {
					ui.Button(c.Key("ov-pacman-upgrade"), locales.PkgUpgradeSystem()).Shrink(0).OnClick(func() { a.Propose(p) })
				})
			} else {
				navListRow(c, "ov-attn-updates", locales.PkgAttentionUpdates(updates), locales.PkgAttentionUpdatesCaption(), locales.PkgReviewUpdates(), func() {
					a.pkg.view = pkgViewUpdates
					a.pkg.detailKey = ""
				})
			}
			if updates > 1 && choosable(m) {
				ui.Box(c).Padding(10, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
					warningRow(c, locales.PkgUpgradeAllWarning(updates))
				})
			}
		}
		if len(orphans) > 0 {
			any = true
			if plan, ok := ops.PackageAutoremovePlan(m, orphans, a.reread(m)); ok {
				listRow(c.Key("ov-attn-unused"), locales.PkgAttentionUnused(len(orphans)), false, locales.PkgAttentionUnusedCaption(), func() {
					ui.Button(c.Key("ov-autoremove"), locales.PkgAutoremoveAction()).Shrink(0).OnClick(func() { a.Propose(plan) })
				})
			} else {
				navListRow(c, "ov-attn-unused", locales.PkgAttentionUnused(len(orphans)), locales.PkgAttentionUnusedCaption(), locales.PkgReviewUpdates(), func() {
					a.pkg.view = pkgViewDependencies
					a.pkg.depsFilter = 1
					a.pkg.detailKey = ""
				})
			}
		}
		if !any {
			cardNote(c, locales.PkgNoAttention())
		}
	})
}

func (a *App) packagesOverviewDisk(c *ui.Context, m inventory.PackageManager, total inventory.Usage, hasDisk bool) {
	pal := Colors(c.Theme())
	aside := ""
	if hasDisk {
		aside = formatUsage(total)
	}
	listCard(c, locales.PkgDiskCard(), aside, func() {
		if !hasDisk || total.Bytes <= 0 {
			cardNote(c, locales.PkgDiskNoSize())
		} else {
			type sizedPkg struct {
				pkg   inventory.SystemPackage
				usage inventory.Usage
			}
			var sized []sizedPkg
			for _, pkg := range m.Packages {
				if u, ok := a.Sizes.Size(pkg); ok && u.Bytes > 0 {
					sized = append(sized, sizedPkg{pkg: pkg, usage: u})
				}
			}
			slices.SortFunc(sized, func(x, y sizedPkg) int {
				return cmp.Compare(y.usage.Bytes, x.usage.Bytes)
			})
			limit := min(len(sized), 5)
			for _, item := range sized[:limit] {
				pkg := item.pkg
				ui.Column(c.Key("ov-disk-"+packageKey(pkg))).Gap(5).Padding(8, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						clipped(c, pkg.Name).Font(FontMono).FontSize(12).Grow(1).MinWidth(0)
						ui.Text(c, formatUsage(item.usage)).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
					})
					requestBar(c, float64(item.usage.Bytes)/float64(total.Bytes), pal.InfoText)
				})
			}
		}
		navListRow(c, "pkg-ov-cleanup", locales.PkgOpenCleanup(), locales.PkgMaintenanceCleanupCaption(), locales.PkgOpenCleanup(), func() {
			if idx := slices.Index(cleanupGroups, cleanup.Caches); idx >= 0 {
				a.cleanupTab = idx + 1
			} else {
				a.cleanupTab = 0
			}
			a.cleanupFilter = ""
			a.router.Push(cleanupPage)
		})
	})
}

func (a *App) packagesStatusCard(c *ui.Context, m inventory.PackageManager, _ int) {
	statusAside := ""
	if !ops.PackageManaged(m.ID) {
		statusAside = locales.PkgReadOnlyLabel()
	}
	listCard(c, locales.PkgStatus(), statusAside, func() {
		ver := cmp.Or(m.Version, locales.UnknownVersion())
		listRow(c.Key("ov-status-bin"), m.Name+" · "+ver, false, tilde(m.Path, a.Home), func() {
			a.pathActions(c, "pkg-mgr-path", m.Path)
		})
		if m.Prefix != "" && m.Prefix != m.Path {
			listRow(c.Key("ov-status-prefix"), locales.PkgDetailPath(), false, tilde(m.Prefix, a.Home), func() {
				a.pathActions(c, "pkg-mgr-prefix", m.Prefix)
			})
		}
		if !ops.PackageManaged(m.ID) {
			cardNote(c, locales.PkgReadOnly())
		}
		cardNote(c, locales.PkgCachedNote())
		if !m.UpdatesFetched.IsZero() {
			cardNote(c, locales.VersionsAsOf(m.UpdatesFetched))
		}
	})
}
