package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/locales"
)

// problemsPage is the Issues page's path.
const problemsPage = "/issues"

// The areas of the Issues page's tabs, after All.
const (
	areaTools = iota + 1
	areaPath
	areaAI
	areaProjects
)

// issueArea is the tab an issue shows under: a skill's or server's in AI,
// a project's in Projects, the PATH's in PATH, the rest in Tools.
func issueArea(is issues.Issue) int {
	switch page := issuePage(is); {
	case page == skillsPage || page == mcpPage:
		return areaAI
	case page == pathPage:
		return areaPath
	case strings.HasPrefix(page, projectPage+"?"):
		return areaProjects
	}
	return areaTools
}

// problemsView is every issue of the last scan: warnings, then what is
// only worth knowing, each with its whole explanation, a fix when an
// operation makes one, and the page it lives on.
func (a *App) problemsView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		t := c.Theme()
		pal := Colors(t)
		items := a.attentionItems(a.issues(c))
		hidden := len(items)
		items = slices.DeleteFunc(items, func(it attentionItem) bool {
			return it.severity == issues.Info && slices.Contains(a.Hidden, it.key)
		})
		hidden -= len(items)
		a.header(c, p.title(), p.subtitle())
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				warnings := 0
				for _, it := range items {
					if it.severity == issues.Warning {
						warnings++
					}
				}
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.WarningSoft).Center().Children(func() {
						ui.Icon(c, Icon("triangle-alert")).Size(26, 26).TextColor(t.Warning)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, locales.ProblemsTitle()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.ProblemsLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					important, info, checkedAt := "—", "—", "—"
					if !a.firstScan() {
						important, info = strconv.Itoa(warnings), strconv.Itoa(len(items)-warnings)
						checkedAt = ago(c, a.Snapshot.Taken)
					}
					stateCard(c, t.Warning, locales.ProblemsImportant(), important, locales.HeroProblemsCaption())
					stateCard(c, pal.TextFaint, locales.ProblemsInfo(), info, locales.ProblemsInfoCaption())
					stateCard(c, t.Success, locales.ProblemsChecked(), checkedAt, locales.ProblemsCheckedCaption())
				})
				count := func(area int) string {
					n := 0
					for _, it := range items {
						if it.area == area {
							n++
						}
					}
					return strconv.Itoa(n)
				}
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "problems-tabs", &a.problemsTab, []segment{
						{locales.TabAll(), strconv.Itoa(len(items))},
						{locales.SectionTools(), count(areaTools)},
						{"PATH", count(areaPath)},
						{"AI", count(areaAI)},
						{locales.ProjectsTitle(), count(areaProjects)},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("problems-filter"), &a.problemsFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Width(200)
				})
				if hidden > 0 && !a.firstScan() {
					// Under the table, or the note that stands for it.
					defer func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, locales.ProblemsHidden(hidden)).FontSize(12).TextColor(t.TextMuted)
							ui.Button(c.Key("problems-unhide"), locales.ProblemsShowHidden()).OnClick(func() { a.setHidden(nil) })
						})
					}()
				}
				switch {
				case a.firstScan():
					toolNote(c, locales.IssuesAfterScanText())
					return
				case len(items) == 0:
					toolNote(c, locales.NoIssues()+". "+locales.NoIssuesText())
					return
				}
				filter := strings.ToLower(a.problemsFilter)
				ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
					shown := 0
					for _, level := range []issues.Severity{issues.Warning, issues.Info} {
						var rows []attentionItem
						for _, it := range items {
							if it.severity == level && (a.problemsTab == 0 || it.area == a.problemsTab) &&
								strings.Contains(strings.ToLower(it.title+" "+it.explain), filter) {
								rows = append(rows, it)
							}
						}
						if len(rows) == 0 {
							continue
						}
						title := locales.ProblemsGroupImportant()
						if level == issues.Info {
							title = locales.ProblemsGroupInfo()
						}
						ui.Row(c).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
							ui.Text(c, strings.ToUpper(title)).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
							ui.Text(c, strconv.Itoa(len(rows))).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
						})
						for _, it := range rows {
							shown++
							a.problemRow(c, it)
						}
					}
					if shown == 0 {
						cardNote(c, locales.ProblemsNone())
					}
				})
			})
		})
	})
}

// setHidden keeps keys as the hidden issues.
func (a *App) setHidden(keys []string) {
	a.Hidden = keys
	if a.OnSettings != nil {
		a.OnSettings()
	}
}

// problemRow is an issue in the table: its level, what it is and the whole
// of why it matters, and on the right its fix and the page it lives on.
func (a *App) problemRow(c *ui.Context, it attentionItem) {
	t := c.Theme()
	pal := Colors(t)
	icon, bg, color := "triangle-alert", pal.WarningSoft, t.Warning
	if it.severity == issues.Info {
		icon, bg, color = "info", pal.Muted, t.TextMuted
	}
	ui.Row(c.Key("problem:"+it.key)).Gap(10).Padding(13, 14).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Box(c).Size(28, 28).Shrink(0).Radius(8).Center().Background(bg).Children(func() {
			ui.Icon(c, Icon(icon)).Size(16, 16).TextColor(color)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, it.title).FontSize(13.5).FontWeight(500)
			ui.Text(c, it.explain).FontSize(11.5).TextColor(t.TextMuted)
		})
		ui.Row(c).Gap(6).Shrink(0).Children(func() {
			if it.fix != nil {
				ui.Button(c.Key("fix"), "").Height(22).Padding(0, 10).Background(t.Text).BorderColor(t.Text).
					Label(locales.IssueFix()).OnClick(it.fix).Children(func() {
					ui.Text(c, locales.IssueFix()).FontSize(12).TextColor(t.Background).SingleLine()
				})
			}
			if it.severity == issues.Info {
				ui.Button(c.Key("hide"), "").Height(22).Padding(0, 10).Label(locales.ProblemHide()).Tooltip(locales.ProblemHideHint()).
					OnClick(func() { a.setHidden(append(slices.Clone(a.Hidden), it.key)) }).Children(func() {
					ui.Text(c, locales.ProblemHide()).FontSize(12).SingleLine()
				})
			}
			ui.Button(c.Key("open"), "").Height(22).Padding(0, 10).
				Label(locales.IssueOpen()).OnClick(func() { a.router.Push(it.page) }).Children(func() {
				ui.Text(c, locales.IssueOpen()).FontSize(12).SingleLine()
			})
		})
	})
}

// problemsEnd is the Issues item's end in the sidebar: how many there are.
func (a *App) problemsEnd(c *ui.Context) func() {
	return warningCount(c, len(needAttention(a.attentionItems(a.issues(c)))))
}

// warningCount shows n in the warning color, as the other items show
// their counts; nil for none.
func warningCount(c *ui.Context, n int) func() {
	if n == 0 {
		return nil
	}
	return func() {
		ui.Text(c, strconv.Itoa(n)).FontSize(11).FontWeight(500).TextColor(Colors(c.Theme()).WarningText).SingleLine()
	}
}
