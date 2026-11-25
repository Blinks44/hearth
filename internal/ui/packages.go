package ui

import (
	"cmp"
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/pkgmgr"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const (
	packagesPage        = "/system/packages"
	packagesManagerPage = packagesPage + "/manager"
	maxPackageRows      = 300
)

// packagesManagerPath is the drill-down page for the package manager at path.
func packagesManagerPath(path string) string {
	return packagesManagerPage + "?path=" + url.QueryEscape(path)
}

// packagesEntryPath returns the sidebar destination for Package Managers:
// directly into the active/first detected manager when present, otherwise the
// landing page.
func (a *App) packagesEntryPath() string {
	if m, ok := a.activePackageManager(); ok {
		return packagesManagerPath(m.Path)
	}
	return packagesPage
}

// openPackageSearch jumps directly to mgrPath's Packages tab filtered to pkgName.
func (a *App) openPackageSearch(mgrPath, pkgName string) {
	a.packageManager = mgrPath
	a.packageTab = 0
	a.packageFilter = pkgName
	a.pkg.view = pkgViewPackages
	a.pkg.detailKey = ""
	if mgrPath != "" {
		a.router.Push(packagesManagerPath(mgrPath))
		return
	}
	a.router.Push(packagesPage)
}

// packageManagersWithUpdates returns detected package managers with cached
// online update overlays applied.
func (a *App) packageManagersWithUpdates() []inventory.PackageManager {
	st, ok := a.Snapshot.Tool("packages")
	if !ok || len(st.PackageManagers) == 0 {
		return nil
	}
	out := make([]inventory.PackageManager, len(st.PackageManagers))
	for i, m := range st.PackageManagers {
		if checked, has := a.PackageUpdates[m.Path]; has {
			m = checked
		}
		out[i] = m
	}
	return out
}

func (a *App) packageManagerByPath(path string) (inventory.PackageManager, bool) {
	for _, m := range a.packageManagersWithUpdates() {
		if m.Path == path {
			return m, true
		}
	}
	return inventory.PackageManager{}, false
}

func (a *App) activePackageManager() (inventory.PackageManager, bool) {
	managers := a.packageManagersWithUpdates()
	if len(managers) == 0 {
		return inventory.PackageManager{}, false
	}
	for _, m := range managers {
		if m.Path == a.packageManager {
			return m, true
		}
	}
	return managers[0], true
}

func pkgManagerIcon(id string) string {
	switch id {
	case "brew":
		return "brand/homebrew"
	case "apt":
		return "brand/debian"
	case "dnf":
		return "brand/fedora"
	case "pacman":
		return "brand/archlinux"
	case "zypper":
		return "brand/opensuse"
	case "apk":
		return "brand/alpinelinux"
	case "snap":
		return "brand/snapcraft"
	case "flatpak":
		return "brand/flatpak"
	case "nix":
		return "brand/nixos"
	case "port":
		return "brand/macports"
	case "winget":
		return "brand/windows"
	case "choco":
		return "brand/chocolatey"
	case "scoop":
		return "lucide/terminal"
	default:
		return "lucide/package"
	}
}

func pkgAvailableDescription(id string) string {
	switch id {
	case "brew":
		return locales.PkgAvailableDescBrew()
	case "nix":
		return locales.PkgAvailableDescNix()
	case "port":
		return locales.PkgAvailableDescPort()
	case "winget":
		return locales.PkgAvailableDescWinget()
	case "scoop":
		return locales.PkgAvailableDescScoop()
	case "choco":
		return locales.PkgAvailableDescChoco()
	case "flatpak":
		return locales.PkgAvailableDescFlatpak()
	default:
		return ""
	}
}

func packageUpdateCount(m inventory.PackageManager) int {
	n := 0
	for _, pkg := range m.Packages {
		if pkg.Available != "" {
			n++
		}
	}
	return n
}

func packageDiskUsage(m inventory.PackageManager, sizes *inventory.Sizes) (inventory.Usage, bool) {
	if sizes == nil {
		return inventory.Usage{}, false
	}
	var total inventory.Usage
	found := false
	for _, pkg := range m.Packages {
		if u, ok := sizes.Size(pkg); ok {
			total.Bytes += u.Bytes
			total.Partial = total.Partial || u.Partial
			found = true
		}
	}
	return total, found
}

