package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

type setupCatData struct {
	info  setupCatInfo
	all   []catalog.Entry
	avail []catalog.Entry
	have  []catalog.Entry
	total int
}

// setupChoiceView is step 1: a single-column categorized catalog on the left
// with category filter chips, beside a compact selection queue, base status,
// and computer set card on the right.
func (a *App) setupChoiceView(c *ui.Context) {
	in := a.setupInput()
	wizard := a.setupWizard()
	query := strings.ToLower(strings.TrimSpace(a.setup.query))
	if a.setup.setNotice != "" {
		toolNote(c, a.setup.setNotice)
	}
	offers := a.setupAllOffers(in)
	var cats []setupCatData
	totalVisible := 0
	for _, info := range setupCategories {
		cd := setupCatData{info: info}
		for _, e := range offers {
			if setupCategory(e) != info.cat {
				continue
			}
			cd.all = append(cd.all, e)
			h := a.setupHave(e.ID)
			if h {
				cd.total++
			}
			if !wizard && a.setup.filter != 0 && (a.setup.filter == 2) != h {
				continue
			}
			if query != "" && !setupMatches(e, query) {
				continue
			}
			if h {
				cd.have = append(cd.have, e)
			} else {
				cd.avail = append(cd.avail, e)
			}
		}
		totalVisible += len(cd.avail) + len(cd.have)
		cats = append(cats, cd)
	}
	var baseRows []catalog.Entry
	for _, e := range a.setupOffers(in, catalog.Base) {
		h := a.setupHave(e.ID)
		if !wizard && a.setup.filter != 0 && (a.setup.filter == 2) != h {
			continue
		}
		if query != "" && !setupMatches(e, query) {
			continue
		}
		baseRows = append(baseRows, e)
	}
	if totalVisible == 0 && len(baseRows) == 0 {
		if query != "" {
			cardNote(c, locales.NothingMatches())
		} else {
			cardNote(c, locales.InstallNothingHere())
		}
		return
	}
	if totalVisible > 0 {
		a.setupCategoryBar(c, cats)
	}
	columns(c, 12, func() {
		shown := 0
		for _, cd := range cats {
			if a.setup.cat != catAll && cd.info.cat != a.setup.cat {
				continue
			}
			if len(cd.avail) == 0 && len(cd.have) == 0 {
				continue
			}
			shown++
			a.setupCategoryCard(c, in, cd, wizard, query)
		}
		if shown == 0 {
			if query != "" {
				cardNote(c, locales.NothingMatches())
			} else {
				cardNote(c, locales.InstallNothingHere())
			}
		}
	}, func() {
		a.setupQueueCard(c, in, wizard)
		if len(baseRows) > 0 {
			a.setupBaseCard(c, in, baseRows, wizard)
		}
		if !wizard && (a.SaveSet != nil || a.OpenSet != nil) {
			a.setupSetCard(c)
		}
	})
}

// setupCategoryBar draws pill filter chips for the catalog's categories.
func (a *App) setupCategoryBar(c *ui.Context, cats []setupCatData) {
	t := c.Theme()
	pal := Colors(t)
	chipFor := func(cat installCat, icon, label string, count int) {
		on := a.setup.cat == cat
		b := ui.ButtonBase(c.Key("install-cat:"+strconv.Itoa(int(cat)))).Gap(6).Padding(5, 10, 5, 9).Radius(13).
			AlignItems(ui.Center).Label(label).BorderWidth(1)
		text, faint := t.Text, pal.TextFaint
		switch {
		case on:
			b.Background(t.Text).BorderColor(t.Text)
			text, faint = t.Background, t.Background.Alpha(0.6)
		case b.Hovered():
			b.Background(pal.Subtle).BorderColor(pal.ControlBorder)
		default:
			b.Background(t.Background).BorderColor(pal.ControlBorder)
		}
		b.Children(func() {
			if icon != "" {
				iconOrLogoOn(c, icon, 13, text, t.Dark != on)
			}
			ui.Text(c, label).FontSize(12).TextColor(text).SingleLine()
			if count > 0 {
				ui.Text(c, strconv.Itoa(count)).FontSize(11).TextColor(faint).SingleLine()
			}
		})
		if b.Clicked() {
			a.setup.cat = cat
		}
	}
	ui.Row(c).Gap(6).Wrap().Children(func() {
		chipFor(catAll, "", locales.TabAll(), 0)
		for _, cd := range cats {
			n := len(cd.avail) + len(cd.have)
			if n == 0 && a.setup.cat != cd.info.cat {
				continue
			}
			chipFor(cd.info.cat, cd.info.icon, cd.info.chip(), n)
		}
	})
}

