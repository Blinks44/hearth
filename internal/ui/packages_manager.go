package ui

import (
	"cmp"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/pkgmgr"
	"github.com/Blinks44/hearth/locales"
)

// The views inside a package manager, in their kind-chips order.
const (
	pkgViewOverview = iota
	pkgViewPackages
	pkgViewUpdates
	pkgViewDependencies
	pkgViewInstall
)

type packageView struct{ label, icon string }

func packageViews() []packageView {
	return []packageView{
		{locales.PkgViewOverview(), "layout-dashboard"},
		{locales.PkgViewPackages(), "package"},
		{locales.PkgViewUpdates(), "circle-arrow-up"},
		{locales.PkgViewDependencies(), "git-branch"},
		{locales.PkgViewInstall(), "download"},
	}
}

type packageManagerState struct {
	view        int
	managerOpen bool
	viewsOpen   bool
	detailKey   string
	detailTab   int
	depsFilter  int // 0 = all dependencies, 1 = unused
	activePath  string

	// Stage 2 explicit catalog search state.
	searchQuery     string
	searchKind      int // 0 = all, 1 = formula, 2 = cask (Homebrew)
	searchLastQuery string
	searchLastKind  string
	searchLastPath  string
	searchLoading   bool
	searchErr       string
	searchResults   []pkgmgr.SearchResult
	searchSeq       int
	searchStop      func()
}

func (a *App) cancelPackageSearch() {
	if a.pkg.searchStop != nil {
		a.pkg.searchStop()
		a.pkg.searchStop = nil
	}
	a.pkg.searchLoading = false
	a.pkg.searchSeq++
}

// syncPackages cancels any in-flight catalog search when leaving the manager
// view or switching to a different manager.
func (a *App) syncPackages() {
	if a.router.Path() != packagesManagerPage {
		if a.pkg.searchStop != nil {
			a.cancelPackageSearch()
		}
		a.pkg.managerOpen = false
		a.pkg.viewsOpen = false
		return
	}
	cur := a.router.Query("path")
	if cur != a.pkg.activePath {
		a.cancelPackageSearch()
		a.pkg.activePath = cur
		a.pkg.detailKey = ""
		a.pkg.detailTab = 0
		a.pkg.searchQuery = ""
		a.pkg.searchKind = 0
		a.pkg.searchErr = ""
		a.pkg.searchResults = nil
		a.pkg.searchLastQuery = ""
		a.pkg.searchLastKind = ""
		a.pkg.searchLastPath = ""
		a.packageChosen = nil
		if cur != "" {
			a.packageManager = cur
		}
	}
}

func (a *App) togglePackageViews() {
	a.pkg.viewsOpen = !a.pkg.viewsOpen
	if a.pkg.viewsOpen {
		a.pkg.managerOpen = false
	}
}

func (a *App) packagesManagerView(c *ui.Context, path string) {
	s := &a.pkg
	ui.Column(c).Fill().Children(func() {
		m, ok := a.packageManagerByPath(path)
		if !ok {
			if fallback, has := a.activePackageManager(); has && path == "" {
				m, ok = fallback, true
			}
		}
		if !ok {
			a.headerWith(c, locales.PackageManagersTitle(), locales.SectionSystem(), headerExtras{
				backLabel: locales.PkgBackToAll(),
				back:      func() { a.router.Push(packagesPage) },
			})
			ui.Column(c).Grow(1).Padding(22, 28).Children(func() {
				toolNote(c, locales.PkgAvailableNoneFound())
			})
			return
		}
		a.packagesManagerHeader(c, m)
		updates := packageUpdateCount(m)
		orphans := pkgmgr.UnusedDependencies(m)
		ui.Column(c).Grow(1).MinHeight(0).Gap(14).Padding(20, 28, 24, 28).Children(func() {
			ui.Row(c).Gap(6).Wrap().Shrink(0).Children(func() {
				for i, v := range packageViews() {
					badge := 0
					switch i {
					case pkgViewUpdates:
						badge = updates
					case pkgViewDependencies:
						badge = len(orphans)
					}
					kindChip(c, &s.view, i, v.icon, v.label, badge, func() {
						s.detailKey = ""
						a.packageChosen = nil
					})
				}
			})
			switch s.view {
			case pkgViewOverview:
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Gap(14).Children(func() {
						a.packagesOverviewView(c, m, orphans)
					})
				})
			case pkgViewPackages:
				a.packagesListView(c, m, false)
			case pkgViewUpdates:
				a.packagesListView(c, m, true)
			case pkgViewDependencies:
				a.packagesDepsView(c, m, orphans)
			case pkgViewInstall:
				a.packagesInstallView(c, m)
			}
		})
	})
}

