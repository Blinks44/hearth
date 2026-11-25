package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

// packagesListView renders the Packages (updatesOnly=false) or Updates
// (updatesOnly=true) view inside a package manager, with split-view details
// and a floating dark bulk-action bar.
func (a *App) packagesListView(c *ui.Context, m inventory.PackageManager, updatesOnly bool) {
	partial := m.Unlisted && !m.UpdatesFetched.IsZero()
	updates := packageUpdateCount(m)

	// Sub-filter definitions for the Packages view.
	type subFilter struct {
		label string
		count int
		match func(inventory.SystemPackage) bool
	}
	var subs []subFilter
	if !updatesOnly {
		asked, formulae, casks := 0, 0, 0
		for _, pkg := range m.Packages {
			if !pkg.Dependency {
				asked++
			}
			switch pkg.Kind {
			case "formula":
				formulae++
			case "cask":
				casks++
			}
		}
		subs = append(subs, subFilter{label: locales.TabAll(), count: len(m.Packages), match: func(inventory.SystemPackage) bool { return true }})
		if m.ReasonsKnown {
			subs = append(subs, subFilter{label: locales.PkgAsked(), count: asked, match: func(p inventory.SystemPackage) bool { return !p.Dependency }})
		}
		if m.ID == "brew" && (formulae > 0 || casks > 0) {
			subs = append(subs,
				subFilter{label: locales.PkgKindFormulae(), count: formulae, match: func(p inventory.SystemPackage) bool { return p.Kind == "formula" }},
				subFilter{label: locales.PkgKindCasks(), count: casks, match: func(p inventory.SystemPackage) bool { return p.Kind == "cask" }},
			)
		}
		if a.packageTab < 0 || a.packageTab >= len(subs) {
			a.packageTab = 0
		}
	}

	toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0)
	if narrow(c) {
		toolbar.Wrap()
	}
	toolbar.Children(func() {
		if !updatesOnly && len(subs) > 1 {
			segs := make([]segment, len(subs))
			for i, sf := range subs {
				cnt := strconv.Itoa(sf.count)
				if i == 0 && m.Unlisted {
					cnt = atLeast(sf.count)
				}
				segs[i] = segment{sf.label, cnt}
			}
			segments(c, "pkg-sub-tabs", &a.packageTab, segs)
		}
		ui.Box(c).Grow(1).Height(1)
		if m.ID == "pacman" && updates > 0 {
			if p, ok := a.packagePlan(ops.PackageUpgrade, m, upgradable(m)); ok {
				ui.PrimaryButton(c.Key("pkg-pacman-upgrade"), locales.PkgUpgradeSystem()).Height(controlHeight).OnClick(func() { a.Propose(p) })
			}
		}
		if sorts := packageSorts(m, a.Sizes); len(sorts) > 1 {
			labels := make([]string, len(sorts))
			for i, sort := range sorts {
				labels[i] = sortLabel(sort)
			}
			value := labels[max(slices.Index(sorts, a.packageSort), 0)]
			ui.Select(c.Key("package-sort"), &value, labels).Label(locales.PkgSort()).Height(controlHeight).Width(170).OnChange(func() {
				a.packageSort = sorts[slices.Index(labels, value)]
			})
		}
		ui.SearchField(c.Key("package-filter"), &a.packageFilter).Bind(&a.filterField).Placeholder(locales.PkgFilter()).Height(controlHeight).Width(200)
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
	if updatesOnly && updates > 1 && choosable(m) {
		toolNote(c, locales.PkgUpgradeAllWarning(updates))
	}

	var matchFn func(inventory.SystemPackage) bool
	if updatesOnly {
		matchFn = func(p inventory.SystemPackage) bool { return p.Available != "" }
	} else if len(subs) > 0 {
		matchFn = subs[a.packageTab].match
	} else {
		matchFn = func(inventory.SystemPackage) bool { return true }
	}
	emptyMsg := locales.PkgNone()
	if updatesOnly && updates == 0 && m.UpdatesKnown && strings.TrimSpace(a.packageFilter) == "" {
		emptyMsg = locales.PkgUpdatesUpToDate()
	}
	a.packagesTablePane(c, m, matchFn, updatesOnly, emptyMsg, "list")
}

// packagesDepsView renders the Dependencies view (All dependencies / Unused)
// with safe Autoremove support.
func (a *App) packagesDepsView(c *ui.Context, m inventory.PackageManager, orphans []inventory.SystemPackage) {
	orphanSet := make(map[string]bool, len(orphans))
	for _, p := range orphans {
		orphanSet[packageKey(p)] = true
	}
	depCount := 0
	for _, p := range m.Packages {
		if p.Dependency {
			depCount++
		}
	}
	if a.pkg.depsFilter < 0 || a.pkg.depsFilter > 1 {
		a.pkg.depsFilter = 0
	}

	toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0)
	if narrow(c) {
		toolbar.Wrap()
	}
	toolbar.Children(func() {
		segments(c, "pkg-deps-tabs", &a.pkg.depsFilter, []segment{
			{locales.PkgDepsFilterAll(), strconv.Itoa(depCount)},
			{locales.PkgDepsFilterUnused(), strconv.Itoa(len(orphans))},
		})
		ui.Box(c).Grow(1).Height(1)
		if plan, ok := ops.PackageAutoremovePlan(m, orphans, a.reread(m)); ok {
			ui.PrimaryButton(c.Key("pkg-deps-autoremove"), locales.PkgAutoremoveAction()).Height(controlHeight).OnClick(func() {
				a.Propose(plan)
			})
		}
		if sorts := packageSorts(m, a.Sizes); len(sorts) > 1 {
			labels := make([]string, len(sorts))
			for i, sort := range sorts {
				labels[i] = sortLabel(sort)
			}
			value := labels[max(slices.Index(sorts, a.packageSort), 0)]
			ui.Select(c.Key("package-deps-sort"), &value, labels).Label(locales.PkgSort()).Height(controlHeight).Width(170).OnChange(func() {
				a.packageSort = sorts[slices.Index(labels, value)]
			})
		}
		ui.SearchField(c.Key("package-deps-filter"), &a.packageFilter).Bind(&a.filterField).Placeholder(locales.PkgFilter()).Height(controlHeight).Width(200)
	})

	if len(orphans) > 0 {
		warningRow(c, locales.PkgDepsUnusedBanner(len(orphans)))
	}

	matchFn := func(p inventory.SystemPackage) bool {
		if a.pkg.depsFilter == 1 {
			return orphanSet[packageKey(p)]
		}
		return p.Dependency
	}
	emptyMsg := locales.PkgDepsNone()
	if a.pkg.depsFilter == 1 {
		emptyMsg = locales.PkgDepsNoUnused()
	}
	a.packagesTablePane(c, m, matchFn, false, emptyMsg, "deps")
}

