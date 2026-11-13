package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

var jsManagers = []string{"npm", "pnpm", "yarn"}

// jsTool reports that the tool id is shown on the package managers page.
func jsTool(id string) bool { return id == "corepack" || slices.Contains(jsManagers, id) }

// jspm shows JavaScript package managers, their local Corepack cache and
// the managers requested by project manifests and lockfiles.
func (a *App) jspm(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				toolHead(c, "pnpm", "npm · pnpm · Yarn", func() {}, locales.JsManagersSubtitle(), func() {
					for _, id := range jsManagers {
						st, _ := a.Snapshot.Tool(id)
						active := inventory.TerminalCopy(st.Installations)
						value, caption := "—", locales.NotInPath()
						dot := Colors(c.Theme()).TextFaint
						if active >= 0 {
							in := st.Installations[active]
							value, caption = versionText(in.Version), sourceName(in.Source)
							dot = c.Theme().Success
						} else if slices.ContainsFunc(st.Installations, func(in inventory.Installation) bool {
							return in.OnPath == 1 && in.Source.Shim && in.Source.Manager == "corepack"
						}) {
							// Corepack's shim with nothing cached picks the version per project.
							value, caption = locales.JsCorepackShim(), locales.JsShimVersionCaption()
							dot = c.Theme().Success
						}
						stateCard(c, dot, id, value, caption)
					}
				})
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					segments(c, "jspm-tabs", &a.jspmTab, []segment{{locales.JsManagers(), strconv.Itoa(a.jsCount())}, {locales.GlobalPackages(), ""}, {locales.ProjectsTitle(), strconv.Itoa(len(a.Projects))}, {"PATH", ""}})
					ui.Box(c).Grow(1)
					if a.jspmTab == 0 {
						ui.SearchField(c.Key("jspm-filter"), &a.jspmFilter).Bind(&a.filterField).Placeholder(locales.JsFilter()).Width(200)
					}
				})
				var side func()
				if a.jspmTab == 0 {
					side = func() { a.jsUpdates(c); a.jsCorepack(c); a.jsGlobals(c) }
				}
				columns(c, 12, func() {
					switch a.jspmTab {
					case 0:
						a.jsTable(c)
						if a.corepackInstalled() {
							toolNote(c, locales.JsCorepackNote())
						}
						a.jsProjects(c)
					case 1:
						a.jsGlobals(c)
					case 2:
						a.jsProjects(c)
					case 3:
						for _, id := range jsManagers {
							st, _ := a.Snapshot.Tool(id)
							pathCard(c, id, st.Installations, inventory.TerminalCopy(st.Installations), a.Home)
						}
					}
				}, side)
			})
		})
	})
}

func (a *App) jsCount() int {
	n := 0
	for _, id := range jsManagers {
		st, _ := a.Snapshot.Tool(id)
		// As the table: Corepack's shims are rows, other shims are not.
		for _, in := range st.Installations {
			if !in.Source.Shim || in.Source.Manager == "corepack" {
				n++
			}
		}
	}
	return n
}

func (a *App) jsTable(c *ui.Context) {
	pal := Colors(c.Theme())
	// The projects each copy runs in, as the terminal there picks it.
	usedBy := map[string][]string{}
	for _, p := range a.Projects {
		for _, st := range projects.Resolve(p, a.Snapshot, a.nodeLines()) {
			if st.Terminal != nil && slices.Contains(jsManagers, st.Tool) {
				key := st.Tool + "\x00" + st.Terminal.Path
				usedBy[key] = append(usedBy[key], filepath.Base(p.Dir))
			}
		}
	}
	listCard(c, locales.JsManagers(), locales.ColumnPath(), func() {
		count := 0
		for _, id := range jsManagers {
			st, checked := a.Snapshot.Tool(id)
			if st.Err != "" {
				cardNote(c, locales.ToolCheckFailed(id, st.Err))
				continue
			}
			if !checked {
				cardNote(c, id+" · "+locales.NotChecked())
				continue
			}
			for _, in := range st.Installations {
				if in.Source.Shim && in.Source.Manager != "corepack" {
					continue
				}
				text := id + " " + versionText(in.Version)
				if in.Source.Shim {
					text = id
				}
				if q := strings.ToLower(strings.TrimSpace(a.jspmFilter)); q != "" && !strings.Contains(strings.ToLower(text+" "+in.Path+" "+in.Source.Manager), q) {
					continue
				}
				count++
				ui.Row(c).Gap(12).Padding(12, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Width(132).Shrink(0).Gap(3).Children(func() {
						ui.Text(c, text).Font(FontMono).FontSize(12).SingleLine()
						if in.Source.Shim {
							ui.Text(c, locales.JsCorepackShim()).FontSize(11).TextColor(pal.TextFaint).SingleLine()
						}
					})
					ui.Row(c).Width(100).Shrink(0).Children(func() { sourceBadge(c, in.Source) })
					ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
						clipped(c, tilde(in.Path, a.Home)).Font(FontMono).FontSize(11.5).TextColor(c.Theme().TextMuted)
						ui.Row(c).Gap(5).Wrap().Children(func() {
							if in.OnPath > 0 {
								chip(c, c.Theme().Success, pal.SuccessSoft, pal.SuccessText, locales.ChipInPath())
							}
							for _, name := range usedBy[id+"\x00"+in.Path] {
								chip(c, c.Theme().Accent, pal.AccentSoft, pal.AccentInk, name)
							}
						})
					})
					a.rowActions(c, in)
				})
			}
		}
		if count == 0 {
			cardNote(c, locales.JsNoManagers())
			ui.Row(c).Padding(0, 14, 12, 14).Children(func() { a.installPrimary(c, "node") })
		}
		for _, id := range []string{"pnpm", "yarn"} {
			if a.canInstallTool(id) {
				listRow(c, locales.ToolNotInstalled(setupTitle(id)), false, "", func() { a.installToolButton(c, id) })
			}
		}
	})
}

