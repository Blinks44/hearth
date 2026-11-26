package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/pkgmgr"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) packagesInstallView(c *ui.Context, m inventory.PackageManager) {
	s := &a.pkg
	t := c.Theme()
	pal := Colors(t)

	if !pkgmgr.CanSearch(m.ID) || !ops.PackageManaged(m.ID) {
		toolNote(c, locales.PkgSearchUnsupported(m.Name))
		return
	}

	kindArgFor := func(kind int) string {
		if m.ID != "brew" {
			return ""
		}
		switch kind {
		case 1:
			return "formula"
		case 2:
			return "cask"
		default:
			return ""
		}
	}

	runSearch := func() {
		q := strings.TrimSpace(s.searchQuery)
		if q == "" || a.SearchPackages == nil {
			return
		}
		kindArg := kindArgFor(s.searchKind)
		a.cancelPackageSearch()
		seq := s.searchSeq
		s.searchLoading = true
		s.searchErr = ""
		s.searchLastQuery = q
		s.searchLastKind = kindArg
		s.searchLastPath = m.Path
		s.searchStop = a.SearchPackages(m, q, kindArg, func(results []pkgmgr.SearchResult, err error) {
			if s.searchSeq != seq {
				return
			}
			s.searchLoading = false
			s.searchStop = nil
			if err != nil {
				s.searchErr = err.Error()
				s.searchResults = nil
				return
			}
			s.searchErr = ""
			s.searchResults = results
		})
	}

	toolbar := ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0)
	if narrow(c) {
		toolbar.Wrap()
	}
	toolbar.Children(func() {
		ui.TextInput(c.Key("pkg-catalog-query"), &s.searchQuery).
			Bind(&a.filterField).
			Placeholder(locales.PkgSearchPlaceholder(m.Name)).
			Height(controlHeight).Grow(1).MinWidth(180).
			OnSubmit(runSearch)
		if m.ID == "brew" {
			prevKind := s.searchKind
			segments(c, "pkg-catalog-kind", &s.searchKind, []segment{
				{locales.TabAll(), ""},
				{locales.PkgKindFormulae(), ""},
				{locales.PkgKindCasks(), ""},
			})
			if s.searchKind != prevKind && strings.TrimSpace(s.searchQuery) != "" {
				runSearch()
			}
		}
		ui.PrimaryButton(c.Key("pkg-catalog-search"), locales.PkgSearchButton()).
			Height(controlHeight).
			Disabled(s.searchLoading || strings.TrimSpace(s.searchQuery) == "" || a.SearchPackages == nil).
			OnClick(runSearch)
	})

	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Gap(12).Children(func() {
			switch {
			case s.searchLoading:
				tableSkeleton(c, locales.PkgSearchSearching(m.Name))
			case s.searchErr != "":
				warningRow(c, s.searchErr)
			case s.searchLastQuery == "" || s.searchLastPath != m.Path:
				ui.Row(c).Gap(10).Padding(14, 16).Radius(12).AlignItems(ui.Center).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
					ui.Icon(c, Icon("search")).Size(16, 16).Shrink(0).TextColor(t.TextMuted)
					ui.Text(c, locales.PkgSearchIdle()).FontSize(12.5).TextColor(t.TextMuted).Grow(1).MinWidth(0).Wrap()
				})
			case len(s.searchResults) == 0:
				listCard(c, m.Name, "0", func() {
					cardNote(c, locales.PkgSearchEmpty(s.searchLastQuery))
				})
			default:
				a.packagesSearchResultsTable(c, m, s.searchResults)
			}
			toolNote(c, locales.PkgSearchFooter())
		})
	})
}

func (a *App) packagesSearchResultsTable(c *ui.Context, m inventory.PackageManager, results []pkgmgr.SearchResult) {
	pal := Colors(c.Theme())
	installed := make(map[string]inventory.SystemPackage, len(m.Packages)*2)
	for _, p := range m.Packages {
		installed[strings.ToLower(p.Name)+"\x00"+p.Kind] = p
		installed[strings.ToLower(p.Name)+"\x00"] = p
		if p.Reference != "" {
			installed[strings.ToLower(p.Reference)+"\x00"+p.Kind] = p
			installed[strings.ToLower(p.Reference)+"\x00"] = p
		}
	}
	findInstalled := func(res pkgmgr.SearchResult) (inventory.SystemPackage, bool) {
		if res.Kind != "" {
			if p, ok := installed[strings.ToLower(res.Name)+"\x00"+res.Kind]; ok {
				return p, true
			}
		}
		if res.Kind == "" || m.ID != "brew" {
			if p, ok := installed[strings.ToLower(res.Name)+"\x00"]; ok {
				return p, true
			}
		}
		return inventory.SystemPackage{}, false
	}
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).AlignItems(ui.Center).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.PkgPackage()).Width(nameWidth(c)).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.PkgKind()).Width(64).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.PkgSearchColDesc()).Grow(1).MinWidth(0).FontSize(11.5).TextColor(pal.TextFaint)
		})
		for i, item := range results {
			res := item
			installedPkg, isInst := findInstalled(res)
			var installPlan ops.Plan
			canInstall := false
			if !isInst {
				pkg := inventory.SystemPackage{
					Name:      res.Name,
					Version:   res.Version,
					Kind:      res.Kind,
					Reference: res.Name,
					Origin:    res.Origin,
				}
				installPlan, canInstall = a.packagePlan(ops.PackageInstall, m, []inventory.SystemPackage{pkg})
			}
			line := ui.Row(c.Key("pkg-search-"+strconv.Itoa(i)+"-"+res.Name+"-"+res.Kind)).Gap(12).Padding(0, 16).MinHeight(44).
				AlignItems(ui.Stretch).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
			line.Children(func() {
				row := ui.ButtonBase(c.Key("row")).Row().Grow(1).MinWidth(0).Padding(10, 0).Gap(12).
					AlignItems(ui.Center).Justify(ui.Start).Label(res.Name)
				switch {
				case isInst:
					row.OnClick(func() {
						a.pkg.view = pkgViewPackages
						a.packageTab = 0
						a.pkg.detailKey = packageKey(installedPkg)
						a.pkg.detailTab = 0
						a.packageFilter = ""
					})
				case canInstall:
					row.OnClick(func() { a.Propose(installPlan) })
				}
				bg := c.Theme().Background.Alpha(0)
				if (isInst || canInstall) && (line.Hovered() || row.Hovered()) {
					bg = pal.Subtle
				}
				line.Background(line.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
				row.Children(func() {
					ui.Column(c).Width(nameWidth(c)).Shrink(0).Gap(2).Children(func() {
						clipped(c, res.Name).Font(FontMono).FontSize(12)
						ver := res.Version
						if res.Origin != "" {
							if ver != "" {
								ver += " · " + res.Origin
							} else {
								ver = res.Origin
							}
						}
						if ver != "" {
							clipped(c, ver).FontSize(10.5).TextColor(pal.TextFaint)
						}
					})
					ui.Text(c, res.Kind).Width(64).Shrink(0).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						if res.Description != "" {
							clipped(c, res.Description).FontSize(11.5).TextColor(c.Theme().TextMuted)
						}
					})
					if isInst {
						chip(c, c.Theme().Success, pal.SuccessSoft, pal.SuccessText, locales.PkgSearchInstalled())
					}
				})
				if !isInst && canInstall {
					ui.Row(c).AlignItems(ui.Center).Shrink(0).Children(func() {
						ui.Button(c.Key("install"), locales.PkgSearchInstall()).Height(controlHeight).OnClick(func() {
							a.Propose(installPlan)
						})
					})
				}
			})
		}
	})
}