func (a *App) packagesManagerHeader(c *ui.Context, m inventory.PackageManager) {
	s := &a.pkg
	t := c.Theme()
	pal := Colors(t)
	right := float32(20)
	if bar := c.TitleBar(); bar.Right > 0 {
		right = bar.Right + 12
	}
	isNarrow := narrow(c)
	crumbWidth := float32(0)
	if isNarrow {
		crumbWidth = 160
	}
	ui.Row(c).Height(56).Shrink(0).AlignItems(ui.Center).Gap(6).Padding(0, right, 0, 24).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.Divider).DragWindow().Children(func() {
		headerBackButton(c, locales.PkgBackToAll(), func() { a.router.Push(packagesPage) })
		anchor := pathButton(c, "pkg-mgr-switch", pkgManagerIcon(m.ID), m.Name, crumbWidth)
		if anchor.Clicked() {
			s.managerOpen = !s.managerOpen
			s.viewsOpen = false
		}
		ui.Popover(c, anchor, &s.managerOpen, func() { a.packagesManagerPopover(c, m.Path) })
		if !isNarrow {
			if ops.PackageManaged(m.ID) {
				aiTag(c, locales.PkgManagedBadge(), pal.InfoSoft, pal.InfoText).Shrink(0)
			} else {
				aiTag(c, locales.PkgReadOnlyLabel(), pal.Muted, t.TextMuted).Shrink(0)
			}
		}
		pathSlash(c)
		v := packageViews()[s.view]
		viewBtn := pathButton(c, "pkg-view-switch", "lucide/"+v.icon, v.label, crumbWidth)
		viewBtn.OnClick(a.togglePackageViews)
		ui.Popover(c, viewBtn, &s.viewsOpen, func() { a.packagesViewPicker(c) })
		ui.Box(c).Grow(1).Height(1)
		if !isNarrow {
			a.packageFreshness(c, m)
		}
		a.refreshIndexButton(c, m)
	})
}

func (a *App) packageFreshness(c *ui.Context, m inventory.PackageManager) {
	t := c.Theme()
	dot, text := Colors(t).TextFaint, locales.PkgFreshnessLocal()
	switch {
	case a.PackageChecks[m.Path]:
		dot, text = Colors(t).Info, locales.Checking()
	case m.UpdateError != "":
		dot, text = t.Warning, locales.PkgPartialNote()
	case !m.UpdatesFetched.IsZero():
		dot, text = t.Success, locales.VersionsAsOf(m.UpdatesFetched)
	case m.UpdatesKnown:
		dot, text = t.Success, locales.PkgCached()
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, text).FontSize(11.5).TextColor(t.TextMuted).SingleLine().MaxWidth(220)
	})
}

func (a *App) packagesManagerPopover(c *ui.Context, currentPath string) {
	t := c.Theme()
	pal := Colors(t)
	managers := a.packageManagersWithUpdates()
	ui.Column(c).Width(380).Gap(2).Padding(6).Children(func() {
		for _, m := range managers {
			mgr := m
			row := ui.ButtonBase(c.Key("pkg-switch-"+mgr.Path)).Padding(7, 10).Radius(7).AlignItems(ui.Center).Gap(10).Label(mgr.Name)
			if mgr.Path == currentPath {
				row.Background(pal.ActiveRow)
			} else if row.Hovered() {
				row.Background(pal.Muted)
			}
			row.OnClick(func() {
				a.pkg.managerOpen = false
				if mgr.Path != currentPath {
					a.packageManager = mgr.Path
					a.router.Replace(packagesManagerPath(mgr.Path))
				}
			}).Children(func() {
				entityTile(c, pkgManagerIcon(mgr.ID), 28)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						clipped(c, mgr.Name).FontSize(13).FontWeight(500)
						if updates := packageUpdateCount(mgr); updates > 0 {
							aiTag(c, locales.PkgUpdatesChip(updates), pal.WarningSoft, pal.WarningText)
						}
					})
					loc := cmp.Or(mgr.Prefix, mgr.Path)
					caption := tilde(loc, a.Home)
					if mgr.Version != "" {
						caption = mgr.Version + " · " + caption
					}
					clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint)
				})
			})
		}
		ui.Box(c).Height(1).Background(pal.Line).Margin(4, 0)
		ui.Row(c).Gap(8).Padding(4, 10).Children(func() {
			ui.Icon(c, Icon("info")).Size(14, 14).TextColor(pal.Info)
			ui.Text(c, locales.PkgPickNote()).FontSize(11.5).TextColor(t.TextMuted).Wrap().Grow(1)
		})
		ui.Row(c).Padding(4, 10, 6, 10).Children(func() {
			ui.Box(c).Grow(1)
			ui.Button(c.Key("pkg-all-managers"), locales.PkgAllManagers()+" ›").OnClick(func() {
				a.pkg.managerOpen = false
				a.router.Push(packagesPage)
			})
		})
	})
}

func (a *App) packagesViewPicker(c *ui.Context) {
	s := &a.pkg
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Width(250).Gap(2).Padding(6).Children(func() {
		ui.Text(c, locales.PkgViews()).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).Padding(6, 10, 4, 10)
		for i, v := range packageViews() {
			view := i
			row := ui.ButtonBase(c.Key("pkg-view-"+strconv.Itoa(view))).Padding(7, 10).Radius(7).AlignItems(ui.Center).Gap(10).Label(v.label)
			if view == s.view {
				row.Background(pal.ActiveRow)
			} else if row.Hovered() {
				row.Background(pal.Muted)
			}
			row.OnClick(func() {
				s.view, s.viewsOpen, s.detailKey = view, false, ""
				a.packageChosen = nil
			}).Children(func() {
				ui.Icon(c, Icon(v.icon)).Size(14, 14).TextColor(t.TextMuted)
				ui.Text(c, v.label).FontSize(13).Grow(1)
			})
		}
	})
}