// packages renders the /system/packages landing page: summary KPIs, detected
// managers, available managers for this OS, maintenance links, and quick jumps.
func (a *App) packages(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		managers := a.packageManagersWithUpdates()
		active, hasActive := a.activePackageManager()
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			if hasActive {
				a.refreshIndexButton(c, active)
				target := active
				ui.PrimaryButton(c.Key("pkg-open-active"), locales.PkgOpenManager(target.Name)).Height(controlHeight).OnClick(func() {
					a.packageManager = target.Path
					a.router.Push(packagesManagerPath(target.Path))
				})
			}
		}})
		if a.Snapshot == nil {
			a.toolEmpty(c, "packages", p.title())
			return
		}
		available := pkgmgr.AvailableManagers()
		var allServices []inventory.Database
		for _, m := range managers {
			allServices = append(allServices, packageServices(a.Snapshot, m.ID)...)
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.packagesLandingHeader(c, p, managers)
				segs := []segment{
					{locales.PkgTabManagers(), strconv.Itoa(len(managers))},
					{locales.PkgTabAvailable(), strconv.Itoa(len(available))},
				}
				servicesIdx := -1
				if len(allServices) > 0 {
					servicesIdx = len(segs)
					segs = append(segs, segment{locales.PkgTabServices(), strconv.Itoa(len(allServices))})
				}
				pathIdx := len(segs)
				segs = append(segs, segment{"PATH", ""})
				a.packageLandingTab = min(a.packageLandingTab, len(segs)-1)
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					segments(c, "pkg-landing-tabs", &a.packageLandingTab, segs)
				})
				columns(c, 12, func() {
					switch a.packageLandingTab {
					case 0:
						a.packagesDetectedCard(c, managers)
						if hasActive {
							a.packagesQuickViewsCard(c, active)
						}
					case 1:
						a.packagesAvailableCard(c, managers, available)
					case servicesIdx:
						a.packagesServicesCard(c, allServices)
					case pathIdx:
						a.packagesPathCard(c, managers)
					}
				}, func() {
					a.packagesLandingAttentionCard(c, managers)
					a.packagesMaintenanceCard(c, active, hasActive, len(allServices) > 0)
					listCard(c, locales.PkgHowTitle(), "", func() {
						listRowWrapped(c, locales.PkgHowScan(), locales.PkgHowScanCaption(), func() {})
						listRowWrapped(c, locales.PkgHowPlan(), locales.PkgHowPlanCaption(), func() {})
						listRowWrapped(c, locales.PkgHowSearch(), locales.PkgHowSearchCaption(), func() {})
					})
				})
			})
		})
	})
}

func (a *App) packagesLandingHeader(c *ui.Context, p page, managers []inventory.PackageManager) {
	pal := Colors(c.Theme())
	totalPkgs, totalUpdates := 0, 0
	anyUnlisted, anyUpdatesKnown, anyPartial, anyChecking := false, false, false, false
	var disk inventory.Usage
	hasDisk := false
	for _, m := range managers {
		totalPkgs += len(m.Packages)
		totalUpdates += packageUpdateCount(m)
		if m.Unlisted {
			anyUnlisted = true
		}
		if m.UpdatesKnown {
			anyUpdatesKnown = true
		}
		if m.Unlisted && !m.UpdatesFetched.IsZero() {
			anyPartial = true
		}
		if a.PackageChecks[m.Path] {
			anyChecking = true
		}
		if u, ok := packageDiskUsage(m, a.Sizes); ok {
			disk.Bytes += u.Bytes
			disk.Partial = disk.Partial || u.Partial
			hasDisk = true
		}
	}
	installedVal := strconv.Itoa(totalPkgs)
	if anyUnlisted {
		installedVal = atLeast(totalPkgs)
	}
	updatesVal := "—"
	if anyUpdatesKnown {
		updatesVal = strconv.Itoa(totalUpdates)
	} else if anyPartial {
		updatesVal = atLeast(totalUpdates)
	}
	updateDot := pal.TextFaint
	switch {
	case totalUpdates > 0:
		updateDot = c.Theme().Warning
	case anyUpdatesKnown:
		updateDot = c.Theme().Success
	}
	updateCaption := locales.PkgCached()
	if anyChecking {
		updateCaption = locales.Checking()
	}
	diskVal, diskCaption := "—", locales.PkgDiskNoSize()
	if hasDisk {
		diskVal, diskCaption = formatUsage(disk), locales.PkgDiskPackagesCaption()
	}
	headRow(c, func() {
		ui.Box(c).Size(48, 48).Radius(12).Background(pal.SuccessSoft).Center().Children(func() {
			ui.Icon(c, Icon("package")).Size(24, 24).TextColor(pal.MonoText)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, p.title()).FontSize(22).FontWeight(600)
			ui.Text(c, locales.PkgHeroSubtitle()).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
		})
	}, func() {
		dot := pal.TextFaint
		if len(managers) > 0 {
			dot = c.Theme().Success
		}
		stateCard(c, dot, locales.PkgInstalled(), installedVal, locales.PkgCountNote())
		stateCard(c, updateDot, locales.PkgUpdateSummary(), updatesVal, updateCaption)
		stateCard(c, pal.TextFaint, locales.PkgDiskCard(), diskVal, diskCaption)
	})
}