// packagesTablePane draws the shared filterable/sortable table inside
// workloadPanes, plus the detail inspector and floating bulk action bar.
func (a *App) packagesTablePane(c *ui.Context, m inventory.PackageManager, match func(inventory.SystemPackage) bool, updatesOnly bool, emptyMsg, scopeKey string) {
	pal := Colors(c.Theme())
	filter := strings.ToLower(strings.TrimSpace(a.packageFilter))
	dependents := packageDependents(m)
	managed := ops.PackageManaged(m.ID)

	// Build sorted index order.
	order := make([]int, len(m.Packages))
	for i := range order {
		order[i] = i
	}
	switch sorts := packageSorts(m, a.Sizes); {
	case a.packageSort == sortByDependents && slices.Contains(sorts, sortByDependents):
		slices.SortStableFunc(order, func(x, y int) int {
			return cmp.Compare(len(dependents[m.Packages[y].Name]), len(dependents[m.Packages[x].Name]))
		})
	case a.packageSort == sortBySize && slices.Contains(sorts, sortBySize):
		slices.SortStableFunc(order, func(x, y int) int {
			sx, _ := a.Sizes.Size(m.Packages[x])
			sy, _ := a.Sizes.Size(m.Packages[y])
			return cmp.Compare(sy.Bytes, sx.Bytes)
		})
	}

	canSelect := func(pkg inventory.SystemPackage) bool {
		if !managed {
			return false
		}
		if updatesOnly {
			return choosable(m) && pkg.Available != ""
		}
		return removable(pkg) || (choosable(m) && pkg.Available != "")
	}

	var matched []int
	var selectableKeys []string
	var detailPkg *inventory.SystemPackage
	for _, idx := range order {
		pkg := m.Packages[idx]
		if a.pkg.detailKey != "" && packageKey(pkg) == a.pkg.detailKey {
			p := pkg
			detailPkg = &p
		}
		if !match(pkg) {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(pkg.Name+" "+pkg.Description), filter) {
			continue
		}
		matched = append(matched, idx)
		if canSelect(pkg) {
			selectableKeys = append(selectableKeys, m.Path+"\x00"+packageKey(pkg))
		}
	}
	if a.pkg.detailKey != "" && detailPkg == nil {
		a.pkg.detailKey = ""
	}
	compact := detailPkg != nil

	limitKey := m.Path + "\x00" + scopeKey + "\x00" + strconv.Itoa(a.pkg.view) + "\x00" + strconv.Itoa(a.packageTab) + "\x00" + strconv.Itoa(a.pkg.depsFilter) + "\x00" + strconv.Itoa(a.packageSort) + "\x00" + filter
	if a.packageLimitKey != limitKey || a.packageLimit < maxPackageRows {
		a.packageLimitKey = limitKey
		a.packageLimit = maxPackageRows
	}
	hasPath := slices.ContainsFunc(m.Packages, func(pkg inventory.SystemPackage) bool { return pkg.Path != "" })
	hasMenu := slices.ContainsFunc(m.Packages, func(pkg inventory.SystemPackage) bool { return a.packageCan(m, pkg) })

	// Collect selected packages across m for the floating bulk bar.
	var selectedAll, selectedUpgradable, selectedRemovable []inventory.SystemPackage
	if managed && len(a.packageChosen) > 0 {
		for _, pkg := range m.Packages {
			if !a.packageChosen[m.Path+"\x00"+packageKey(pkg)] {
				continue
			}
			selectedAll = append(selectedAll, pkg)
			if choosable(m) && pkg.Available != "" {
				selectedUpgradable = append(selectedUpgradable, pkg)
			}
			if removable(pkg) {
				selectedRemovable = append(selectedRemovable, pkg)
			}
		}
	}

	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		workloadPanes(c, func() {
			ui.Row(c).Gap(12).Padding(8, 16).AlignItems(ui.Center).Background(pal.Subtle).Children(func() {
				if len(selectableKeys) > 0 {
					selectedCount := 0
					for _, key := range selectableKeys {
						if a.packageChosen[key] {
							selectedCount++
						}
					}
					state := ui.CheckOff
					switch {
					case selectedCount == len(selectableKeys):
						state = ui.CheckOn
					case selectedCount > 0:
						state = ui.CheckMixed
					}
					ui.Column(c).Width(18).Shrink(0).AlignItems(ui.Start).Justify(ui.Center).Children(func() {
						ui.CheckboxState(c.Key("select-all-"+scopeKey), &state, "").Label(locales.PkgSelectAll()).OnClick(func() {
							if a.packageChosen == nil {
								a.packageChosen = map[string]bool{}
							}
							selectAll := selectedCount < len(selectableKeys)
							for _, key := range selectableKeys {
								if selectAll {
									a.packageChosen[key] = true
								} else {
									delete(a.packageChosen, key)
								}
							}
						})
					})
				}
				nameHead := ui.Text(c, locales.PkgPackage()).FontSize(11.5).TextColor(pal.TextFaint)
				if compact {
					nameHead.Grow(1).MinWidth(0)
				} else {
					nameHead.Width(nameWidth(c))
				}
				if !compact {
					ui.Text(c, locales.PkgKind()).Width(64).FontSize(11.5).TextColor(pal.TextFaint)
					usage := locales.PkgPathUsage()
					if m.ReasonsKnown {
						usage = locales.PkgSizeUsage()
					}
					ui.Text(c, usage).FontSize(11.5).TextColor(pal.TextFaint)
				}
			})
		}, func() {
			shown := 0
			for _, idx := range matched {
				if shown == a.packageLimit {
					break
				}
				shown++
				pkg := m.Packages[idx]
				pKey := packageKey(pkg)
				fullKey := m.Path + "\x00" + pKey
				isDetail := a.pkg.detailKey == pKey
				line := ui.Row(c.Key(scopeKey+"-"+strconv.Itoa(idx))).Gap(12).Padding(0, 16).MinHeight(44).
					AlignItems(ui.Stretch).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
				line.Children(func() {
					if len(selectableKeys) > 0 {
						ui.Column(c).Width(18).Shrink(0).AlignItems(ui.Start).Justify(ui.Center).Children(func() {
							if canSelect(pkg) {
								on := a.packageChosen[fullKey]
								if ui.Checkbox(c.Key("check"), &on, "").Label(locales.KubeBulkSelect(pkg.Name)).Changed() {
									if a.packageChosen == nil {
										a.packageChosen = map[string]bool{}
									}
									if on {
										a.packageChosen[fullKey] = true
									} else {
										delete(a.packageChosen, fullKey)
									}
								}
							}
						})
					}
					row := ui.ButtonBase(c.Key("open-detail")).Row().Grow(1).MinWidth(0).Padding(10, 0).Gap(12).
						AlignItems(ui.Center).Justify(ui.Start).Label(pkg.Name).OnClick(func() {
						if a.pkg.detailKey == pKey {
							a.pkg.detailKey = ""
						} else {
							a.pkg.detailKey = pKey
							a.pkg.detailTab = 0
						}
					})
					if a.packageCan(m, pkg) {
						row.ContextMenu(func(menu *ui.Menu) { a.packageItems(menu, m, pkg) })
					}
					bg := c.Theme().Background.Alpha(0)
					switch {
					case isDetail:
						bg = pal.ActiveRow
					case line.Hovered() || row.Hovered():
						bg = pal.Subtle
					}
					line.Background(line.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
					row.Children(func() {
						nameCol := ui.Column(c).Gap(2).MinWidth(0)
						if compact {
							nameCol.Grow(1)
						} else {
							nameCol.Width(nameWidth(c)).Shrink(0)
						}
						nameCol.Children(func() {
							clipped(c, pkg.Name).Font(FontMono).FontSize(12)
							verLine := pkg.Version
							if compact && pkg.Available != "" {
								verLine += " → " + pkg.Available
							}
							clipped(c, verLine).FontSize(10.5).TextColor(pal.TextFaint)
						})
						if compact {
							if pkg.Available != "" {
								ui.Box(c).Size(7, 7).Radius(4).Shrink(0).Background(c.Theme().Accent)
							}
							return
						}
						ui.Text(c, pkg.Kind).Width(64).Shrink(0).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
							if pkg.Reference != "" {
								clipped(c, pkg.Scope+" · "+pkg.Reference).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
							}
							var usage []string
							usageTip := ""
							if size, ok := a.Sizes.Size(pkg); ok {
								usage = append(usage, formatUsage(size))
							}
							if m.ReasonsKnown {
								reason, tip := packageReason(m, pkg, dependents[pkg.Name])
								usage = append(usage, reason)
								usageTip = tip
							}
							if len(usage) > 0 {
								u := ui.Text(c, strings.Join(usage, " · ")).FontSize(10.5).TextColor(pal.TextFaint)
								if usageTip != "" {
									u.Tooltip(usageTip)
								}
							}
							if pkg.Path != "" && !m.ReasonsKnown {
								clipped(c, tilde(pkg.Path, a.Home)).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
							}
							if pkg.Available != "" {
								ui.Row(c).Children(func() {
									chip(c, c.Theme().Accent, pal.Muted, pal.MonoText, locales.PkgAvailable()+" "+pkg.Available)
								})
							}
							for _, tool := range packageTools(a.Snapshot, m.ID, pkg) {
								ui.Text(c, tool).FontSize(10.5).TextColor(pal.TextFaint)
							}
						})
					})
					if !compact && (hasPath || hasMenu) {
						ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
							if hasPath {
								if pkg.Path != "" {
									iconButton(c, "copy", locales.CopyPath()).OnClick(func() { c.WriteClipboard(pkg.Path); c.Toast(locales.PathCopied()) })
									if a.Shell != nil {
										iconButton(c, "folder-open", showInFolder()).OnClick(func() { a.Shell.ShowItemInFolder(pkg.Path) })
									}
								} else {
									iconSpacer(c)
									if a.Shell != nil {
										iconSpacer(c)
									}
								}
							}
							if hasMenu {
								if a.packageCan(m, pkg) {
									iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) { a.packageItems(menu, m, pkg) })
								} else {
									iconSpacer(c)
								}
							}
						})
					}
				})
			}
			if shown == 0 {
				cardNote(c, emptyMsg)
			} else if shown < len(matched) {
				more := min(maxPackageRows, len(matched)-shown)
				ui.Row(c.Key("packages-more-row")).Gap(12).Padding(10, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
					ui.Text(c, locales.PkgShownMore(shown, len(matched))).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
					ui.Button(c.Key("packages-more"), locales.PkgShowMore(more)).Height(controlHeight).OnClick(func() {
						a.packageLimit += maxPackageRows
					})
				})
			}
			// Bottom padding when bulk bar is visible so the last row stays reachable.
			if len(selectedAll) > 0 {
				ui.Box(c).Height(56)
			}
		}, a.pkg.detailKey, func() {
			if detailPkg != nil {
				a.packageDetailView(c, m, *detailPkg, dependents[detailPkg.Name])
			}
		})

		if len(selectedAll) > 0 {
			blocked := a.op.running()
			caption := locales.PkgBulkSelected(len(selectedAll))
			floatingBulkBar(c, "pkg-bulk-bar", caption, func() { a.packageChosen = nil }, func(fg, dangerFg, dangerBg ui.Color) {
				if len(selectedUpgradable) > 0 && choosable(m) {
					upgs := selectedUpgradable
					bulkAction(c.Key("pkg-bulk-upgrade"), "circle-arrow-up", locales.PkgBulkUpgrade(len(upgs)), fg, ui.Transparent, blocked, func() {
						if p, ok := a.packagePlan(ops.PackageUpgrade, m, upgs); ok {
							a.proposeThen(func() { a.packageChosen = nil })(p, nil)
						}
					})
				}
				if len(selectedRemovable) > 0 && managed {
					rems := selectedRemovable
					bulkAction(c.Key("pkg-bulk-remove"), "trash-2", locales.PkgBulkRemove(len(rems)), dangerFg, dangerBg, blocked, func() {
						if p, ok := a.packagePlan(ops.PackageUninstall, m, rems); ok {
							a.proposeThen(func() { a.packageChosen = nil })(p, nil)
						}
					})
				}
			})
		}
	})
}
