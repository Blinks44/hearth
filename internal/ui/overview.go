package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/report"
	"github.com/Blinks44/hearth/locales"
)

// shownIssues is how many issues the overview's card lists; the rest go
// in a line under them.
const shownIssues = 3

// overviewPage is how the environment stands: a summary, the tools found,
// the projects, and what needs attention.
func (a *App) overviewPage(c *ui.Context, p page) {
	found := a.issues(c)
	items := needAttention(a.attentionItems(found))
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			ui.Button(c.Key("copy-report"), "").Label(locales.CopyReport()).Tooltip(locales.CopyReportHint()).OnClick(func() {
				c.WriteClipboard(a.report(found))
				c.Toast(locales.ReportCopied())
			}).Children(func() {
				ui.Icon(c, Icon("copy")).Size(13, 13).TextColor(c.Theme().TextMuted)
				ui.Text(c, locales.CopyReport()).SingleLine()
			})
		}})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.hero(c, items)
				if !a.StartHidden {
					a.startActions(c)
				}
				// A narrow window has one column: what needs attention
				// right under the summary, then the rest.
				if narrow(c) {
					a.attention(c, items)
					a.overviewProjects(c)
					a.toolCards(c, found)
					a.overviewAgents(c)
					return
				}
				// Projects sit in the side column: the tool grid grows with
				// every install and would push them out of sight.
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(18).Children(func() {
						a.toolCards(c, found)
					})
					ui.Column(c).Width(342).Shrink(0).Padding(27, 0, 0, 0).Gap(18).Children(func() {
						a.attention(c, items)
						a.overviewAgents(c)
						a.overviewProjects(c)
					})
				})
			})
		})
	})
}

// startActions introduces the existing pages until the user dismisses it.
// Following a link does not imply the task succeeded or hide the other choices.
func (a *App) startActions(c *ui.Context) {
	pal := Colors(c.Theme())
	ui.Column(c.Key("start-actions")).Gap(10).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.StartTitle()).FontSize(14).FontWeight(600).Grow(1)
			ui.ButtonBase(c.Key("start-dismiss")).Label(locales.StartDismiss()).OnClick(func() {
				a.StartHidden = true
				a.savePreferences()
			}).Children(func() {
				ui.Text(c, locales.StartDismiss()).FontSize(12).TextColor(pal.TextFaint)
			})
		})
		columns := 4
		if narrow(c) {
			columns = 2
		}
		ui.Grid(c).Columns(columns).Gap(12).Children(func() {
			install := func() {
				a.startCard(c, "start-install", locales.StartInstall(), locales.StartInstallHint(), true, func() {
					a.setup.wizard = a.bareComputer()
					a.router.Push(setupPage)
				})
			}
			if a.bareComputer() {
				install()
			}
			title, hint := locales.StartProject(), locales.StartProjectHint()
			if len(a.Projects) == 0 {
				title, hint = locales.AddProjectTitle(), locales.StartAddProjectHint()
			}
			a.startCard(c, "start-project", title, hint, len(a.Projects) > 0 || a.ChooseFolder != nil, func() {
				if len(a.Projects) == 0 {
					a.ChooseFolder()
				} else {
					a.router.Push(projectsPage.path)
				}
			})
			hint = locales.StartIssuesHint()
			if a.firstScan() {
				hint = locales.StartCheckingHint()
			}
			a.startCard(c, "start-issues", locales.StartIssues(), hint, !a.firstScan(), func() { a.router.Push(problemsPage) })
			if !a.bareComputer() {
				install()
			}
			a.startCard(c, "start-ai", locales.StartAI(), locales.StartAIHint(), true, func() { a.router.Push("/ai/agents") })
		})
	})
}

func (a *App) startCard(c *ui.Context, key, title, hint string, enabled bool, open func()) {
	t := c.Theme()
	pal := Colors(t)
	e := ui.ButtonBase(c.Key(key)).Column().AlignItems(ui.Stretch).TextAlign(ui.Start).
		Gap(8).Padding(16).MinHeight(104).MinWidth(0).
		Label(title).Disabled(!enabled).OnClick(open).Radius(12).Background(t.Background).
		BorderWidth(1).BorderColor(pal.Divider).Transition(colorFade)
	chevron := pal.TextFaint
	if e.Hovered() && enabled {
		e.Background(pal.Subtle)
		chevron = Accent
	}
	e.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(600).Grow(1).MinWidth(0)
			ui.Icon(c, Icon("chevron-right")).Size(14, 14).Shrink(0).TextColor(chevron)
		})
		ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
	})
}