func (a *App) packagesDetectedCard(c *ui.Context, managers []inventory.PackageManager) {
	pal := Colors(c.Theme())
	listCard(c, locales.PkgManagersDetected(), strconv.Itoa(len(managers)), func() {
		if len(managers) == 0 {
			cardNote(c, locales.PkgAvailableNoneFound())
			return
		}
		for _, m := range managers {
			mgr := m
			updates := packageUpdateCount(mgr)
			var parts []string
			if mgr.Version != "" {
				parts = append(parts, mgr.Version)
			}
			loc := cmp.Or(mgr.Prefix, mgr.Path)
			if loc != "" {
				parts = append(parts, tilde(loc, a.Home))
			}
			if mgr.Unlisted {
				parts = append(parts, atLeast(len(mgr.Packages))+" "+locales.PkgCountNote())
			} else {
				parts = append(parts, locales.PkgCountPlural(len(mgr.Packages)))
			}
			listRowWithTile(c.Key("mgr:"+mgr.Path), pkgManagerIcon(mgr.ID), mgr.Name, false, strings.Join(parts, " · "), func() {
				if updates > 0 {
					chip(c, c.Theme().Warning, pal.WarningSoft, pal.WarningText, locales.PkgUpdatesChip(updates))
				}
				if ops.PackageManaged(mgr.ID) {
					aiTag(c, locales.PkgManagedBadge(), pal.InfoSoft, pal.InfoText)
				} else {
					aiTag(c, locales.PkgReadOnlyLabel(), pal.Muted, c.Theme().TextMuted)
				}
				ui.Button(c.Key("open:"+mgr.Path), locales.KubeClusterOpen()).Shrink(0).OnClick(func() {
					a.packageManager = mgr.Path
					a.router.Push(packagesManagerPath(mgr.Path))
				})
			})
		}
	})
}

func (a *App) packagesQuickViewsCard(c *ui.Context, m inventory.PackageManager) {
	t := c.Theme()
	pal := Colors(t)
	listCard(c, locales.PkgManagersSections(), m.Name, func() {
		ui.Row(c).Gap(8).Padding(12, 14).Wrap().Children(func() {
			for i, v := range packageViews() {
				view := i
				tile := ui.ButtonBase(c.Key("pkg-quick-"+strconv.Itoa(view))).Padding(8, 12).
					Radius(9).BorderWidth(1).BorderColor(pal.Line).Gap(7).AlignItems(ui.Center).
					Label(v.label).Transition(colorFade).OnClick(func() {
					a.packageManager = m.Path
					a.pkg.view = view
					a.router.Push(packagesManagerPath(m.Path))
				})
				if tile.Hovered() {
					tile.Background(pal.Subtle).BorderColor(pal.ControlBorder)
				} else {
					tile.Background(t.Background)
				}
				tile.Children(func() {
					ui.Icon(c, Icon(v.icon)).Size(15, 15).Shrink(0).TextColor(t.TextMuted)
					clipped(c, v.label).FontSize(12.5).FontWeight(500).MinWidth(0)
				})
			}
		})
	})
}

