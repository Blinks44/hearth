package ui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) packageDetailView(c *ui.Context, m inventory.PackageManager, pkg inventory.SystemPackage, dependents []string) {
	t := c.Theme()
	pal := Colors(t)
	if a.pkg.detailTab < 0 || a.pkg.detailTab > 2 {
		a.pkg.detailTab = 0
	}
	detailFrame(c, a.pkg.detailTab, false, func() {
		ui.Column(c).Gap(10).Padding(14, 16, 12, 16).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
						ui.Text(c, pkg.Name).Font(FontMono).FontSize(15).FontWeight(600)
						if pkg.Kind != "" {
							aiTag(c, pkg.Kind, pal.Muted, pal.MonoText)
						}
						if pkg.KegOnly {
							aiTag(c, "keg-only", pal.WarningSoft, pal.WarningText)
						}
					})
					verLine := pkg.Version
					if pkg.Available != "" {
						verLine += " → " + pkg.Available
					}
					if verLine != "" {
						ui.Text(c, verLine).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint)
					}
				})
				iconButton(c.Key("pkg-detail-close"), "x", locales.PkgDetailClose()).OnClick(func() {
					a.pkg.detailKey = ""
				})
			})
			one := []inventory.SystemPackage{pkg}
			upgrade, canUpgrade := a.packagePlan(ops.PackageUpgrade, m, one)
			canUpgrade = canUpgrade && pkg.Available != ""
			remove, canRemove := a.packagePlan(ops.PackageUninstall, m, one)
			canRemove = canRemove && removable(pkg)
			hasFolder := pkg.Path != "" && a.Shell != nil
			hasSite := pkg.Homepage != ""
			if canUpgrade || canRemove || hasFolder || hasSite {
				ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
					if canUpgrade {
						label := locales.PkgDetailUpgrade(pkg.Available)
						if m.ID == "pacman" {
							label = locales.PkgUpgradeSystem()
						}
						ui.PrimaryButton(c.Key("pkg-detail-upgrade"), label).Height(controlHeight).OnClick(func() { a.Propose(upgrade) })
					}
					if canRemove {
						ui.Button(c.Key("pkg-detail-remove"), locales.RemoveEllipsis()).Height(controlHeight).OnClick(func() {
							a.Propose(remove)
						})
					}
					if hasFolder {
						iconButton(c.Key("pkg-detail-folder"), "folder-open", locales.PkgDetailOpenFolder()).OnClick(func() {
							a.Shell.ShowItemInFolder(pkg.Path)
						})
					}
					if hasSite {
						iconButton(c.Key("pkg-detail-site"), "external-link", locales.PkgDetailHomepage()).OnClick(func() {
							c.OpenURL(pkg.Homepage)
						})
					}
				})
			}
			segments(c, "pkg-detail-tabs", &a.pkg.detailTab, []segment{
				{locales.PkgDetailOverview(), ""},
				{locales.PkgDetailDeps(), strconv.Itoa(len(pkg.Dependencies))},
				{locales.PkgDetailUsedBy(), strconv.Itoa(len(dependents))},
			})
		})
	}, func() {
		switch a.pkg.detailTab {
		case 0:
			a.packageDetailOverview(c, m, pkg, dependents)
		case 1:
			a.packageDetailRelationList(c, m, pkg.Dependencies, locales.PkgDetailNoDeps())
		case 2:
			a.packageDetailRelationList(c, m, dependents, locales.PkgDetailNoDependents())
		}
	})
}