// report is the environment as text to paste elsewhere, without secrets
// and personal paths.
func (a *App) report(found []issues.Issue) string {
	return report.Write(a.Snapshot, a.Projects, found, report.Info{Version: a.AppVersion, OS: a.OS, Home: a.Home})
}

// issues are what needs attention in the last scan, warnings first; nil
// before the first scan.
func (a *App) issues(c *ui.Context) []issues.Issue {
	found := issues.Find(a.Snapshot, a.Projects, a.NodeVersions, c.Now())
	slices.SortStableFunc(found, func(x, y issues.Issue) int { return cmp.Compare(y.Severity, x.Severity) })
	return found
}

// firstScan reports that no scan has finished yet.
func (a *App) firstScan() bool { return a.Snapshot == nil }

// hero sums the environment up: a chip and a title by the issues, and on
// the right the counts of tools installed, on PATH and in projects; in a
// narrow window the counts go under the text.
func (a *App) hero(c *ui.Context, found []attentionItem) {
	t := c.Theme()
	pal := Colors(t)
	tools := a.foundTools()
	var box ui.Element
	if narrow(c) {
		box = ui.Column(c).Gap(14).Padding(18, 22).AlignItems(ui.Stretch)
	} else {
		box = ui.Row(c).Gap(24).Padding(22, 22, 22, 26).AlignItems(ui.Center)
	}
	box.Radius(14).
		LinearGradient(ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
			{Color: pal.HeroFrom, At: 0}, {Color: pal.HeroMid, At: 0.55}, {Color: pal.HeroTo, At: 1},
		}}).BorderWidth(1).BorderColor(pal.HeroBorder).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
			foundText := locales.HeroFound(len(tools))
			if n := len(a.foundAgents()); n > 0 {
				foundText = locales.HeroFoundAll(len(tools), n)
			}
			title, text := locales.HeroAllGood(), foundText+" "+locales.HeroNoIssues()
			bare := a.bareComputer()
			switch n := len(found); {
			case bare:
				// What the system brings is worth knowing, not a problem
				// of a computer nothing is set up on yet.
				heroChip(c, pal.Info, pal.Info.Alpha(0.12), pal.Info, locales.ChipBare())
			case a.firstScan():
				heroChip(c, pal.Info, pal.Info.Alpha(0.12), pal.Info, locales.ChipChecking())
				title, text = locales.HeroFirstScan(), locales.HeroFirstScanText()
			case n == 0:
				heroChip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ChipReadyToWork())
			default:
				heroChip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ChipIssues(n))
				title = locales.HeroAlmost()
				if n >= 3 {
					title = locales.HeroAttention()
				}
				text = foundText + " " + locales.HeroSomeIssues(n)
			}
			if bare {
				title, text = locales.HeroBare(), locales.HeroBareText()
			}
			ui.Text(c, title).FontSize(24).FontWeight(600).SingleLine()
			txt := ui.Text(c, text).FontSize(13).TextColor(t.TextMuted)
			if !narrow(c) {
				txt.MaxWidth(380)
			}
			if bare {
				ui.PrimaryButton(c.Key("hero-setup"), locales.HeroBareAction()).AlignSelf(ui.Start).OnClick(func() {
					a.setup.wizard = true
					a.router.Push(setupPage)
				})
			}
			// What the last check changed opens the history.
			if n := a.changesSince(); n > 0 {
				link := ui.ButtonBase(c.Key("changes-link")).Row().AlignSelf(ui.Start).Gap(4).AlignItems(ui.Center).
					Label(locales.ChangesLink(n)).OnClick(func() { a.router.Push(changesPage.path) })
				color := Accent
				if link.Hovered() {
					color = pal.AccentInk
				}
				link.Children(func() {
					ui.Text(c, locales.ChangesLink(n)).FontSize(12.5).FontWeight(500).TextColor(color).SingleLine()
					ui.Icon(c, Icon("chevron-right")).Size(12, 12).TextColor(color)
				})
			}
		})
		ui.Row(c).Gap(10).Shrink(0).Children(func() {
			problems, dot := "—", pal.TextFaint
			if !a.firstScan() {
				problems = strconv.Itoa(len(found))
				if len(found) > 0 {
					dot = t.Warning
				}
			}
			stat(c, "stat-issues", dot, problems, locales.HeroProblems(), locales.HeroProblemsCaption(), func() { a.router.Push(problemsPage) })
			stat(c, "stat-updates", pal.Info, a.updateCount(c), locales.HeroUpdates(), locales.HeroUpdatesCaption(), func() { a.router.Push(updatesPage) })
			free, caption := "—", locales.CleanupCounting()
			if a.Sizes != nil {
				free, caption = cleanupAmount(a.candidates(c)), locales.StatFreeCaption()
			}
			stat(c, "stat-free", Accent, free, locales.StatFree(), caption, func() { a.router.Push(cleanupPage) })
		})
	})
}

