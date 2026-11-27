package ui

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

// commonRustTargets are popular cross-compilation targets offered in the
// quick-add menu beside the custom target input.
var commonRustTargets = []string{
	"wasm32-unknown-unknown",
	"wasm32-wasip1",
	"x86_64-unknown-linux-gnu",
	"aarch64-unknown-linux-gnu",
	"x86_64-pc-windows-msvc",
	"aarch64-apple-darwin",
	"x86_64-apple-darwin",
}

// rust is the Rust page: the tool's header with where it stands, the
// toolchains and other copies installed, how the terminal finds rustc,
// the channels available, rustup, the active toolchain's targets and
// the commands cargo install put in CARGO_HOME/bin.
func (a *App) rust(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() { a.rustInstallHeader(c) }})
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
		projs := a.toolProjects("rust")
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.rustHeader(c, ins, rows, active, projs)
				toolTabs(c, "rust-tabs", &a.rustTab, len(rows), len(projs))
				switch a.rustTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, rustBuild)
						if rustToolchains(ins, rows) > 0 {
							toolNote(c, locales.RustToolchainNote())
						}
						pathCard(c, "rustc", ins, active, a.Home)
					}, func() {
						a.rustAvailable(c, st, rows, active)
						a.managerCard(c, st, rows, active, nil)
						if tc, all, ok := a.selectedRustToolchain(ins, rows, active); ok {
							a.rustTargets(c, st, tc, all)
							a.rustComponents(c, st, tc)
						}
						if active >= 0 {
							a.managedGlobalsCard(c, "rust", locales.RustCargoTools(), locales.NoCargoTools(), ins[active].Packages, "cargo install")
						}
					})
				case 1:
					a.rustAvailable(c, st, rows, active)
				case 2:
					a.toolProjectsTab(c, "rust", st, projs)
				case 3:
					pathCard(c, "rustc", ins, active, a.Home)
				}
			})
		})
	})
}

// rustInstallHeader shows the header button that installs or updates the
// stable toolchain through rustup when rustup is available.
func (a *App) rustInstallHeader(c *ui.Context) {
	st, ok := a.toolState("rust")
	if !ok || a.installManager(st) != "rustup" {
		return
	}
	stableIdx := -1
	for i, in := range st.Installations {
		if rustChannel(in) == "stable" {
			stableIdx = i
			break
		}
	}
	upToDate := false
	if stableIdx >= 0 {
		upToDate = true
		if a.RustVersions != nil {
			if train, err := a.RustVersions.Train(); err == nil && rustVersionOlder(st.Installations[stableIdx].Version, train.Stable) {
				upToDate = false
			}
		}
	}
	installVersionButton(c, "rust", "stable", locales.InstallVersionVia("stable", "rustup"), upToDate, func() { a.installRustToolchain("stable") })
}

// rustVersionOlder reports whether installed version have is strictly older
// than available version want using semver comparison.
func rustVersionOlder(have, want string) bool {
	hv, wv := "v"+strings.TrimPrefix(have, "v"), "v"+strings.TrimPrefix(want, "v")
	if !semver.IsValid(hv) || !semver.IsValid(wv) {
		return have != "" && want != "" && have != want
	}
	return semver.Compare(hv, wv) < 0
}

// installRustToolchain proposes installing or updating a rustup toolchain.
func (a *App) installRustToolchain(toolchain string) {
	toolchain = strings.TrimSpace(toolchain)
	if toolchain == "" {
		return
	}
	if p, ok := a.versionPlan(ops.VersionInstall, "rust", "rustup", toolchain, toolchain, ""); ok {
		a.Propose(p)
	}
}

// proposeRustTarget proposes adding or removing a rustup target on toolchain.
func (a *App) proposeRustTarget(add bool, toolchain, target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	a.proposeOrSay(ops.RustTargetPlan(add, toolchain, target, a.detect("rust")))
}

// proposeRustComponent proposes adding or removing a rustup component on toolchain.
func (a *App) proposeRustComponent(add bool, toolchain, component string) {
	component = strings.TrimSpace(component)
	if component == "" {
		return
	}
	a.proposeOrSay(ops.RustComponentPlan(add, toolchain, component, a.detect("rust")))
}

// selectedRustToolchain returns the rustup toolchain selected for Targets and
// Components plus all installed rustup toolchains, without mutating App state.
func (a *App) selectedRustToolchain(ins []inventory.Installation, rows []int, active int) (inventory.RustToolchain, []inventory.RustToolchain, bool) {
	var all []inventory.RustToolchain
	for _, i := range rows {
		if ins[i].Rust != nil {
			all = append(all, *ins[i].Rust)
		}
	}
	if len(all) == 0 {
		return inventory.RustToolchain{}, nil, false
	}
	if a.rustToolchain != "" {
		for _, tc := range all {
			if tc.Name == a.rustToolchain {
				return tc, all, true
			}
		}
	}
	if active >= 0 && ins[active].Rust != nil {
		return *ins[active].Rust, all, true
	}
	return all[0], all, true
}