func (a *App) packageDetailOverview(c *ui.Context, m inventory.PackageManager, pkg inventory.SystemPackage, dependents []string) {
	pal := Colors(c.Theme())
	ui.Column(c).Gap(12).Padding(12, 16).Children(func() {
		if pkg.Description != "" {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, locales.PkgDetailDescription()).FontSize(11).FontWeight(500).TextColor(pal.TextFaint)
				ui.Text(c, pkg.Description).FontSize(12.5).Wrap()
			})
		}
		if pkg.Dependency {
			toolNote(c, locales.PkgDetailNotRemovable())
		}
		size, hasSize := a.Sizes.Size(pkg)
		hasMeta := hasSize || m.ReasonsKnown || pkg.Tap != "" || pkg.License != "" || pkg.KegOnly || pkg.Reference != "" || pkg.Path != "" || pkg.Homepage != ""
		if hasMeta {
			listCard(c, locales.PkgDetailMetadata(), "", func() {
				if hasSize {
					listRow(c.Key("meta-size"), locales.PkgDiskCard(), false, formatUsage(size), func() {})
				}
				if m.ReasonsKnown {
					reason, _ := packageReason(m, pkg, dependents)
					listRow(c.Key("meta-reason"), locales.PkgStatus(), false, reason, func() {})
				}
				if pkg.Tap != "" {
					listRow(c.Key("meta-tap"), locales.PkgDetailTap(), false, pkg.Tap, func() {})
				}
				if pkg.License != "" {
					listRow(c.Key("meta-license"), locales.PkgDetailLicense(), false, pkg.License, func() {})
				}
				if pkg.KegOnly {
					listRow(c.Key("meta-keg"), "Keg-only", false, locales.PkgDetailKegOnly(), func() {})
				}
				if pkg.Reference != "" {
					listRow(c.Key("meta-ref"), "Ref", false, strings.TrimSpace(pkg.Scope+" "+pkg.Reference), func() {})
				}
				if pkg.Path != "" {
					listRow(c.Key("meta-path"), locales.PkgDetailPath(), false, tilde(pkg.Path, a.Home), func() {
						a.pathActions(c, "pkg-detail-path", pkg.Path)
					})
				}
				if pkg.Homepage != "" {
					listRow(c.Key("meta-homepage"), locales.PkgDetailHomepage(), false, pkg.Homepage, func() {
						iconButton(c.Key("meta-open-home"), "external-link", locales.PkgDetailHomepage()).OnClick(func() { c.OpenURL(pkg.Homepage) })
					})
				}
			})
		}
		if len(pkg.Artifacts) > 0 {
			listCard(c, locales.PkgDetailArtifacts(), strconv.Itoa(len(pkg.Artifacts)), func() {
				for i, art := range pkg.Artifacts {
					artPath := art
					listRow(c.Key("art-"+strconv.Itoa(i)), tilde(artPath, a.Home), true, "", func() {
						a.pathActions(c, "pkg-art-"+strconv.Itoa(i), artPath)
					})
				}
			})
		}
		if tools := packageTools(a.Snapshot, m.ID, pkg); len(tools) > 0 {
			listCard(c, locales.PkgDetailRelatedTools(), strconv.Itoa(len(tools)), func() {
				for _, tool := range tools {
					listRow(c.Key("tool-"+tool), tool, false, "", func() {})
				}
			})
		}
	})
}

func (a *App) packageDetailRelationList(c *ui.Context, m inventory.PackageManager, names []string, empty string) {
	pal := Colors(c.Theme())
	if len(names) == 0 {
		cardNote(c, empty)
		return
	}
	byName := make(map[string]inventory.SystemPackage, len(m.Packages))
	for _, p := range m.Packages {
		if _, exists := byName[p.Name]; !exists {
			byName[p.Name] = p
		}
	}
	ui.Column(c).Padding(8, 12).Gap(2).Children(func() {
		for _, name := range names {
			target, found := byName[name]
			if found {
				targetKey := packageKey(target)
				row := ui.ButtonBase(c.Key("rel-"+name)).Padding(8, 10).Radius(8).AlignItems(ui.Center).Gap(10).Label(name).OnClick(func() {
					a.pkg.detailKey = targetKey
					a.pkg.detailTab = 0
				})
				if row.Hovered() {
					row.Background(pal.Subtle)
				}
				row.Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						clipped(c, target.Name).Font(FontMono).FontSize(12.5)
						if target.Version != "" {
							clipped(c, target.Version).FontSize(10.5).TextColor(pal.TextFaint)
						}
					})
					if target.Kind != "" {
						aiTag(c, target.Kind, pal.Muted, pal.MonoText)
					}
					ui.Icon(c, Icon("chevron-right")).Size(13, 13).TextColor(pal.TextFaint)
				})
			} else {
				ui.Row(c.Key("rel-ext-"+name)).Padding(8, 10).AlignItems(ui.Center).Children(func() {
					clipped(c, name).Font(FontMono).FontSize(12.5).TextColor(c.Theme().TextMuted)
				})
			}
		}
	})
}