func (a *App) packagesAvailableCard(c *ui.Context, detected []inventory.PackageManager, available []pkgmgr.AvailableManager) {
	pal := Colors(c.Theme())
	listCard(c, locales.PkgAvailableTitle(), locales.PkgAvailableFor(pkgmgr.PlatformLabel()), func() {
		for _, avail := range available {
			item := avail
			var installed *inventory.PackageManager
			for i := range detected {
				if detected[i].ID == item.ID {
					installed = &detected[i]
					break
				}
			}
			listRowWithTile(c.Key("avail:"+item.ID), pkgManagerIcon(item.ID), item.Name, false, pkgAvailableDescription(item.ID), func() {
				if item.Recommended {
					chip(c, c.Theme().Success, pal.SuccessSoft, pal.SuccessText, locales.ColimaRecommended())
				}
				if installed != nil {
					chip(c, c.Theme().Success, pal.SuccessSoft, pal.SuccessText, locales.DockerInstalled())
					mgrPath := installed.Path
					ui.Button(c.Key("avail-open:"+item.ID), locales.KubeClusterOpen()).Shrink(0).OnClick(func() {
						a.packageManager = mgrPath
						a.router.Push(packagesManagerPath(mgrPath))
					})
					return
				}
				ui.Text(c, locales.DockerNotInstalled()).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
				if item.Installable && a.BuildPlan != nil {
					iconButton(c.Key("avail-install:"+item.ID), "download", locales.PkgSearchInstall()).OnClick(func() {
						a.installPackageManager(item.ID, detected)
					})
				} else if item.WebsiteURL != "" {
					iconButton(c.Key("avail-site:"+item.ID), "external-link", locales.PkgAvailableSite()).OnClick(func() {
						c.OpenURL(item.WebsiteURL)
					})
				}
			})
		}
		var terms []pkgmgr.AvailableManager
		for _, item := range available {
			if item.TermsURL != "" {
				terms = append(terms, item)
			}
		}
		if len(terms) > 0 {
			ui.Row(c).Gap(10).Padding(10, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				for _, item := range terms {
					ui.Link(c, locales.PkgAvailableTerms(item.Name), item.TermsURL).FontSize(11)
				}
			})
		}
	})
	toolNote(c, locales.PkgAvailableNote())
}

func (a *App) installPackageManager(id string, detected []inventory.PackageManager) {
	if a.BuildPlan == nil {
		return
	}
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		p, ok := ops.PackageManagerInstallPlan(id, detected, env)
		if !ok {
			return ops.Plan{}, ops.ErrNoFix
		}
		return p, nil
	}, a.proposeOrSay)
}

func (a *App) packagesServicesCard(c *ui.Context, services []inventory.Database) {
	listCard(c, locales.DatabaseServices(), strconv.Itoa(len(services)), func() {
		for _, server := range services {
			status, _ := databaseStatus(c, server.Status)
			listRow(c.Key("svc:"+server.Service), server.Service, false, status, func() {})
		}
		navListRow(c, "pkg-databases-tab", locales.DatabasesTitle(), locales.PkgMaintenanceDatabasesCaption(), locales.DatabasesTitle(), func() {
			a.router.Push("/tools/databases")
		})
	})
}

func (a *App) packagesPathCard(c *ui.Context, managers []inventory.PackageManager) {
	listCard(c, locales.PkgManagersDetected(), "PATH", func() {
		if len(managers) == 0 {
			cardNote(c, locales.PkgAvailableNoneFound())
			return
		}
		for _, m := range managers {
			mgr := m
			listRowWithTile(c.Key("path:"+mgr.Path), pkgManagerIcon(mgr.ID), mgr.Name, false, tilde(mgr.Path, a.Home), func() {
				a.pathActions(c, "pkg-path-"+mgr.ID, mgr.Path)
			})
		}
	})
}