// heroChip is the hero's status: a dot and a text on a soft background.
func heroChip(c *ui.Context, dot, bg, text ui.Color, label string) {
	ui.Row(c).AlignSelf(ui.Start).Gap(6).Padding(3, 9, 3, 8).Radius(10).AlignItems(ui.Center).Background(bg).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, label).FontSize(11.5).FontWeight(500).TextColor(text).SingleLine()
	})
}

// stat is one of the hero's counts: the number, a dot and a label, and a
// caption. It opens the page open goes to, as its chevron says.
func stat(c *ui.Context, key string, dot ui.Color, value, label, caption string, open func()) {
	t := c.Theme()
	pal := Colors(t)
	e := ui.ButtonBase(c.Key(key)).Column().AlignItems(ui.Stretch).MinWidth(178).MinHeight(96).Gap(4).Padding(14, 16).Label(label).OnClick(open).Transition(colorFade)
	if narrow(c) {
		e.Grow(1).Padding(10, 14).MinHeight(88)
	}
	bg, chevron := pal.StatBg, pal.TextFaint
	if e.Hovered() {
		bg, chevron = t.Background, Accent
		e.Shadow(0, 2, 8, 0, pal.Shadow)
	}
	e.Radius(10).Background(bg).
		BorderWidth(1).BorderColor(pal.HeroBorder).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			fontSize := float32(26)
			if len([]rune(value)) > 8 {
				fontSize = 22
			}
			ui.Text(c, value).FontSize(fontSize).Grow(1).MinWidth(0).SingleLine()
			ui.Icon(c, Icon("chevron-right")).Size(14, 14).Shrink(0).TextColor(chevron)
		})
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(dot)
			ui.Text(c, label).FontSize(12.5).FontWeight(500).SingleLine()
		})
		ui.Text(c, caption).FontSize(11).TextColor(pal.TextFaint).SingleLine()
	})
}

// foundTools are the tools the last scan found a copy of, in its order.
func (a *App) foundTools() []inventory.ToolState {
	if a.Snapshot == nil {
		return nil
	}
	var out []inventory.ToolState
	for _, ts := range a.Snapshot.Tools {
		if len(listed(ts.Installations)) > 0 {
			out = append(out, ts)
		}
	}
	return out
}

// bareComputer reports that the last scan found only what the system
// brings and no AI agent: a new computer, which the wizard sets up.
func (a *App) bareComputer() bool {
	if a.Snapshot == nil || len(a.foundAgents()) > 0 {
		return false
	}
	for _, ts := range a.foundTools() {
		for _, i := range listed(ts.Installations) {
			if ts.Installations[i].Source.Kind != inventory.KindSystem {
				return false
			}
		}
	}
	return true
}