// rustHeader is Rust's logo, name and how its versions are managed, and
// the cards of where it stands: installed, in the terminal, in a project.
func (a *App) rustHeader(c *ui.Context, ins []inventory.Installation, rows []int, active int, projs []toolProject) {
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
		a.toolProjectCard(c, "rust", projs)
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
// the toolchains installed, plus rustup install/update actions when rustup
// is present.
func (a *App) rustAvailable(c *ui.Context, st inventory.ToolState, rows []int, active int) {
	ins := st.Installations
	pal := Colors(c.Theme())
	canRustup := a.installManager(st) == "rustup"
	asOf := ""
	if a.RustVersions != nil && !a.RustVersions.Fetched.IsZero() {
		asOf = locales.VersionsAsOf(a.RustVersions.Fetched)
	}
	aside := asOf
	if canRustup {
		aside = locales.Through("rustup")
	}
	listCard(c, locales.AvailableTitle(), aside, func() {
		if a.RustVersions == nil {
			text := locales.VersionsLoading()
			if a.RustVersionsErr != "" {
				text = locales.VersionsFailed(a.RustVersionsErr)
			}
			cardNote(c, text)
		} else if train, err := a.RustVersions.Train(); err != nil {
			cardNote(c, locales.VersionsFailed(err.Error()))
		} else {
			installed := func(channel string) int {
				for _, i := range rows {
					if rustChannel(ins[i]) == channel {
						return i
					}
				}
				return -1
			}
			stableIdx := installed("stable")
			caption, update, stableOlder := "", false, false
			if stableIdx >= 0 {
				caption = locales.NoteInstalled()
				stableOlder = rustVersionOlder(ins[stableIdx].Version, train.Stable)
				if stableOlder {
					caption, update = locales.NoteUpdateFor(ins[stableIdx].Version), stableIdx == active
				}
			}
			listRow(c, "stable "+train.Stable, true, caption, func() {
				if update {
					ui.Text(c, locales.TagRecommended()).FontSize(10.5).FontWeight(500).TextColor(pal.AccentInk).
						Padding(1, 6).Radius(5).Background(pal.AccentSoft).SingleLine()
				}
				if canRustup {
					if stableIdx < 0 {
						iconButton(c.Key("install:stable"), "download", locales.InstallNamed("stable")).
							OnClick(func() { a.installRustToolchain("stable") })
					} else if stableOlder {
						iconButton(c.Key("update:stable"), "download", locales.RustUpdateChannel("stable")).
							OnClick(func() { a.installRustToolchain("stable") })
					}
				}
			})

			betaIdx := installed("beta")
			betaOlder := false
			caption = locales.RustBetaNext()
			if days := train.NextStable.Sub(c.Now()).Hours() / 24; days > 0 {
				caption = locales.RustBetaWeeks(int(math.Ceil(days / 7)))
			}
			if betaIdx >= 0 {
				base, _, _ := strings.Cut(ins[betaIdx].Version, "-")
				if rustVersionOlder(base, train.Beta) {
					betaOlder = true
					caption = locales.NoteUpdateFor(ins[betaIdx].Version)
				} else {
					caption = locales.NoteInstalled()
				}
			}
			listRow(c, "beta "+train.Beta, true, caption, func() {
				if canRustup {
					if betaIdx < 0 {
						iconButton(c.Key("install:beta"), "download", locales.InstallNamed("beta")).
							OnClick(func() { a.installRustToolchain("beta") })
					} else if betaOlder {
						iconButton(c.Key("update:beta"), "download", locales.RustUpdateChannel("beta")).
							OnClick(func() { a.installRustToolchain("beta") })
					}
				}
			})

			nightlyIdx := installed("nightly")
			caption = locales.RustNightlyNote()
			if nightlyIdx >= 0 && ins[nightlyIdx].Rust.Date != "" {
				caption = locales.RustNightlyInstalled(ins[nightlyIdx].Rust.Date)
			}
			listRow(c, "nightly "+train.Nightly, true, caption, func() {
				if canRustup {
					label := locales.InstallNamed("nightly")
					if nightlyIdx >= 0 {
						label = locales.RustUpdateChannel("nightly")
					}
					iconButton(c.Key("install:nightly"), "download", label).
						OnClick(func() { a.installRustToolchain("nightly") })
				}
			})
		}
		if canRustup {
			install := func() {
				v := strings.TrimSpace(a.rustToolchainInput)
				if !ops.ValidRustupIdent(v) {
					return
				}
				a.rustToolchainInput = ""
				a.installRustToolchain(v)
			}
			ui.Row(c).Gap(8).Padding(8, 12, 10, 14).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.TextInput(c.Key("rust-toolchain-input"), &a.rustToolchainInput).
					Placeholder(locales.RustToolchainPlaceholder()).Height(28).Grow(1).MinWidth(0).
					OnChange(c.Invalidate).OnSubmit(install)
				v := strings.TrimSpace(a.rustToolchainInput)
				label := locales.InstallVersion()
				if v != "" {
					label = locales.InstallNamed(v)
				}
				ui.Button(c.Key("rust-toolchain-install"), locales.Install()).Label(label).
					Disabled(!ops.ValidRustupIdent(v)).Shrink(0).OnClick(install)
			})
		}
	})
}

