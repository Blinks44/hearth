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
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() { a.installHeader(c, "ruby", a.RubyVersions.Newest(c.Now())) }})
		st, ok := a.toolState("ruby")
		if !ok {
			a.toolEmpty(c, "ruby", "Ruby")
			return
		}
		ins := st.Installations
		rows := listed(ins)
		active := inventory.TerminalCopy(ins)
		projs := a.toolProjects("ruby")
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.rubyHeader(c, ins, rows, active, projs)
				toolTabs(c, "ruby-tabs", &a.rubyTab, len(rows), len(projs))
				switch a.rubyTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, a.rubyChannel(c))
						if slices.ContainsFunc(rows, func(i int) bool { return inventory.NeededBySystem(ins[i]) }) {
							toolNote(c, locales.RubySystemNote())
						}
						pathCard(c, "ruby", ins, active, a.Home)
					}, func() {
						a.rubyAvailable(c, st, rows, active)
						a.managerCard(c, st, rows, active, nil)
						a.rubyBundler(c, st, ins, active)
						if active >= 0 {
							a.managedGlobalsCard(c, "ruby", locales.RubyGems(copyVersion(ins[active])), locales.RubyNoGems(), ins[active].Packages, "gem install")
						}
					})
				case 1:
					a.rubyAvailable(c, st, rows, active)
				case 2:
					a.toolProjectsTab(c, "ruby", st, projs)
				case 3:
					pathCard(c, "ruby", ins, active, a.Home)
				}
			})
		})
	})
}

// rubyHeader is Ruby's logo, name and how its versions are managed,
// and the cards of where it stands: installed, in the terminal, in a
// project.
func (a *App) rubyHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int, projs []toolProject) {
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
		a.toolProjectCard(c, "ruby", projs)
	})
}

// rubyChannel marks versions whose security support has ended.
func (a *App) rubyChannel(c *ui.Context) func(inventory.Installation) (string, bool) {
	return func(in inventory.Installation) (string, bool) {
		if a.RubyVersions != nil {
			if l, ok := a.RubyVersions.Line(in.Version); ok && l.Ended(c.Now()) {
				return "", true
			}
		}
		return "", false
	}
}

// rubyAvailable lists the newest release of each line released and still
// supported, newest first, with how each stands to the copies installed.
func (a *App) rubyAvailable(c *ui.Context, st inventory.ToolState, rows []int, active int) {
	ins := st.Installations
	pal := Colors(c.Theme())
	asOf := ""
	if a.RubyVersions != nil && !a.RubyVersions.Fetched.IsZero() {
		asOf = locales.VersionsAsOf(a.RubyVersions.Fetched)
	}
	listCard(c, locales.AvailableTitle(), a.availableAside(st, asOf), func() {
		if a.RubyVersions == nil {
			text := locales.VersionsLoading()
			if a.RubyVersionsErr != "" {
				text = locales.VersionsFailed(a.RubyVersionsErr)
			}
			cardNote(c, text)
			a.customVersionRow(c, st, "3.4.1")
			return
		}
		now := c.Now()
		for _, l := range a.RubyVersions.Lines {
			if l.Latest == "" || l.Ended(now) || !l.Release.IsZero() && now.Before(l.Release) {
				continue
			}
			caption, update := goReleaseNote(l.Latest, ins, rows, active)
			listRow(c, l.Latest, true, caption, func() {
				if update {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
				a.installButton(c, st, l.Latest, l.Latest)
			})
		}
		a.customVersionRow(c, st, "3.4.1")
	})
}

// rubyBundler shows whether Bundler is on PATH and offers to install it
// when absent.
func (a *App) rubyBundler(c *ui.Context, st inventory.ToolState, ins []inventory.Installation, active int) {
	ver := ""
	if active >= 0 {
		for _, pkg := range ins[active].Packages {
			if pkg.Name == "bundler" {
				ver = pkg.Version
				break
			}
		}
	}
	listCard(c, "Bundler", ver, func() {
		if path := st.Vars["bundle"]; path != "" {
			listRow(c, "bundle", true, tilde(path, a.Home), func() {
				a.pathActions(c, "ruby-bundle", path)
			})
			return
		}
		if a.canInstallTool("bundler") {
			listRow(c, "Bundler", false, locales.NotInPath(), func() { a.installToolIconButton(c, "bundler") })
			return
		}
		cardNote(c, locales.RubyNoBundler())
	})
}