func (a *App) packagesLandingAttentionCard(c *ui.Context, managers []inventory.PackageManager) {
	listCard(c, locales.AttentionTitle(), "", func() {
		any := false
		for _, m := range managers {
			mgr := m
			if updates := packageUpdateCount(mgr); updates > 0 {
				any = true
				navListRow(c, "attn-upd:"+mgr.Path, mgr.Name+" · "+locales.PkgAttentionUpdates(updates), locales.PkgAttentionUpdatesCaption(), locales.PkgReviewUpdates(), func() {
					a.packageManager = mgr.Path
					a.pkg.view = pkgViewUpdates
					a.router.Push(packagesManagerPath(mgr.Path))
				})
			}
			if orphans := pkgmgr.UnusedDependencies(mgr); len(orphans) > 0 {
				any = true
				if plan, ok := ops.PackageAutoremovePlan(mgr, orphans, a.reread(mgr)); ok {
					listRow(c.Key("attn-dep:"+mgr.Path), mgr.Name+" · "+locales.PkgAttentionUnused(len(orphans)), false, locales.PkgAttentionUnusedCaption(), func() {
						ui.Button(c.Key("attn-auto:"+mgr.Path), locales.PkgAutoremoveAction()).Shrink(0).OnClick(func() { a.Propose(plan) })
					})
				} else {
					navListRow(c, "attn-dep:"+mgr.Path, mgr.Name+" · "+locales.PkgAttentionUnused(len(orphans)), locales.PkgAttentionUnusedCaption(), locales.PkgReviewUpdates(), func() {
						a.packageManager = mgr.Path
						a.pkg.view = pkgViewDependencies
						a.pkg.depsFilter = 1
						a.router.Push(packagesManagerPath(mgr.Path))
					})
				}
			}
		}
		if !any {
			cardNote(c, locales.PkgNoAttention())
		}
	})
}

func (a *App) packagesMaintenanceCard(c *ui.Context, active inventory.PackageManager, hasActive, hasServices bool) {
	listCard(c, locales.PkgMaintenance(), "", func() {
		if hasActive {
			if p, ok := ops.PackageRefreshPlan(active, a.reread(active)); ok {
				listRow(c.Key("maint-refresh"), locales.PkgMaintenanceRefresh(), false, locales.PkgMaintenanceRefreshCaption(), func() {
					iconButton(c.Key("maint-refresh-btn"), "refresh-cw", locales.PkgRefresh()).OnClick(func() { a.Propose(p) })
				})
			}
		}
		navListRow(c, "maint-cleanup", locales.PkgMaintenanceCleanup(), locales.PkgMaintenanceCleanupCaption(), locales.PkgOpenCleanup(), func() {
			if idx := slices.Index(cleanupGroups, cleanup.Caches); idx >= 0 {
				a.cleanupTab = idx + 1
			} else {
				a.cleanupTab = 0
			}
			a.cleanupFilter = ""
			a.router.Push(cleanupPage)
		})
		if hasServices {
			navListRow(c, "maint-db", locales.PkgMaintenanceDatabases(), locales.PkgMaintenanceDatabasesCaption(), locales.DatabasesTitle(), func() {
				a.router.Push("/tools/databases")
			})
		}
	})
}

// packageServices are the database servers that run as the manager's services.
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

// packageDependents lists, by package name, the installed packages that depend
// on each (user-installed packages first, then alphabetically).
func packageDependents(m inventory.PackageManager) map[string][]string {
	isDep := make(map[string]bool, len(m.Packages))
	for _, pkg := range m.Packages {
		isDep[pkg.Name] = pkg.Dependency
	}
	out := map[string][]string{}
	for _, pkg := range m.Packages {
		for _, d := range pkg.Dependencies {
			if !slices.Contains(out[d], pkg.Name) {
				out[d] = append(out[d], pkg.Name)
			}
		}
	}
	for name, deps := range out {
		slices.SortFunc(deps, func(a, b string) int {
			if isDep[a] != isDep[b] {
				if !isDep[a] {
					return -1
				}
				return 1
			}
			return strings.Compare(a, b)
		})
		out[name] = deps
	}
	return out
}