// toolCards are the tools found, three in a row, those with issues first;
// skeletons stand for them until the first scan ends.
func (a *App) toolCards(c *ui.Context, found []issues.Issue) {
	pal := Colors(c.Theme())
	tools := a.foundTools()
	hasIssue := func(ts inventory.ToolState) bool {
		return slices.ContainsFunc(found, func(is issues.Issue) bool { return toolChip(is, ts.Tool) != "" })
	}
	slices.SortStableFunc(tools, func(x, y inventory.ToolState) int {
		switch hx, hy := hasIssue(x), hasIssue(y); {
		case hx && !hy:
			return -1
		case hy && !hx:
			return 1
		}
		return a.byTitle(toolName(x.Tool), toolName(y.Tool))
	})
	ui.Column(c).Gap(10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SectionTools()).FontSize(14).FontWeight(600).SingleLine()
			if !a.firstScan() {
				ui.Text(c, strconv.Itoa(len(tools))).FontSize(12.5).TextColor(pal.TextFaint).SingleLine()
			}
		})
		if !a.firstScan() && len(tools) == 0 {
			ui.Text(c, locales.ToolsNoneYet()).FontSize(12.5).TextColor(c.Theme().TextMuted)
			return
		}
		ui.Grid(c).Columns(3).Gap(10).Children(func() {
			if a.firstScan() {
				for i := range 3 {
					skeletonCard(c.Key(i))
				}
			}
			for _, ts := range tools {
				a.toolCard(c, ts, found)
			}
		})
	})
}

// toolCard is a tool found: its logo, name and source, the version the
// terminal runs, and chips of where it stands. It opens the tool's page.
func (a *App) toolCard(c *ui.Context, ts inventory.ToolState, found []issues.Issue) {
	t := c.Theme()
	pal := Colors(t)
	rows := listed(ts.Installations)
	active := inventory.TerminalCopy(ts.Installations)
	shown := ts.Installations[rows[0]]
	if active >= 0 {
		shown = ts.Installations[active]
	}
	name, path := toolTitle(ts.Tool), ""
	if jsTool(ts.Tool) {
		path = "/tools/jspm"
	}
	if ts.Tool == "git" {
		path = gitPage
	}
	for _, l := range languages {
		if l.id == ts.Tool {
			name, path = l.title(), l.path
		}
	}
	e := ui.ButtonBase(c.Key("tool:" + ts.Tool)).Column().AlignItems(ui.Stretch).Gap(12).Padding(14).Radius(12).
		Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Label(name).Transition(cardEnter)
	if e.Hovered() {
		e.Background(pal.Subtle)
	}
	e.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			toolLogo(c, ts.Tool, 28)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, name).FontSize(13.5).FontWeight(500).SingleLine()
				ui.Text(c, sourceName(shown.Source)+" · "+locales.VersionsCount(len(rows))).
					FontSize(11).TextColor(pal.TextFaint).SingleLine()
			})
		})
		ui.Text(c, versionText(shown.Version)).Font(FontMono).FontSize(17).SingleLine()
		ui.Row(c).Gap(6).Wrap().Children(func() {
			if active >= 0 {
				chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ChipInPath())
			}
			if n := a.projectsNeeding(ts.Tool); n > 0 {
				chip(c, Accent, pal.AccentSoft, pal.AccentInk, locales.ChipProjects(n))
			}
			for _, is := range found {
				switch label := toolChip(is, ts.Tool); {
				case label == "":
				case is.Severity == issues.Info:
					chip(c, pal.TextFaint, pal.Muted, pal.MonoText, label)
				default:
					chip(c, t.Warning, pal.WarningSoft, pal.WarningText, label)
				}
			}
		})
	})
	if e.Clicked() && path != "" {
		a.router.Push(path)
	}
}

// toolChip is the short label of an issue on the card of tool, "" when
// the issue is not the tool's.
func toolChip(is issues.Issue, tool string) string {
	if is.Tool != tool {
		return ""
	}
	switch is.Kind {
	case issues.Duplicate:
		return locales.ChipCopies(len(is.Copies))
	case issues.GUIDiffers:
		if len(is.Copies) < 2 {
			return locales.ChipGuiMissing()
		}
		return locales.ChipGuiDiffers()
	case issues.Unsupported:
		return locales.ChipSupportEnded()
	case issues.ProjectMissing:
		return locales.ProblemMissing(is.Check.Want)
	}
	return ""
}