func (a *App) setupCategoryCard(c *ui.Context, in ops.Setup, cd setupCatData, wizard bool, query string) {
	aside := locales.SetupHaveOf(cd.total, len(cd.all))
	cat := cd.info.cat
	// Collapse only when viewing all categories without search and more than
	// a couple of available tools would sit behind "N more".
	collapse := query == "" && a.setup.cat == catAll && !a.setup.more[cat] && (wizard || a.setup.filter == 0) && len(cd.avail) > setupFeatured+2
	setupGroupCard(c, cd.info.icon, cd.info.title(), aside, func() {
		var more []string
		for i, e := range cd.avail {
			_, system := a.setupSystemCopy(e.ID)
			if !collapse || i < setupFeatured || system || a.setupChosen(e.ID) {
				a.setupRow(c, in, e)
			} else {
				more = append(more, setupTitle(e.ID))
			}
		}
		if len(more) > 0 {
			pal := Colors(c.Theme())
			b := ui.ButtonBase(c.Key("setup-more:"+strconv.Itoa(int(cat)))).FillWidth().Gap(12).Padding(9, 14).
				AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).
				Label(locales.SetupShowMore()).OnClick(func() {
				if a.setup.more == nil {
					a.setup.more = map[installCat]bool{}
				}
				a.setup.more[cat] = true
			})
			if b.Hovered() {
				b.Background(pal.Subtle)
			}
			b.Children(func() {
				ui.Box(c).Size(28, 28).Shrink(0).Radius(7).Center().Background(pal.Subtle).
					BorderWidth(1).BorderColor(pal.Line).Children(func() {
					ui.Icon(c, Icon("plus")).Size(13, 13).TextColor(pal.TextFaint)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, locales.SetupMore(len(more))).FontSize(13).SingleLine()
					clipped(c, strings.Join(more, ", ")).FontSize(11.5).TextColor(pal.TextFaint)
				})
				ui.Text(c, locales.SetupShowMore()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			})
		}
		if len(cd.have) == 0 {
			return
		}
		// On the "All" tab of the catalog without search, collapse installed
		// tools into one calm summary row so green pills do not interleave
		// with switches.
		compactHave := !wizard && a.setup.filter == 0 && query == ""
		if compactHave && !a.setup.showHave[cat] {
			a.setupInstalledSummaryRow(c, cat, cd.have)
			return
		}
		if compactHave {
			a.setupInstalledExpandedHeader(c, cat, len(cd.have))
		}
		for _, e := range cd.have {
			a.setupRow(c, in, e)
		}
	})
}

func (a *App) setupInstalledSummaryRow(c *ui.Context, cat installCat, have []catalog.Entry) {
	t := c.Theme()
	pal := Colors(t)
	var names []string
	limit := min(len(have), 3)
	for _, e := range have[:limit] {
		label := setupTitle(e.ID)
		if found, r := a.setupResult(e.ID); r != checkMissing && found.Version != "" {
			label += " " + found.Version
		}
		names = append(names, label)
	}
	summary := strings.Join(names, ", ")
	if len(have) > limit {
		summary += " +" + strconv.Itoa(len(have)-limit)
	}
	b := ui.ButtonBase(c.Key("setup-have-show:"+strconv.Itoa(int(cat)))).FillWidth().Gap(8).Padding(9, 14).
		AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).
		Label(locales.InstallAlreadyHave(len(have), summary)).OnClick(func() {
		if a.setup.showHave == nil {
			a.setup.showHave = map[installCat]bool{}
		}
		a.setup.showHave[cat] = true
	})
	if b.Hovered() {
		b.Background(pal.Muted)
	} else {
		b.Background(pal.Subtle)
	}
	b.Children(func() {
		ui.Icon(c, Icon("circle-check")).Size(13, 13).Shrink(0).TextColor(t.Success)
		clipped(c, locales.InstallAlreadyHave(len(have), summary)).Grow(1).FontSize(11.5).TextColor(t.TextMuted)
		ui.Text(c, locales.SetupShowMore()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
	})
}

func (a *App) setupInstalledExpandedHeader(c *ui.Context, cat installCat, count int) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(8).Padding(6, 14).AlignItems(ui.Center).Background(pal.Subtle).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Text(c, strings.ToUpper(locales.InstallFilterHave())+" · "+strconv.Itoa(count)).Grow(1).
			FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
		ui.ButtonBase(c.Key("setup-have-hide:" + strconv.Itoa(int(cat)))).Label(locales.InstallHide()).OnClick(func() {
			if a.setup.showHave != nil {
				a.setup.showHave[cat] = false
			}
		}).Children(func() {
			ui.Text(c, locales.InstallHide()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		})
	})
}

