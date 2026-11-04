package ui

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// rust is the Rust page: the tool's header with where it stands, the
// toolchains and other copies installed, how the terminal finds rustc,
// the channels available, rustup, the active toolchain's targets and
// the commands cargo install put in CARGO_HOME/bin.
func (a *App) rust(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		st, ok := a.toolState("rust")
		if !ok {
			a.toolEmpty(c, "rust", "Rust")
			return
		}
		ins := st.Installations
		// rustup's toolchains first, as the terminal finds them through
		// its proxies; other copies after.
		rows := listed(ins)
		slices.SortStableFunc(rows, func(i, j int) int { return cmp.Compare(rustOrder(ins[i]), rustOrder(ins[j])) })
		active := inventory.TerminalCopy(ins)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				rustHeader(c, ins, rows, active)
				toolTabs(c, "rust-tabs", &a.rustTab, len(rows))
				switch a.rustTab {
				case 0:
					ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
							a.versionTable(c, ins, rows, active, rustBuild)
							if rustToolchains(ins, rows) > 0 {
								toolNote(c, locales.RustToolchainNote())
							}
							pathCard(c, "rustc", ins, active, a.Home)
						})
						ui.Column(c).Width(342).Shrink(0).Gap(14).Children(func() {
							a.rustAvailable(c, ins, rows, active)
							managerCard(c, st, rows, active, nil)
							if active >= 0 && ins[active].Rust != nil {
								rustTargets(c, *ins[active].Rust)
							}
							if active >= 0 {
								globalsCard(c, locales.RustCargoTools(), locales.NoCargoTools(), ins[active].Packages, "cargo install")
							}
						})
					})
				case 1:
					a.rustAvailable(c, ins, rows, active)
				case 3:
					pathCard(c, "rustc", ins, active, a.Home)
				default:
					comingSoon(c)
				}
			})
		})
	})
}

// rustHeader is Rust's logo, name and how its versions are managed, and
// the cards of where it stands: installed, in the terminal, in a project.
func rustHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	t := c.Theme()
	pal := Colors(t)
	toolHead(c, "rust", "Rust", func() {}, managedBy(ins, rows, active), func() {
		installed := locales.CopiesCount(len(rows))
		if n := rustToolchains(ins, rows); n > 0 {
			installed = locales.RustToolchainsCount(n)
		}
		stateCard(c, pal.TextFaint, locales.Installed(), installed, strings.Join(sourceNames(ins, rows, active), " · "))
		if active >= 0 {
			via := sourceName(ins[active].Source)
			if first := pathOrder(ins); len(first) > 0 && ins[first[0]].Source.Shim {
				via = ins[first[0]].Source.Manager
			}
			stateCard(c, t.Success, locales.InTerminal(), copyVersion(ins[active]), "rustc → "+via)
		} else {
			stateCard(c, pal.TextFaint, locales.InTerminal(), "—", locales.NotInPath())
		}
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
	})
}

// rustOrder puts rustup's toolchains before other copies.
func rustOrder(in inventory.Installation) int {
	if in.Rust != nil {
		return 0
	}
	return 1
}

// rustToolchains counts the copies at rows that are rustup's toolchains.
func rustToolchains(ins []inventory.Installation, rows []int) int {
	n := 0
	for _, i := range rows {
		if ins[i].Rust != nil {
			n++
		}
	}
	return n
}

// rustChannel is the channel a rustup toolchain follows, as stable of
// stable-aarch64-apple-darwin; "" for one pinned to a version, or a copy
// rustup does not keep.
func rustChannel(in inventory.Installation) string {
	if in.Rust == nil {
		return ""
	}
	ch, _, _ := strings.Cut(in.Rust.Name, "-")
	switch ch {
	case "stable", "beta", "nightly":
		return ch
	}
	return ""
}

// rustBuild tells a nightly toolchain by the day it was built.
func rustBuild(in inventory.Installation) (string, bool) {
	if rustChannel(in) == "nightly" && in.Rust.Date != "" {
		return in.Rust.Date, false
	}
	return "", false
}

// rustAvailable lists what each channel ships now, with how it stands to
// the toolchains installed.
func (a *App) rustAvailable(c *ui.Context, ins []inventory.Installation, rows []int, active int) {
	pal := Colors(c.Theme())
	asOf := ""
	if a.RustVersions != nil {
		asOf = locales.VersionsAsOf(a.RustVersions.Fetched)
	}
	listCard(c, locales.AvailableTitle(), asOf, func() {
		if a.RustVersions == nil {
			text := locales.VersionsLoading()
			if a.RustVersionsErr != "" {
				text = locales.VersionsFailed(a.RustVersionsErr)
			}
			cardNote(c, text)
			return
		}
		train, err := a.RustVersions.Train()
		if err != nil {
			cardNote(c, locales.VersionsFailed(err.Error()))
			return
		}
		installed := func(channel string) int {
			for _, i := range rows {
				if rustChannel(ins[i]) == channel {
					return i
				}
			}
			return -1
		}
		caption, update := "", false
		if i := installed("stable"); i >= 0 {
			caption = locales.NoteInstalled()
			if ins[i].Version != train.Stable {
				caption, update = locales.NoteUpdateFor(ins[i].Version), i == active
			}
		}
		listRow(c, "stable "+train.Stable, true, caption, func() {
			if update {
				ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
					Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
			}
		})
		caption = locales.RustBetaNext()
		if days := train.NextStable.Sub(c.Now()).Hours() / 24; days > 0 {
			caption = locales.RustBetaWeeks(int(math.Ceil(days / 7)))
		}
		listRow(c, "beta "+train.Beta, true, caption, func() {})
		caption = locales.RustNightlyNote()
		if i := installed("nightly"); i >= 0 && ins[i].Rust.Date != "" {
			caption = locales.RustNightlyInstalled(ins[i].Rust.Date)
		}
		listRow(c, "nightly "+train.Nightly, true, caption, func() {})
	})
}

// rustTargets lists the targets the toolchain tc can build for, its host
// marked.
func rustTargets(c *ui.Context, tc inventory.RustToolchain) {
	ch, _, _ := strings.Cut(tc.Name, "-")
	listCard(c, locales.RustTargets(ch), "rustup target", func() {
		for _, target := range tc.Targets {
			caption := ""
			if strings.HasSuffix(tc.Name, "-"+target) {
				caption = locales.RustHostTarget()
			}
			listRow(c, target, false, caption, func() {})
		}
		if len(tc.Targets) == 0 {
			cardNote(c, locales.RustNoTargets())
		}
	})
}
