package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/history"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// changesPage is "What changed": the history of the checks. It is under
// the overview, which the sidebar keeps chosen.
var changesPage = page{"/overview/changes", "lucide/history", locales.ChangesTitle, locales.ChangesSubtitle}

// shownChanges are an entry's changes as the page lists them: the
// terminal's new version goes with its copy's update when they tell the
// same.
func shownChanges(e history.Entry) []history.Change {
	return slices.DeleteFunc(slices.Clone(e.Changes), func(ch history.Change) bool {
		return ch.Area == history.Terminal && slices.ContainsFunc(e.Changes, func(o history.Change) bool {
			return o.Area == history.Copy && o.Kind == history.Updated && o.Name == ch.Name && o.From == ch.From && o.To == ch.To
		})
	})
}

func (a *App) changesView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{
			backLabel: locales.BackToOverview(),
			back:      func() { a.router.Push("/") },
		})
		t := c.Theme()
		pal := Colors(t)
		count := map[history.Kind]int{}
		for _, e := range a.History {
			for _, ch := range shownChanges(e) {
				count[ch.Kind]++
			}
		}
		total := count[history.Added] + count[history.Updated] + count[history.Removed]
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Info.Alpha(0.12)).Center().Children(func() {
						ui.Icon(c, Icon("history")).Size(26, 26).TextColor(pal.Info)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, locales.ChangesTitle()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.ChangesLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					since := "—"
					if n := len(a.History); n > 0 {
						since = locales.ChangesSince(a.History[n-1].Taken)
					}
					stateCard(c, t.Success, locales.ChangesAdded(), strconv.Itoa(count[history.Added]), since)
					stateCard(c, pal.Info, locales.ChangesUpdated(), strconv.Itoa(count[history.Updated]), locales.ChangesUpdatedCaption())
					stateCard(c, t.Danger, locales.ChangesRemoved(), strconv.Itoa(count[history.Removed]), locales.ChangesRemovedCaption())
				})
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				kinds := []history.Kind{history.Added, history.Updated, history.Removed}
				toolbar.Children(func() {
					segments(c, "changes-tabs", &a.changesTab, []segment{
						{locales.TabAll(), strconv.Itoa(total)},
						{locales.ChangesAdded(), strconv.Itoa(count[history.Added])},
						{locales.ChangesUpdated(), strconv.Itoa(count[history.Updated])},
						{locales.ChangesRemoved(), strconv.Itoa(count[history.Removed])},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("changes-filter"), &a.changesFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Height(controlHeight).Width(200)
				})
				columns(c, 12, func() {
					if len(a.History) == 0 {
						toolNote(c, locales.ChangesEmpty())
						return
					}
					filter := strings.ToLower(a.changesFilter)
					ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
						shown := 0
						for i, e := range a.History {
							var rows []history.Change
							for _, ch := range shownChanges(e) {
								title, caption := a.changeText(ch)
								if (a.changesTab == 0 || kinds[a.changesTab-1] == ch.Kind) && strings.Contains(strings.ToLower(title+" "+caption), filter) {
									rows = append(rows, ch)
								}
							}
							if len(rows) == 0 {
								continue
							}
							ui.Row(c.Key("entry"+strconv.Itoa(i))).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
								ui.Text(c, strings.ToUpper(checkDate(c, e.Taken))).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
								ui.Text(c, locales.ChangesCount(len(rows))).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
							})
							for j, ch := range rows {
								shown++
								a.changeRow(c.Key("change"+strconv.Itoa(i)+"-"+strconv.Itoa(j)), ch)
							}
						}
						if shown == 0 {
							cardNote(c, locales.ChangesNone())
						}
					})
				}, func() {
					a.triesCard(c)
					listCard(c, locales.ChangesKept(), "", func() {
						listRow(c, locales.ChangesKeptWhat(), false, locales.ChangesKeptWhatCaption(), func() {})
						listRow(c, locales.ChangesKeptNoFiles(), false, locales.ChangesKeptNoFilesCaption(), func() {})
						listRow(c, locales.ChangesKeptLast(history.Limit), false, locales.ChangesKeptLastCaption(), func() {})
					})
				})
			})
		})
	})
}