// setupQueueCard shows the user's current selection on the right side along
// with any dependencies Hearth will install automatically first.
func (a *App) setupQueueCard(c *ui.Context, in ops.Setup, wizard bool) {
	t := c.Theme()
	pal := Colors(t)
	parts := in.Parts()
	var explicit, autoDeps []ops.SetupPart
	for _, p := range parts {
		e, _ := catalog.Find(p.Tool)
		requiredBase := wizard && e.Group == catalog.Base
		if !requiredBase && a.setupChosen(p.Tool) && !a.setupHave(p.Tool) {
			explicit = append(explicit, p)
		} else {
			autoDeps = append(autoDeps, p)
		}
	}
	aside := ""
	if len(parts) > 0 {
		aside = strconv.Itoa(len(parts))
	}
	setupGroupCard(c, "lucide/download", locales.InstallQueueTitle(), aside, func() {
		if len(explicit) == 0 && len(autoDeps) == 0 {
			ui.Row(c).Padding(10, 14, 12, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, locales.InstallNoteNone()).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		for _, p := range explicit {
			id := p.Tool
			title := setupTitle(id)
			tv := a.setupTargetVersion(id, p.Method, c.Now())
			ui.Row(c.Key("queue:"+id)).Gap(8).Padding(7, 10, 7, 14).MinHeight(38).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				iconOrLogo(c, setupToolIcon(id), 15, t.TextMuted)
				clipped(c, title).Grow(1).FontSize(12.5)
				if tv != "" {
					ui.Text(c, tv).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
				}
				rm := ui.ButtonBase(c.Key("queue-rm:"+id)).Size(22, 22).Shrink(0).Radius(6).Center().
					Label(locales.InstallQueueRemove(title)).OnClick(func() {
					if a.setup.choice == nil {
						a.setup.choice = map[string]bool{}
					}
					a.setup.choice[id] = false
					delete(a.setup.packages, id)
					delete(a.setup.versions, id)
				})
				if rm.Hovered() {
					rm.Background(pal.Muted)
				}
				rm.Children(func() {
					ui.Icon(c, Icon("x")).Size(12, 12).TextColor(pal.TextFaint)
				})
			})
		}
		if len(autoDeps) > 0 {
			ui.Row(c).Padding(6, 14).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, strings.ToUpper(locales.InstallQueueDeps())).FontSize(10).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
			})
			for _, p := range autoDeps {
				id := p.Tool
				ui.Row(c.Key("queue-dep:"+id)).Gap(8).Padding(6, 14).MinHeight(32).AlignItems(ui.Center).
					BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
					iconOrLogo(c, setupToolIcon(id), 14, pal.TextFaint)
					clipped(c, setupTitle(id)).Grow(1).FontSize(12).TextColor(t.TextMuted)
				})
			}
		}
		note := ""
		if wizard {
			note = locales.SetupChoiceNote(len(parts))
		} else if len(explicit) > 0 {
			names := make([]string, len(explicit))
			for i, p := range explicit {
				names[i] = setupTitle(p.Tool)
			}
			note = locales.InstallNote(len(explicit), strings.Join(names, ", "))
		}
		if note != "" {
			ui.Row(c).Padding(9, 14).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, note).FontSize(11.5).TextColor(t.TextMuted)
			})
		}
	})
}

// setupBaseCard renders the compact foundation card (Homebrew, mise) in the
// side column without multi-line descriptions.
func (a *App) setupBaseCard(c *ui.Context, in ops.Setup, rows []catalog.Entry, wizard bool) {
	setupGroupCard(c, "lucide/layers", locales.SetupBase(), "", func() {
		for _, e := range rows {
			a.setupBaseRow(c, in, e, wizard)
		}
	})
}

