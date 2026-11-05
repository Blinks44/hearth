package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// ruby is the Ruby page: the tool's header with where it stands, the
// copies installed, how the terminal finds ruby, the versions
// available, the version manager and gems.
func (a *App) ruby(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.toolState("ruby")
		if !ok {
			a.toolEmpty(c, "ruby", "Ruby")
			return
		}
		ins := st.Installations
		rows := listed(ins)
		active := inventory.TerminalCopy(ins)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				rubyHeader(c, ins, rows, active)
				toolTabs(c, "ruby-tabs", &a.rubyTab, len(rows))
				switch a.rubyTab {
				case 0:
					ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
							a.versionTable(c, ins, rows, active, a.rubyChannel(c))
							if slices.ContainsFunc(rows, func(i int) bool { return inventory.NeededBySystem(ins[i]) }) {
								toolNote(c, locales.RubySystemNote())
							}
							pathCard(c, "ruby", ins, active, a.Home)
						})
						ui.Column(c).Width(342).Shrink(0).Gap(14).Children(func() {
							a.rubyAvailable(c, ins, rows, active)
							a.managerCard(c, st, rows, active, nil)
							if active >= 0 {
								globalsCard(c, locales.RubyGems(copyVersion(ins[active])), locales.RubyNoGems(), ins[active].Packages, "gem install")
							}
						})
					})
				case 1:
					a.rubyAvailable(c, ins, rows, active)
				case 3:
					pathCard(c, "ruby", ins, active, a.Home)
				default:
					comingSoon(c)
				}
			})
		})
	})
}

// rubyHeader is Ruby's logo, name and how its versions are managed,
// and the cards of where it stands: installed, in the terminal, in a
// project.
func rubyHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	t := c.Theme()
	pal := Colors(t)
	toolHead(c, "ruby", "Ruby", func() {}, managedBy(ins, rows, active), func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.CopiesCount(len(rows)),
			strings.Join(sourceNames(ins, rows, -1), " · "))
		if active >= 0 {
			in := ins[active]
			stateCard(c, t.Success, locales.InTerminal(), copyVersion(in), "ruby → "+sourceName(in.Source))
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
	})
}

// rubyChannel marks versions whose security support has ended.
func (a *App) rubyChannel(c *ui.Context) func(inventory.Installation) (string, bool) {
	return func(in inventory.Installation) (string, bool) {
		if a.RubyVersions != nil {
			if l, ok := a.RubyVersions.Line(in.Version); ok && !l.End.IsZero() && !c.Now().Before(l.End) {
				return "", true
			}
		}
		return "", false
	}
}

// rubyAvailable lists the newest release of each line released and still
// supported, newest first, with how each stands to the copies installed.
func (a *App) rubyAvailable(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	pal := Colors(c.Theme())
	asOf := ""
	if a.RubyVersions != nil {
		asOf = locales.VersionsAsOf(a.RubyVersions.Fetched)
	}
	listCard(c, locales.AvailableTitle(), asOf, func() {
		if a.RubyVersions == nil {
			text := locales.VersionsLoading()
			if a.RubyVersionsErr != "" {
				text = locales.VersionsFailed(a.RubyVersionsErr)
			}
			cardNote(c, text)
			return
		}
		now := c.Now()
		for _, l := range a.RubyVersions.Lines {
			if l.Latest == "" || !l.End.IsZero() && !now.Before(l.End) || !l.Release.IsZero() && now.Before(l.Release) {
				continue
			}
			caption, update := goReleaseNote(l.Latest, ins, rows, active)
			listRow(c, l.Latest, true, caption, func() {
				if update {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
			})
		}
	})
}