func (a *App) jsCorepack(c *ui.Context) {
	st, _ := a.Snapshot.Tool("corepack")
	aside := "—"
	if len(st.Installations) > 0 {
		aside = versionText(st.Installations[0].Version)
	}
	listCard(c, "Corepack", aside, func() {
		if !a.corepackInstalled() {
			cardNote(c, locales.JsCorepackNotInstalled())
			return
		}
		text := locales.JsCorepackDisabled()
		if a.corepackShims() {
			text = locales.JsCorepackEnabled()
		}
		cardNote(c, text)
		cardNote(c, locales.JsCorepackReadOnly())
	})
}

// corepackShims reports Corepack's shims on the terminal's PATH.
func (a *App) corepackShims() bool {
	for _, id := range jsManagers {
		ts, _ := a.Snapshot.Tool(id)
		for _, in := range ts.Installations {
			if in.Source.Manager == "corepack" && in.Source.Shim && in.OnPath > 0 {
				return true
			}
		}
	}
	return false
}

// corepackInstalled reports Corepack itself or its shims: since Node 25 it
// comes separately, with npm install -g corepack.
func (a *App) corepackInstalled() bool {
	st, _ := a.Snapshot.Tool("corepack")
	return len(st.Installations) > 0 || a.corepackShims()
}

func (a *App) jsGlobals(c *ui.Context) {
	st, _ := a.Snapshot.Tool("npm")
	var pkgs []inventory.Package
	empty := locales.JsNpmMissing()
	if i := inventory.TerminalCopy(st.Installations); i >= 0 {
		node, _ := a.Snapshot.Tool("node")
		version := ""
		if n := inventory.TerminalCopy(node.Installations); n >= 0 {
			version = node.Installations[n].Version
		}
		pkgs, empty = userPackages(st.Installations[i].Packages, version), locales.NoGlobalPackages()
	}
	globalsCard(c, locales.GlobalPackages(), empty, pkgs, "npm -g")
}

func (a *App) jsProjects(c *ui.Context) {
	listCard(c, locales.JsProjectManagers(), "packageManager · lockfile", func() {
		count := 0
		for _, p := range a.Projects {
			var labels []string
			locks := 0
			for _, r := range p.Requirements {
				if !slices.Contains(jsManagers, r.Tool) {
					continue
				}
				if r.Key == "packageManager" {
					labels = append(labels, r.Tool+"@"+r.Want)
				}
				if r.File == "package-lock.json" || r.File == "pnpm-lock.yaml" || r.File == "yarn.lock" {
					labels = append(labels, r.File)
					locks++
				}
			}
			if len(labels) == 0 {
				continue
			}
			count++
			text := strings.Join(labels, " · ")
			listRow(c, filepath.Base(p.Dir), false, text, func() {
				if locks > 1 {
					ui.Text(c, locales.JsMultipleLocks()).FontSize(11).TextColor(c.Theme().Warning)
				}
			})
			for _, st := range projects.Resolve(p, a.Snapshot, a.nodeLines()) {
				if !slices.Contains(jsManagers, st.Tool) {
					continue
				}
				if ch, ok := st.Unmet(); ok {
					cardNote(c, locales.JsManagerUnmet(st.Tool, ch.Want))
				} else if st.Downloads {
					cardNote(c, locales.JsCorepackDownloads(st.Tool, st.Terminal.Version))
				}
			}
		}
		if count == 0 {
			cardNote(c, locales.NoProjects())
		}
	})
}

func (a *App) jsUpdates(c *ui.Context) {
	listCard(c, locales.Available(), "", func() {
		if a.JSVersions == nil {
			text := locales.VersionsLoading()
			if a.JSVersionsErr != "" {
				text = locales.VersionsFailed(a.JSVersionsErr)
			}
			cardNote(c, text)
			return
		}
		for _, p := range []struct {
			name   string
			latest versions.Latest
		}{{"npm", a.JSVersions.Npm}, {"pnpm", a.JSVersions.Pnpm}, {"yarn", a.JSVersions.Yarn}} {
			st, _ := a.Snapshot.Tool(p.name)
			current := ""
			if i := inventory.TerminalCopy(st.Installations); i >= 0 {
				current = st.Installations[i].Version
			}
			caption := locales.ChannelStable()
			if current == p.latest.Version {
				caption = locales.NoteInstalled()
			} else if current != "" && semver.Compare("v"+current, "v"+p.latest.Version) < 0 {
				caption = locales.NoteUpdateFor(versionText(current))
			}
			listRow(c, p.name+" "+p.latest.Version, true, caption, func() {
				// npm comes with Node; pnpm and Yarn not there install from
				// the managers' table.
				if p.name != "npm" && current != "" {
					st.Tool = p.name
					a.installButton(c, st, p.latest.Version, p.name+" "+p.latest.Version)
				}
			})
		}
	})
}