func (a *App) setupBaseRow(c *ui.Context, in ops.Setup, e catalog.Entry, wizard bool) {
	t := c.Theme()
	pal := Colors(t)
	have := a.setupHave(e.ID)
	var methods []catalog.Method
	for _, m := range e.On(goos) {
		if in.Usable(m) {
			methods = append(methods, m)
		}
	}
	if len(methods) == 0 && !have {
		return
	}
	via := ""
	if len(methods) > 0 {
		via = locales.SetupVia(managerTitle(methods[0]))
	}
	if have {
		version, caption := "", ""
		if found, r := a.setupResult(e.ID); r != checkMissing {
			version = found.Version
			caption = tilde(found.Path, a.Home)
		}
		path := setupToolPath(e.ID)
		drawBody := func() {
			setupToolTile(c, e.ID, 26, false, true)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, setupTitle(e.ID)).FontSize(13).SingleLine()
				if caption != "" {
					clipped(c, caption).FontSize(11).TextColor(pal.TextFaint)
				}
			})
			short, _, trimmed := strings.Cut(version, "-")
			label := locales.InstalledPlain()
			if short != "" {
				label = locales.InstalledVersion(short)
			}
			tag := chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, label)
			if trimmed {
				tag.Tooltip(locales.InstalledVersion(version))
			}
			if path != "" {
				ui.Icon(c, Icon("chevron-right")).Size(13, 13).Shrink(0).TextColor(pal.TextFaint)
			}
		}
		if path != "" {
			b := ui.ButtonBase(c.Key("setup:"+e.ID)).FillWidth().Gap(10).Padding(9, 12, 9, 14).MinHeight(46).
				AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).
				Label(locales.InstallOpenTool(setupTitle(e.ID))).OnClick(func() { a.router.Push(path) })
			if b.Hovered() {
				b.Background(pal.Subtle)
			}
			b.Children(drawBody)
			return
		}
		ui.Row(c.Key("setup:"+e.ID)).Gap(10).Padding(9, 14).MinHeight(46).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(drawBody)
		return
	}
	on := a.setupChosen(e.ID)
	row := ui.Row(c.Key("setup:"+e.ID)).Gap(10).Padding(9, 14).MinHeight(48).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	if !wizard && on {
		row.Background(pal.AccentSoft.Alpha(0.45))
	}
	row.Children(func() {
		setupToolTile(c, e.ID, 26, !wizard && on, false)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, setupTitle(e.ID)).FontSize(13).FontWeight(500).SingleLine()
				if via != "" {
					clipped(c, "· "+via).FontSize(11).TextColor(pal.TextFaint)
				}
			})
			if !wizard {
				ui.Text(c, locales.InstallQueueAutoHint()).FontSize(11).TextColor(t.TextMuted)
			}
		})
		if wizard {
			ui.Text(c, locales.SetupRequired()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			return
		}
		if toggle(c, "on", &on).Label(locales.InstallNamed(setupTitle(e.ID))).Changed() {
			if a.setup.choice == nil {
				a.setup.choice = map[string]bool{}
			}
			a.setup.choice[e.ID] = on
		}
	})
}

// setupSetCard surfaces saving and opening hearth-set.json in the side column.
func (a *App) setupSetCard(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	setupGroupCard(c, "lucide/sliders-horizontal", locales.InstallSetTitle(), "", func() {
		ui.Column(c).Gap(10).Padding(10, 14, 12, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			ui.Text(c, locales.InstallSetCaption()).FontSize(11.5).TextColor(t.TextMuted)
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if a.SaveSet != nil {
					ui.Button(c.Key("set-card-save"), locales.SetSave()).Height(controlHeight).
						Disabled(len(a.setupSetTools()) == 0).OnClick(a.saveSet)
				}
				if a.OpenSet != nil {
					ui.Button(c.Key("set-card-open"), locales.SetOpen()).Height(controlHeight).OnClick(a.openSet)
				}
			})
		})
	})
}