// projectsNeeding counts the projects that need tool.
func (a *App) projectsNeeding(tool string) int {
	n := 0
	for _, p := range a.Projects {
		if slices.ContainsFunc(p.Requirements, func(r projects.Requirement) bool { return r.Tool == tool }) {
			n++
		}
	}
	return n
}

// toolLogo is a tool's logo, in its colors, on a tile of size.
func toolLogo(c *ui.Context, tool string, size float32) {
	pal := Colors(c.Theme())
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 4).Center().Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Children(func() {
		iconOrLogo(c, toolIcon(tool), size*0.57, c.Theme().TextMuted)
	})
}

// skeletonCard stands for a tool card not checked yet.
func skeletonCard(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	bar := func(w, h float32) {
		ui.Box(c).Width(w).Height(h).Radius(h / 2).Background(pal.Muted)
	}
	ui.Column(c).Transition(skeletonExit).Gap(12).Padding(14).Radius(12).Height(122).
		Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(28, 28).Radius(7).Background(pal.Muted)
			ui.Column(c).Gap(6).Children(func() {
				bar(90, 8)
				bar(60, 6)
			})
		})
		bar(80, 12)
		bar(56, 14)
	})
}

// card is an overview card: a title and a count, what end builds on the
// right of them, and rows under them.
func card(c *ui.Context, title, count string, end, rows func()) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(12, 14, 10, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(14).FontWeight(600).SingleLine()
			if count != "" {
				ui.Text(c, count).FontSize(12.5).TextColor(pal.TextFaint).SingleLine()
			}
			ui.Box(c).Grow(1)
			if end != nil {
				end()
			}
		})
		rows()
	})
}

// overviewProjects are the projects with their tools and state, or a way
// to add the first.
func (a *App) overviewProjects(c *ui.Context) {
	t := c.Theme()
	count := ""
	if len(a.Projects) > 0 {
		count = strconv.Itoa(len(a.Projects))
	}
	var add func()
	if a.ChooseFolder != nil && len(a.Projects) > 0 {
		add = func() {
			b := ui.ButtonBase(c.Key("overview-add")).Gap(4).AlignItems(ui.Center).Label(locales.AddFolder()).OnClick(a.ChooseFolder)
			color := t.TextMuted
			if b.Hovered() {
				color = t.Text
			}
			b.Children(func() {
				ui.Icon(c, Icon("plus")).Size(13, 13).TextColor(color)
				ui.Text(c, locales.AddFolder()).FontSize(12.5).TextColor(color).SingleLine()
			})
		}
	}
	card(c, projectsPage.title(), count, add, func() {
		if len(a.Projects) == 0 {
			a.noProjects(c, locales.NoProjectsYet())
			return
		}
		lines := a.nodeLines()
		for _, p := range a.Projects {
			a.overviewProject(c, p, projects.Resolve(p, a.Snapshot, lines))
		}
	})
}

// overviewProject is a project's row: its folder, the tools it needs and
// its state. It opens the project's page.
func (a *App) overviewProject(c *ui.Context, p projects.Project, statuses []projects.Status) {
	t := c.Theme()
	pal := Colors(t)
	e := ui.ButtonBase(c.Key("overview-project:"+p.Dir)).Gap(8).Padding(9, 14).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(filepath.Base(p.Dir)).Transition(colorFade)
	if e.Hovered() {
		e.Background(pal.Subtle)
	}
	problem := projectProblem(statuses)
	if problem == "" && len(p.Errs) > 0 {
		problem = locales.ChipReadFailed()
	}
	e.AlignItems(ui.Start).Children(func() {
		folderTile(c, 28)
		// The side column is narrow: the tools wrap under the name, and
		// the state is a line of its own rather than a chip.
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			ui.Text(c, filepath.Base(p.Dir)).FontSize(13.5).SingleLine()
			if len(statuses) > 0 {
				ui.Row(c).Gap(4).Wrap().Children(func() {
					for _, st := range statuses {
						toolTag(c, st)
					}
				})
			}
			switch {
			case problem != "":
				ui.Text(c, problem).FontSize(11.5).TextColor(pal.WarningText).MaxLines(2)
			case !anyChecked(statuses):
				ui.Text(c, locales.ChipNotChecked()).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			}
		})
		switch {
		case problem != "":
			ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).Margin(6, 0, 0, 0).TextColor(t.Warning)
		case anyChecked(statuses):
			ui.Icon(c, Icon("circle-check")).Size(16, 16).Shrink(0).Margin(6, 0, 0, 0).TextColor(t.Success)
		}
	})
	if e.Clicked() {
		a.router.Push(projectPath(p.Dir))
	}
}