// changeRow is a change in the table: a dot of its kind, what changed, and
// the space or a mark. Clicking opens the page the change belongs to.
func (a *App) changeRow(c *ui.Context, ch history.Change) {
	t := c.Theme()
	pal := Colors(t)
	dot, mark := t.Success, "+"
	switch ch.Kind {
	case history.Updated:
		dot, mark = pal.Info, "↑"
	case history.Removed:
		dot, mark = t.Danger, "−"
	}
	if ch.Area == history.Copy {
		var counted *inventory.Usage
		if a.Sizes != nil {
			if u, ok := a.Sizes.Paths[ch.Root]; ok {
				counted = &u
			}
		}
		switch {
		case ch.Kind == history.Added && counted != nil:
			mark = "+ " + formatBytes(counted.Bytes)
		case ch.Kind == history.Removed && ch.Bytes > 0:
			mark = "− " + formatBytes(ch.Bytes)
		}
	}
	title, caption := a.changeText(ch)
	open := a.changeAction(ch)
	content := func(chevron ui.Color) {
		ui.Box(c).Size(8, 8).Radius(4).Shrink(0).Background(dot)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, title).FontSize(13)
			clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint)
		})
		ui.Text(c, mark).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
		if open != nil {
			ui.Icon(c, Icon("chevron-right")).Size(13, 13).Shrink(0).TextColor(chevron)
		}
	}
	if open == nil {
		ui.Row(c).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
			content(pal.TextFaint)
		})
		return
	}
	row := ui.ButtonBase(c).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Label(title).OnClick(open).Transition(colorFade)
	chevron := pal.TextFaint
	if row.Hovered() {
		row.Background(pal.Subtle)
		chevron = Accent
	}
	row.Children(func() { content(chevron) })
}

// changeAction returns the navigation handler for a history change row, or nil
// when there is no matching destination page.
func (a *App) changeAction(ch history.Change) func() {
	switch ch.Area {
	case history.Copy, history.Terminal:
		if _, path := a.toolCardTarget(ch.Name); path != "" {
			return func() { a.router.Push(path) }
		}
	case history.Packages:
		return func() {
			for _, m := range a.packageManagers() {
				if m.ID == ch.Name || m.Name == ch.Manager {
					a.packageManager = m.Path
					break
				}
			}
			a.packageTab = 0
			if len(ch.Names) == 1 {
				a.packageFilter = ch.Names[0]
			} else {
				a.packageFilter = ""
			}
			a.router.Push("/system/packages")
		}
	case history.PathEntry:
		return func() {
			a.pathTab = 0
			a.pathFilter = ch.Path
			a.router.Push(pathPage)
		}
	case history.Agent:
		if ch.Name != "" {
			return func() { a.router.Push(agentPath(ch.Name)) }
		}
		return func() { a.router.Push("/ai/agents") }
	case history.Skill:
		return func() {
			a.skillAgent, a.skillQuery, a.skillScope = "", "", 0
			if i := slices.IndexFunc(a.allSkills(), func(sk inventory.Skill) bool { return sk.Name == ch.Name && len(sk.Paths) > 0 }); i >= 0 {
				sk := a.allSkills()[i]
				a.skillChosen = skillKey(sk)
				a.loadSkill(sk)
			} else {
				a.skillQuery = ch.Name
			}
			a.router.Push(skillsPage)
		}
	case history.MCP:
		return func() {
			a.mcpAgent, a.mcpQuery, a.mcpScope = "", "", 0
			items := a.mcpItems()
			if i := slices.IndexFunc(items, func(m mcpItem) bool { return m.name == ch.Name }); i >= 0 {
				a.mcpChosen = items[i].key()
			} else {
				a.mcpQuery = ch.Name
			}
			a.router.Push(mcpPage)
		}
	}
	return nil
}