// packageReason says who asked for pkg: the user, or other packages, naming
// the main dependents with the full list in tooltip when truncated.
func packageReason(m inventory.PackageManager, pkg inventory.SystemPackage, dependents []string) (string, string) {
	switch {
	case !pkg.Dependency:
		return locales.PkgAskedByYou(), ""
	case len(dependents) > 0:
		full := locales.PkgNeededByNames(strings.Join(dependents, ", "))
		const maxShown = 3
		if len(dependents) <= maxShown {
			return full, full
		}
		shown := strings.Join(dependents[:maxShown], ", ")
		return locales.PkgNeededByMore(shown, len(dependents)-maxShown), full
	case pkg.Orphan || m.ID == "brew": // brew lists every dependency, so none is none
		return locales.PkgNotNeeded(), ""
	}
	return locales.PkgDependency(), ""
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

// packageKey tells a package from the others of its manager.
func packageKey(pkg inventory.SystemPackage) string {
	return pkg.Name + "\x00" + pkg.Kind + "\x00" + pkg.Architecture + "\x00" + pkg.Scope
}

// choosable reports that m updates the packages one chooses: Arch's
// pacman updates the whole system only.
func choosable(m inventory.PackageManager) bool {
	return m.ID != "pacman" && ops.PackageManaged(m.ID)
}

// packagePlan is the plan that does action to pkgs of m, as ops.PackagePlan,
// checked by reading m again as the scan found it.
func (a *App) packagePlan(action ops.PackageAction, m inventory.PackageManager, pkgs []inventory.SystemPackage) (ops.Plan, bool) {
	if action == ops.PackageUpgrade && m.ID == "pacman" {
		pkgs = upgradable(m)
	}
	return ops.PackagePlan(action, m, pkgs, a.reread(m))
}

// reread lists m's packages again as the scan found m, for a plan's
// check; nil when the app cannot.
func (a *App) reread(m inventory.PackageManager) ops.Reread {
	r := a.RereadPackages
	if r == nil {
		return nil
	}
	base := m
	if st, ok := a.Snapshot.Tool("packages"); ok {
		for _, found := range st.PackageManagers {
			if found.Path == m.Path {
				base = found
			}
		}
	}
	return func(ctx context.Context) (inventory.PackageManager, error) { return r(ctx, base) }
}

// upgradable are m's packages with an update.
func upgradable(m inventory.PackageManager) []inventory.SystemPackage {
	var out []inventory.SystemPackage
	for _, pkg := range m.Packages {
		if pkg.Available != "" {
			out = append(out, pkg)
		}
	}
	return out
}

// removable reports that Hearth offers to remove pkg: not a package
// another pulled in, which others may need where the manager does not say.
func removable(pkg inventory.SystemPackage) bool {
	return !pkg.Dependency
}

// packageCan reports that pkg of m has an operation for its menu.
func (a *App) packageCan(m inventory.PackageManager, pkg inventory.SystemPackage) bool {
	if _, ok := a.packagePlan(ops.PackageUpgrade, m, []inventory.SystemPackage{pkg}); ok && pkg.Available != "" {
		return true
	}
	_, ok := a.packagePlan(ops.PackageUninstall, m, []inventory.SystemPackage{pkg})
	return ok && removable(pkg)
}

// packageItems adds pkg's operations to its menu: update it, remove it,
// through m.
func (a *App) packageItems(menu *ui.Menu, m inventory.PackageManager, pkg inventory.SystemPackage) {
	one := []inventory.SystemPackage{pkg}
	if upgrade, ok := a.packagePlan(ops.PackageUpgrade, m, one); ok && pkg.Available != "" {
		label := locales.PkgUpgradeTo(pkg.Available)
		if m.ID == "pacman" {
			label = locales.PkgUpgradeSystem()
		}
		if menu.Item(label).Chosen() {
			a.Propose(upgrade)
		}
	}
	if remove, ok := a.packagePlan(ops.PackageUninstall, m, one); ok && removable(pkg) && menu.Item(locales.RemoveEllipsis()).Chosen() {
		a.Propose(remove)
	}
}

// refreshIndexButton proposes refreshing m's local package index when supported.
func (a *App) refreshIndexButton(c *ui.Context, m inventory.PackageManager) {
	p, ok := ops.PackageRefreshPlan(m, a.reread(m))
	if !ok {
		return
	}
	ui.Button(c.Key("package-refresh"), locales.PkgRefresh()).Height(controlHeight).OnClick(func() { a.Propose(p) })
}