// setupRow is a compact 2-line tool row with an icon tile on the left:
// installed rows link to the tool's page when one exists; available rows show
// title, target version and install method on line 1 and description on line 2.
func (a *App) setupRow(c *ui.Context, in ops.Setup, e catalog.Entry) {
	t := c.Theme()
	pal := Colors(t)
	var methods []catalog.Method
	for _, m := range e.On(goos) {
		if in.Usable(m) {
			methods = append(methods, m)
		}
	}
	have := a.setupHave(e.ID)
	if len(methods) == 0 && !have {
		return
	}
	if have {
		version, caption := "", ""
		if found, r := a.setupResult(e.ID); r != checkMissing {
			version = found.Version
			caption = strings.TrimPrefix(sourceName(found.Source)+" · "+tilde(found.Path, a.Home), " · ")
			if found.Source == (inventory.Source{}) {
				caption = tilde(found.Path, a.Home)
			}
		}
		path := setupToolPath(e.ID)
		drawBody := func() {
			setupToolTile(c, e.ID, 28, false, true)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, setupTitle(e.ID)).FontSize(13).TextColor(t.Text).SingleLine()
				if caption != "" {
					clipped(c, caption).FontSize(11.5).TextColor(pal.TextFaint)
				}
			})
			label := locales.InstalledPlain()
			if version != "" {
				label = locales.InstalledVersion(version)
			}
			chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, label)
			if path != "" {
				ui.Icon(c, Icon("chevron-right")).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
			}
		}
		if path != "" {
			b := ui.ButtonBase(c.Key("setup:"+e.ID)).FillWidth().Gap(12).Padding(9, 12, 9, 14).MinHeight(50).
				AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).
				Label(locales.InstallOpenTool(setupTitle(e.ID))).OnClick(func() { a.router.Push(path) })
			if b.Hovered() {
				b.Background(pal.Subtle)
			}
			b.Children(drawBody)
			return
		}
		ui.Row(c.Key("setup:"+e.ID)).Gap(12).Padding(9, 14).MinHeight(50).AlignItems(ui.Center).
			BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(drawBody)
		return
	}

	about, via := catalog.About(e.ID), ""
	var m catalog.Method
	if len(methods) > 0 {
		m = methods[0]
		if e.ID == "docker" {
			for _, em := range methods {
				if em.Package == a.setup.engine {
					m = em
				}
			}
		}
		via = locales.SetupVia(managerTitle(m))
		switch {
		case m.Engine == "Colima":
			about += " " + locales.ColimaWizard()
			via += " · " + locales.ColimaManaged()
		case e.ID == "docker" && m.Terms != "":
			via = locales.SetupEngineTerms(m.Engine)
		case e.ID == "docker":
			via = locales.SetupEngineOpen(m.Engine)
		case e.ID == "rust" && m.Manager == "installer":
			via = locales.SetupViaRustup()
		}
	}
	target := ""
	var verOpts []setupVersionOption
	if len(methods) > 0 {
		target = a.setupTargetVersion(e.ID, m, c.Now())
		verOpts = a.setupVersionOptions(e.ID, m, c.Now())
	}
	if sysIn, ok := a.setupSystemCopy(e.ID); ok {
		about = locales.SetupSystemCopy(sysIn.Version)
	}
	on := a.setupChosen(e.ID)
	conflict := a.setupConflict(e)
	row := ui.Row(c.Key("setup:"+e.ID)).Gap(12).Padding(9, 14).MinHeight(52).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	if on && conflict == "" {
		row.Background(pal.AccentSoft.Alpha(0.45))
	}
	row.Children(func() {
		setupToolTile(c, e.ID, 28, on && conflict == "", false)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, setupTitle(e.ID)).FontSize(13).FontWeight(500).SingleLine()
				if target != "" {
					aiTag(c, target, pal.Muted, t.TextMuted)
				}
				if via != "" {
					clipped(c, "· "+via).FontSize(11).TextColor(pal.TextFaint)
				}
			})
			if about != "" {
				ui.Text(c, about).FontSize(11.5).TextColor(t.TextMuted)
			}
		})
		switch {
		case conflict != "":
			ui.Text(c, locales.SetupConflicts(setupTitle(conflict))).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		default:
			if e.ID == "docker" && on && len(methods) > 1 {
				labels := make([]string, len(methods))
				for i, em := range methods {
					labels[i] = em.Engine
				}
				value := m.Engine
				ui.Select(c.Key("setup-engine"), &value, labels).Label(locales.SetupDockerEngine()).Width(150).OnChange(func() {
					if idx := slices.Index(labels, value); idx >= 0 {
						a.setup.engine = methods[idx].Package
					}
				})
			} else if on && len(verOpts) > 1 {
				labels := make([]string, len(verOpts))
				for i, o := range verOpts {
					labels[i] = o.label
				}
				selected := a.setupSelectedOption(e.ID, verOpts).label
				ui.Select(c.Key("setup-version:"+e.ID), &selected, labels).Label(locales.SetupVersionLabel()).Width(120).OnChange(func() {
					idx := slices.Index(labels, selected)
					if idx < 0 {
						return
					}
					chosen := verOpts[idx]
					if chosen.isPkg {
						if idx == 0 {
							delete(a.setup.packages, e.ID)
						} else {
							if a.setup.packages == nil {
								a.setup.packages = map[string]string{}
							}
							a.setup.packages[e.ID] = chosen.value
						}
					} else {
						if chosen.value == "" {
							delete(a.setup.versions, e.ID)
						} else {
							if a.setup.versions == nil {
								a.setup.versions = map[string]string{}
							}
							a.setup.versions[e.ID] = chosen.value
						}
					}
				})
			}
			if toggle(c, "on", &on).Label(locales.InstallNamed(setupTitle(e.ID))).Changed() {
				if a.setup.choice == nil {
					a.setup.choice = map[string]bool{}
				}
				a.setup.choice[e.ID] = on
				if !on {
					delete(a.setup.packages, e.ID)
					delete(a.setup.versions, e.ID)
				}
			}
		}
	})
}