// changeText says what changed and where, in plain words.
func (a *App) changeText(ch history.Change) (title, caption string) {
	switch ch.Area {
	case history.Copy:
		source := ch.Manager
		if source == "" {
			source = locales.SourceInstaller()
		}
		folder := tilde(ch.Root, a.Home)
		if ch.Root == "" {
			folder = tilde(filepath.Dir(ch.Path), a.Home)
		}
		switch ch.Kind {
		case history.Added:
			return strings.TrimSpace(toolName(ch.Name)+" "+ch.To) + " · " + source, locales.ChangeCopyAdded(folder)
		case history.Updated:
			return toolName(ch.Name) + " " + ch.From + " → " + ch.To + " · " + source, locales.ChangeCopyUpdated(folder)
		}
		return strings.TrimSpace(toolName(ch.Name)+" "+ch.From) + " · " + source, locales.ChangeCopyRemoved()
	case history.Terminal:
		return toolName(ch.Name) + " " + ch.From + " → " + ch.To, locales.ChangeTerminal()
	case history.Packages:
		names := ch.Names
		if len(names) > 4 {
			names = append(slices.Clone(names[:4]), "…")
		}
		manager := ch.Manager
		if manager == "" {
			manager = ch.Name
		}
		list := strings.Join(names, ", ")
		switch ch.Kind {
		case history.Added:
			return locales.ChangePackagesAdded(manager, len(ch.Names)), list
		case history.Updated:
			return locales.ChangePackagesUpdated(manager, len(ch.Names)), list
		}
		return locales.ChangePackagesRemoved(manager, len(ch.Names)), list
	case history.PathEntry:
		if ch.Kind == history.Added {
			caption := tilde(ch.Path, a.Home)
			if ch.Name != "" {
				caption = locales.ChangePathFrom(caption, tilde(ch.Name, a.Home))
			}
			return locales.ChangePathAdded(), caption
		}
		return locales.ChangePathRemoved(), tilde(ch.Path, a.Home)
	case history.Agent:
		switch ch.Kind {
		case history.Added:
			return strings.TrimSpace(agentTitle(ch.Name) + " " + ch.To), locales.ChangeAgentAdded()
		case history.Updated:
			return agentTitle(ch.Name) + " " + ch.From + " → " + ch.To, locales.ChangeAgentUpdated()
		}
		return agentTitle(ch.Name), locales.ChangeAgentRemoved()
	case history.Skill:
		title := locales.ChangeSkillTitle(ch.Name)
		switch ch.Kind {
		case history.Added:
			return title, locales.ChangeSkillAdded(tilde(ch.Path, a.Home))
		case history.Updated:
			return title, locales.ChangeSkillUpdated()
		}
		return title, locales.ChangeSkillRemoved(tilde(ch.Path, a.Home))
	case history.MCP:
		title := "MCP " + ch.Name + " · " + agentTitle(ch.Agent)
		if ch.Kind == history.Added {
			return title, locales.ChangeMcpAdded(tilde(ch.Path, a.Home))
		}
		return title, locales.ChangeMcpRemoved(tilde(ch.Path, a.Home))
	}
	return ch.Name, ""
}

// triesCard lists the cleanup's candidates that came in the history: a
// version installed to try, and left.
func (a *App) triesCard(c *ui.Context) {
	added := map[string]time.Time{}
	for _, e := range slices.Backward(a.History) {
		for _, ch := range e.Changes {
			if ch.Area == history.Copy && ch.Kind == history.Added && ch.Root != "" {
				added[ch.Root] = e.Taken
			}
		}
	}
	listCard(c, locales.ChangesTries(), "", func() {
		n := 0
		for _, cand := range a.candidates(c) {
			if cand.Kind != cleanup.ExtraCopy && cand.Kind != cleanup.Leftover {
				continue
			}
			var when time.Time
			for _, p := range cand.Paths {
				if t, ok := added[p]; ok && t.After(when) {
					when = t
				}
			}
			if when.IsZero() {
				continue
			}
			n++
			title := a.candidateTitle(cand)
			if cand.Known {
				title += " · " + formatBytes(cand.Frees())
			}
			listRow(c, title, false, locales.ChangesTryCaption(when), func() {})
		}
		if n == 0 {
			cardNote(c, locales.ChangesNoTries())
			return
		}
		pal := Colors(c.Theme())
		open := ui.ButtonBase(c.Key("open-cleanup")).Column().AlignItems(ui.Start).Gap(1).Padding(9, 12, 9, 14).
			BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(locales.ChangesOpenCleanup()).OnClick(func() { a.router.Push(cleanupPage) })
		if open.Hovered() {
			open.Background(pal.Subtle)
		}
		open.Children(func() {
			ui.Text(c, locales.ChangesOpenCleanup()).FontSize(13).TextColor(Accent).SingleLine()
			ui.Text(c, locales.ChangesOpenCleanupCaption()).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
		})
	})
}

// checkDate names when a check was: "Today, 10:42", "Yesterday, 18:20",
// or the date and time.
func checkDate(c *ui.Context, t time.Time) string {
	return formatCheckDate(c.Now(), t)
}

func formatCheckDate(now, t time.Time) string {
	loc := now.Location()
	local := t.In(loc)
	day := func(ts time.Time) time.Time { y, m, d := ts.Date(); return time.Date(y, m, d, 0, 0, 0, 0, loc) }
	today := day(now)
	switch thatDay := day(local); {
	case thatDay.Equal(today):
		return locales.ChangesToday(local)
	case thatDay.Equal(today.AddDate(0, 0, -1)):
		return locales.ChangesYesterday(local)
	}
	return locales.ChangesOn(local)
}

// changesSince is what the last check changed, for the overview's link;
// 0 without a history.
func (a *App) changesSince() int {
	if len(a.History) == 0 {
		return 0
	}
	return len(shownChanges(a.History[0]))
}
