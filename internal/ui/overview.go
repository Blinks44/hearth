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
	"github.com/Blinks44/hearth/locales"
)

// shownIssues is how many issues the overview's card lists; the rest go
// in a line under them.
const shownIssues = 3

// overviewPage is how the environment stands: a summary, the tools found,
// the projects, and what needs attention.
func (a *App) overviewPage(c *ui.Context, p page) {
	found := a.issues(c)
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.hero(c, found)
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(18).Children(func() {
						a.toolCards(c, found)
						a.overviewProjects(c)
					})
					ui.Column(c).Width(342).Shrink(0).Padding(27, 0, 0, 0).Children(func() {
						a.attention(c, found)
					})
				})
			})
		})
		a.addDialog(c)
	})
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
// the right the counts of tools installed, on PATH and in projects.
func (a *App) hero(c *ui.Context, found []issues.Issue) {
	t := c.Theme()
	pal := Colors(t)
	tools := a.foundTools()
	ui.Row(c).Gap(24).Padding(22, 22, 22, 26).AlignItems(ui.Center).Radius(14).
		LinearGradient(ui.LinearGradient{Angle: 135, Stops: []ui.GradientStop{
			{Color: pal.HeroFrom, At: 0}, {Color: pal.HeroMid, At: 0.55}, {Color: pal.HeroTo, At: 1},
		}}).BorderWidth(1).BorderColor(pal.HeroBorder).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
			title, text := locales.HeroAllGood(), locales.HeroFound(len(tools))+" "+locales.HeroNoIssues()
			switch n := len(found); {
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
				text = locales.HeroFound(len(tools)) + " " + locales.HeroSomeIssues(n)
			}
			ui.Text(c, title).FontSize(24).FontWeight(600).SingleLine()
			ui.Text(c, text).FontSize(13).TextColor(t.TextMuted).MaxWidth(380)
		})
		ui.Row(c).Gap(10).Shrink(0).Children(func() {
			inPath, pinned := 0, a.pinnedTools()
			for _, ts := range tools {
				if inventory.TerminalCopy(ts.Installations) >= 0 {
					inPath++
				}
			}
			installed, onPath := locales.StatOnDisk(), locales.StatInPathCaption()
			if a.firstScan() {
				installed, onPath = locales.StatSoFar(), locales.StatSoFar()
			}
			stat(c, pal.TextFaint, strconv.Itoa(len(tools)), locales.Installed(), installed)
			stat(c, t.Success, strconv.Itoa(inPath), locales.StatInPath(), onPath)
			if len(a.Projects) == 0 {
				stat(c, Accent, "—", locales.StatInProjects(), locales.NoProjects())
			} else {
				stat(c, Accent, strconv.Itoa(len(pinned)), locales.StatInProjects(), locales.StatInProjectsCaption())
			}
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
// caption.
func stat(c *ui.Context, dot ui.Color, value, label, caption string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).MinWidth(158).Gap(4).Padding(14, 16).Radius(10).Background(pal.StatBg).
		BorderWidth(1).BorderColor(pal.HeroBorder).Children(func() {
		ui.Text(c, value).FontSize(26).SingleLine()
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

// pinnedTools are the tools the projects ask for, each once.
func (a *App) pinnedTools() []string {
	var out []string
	for _, p := range a.Projects {
		for _, r := range p.Requirements {
			if !slices.Contains(out, r.Tool) {
				out = append(out, r.Tool)
			}
		}
	}
	return out
}

// toolCards are the tools found, three in a row; skeletons stand for
// them until the first scan ends.
func (a *App) toolCards(c *ui.Context, found []issues.Issue) {
	pal := Colors(c.Theme())
	tools := a.foundTools()
	ui.Column(c).Gap(10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SectionTools()).FontSize(14).FontWeight(600).SingleLine()
			if !a.firstScan() {
				ui.Text(c, strconv.Itoa(len(tools))).FontSize(12.5).TextColor(pal.TextFaint).SingleLine()
			}
		})
		ui.Grid(c).Columns(3).Gap(10).Children(func() {
			if a.firstScan() {
				for range 3 {
					skeletonCard(c)
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
	for _, l := range languages {
		if l.id == ts.Tool {
			name, path = l.title(), l.path
		}
	}
	e := ui.ButtonBase(c.Key("tool:" + ts.Tool)).Column().AlignItems(ui.Stretch).Gap(12).Padding(14).Radius(12).
		Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Label(name)
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
				if label := toolChip(is, ts.Tool); label != "" {
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

// toolLogo is a tool's logo on a tile of size: Node's in its green.
func toolLogo(c *ui.Context, tool string, size float32) {
	t := c.Theme()
	pal := Colors(t)
	bg, border, color := pal.Subtle, pal.Line, t.TextMuted
	if tool == "node" {
		tile := nodeTile[0]
		if t.Dark {
			tile = nodeTile[1]
		}
		bg, border, color = tile.bg, tile.border, nodeGreen
	}
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 4).Center().Background(bg).
		BorderWidth(1).BorderColor(border).Children(func() {
		ui.Icon(c, toolIcon(tool)).Size(size*0.57, size*0.57).TextColor(color)
	})
}

// skeletonCard stands for a tool card not checked yet.
func skeletonCard(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	bar := func(w, h float32) {
		ui.Box(c).Width(w).Height(h).Radius(h / 2).Background(pal.Muted)
	}
	ui.Column(c).Gap(12).Padding(14).Radius(12).Height(122).
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
			a.noProjects(c)
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
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(filepath.Base(p.Dir))
	if e.Hovered() {
		e.Background(pal.Subtle)
	}
	e.Children(func() {
		folderTile(c, 28)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, filepath.Base(p.Dir)).FontSize(13.5).SingleLine()
			ui.Text(c, tilde(p.Dir, a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		})
		for _, st := range statuses {
			toolTag(c, st)
		}
		problem := projectProblem(statuses)
		if problem == "" && len(p.Errs) > 0 {
			problem = locales.ChipReadFailed()
		}
		switch {
		case problem != "":
			ui.Text(c, problem).FontSize(11.5).TextColor(pal.WarningText).SingleLine()
			ui.Icon(c, Icon("triangle-alert")).Size(16, 16).TextColor(t.Warning)
		case anyChecked(statuses):
			ui.Icon(c, Icon("circle-check")).Size(16, 16).TextColor(t.Success)
		default:
			ui.Text(c, locales.ChipNotChecked()).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
		}
	})
	if e.Clicked() {
		a.router.Push(projectPath(p.Dir))
	}
}

// noProjects asks to add the first project.
func (a *App) noProjects(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(6).Padding(18, 24, 24, 24).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(40, 40).Radius(10).Center().Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).
			Margin(0, 0, 6, 0).Children(func() {
			ui.Icon(c, Icon("folder-open")).Size(18, 18).TextColor(t.TextMuted)
		})
		ui.Text(c, locales.NoProjectsHeading()).FontSize(13.5).FontWeight(500).SingleLine()
		ui.Text(c, locales.NoProjectsYet()).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(340)
		if a.ChooseFolder != nil {
			ui.Button(c, "").Height(24).Padding(0, 10).Margin(6, 0, 0, 0).Label(locales.AddFolder()).
				OnClick(a.ChooseFolder).Children(func() {
				ui.Text(c, locales.AddFolder()).FontSize(12).SingleLine()
			})
		}
	})
}

// attention lists the first issues, the rest in a line under them.
func (a *App) attention(c *ui.Context, found []issues.Issue) {
	count := ""
	if !a.firstScan() {
		count = strconv.Itoa(len(found))
	}
	card(c, locales.AttentionTitle(), count, nil, func() {
		switch {
		case a.firstScan():
			attentionNote(c, "refresh-cw", false, locales.IssuesAfterScan(), locales.IssuesAfterScanText())
			return
		case len(found) == 0:
			attentionNote(c, "circle-check", true, locales.NoIssues(), locales.NoIssuesText())
			return
		}
		for _, is := range found[:min(len(found), shownIssues)] {
			a.issueRow(c, is)
		}
		if rest := found[min(len(found), shownIssues):]; len(rest) > 0 {
			titles := make([]string, len(rest))
			for i, is := range rest {
				titles[i], _ = issueText(is, a.Home)
			}
			pal := Colors(c.Theme())
			ui.Text(c, locales.MoreIssues(len(rest), strings.Join(titles, ", "))).FontSize(12).
				TextColor(c.Theme().TextMuted).Padding(11, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
		}
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

// issueRow is an issue: its level, what it is and why it matters, and a
// button to the page it lives on.
func (a *App) issueRow(c *ui.Context, is issues.Issue) {
	t := c.Theme()
	pal := Colors(t)
	title, explain := issueText(is, a.Home)
	icon, bg, color := "triangle-alert", pal.WarningSoft, t.Warning
	if is.Severity == issues.Info {
		icon, bg, color = "info", pal.Muted, t.TextMuted
	}
	ui.Row(c).Gap(10).Padding(13, 14).AlignItems(ui.Start).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Box(c).Size(28, 28).Shrink(0).Radius(8).Center().Background(bg).Children(func() {
			ui.Icon(c, Icon(icon)).Size(16, 16).TextColor(color)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(500)
			ui.Text(c, explain).FontSize(11.5).TextColor(t.TextMuted)
			ui.Row(c).Padding(5, 0, 0, 0).Children(func() {
				page := issuePage(is)
				ui.Button(c.Key("details:"+string(is.Kind)+":"+is.Tool+":"+is.Project), "").Height(24).Padding(0, 10).
					Label(locales.Details()).OnClick(func() { a.router.Push(page) }).Children(func() {
					ui.Text(c, locales.Details()).FontSize(12).SingleLine()
				})
			})
		})
	})
}