// noProjects asks to add the first project; text says what it gives.
func (a *App) noProjects(c *ui.Context, text string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(6).Padding(18, 24, 24, 24).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(40, 40).Radius(10).Center().Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).
			Margin(0, 0, 6, 0).Children(func() {
			ui.Icon(c, Icon("folder-open")).Size(18, 18).TextColor(t.TextMuted)
		})
		ui.Text(c, locales.NoProjectsHeading()).FontSize(13.5).FontWeight(500).SingleLine()
		ui.Text(c, text).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(340)
		ui.Row(c).Gap(8).Margin(8, 0, 0, 0).Children(func() {
			if a.ChooseFolder != nil {
				ui.Button(c.Key("overview-add-folder"), locales.AddFolder()).OnClick(a.ChooseFolder)
			}
			if a.FindProjects != nil {
				ui.Button(c.Key("overview-find-projects"), locales.FindProjects()).OnClick(a.FindProjects)
			}
		})
	})
}

// attention lists the first issues, with a link to all of them.
func (a *App) attention(c *ui.Context, found []attentionItem) {
	count := ""
	if !a.firstScan() {
		count = strconv.Itoa(len(found))
	}
	var all func()
	if len(found) > 0 {
		all = func() { a.pageLink(c, "all-issues", locales.AllIssues(), problemsPage) }
	}
	card(c, locales.AttentionTitle(), count, all, func() {
		switch {
		case a.firstScan():
			attentionNote(c, "refresh-cw", false, locales.IssuesAfterScan(), locales.IssuesAfterScanText())
			return
		case len(found) == 0:
			attentionNote(c, "circle-check", true, locales.NoIssues(), locales.NoIssuesText())
			return
		}
		for _, it := range found[:min(len(found), shownIssues)] {
			a.issueRow(c, it)
		}
	})
}

// pageLink is a card header's link to the page at path: its label and a
// chevron.
func (a *App) pageLink(c *ui.Context, key, label, path string) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Gap(4).AlignItems(ui.Center).Label(label).OnClick(func() { a.router.Push(path) })
	color := t.TextMuted
	if b.Hovered() {
		color = t.Text
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).TextColor(color).SingleLine()
		ui.Icon(c, Icon("chevron-right")).Size(13, 13).TextColor(color)
	})
}

// attentionNote stands in the issues card for the issues: before the
// first scan, or when there are none.
func attentionNote(c *ui.Context, icon string, ok bool, title, text string) {
	t := c.Theme()
	pal := Colors(t)
	bg, color := pal.Subtle, t.TextMuted
	if ok {
		bg, color = pal.SuccessSoft, t.Success
	}
	ui.Row(c).Gap(10).Padding(13, 14).AlignItems(ui.Start).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Box(c).Size(28, 28).Shrink(0).Radius(8).Center().Background(bg).Children(func() {
			ui.Icon(c, Icon(icon)).Size(16, 16).TextColor(color)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(500)
			ui.Text(c, text).FontSize(11.5).TextColor(t.TextMuted)
		})
	})
}