// rustTargets lists the targets the toolchain tc can build for, its host
// marked, with add/remove actions when rustup is present.
func (a *App) rustTargets(c *ui.Context, st inventory.ToolState, tc inventory.RustToolchain, all []inventory.RustToolchain) {
	pal := Colors(c.Theme())
	canRustup := a.installManager(st) == "rustup"
	ch, _, _ := strings.Cut(tc.Name, "-")
	var available []string
	for _, target := range commonRustTargets {
		if !slices.Contains(tc.Targets, target) {
			available = append(available, target)
		}
	}
	aside := func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if len(all) > 1 {
				shortCount := map[string]int{}
				for _, item := range all {
					s, _, _ := strings.Cut(item.Name, "-")
					shortCount[s]++
				}
				labels := make([]string, len(all))
				values := make([]string, len(all))
				for i, item := range all {
					label, _, _ := strings.Cut(item.Name, "-")
					if label == "" || shortCount[label] > 1 {
						label = item.Name
					}
					labels[i] = label
					values[i] = item.Name
					if item.Name == tc.Name {
						a.rustToolchainLabel = label
					}
				}
				ui.Select(c.Key("rust-toolchain-select"), &a.rustToolchainLabel, labels).Width(130).Label(locales.RustSelectToolchain()).
					OnChange(func() {
						if i := slices.Index(labels, a.rustToolchainLabel); i >= 0 {
							a.rustToolchain = values[i]
						}
					})
			} else {
				ui.Text(c, "rustup target").Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			}
			if canRustup && len(available) > 0 {
				iconButton(c.Key("rust-target-menu"), "plus", locales.RustTargetAdd()).Menu(func(m *ui.Menu) {
					for _, target := range available {
						if m.Item(target).Chosen() {
							a.proposeRustTarget(true, tc.Name, target)
						}
					}
				})
			}
		})
	}
	listCardHeader(c, locales.RustTargets(ch), aside, func() {
		for _, target := range tc.Targets {
			host := strings.HasSuffix(tc.Name, "-"+target)
			caption := ""
			if host {
				caption = locales.RustHostTarget()
			}
			listRow(c, target, false, caption, func() {
				if canRustup && !host {
					iconButton(c.Key("remove-target:"+target), "trash-2", locales.RustTargetRemove(target)).
						OnClick(func() { a.proposeRustTarget(false, tc.Name, target) })
				}
			})
		}
		if len(tc.Targets) == 0 {
			cardNote(c, locales.RustNoTargets())
		}
		if canRustup {
			add := func() {
				v := strings.TrimSpace(a.rustTargetInput)
				if !ops.ValidRustupIdent(v) || slices.Contains(tc.Targets, v) {
					return
				}
				a.rustTargetInput = ""
				a.proposeRustTarget(true, tc.Name, v)
			}
			ui.Row(c).Gap(8).Padding(8, 12, 10, 14).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.TextInput(c.Key("rust-target-input"), &a.rustTargetInput).
					Placeholder(locales.RustTargetPlaceholder()).Height(28).Grow(1).MinWidth(0).
					OnChange(c.Invalidate).OnSubmit(add)
				v := strings.TrimSpace(a.rustTargetInput)
				label := locales.RustTargetAdd()
				if v != "" {
					label = locales.RustTargetAddTitle(v)
				}
				ui.Button(c.Key("rust-target-add"), locales.RustTargetAdd()).Label(label).
					Disabled(!ops.ValidRustupIdent(v) || slices.Contains(tc.Targets, v)).Shrink(0).OnClick(add)
			})
		}
	})
}

var commonRustComponents = []string{"clippy", "rustfmt", "rust-src", "rust-analyzer"}

// rustComponents lists common rustup components (clippy, rustfmt, rust-src,
// rust-analyzer) for tc with install/remove actions when rustup is present.
func (a *App) rustComponents(c *ui.Context, st inventory.ToolState, tc inventory.RustToolchain) {
	canRustup := a.installManager(st) == "rustup"
	ch, _, _ := strings.Cut(tc.Name, "-")
	listCard(c, locales.RustComponents(ch), "rustup component", func() {
		for _, comp := range commonRustComponents {
			installed := slices.Contains(tc.Components, comp)
			caption := ""
			if installed {
				caption = locales.NoteInstalled()
			}
			name := comp
			listRow(c, name, true, caption, func() {
				if !canRustup {
					return
				}
				if installed {
					iconButton(c.Key("remove-comp:"+tc.Name+":"+name), "trash-2", locales.RustComponentRemove(name)).
						OnClick(func() { a.proposeRustComponent(false, tc.Name, name) })
				} else {
					iconButton(c.Key("install-comp:"+tc.Name+":"+name), "download", locales.RustComponentInstall(name)).
						OnClick(func() { a.proposeRustComponent(true, tc.Name, name) })
				}
			})
		}
	})
}
