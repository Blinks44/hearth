package ui

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// bun is the Bun page: the tool's header with where it stands, the copies
// installed, how the terminal finds bun, the newest version, how the
// script installed it and the global packages.
func (a *App) bun(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.toolState("bun")
		if !ok {
			a.toolEmpty(c, "bun", "Bun")
			return
		}
		ins := st.Installations
		rows := listed(ins)
		active := inventory.TerminalCopy(ins)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.bunHeader(c, ins, rows, active)
				toolTabs(c, "bun-tabs", &a.bunTab, len(rows))
				switch a.bunTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, func(in inventory.Installation) (string, bool) { return bunChannel(in.Version) })
						if note := bunNote(ins, rows); note != "" {
							toolNote(c, note)
						}
						pathCard(c, "bun", ins, active, a.Home)
					}, func() {
						a.bunAvailable(c, ins, rows, active)
						if active >= 0 {
							a.bunInstall(c, ins[active])
							globalsCard(c, locales.GlobalPackages(), locales.NoGlobalPackages(), ins[active].Packages, "bun add -g")
						}
					})
				case 1:
					a.bunAvailable(c, ins, rows, active)
				case 3:
					pathCard(c, "bun", ins, active, a.Home)
				default:
					comingSoon(c)
				}
			})
		})
	})
}

// bunHeader is Bun's logo, name and how it was installed, and the cards
// of where it stands: installed, in the terminal, in a project.
func (a *App) bunHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	t := c.Theme()
	pal := Colors(t)
	subtitle := managedBy(ins, rows, active)
	if active >= 0 && ins[active].Source.Manager == inventory.ScriptInstall {
		subtitle = locales.BunScriptInstalled(tilde(ins[active].Source.Root, a.Home))
		if rest := sourceNames(ins, rows, active)[1:]; len(rest) > 0 {
			subtitle += " · " + locales.AlsoFrom(strings.Join(rest, ", "))
		}
	}
	toolHead(c, "bun", "Bun", func() {}, subtitle, func() {
		stateCard(c, pal.TextFaint, locales.Installed(), locales.CopiesCount(len(rows)),
			strings.Join(sourceNames(ins, rows, -1), " · "))
		if active >= 0 {
			in := ins[active]
			where := sourceName(in.Source)
			if in.Source.Manager == inventory.ScriptInstall {
				where = tilde(in.Source.Root, a.Home)
			}
			stateCard(c, t.Success, locales.InTerminal(), versionText(in.Version), whichCommand+" bun → "+where)
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
	})
}

// bunChannel tells a Bun version's channel: canary builds name it, the
// rest are stable. Bun's support does not end by line.
func bunChannel(version string) (string, bool) {
	switch {
	case version == "":
		return "", false
	case strings.Contains(version, "canary"):
		return "canary", false
	}
	return locales.ChannelStable(), false
}

// bunNote explains how the copies at rows are updated and removed: the
// script's by bun upgrade, npm's with the Node version it lies in.
func bunNote(ins []inventory.Installation, rows []int) string {
	var notes []string
	if slices.ContainsFunc(rows, func(i int) bool { return ins[i].Source.Manager == inventory.ScriptInstall }) {
		notes = append(notes, locales.BunUpgradeNote())
	}
	if slices.ContainsFunc(rows, func(i int) bool { return ins[i].Source.Manager == "npm" }) {
		notes = append(notes, locales.BunNpmNote())
	}
	return strings.Join(notes, " ")
}

// bunAvailable shows Bun's newest stable version, as the npm registry
// says, and how it stands to the copy in use; and the canary channel.
func (a *App) bunAvailable(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	pal := Colors(c.Theme())
	latest := a.BunLatest
	asOf := ""
	if latest != nil && !latest.Fetched.IsZero() {
		asOf = locales.VersionsAsOf(latest.Fetched)
	}
	listCard(c, locales.Available(), asOf, func() {
		if latest == nil {
			text := locales.VersionsLoading()
			if a.BunLatestErr != "" {
				text = locales.VersionsFailed(a.BunLatestErr)
			}
			cardNote(c, text)
			return
		}
		caption := locales.ChannelStable()
		update := false
		switch {
		case slices.ContainsFunc(rows, func(i int) bool { return ins[i].Version == latest.Version }):
			caption += " · " + locales.NoteInstalled()
		case active >= 0 && ins[active].Version != "" &&
			semver.Compare("v"+ins[active].Version, "v"+latest.Version) < 0:
			caption += " · " + locales.NoteUpdateFor(versionText(ins[active].Version))
			update = true
		}
		listRow(c, versionText(latest.Version), true, caption, func() {
			if update {
				ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
					Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
			}
		})
		listRow(c, "canary", true, locales.BunCanary(), func() {})
	})
}

// bunInstall shows where Bun's install script put the copy in use and how
// the terminal finds it; nothing for a copy from elsewhere.
func (a *App) bunInstall(c *ui.Context, in inventory.Installation) {
	if in.Source.Manager != inventory.ScriptInstall {
		return
	}
	pal := Colors(c.Theme())
	value := func(s string) func() {
		return func() {
			ui.Text(c, s).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
		}
	}
	listCard(c, locales.InstallScriptTitle(), "bun.sh/install", func() {
		listRow(c, locales.InstallFolder(), false, "BUN_INSTALL", value(tilde(in.Source.Root, a.Home)))
		caption := ""
		if in.PathFile != "" {
			caption = locales.PathLineIn(tilde(in.PathFile, a.Home))
		}
		listRow(c, locales.AddedToPath(), false, caption, value(tilde(filepath.Dir(in.Path), a.Home)))
	})
}