// issueRow is an issue: its level, what it is and why it matters, a
// button to its fix when an operation makes one, and to the page it lives
// on.
func (a *App) issueRow(c *ui.Context, it attentionItem) {
	t := c.Theme()
	pal := Colors(t)
	icon, bg, color := "triangle-alert", pal.WarningSoft, t.Warning
	if it.severity == issues.Info {
		icon, bg, color = "info", pal.Muted, t.TextMuted
	}
	ui.Row(c).Gap(10).Padding(13, 14).AlignItems(ui.Start).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Box(c).Size(28, 28).Shrink(0).Radius(8).Center().Background(bg).Children(func() {
			ui.Icon(c, Icon(icon)).Size(16, 16).TextColor(color)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, it.title).FontSize(13.5).FontWeight(500)
			// The page tells the rest; the card stays short.
			ui.Text(c, it.explain).FontSize(11.5).TextColor(t.TextMuted).MaxLines(2).Tooltip(it.explain)
			ui.Row(c).Gap(6).Padding(5, 0, 0, 0).Children(func() {
				if it.fix != nil {
					// Dark, as in the design.
					ui.Button(c.Key("fix:"+it.key), "").Height(24).Padding(0, 10).Background(t.Text).BorderColor(t.Text).
						Label(locales.IssueFix()).OnClick(it.fix).Children(func() {
						ui.Text(c, locales.IssueFix()).FontSize(12).TextColor(t.Background).SingleLine()
					})
				}
				ui.Button(c.Key("details:"+it.key), "").Height(24).Padding(0, 10).
					Label(locales.Details()).OnClick(func() { a.router.Push(it.page) }).Children(func() {
					ui.Text(c, locales.Details()).FontSize(12).SingleLine()
				})
			})
		})
	})
}

// attentionItem is a line of the issues card: an issue, or several of one
// kind that only add up, as skills copied to several folders.
type attentionItem struct {
	title, explain, page, key string
	severity                  issues.Severity
	area                      int    // the Issues page's tab it shows under: areaTools, …
	fix                       func() // proposes the plan that fixes it; nil for none
}

// issueKey names is among the scan's issues, for the hidden ones: its
// kind, tool and project, the server or agent of one about secrets, and
// the rule of one about an agent's permissions.
func issueKey(is issues.Issue) string {
	key := string(is.Kind) + ":" + is.Tool + ":" + is.Project
	switch {
	case is.Kind == issues.PlainSecret:
		key += ":" + is.MCP[0].Agent + ":" + is.MCP[0].Name
	case is.Agent != nil:
		key += ":" + is.Agent.ID
	case is.Rule != nil:
		key += ":" + is.Rule.Agent + ":" + is.Rule.File + ":" + is.Rule.Rule
	}
	return key
}

// grouped are the kinds whose issues the card shows as one when there are
// several: each is the same advice about another skill or server.
var grouped = []issues.Kind{issues.SkillCopies, issues.SkillDiffers, issues.MCPMissing}

// needAttention are the items that need attention: warnings, and what is
// only worth knowing when Hearth can fix it. The rest waits on the Issues
// page, so that a computer nothing is wrong with, as macOS with its Java
// stub, counts none.
func needAttention(items []attentionItem) []attentionItem {
	return slices.DeleteFunc(items, func(it attentionItem) bool { return it.severity == issues.Info && it.fix == nil })
}

// attentionItems are the card's lines for found, in its order, a group
// where its first issue was.
func (a *App) attentionItems(found []issues.Issue) []attentionItem {
	byKind := map[issues.Kind][]issues.Issue{}
	for _, is := range found {
		if slices.Contains(grouped, is.Kind) {
			byKind[is.Kind] = append(byKind[is.Kind], is)
		}
	}
	var out []attentionItem
	done := map[issues.Kind]bool{}
	for _, is := range found {
		group := byKind[is.Kind]
		if len(group) < 2 {
			title, explain := issueText(is, a.Home)
			out = append(out, attentionItem{title, explain, issuePage(is), issueKey(is), is.Severity, issueArea(is), a.issueFix(is, found)})
			continue
		}
		if done[is.Kind] {
			continue
		}
		done[is.Kind] = true
		var names []string
		severity := issues.Info
		for _, g := range group {
			severity = max(severity, g.Severity)
			for _, sk := range g.Skills[:min(len(g.Skills), 1)] {
				names = append(names, sk.Name)
			}
			if len(g.MCP) > 0 {
				names = append(names, filepath.Base(g.MCP[0].Command))
			}
		}
		var title string
		switch is.Kind {
		case issues.SkillCopies:
			title = locales.GroupSkillCopies(len(group))
		case issues.SkillDiffers:
			title = locales.GroupSkillDiffers(len(group))
		case issues.MCPMissing:
			title = locales.GroupMCPMissing(len(group))
		}
		out = append(out, attentionItem{title, strings.Join(names, ", "), issuePage(is), string(is.Kind), severity, issueArea(is), a.issueFix(is, found)})
	}
	return out
}
