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
	if pkg.Description != "" {
		detailSection(c, locales.PkgDetailDescription())
		ui.Text(c, pkg.Description).FontSize(12.5).Wrap().Selectable().Padding(2, 16, 10, 16)
	}
	if pkg.Dependency {
		ui.Column(c).Padding(4, 16, 8, 16).Children(func() {
			toolNote(c, locales.PkgDetailNotRemovable())
		})
	}
	var rows []detailFactRow
	if size, hasSize := a.Sizes.Size(pkg); hasSize {
		rows = append(rows, detailFactRow{key: "meta-size", label: locales.PkgDiskCard(), value: formatUsage(size)})
	}
	if m.ReasonsKnown {
		reason, _ := packageReason(m, pkg, dependents)
		rows = append(rows, detailFactRow{key: "meta-reason", label: locales.PkgStatus(), value: reason, plain: true})
	}
	if pkg.Tap != "" {
		tap := pkg.Tap
		rows = append(rows, detailFactRow{key: "meta-tap", label: locales.PkgDetailTap(), value: tap, tooltip: tap})
	}
	if pkg.License != "" {
		rows = append(rows, detailFactRow{key: "meta-license", label: locales.PkgDetailLicense(), value: pkg.License})
	}
	if pkg.KegOnly {
		rows = append(rows, detailFactRow{key: "meta-keg", label: "Keg-only", value: locales.PkgDetailKegOnly(), plain: true})
	}
	if pkg.Reference != "" {
		ref := strings.TrimSpace(pkg.Scope + " " + pkg.Reference)
		rows = append(rows, detailFactRow{key: "meta-ref", label: "Ref", value: ref, tooltip: ref})
	}
	if pkg.Path != "" {
		p := pkg.Path
		rows = append(rows, detailFactRow{
			key:     "meta-path",
			label:   locales.PkgDetailPath(),
			value:   tilde(p, a.Home),
			tooltip: p,
			end:     func() { a.pathActions(c, "pkg-detail-path", p) },
		})
	}
	if pkg.Homepage != "" {
		home := pkg.Homepage
		rows = append(rows, detailFactRow{
			key:     "meta-homepage",
			label:   locales.PkgDetailHomepage(),
			value:   home,
			tooltip: home,
			end: func() {
				iconButton(c.Key("meta-open-home"), "external-link", locales.PkgDetailHomepage()).OnClick(func() { c.OpenURL(home) })
			},
		})
	}
	if len(rows) > 0 {
		detailSection(c, locales.PkgDetailMetadata())
		detailFactsCard(c, rows)
	}
	if len(pkg.Artifacts) > 0 {
		detailSection(c, locales.PkgDetailArtifacts())
		ui.Column(c).Margin(0, 16, 6, 16).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
			for i, art := range pkg.Artifacts {
				artPath := art
				row := ui.Row(c.Key("art-"+strconv.Itoa(i))).Padding(7, 12).Gap(10).AlignItems(ui.Center)
				if i < len(pkg.Artifacts)-1 {
					row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
				}
				row.Children(func() {
					ui.Icon(c, Icon("file-text")).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
					ui.Text(c, tilde(artPath, a.Home)).Font(FontMono).FontSize(11.5).Grow(1).MinWidth(0).Wrap().Selectable().Tooltip(artPath)
					a.pathActions(c, "pkg-art-"+strconv.Itoa(i), artPath)
				})
			}
		})
	}
	if tools := packageTools(a.Snapshot, m.ID, pkg); len(tools) > 0 {
		detailSection(c, locales.PkgDetailRelatedTools())
		ui.Column(c).Margin(0, 16, 6, 16).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
			for i, tool := range tools {
				row := ui.Row(c.Key("tool-"+strconv.Itoa(i))).Padding(7, 12).Gap(10).AlignItems(ui.Center)
				if i < len(tools)-1 {
					row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
				}
				row.Children(func() {
					ui.Icon(c, Icon("terminal")).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
					ui.Text(c, tool).FontSize(12).Grow(1).MinWidth(0).Wrap()
				})
			}
		})
	}
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
