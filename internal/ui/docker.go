package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/ops"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) docker(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			if plan, ok := a.engineStartPlan(); ok {
				ui.PrimaryButton(c.Key("engine-start"), plan.Title).OnClick(func() { a.Propose(plan) })
			}
		}})
		st, checked := a.Snapshot.Tool("docker")
		if !checked || st.Err != "" || st.Docker == nil {
			a.toolEmpty(c, "docker", "Docker")
			return
		}
		d := st.Docker
		active := inventory.TerminalCopy(st.Installations)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				a.dockerHeader(c, st, active)
				for _, context := range d.Contexts {
					if context.Name == d.Selected && context.Status == "unavailable" {
						if context.Denied {
							explain := locales.DockerDeniedExplain()
							if context.Joined {
								explain = locales.DockerSignInExplain()
							}
							warningRow(c, explain)
						} else {
							warningRow(c, locales.DockerUnavailableExplain())
						}
					}
				}
				if active < 0 {
					toolNote(c, locales.DockerNoCli())
				}
				if d.Err != "" {
					warningRow(c, locales.DockerConfigError())
				}
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					segments(c, "docker-tabs", &a.dockerTab, []segment{{locales.DockerContexts(), strconv.Itoa(len(d.Contexts))}, {locales.DockerEngines(), ""}, {locales.ProjectsTitle(), strconv.Itoa(len(a.dockerProjects()))}, {"PATH", ""}})
					ui.Box(c).Grow(1)
					if a.dockerTab == 0 {
						ui.SearchField(c.Key("docker-filter"), &a.dockerFilter).Bind(&a.filterField).Placeholder(locales.DockerFilter()).Width(200)
					}
				})
				var side func()
				if a.dockerTab == 0 {
					side = func() {
						a.dockerEngines(c, st)
						globalsCard(c, locales.DockerPlugins(), locales.DockerNoPlugins(), d.Plugins, tilde(d.PluginDir, a.Home))
					}
				}
				columns(c, 12, func() {
					switch a.dockerTab {
					case 0:
						a.dockerContexts(c, d)
						toolNote(c, locales.DockerContextNote())
						if d.Override != "" {
							toolNote(c, locales.DockerOverrideNote(d.Override))
						}
						pathCard(c, "docker", st.Installations, active, a.Home)
					case 1:
						a.dockerEngines(c, st)
					case 2:
						a.dockerProjectList(c)
					case 3:
						pathCard(c, "docker", st.Installations, active, a.Home)
					}
				}, side)
			})
		})
	})
}

func (a *App) dockerHeader(c *ui.Context, st inventory.ToolState, active int) {
	pal := Colors(c.Theme())
	d := st.Docker
	toolHead(c, "docker", "Docker", func() {}, locales.DockerSubtitle(), func() {
		value, source := "—", locales.NotInPath()
		dot := pal.TextFaint
		if active >= 0 {
			value, source, dot = st.Installations[active].Version, sourceName(st.Installations[active].Source), c.Theme().Success
			if value == "" {
				value = "—"
			}
		}
		stateCard(c, dot, "CLI", value, source)
		status, engine, color := locales.DockerUnchecked(), "—", pal.TextFaint
		for _, context := range d.Contexts {
			if context.Name == d.Selected {
				status, color = dockerStatus(c, context.Status)
				engine = context.Engine
				if engine == "" {
					engine = context.Version
				}
			}
		}
		stateCard(c, color, locales.DockerEngine(), status, engine)
		stateCard(c, pal.TextFaint, locales.DockerContext(), d.Selected, "docker context show")
	})
}

func dockerStatus(c *ui.Context, status string) (string, ui.Color) {
	switch status {
	case "available":
		return locales.DockerAvailable(), c.Theme().Success
	case "unavailable":
		return locales.DockerUnavailable(), c.Theme().Warning
	default:
		return locales.DockerUnchecked(), Colors(c.Theme()).TextFaint
	}
}

// endpointText shows a context's endpoint: a local socket as its path,
// with ~ for home, the rest as Docker writes it.
func endpointText(endpoint, home string) string {
	if p, ok := strings.CutPrefix(endpoint, "unix://"); ok {
		return tilde(p, home)
	}
	return endpoint
}

func (a *App) dockerContexts(c *ui.Context, d *inventory.DockerState) {
	pal := Colors(c.Theme())
	ui.Column(c).Radius(12).Clip().Background(c.Theme().Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.DockerContext()).Width(nameWidth(c)).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.DockerEngine()).Width(84).FontSize(11.5).TextColor(pal.TextFaint)
			ui.Text(c, locales.DockerEndpoint()).FontSize(11.5).TextColor(pal.TextFaint)
		})
		shown := 0
		for _, context := range d.Contexts {
			if !strings.Contains(strings.ToLower(context.Name+" "+context.Engine+" "+context.Endpoint), strings.ToLower(a.dockerFilter)) {
				continue
			}
			shown++
			row := ui.Row(c.Key("context:"+context.Name)).Gap(12).Padding(12, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
			if context.Name == d.Selected {
				row.Background(pal.AccentSoft.Alpha(0.4))
			}
			row.Children(func() {
				clipped(c, context.Name).Font(FontMono).FontSize(12.5).Width(nameWidth(c))
				engine := context.Engine
				if engine == "" {
					engine = "—"
				}
				ui.Text(c, engine).FontSize(11).Width(84).SingleLine().TextColor(pal.MonoText)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					clipped(c, endpointText(context.Endpoint, a.Home)).Font(FontMono).FontSize(10.5).TextColor(pal.MonoText)
					ui.Row(c).Gap(5).Children(func() {
						if context.Name == d.Selected {
							chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.DockerActive())
						} else if context.Status == "" {
							// Only the selected context is probed: saying the
							// others were not checked tells nothing.
							return
						}
						status, dot := dockerStatus(c, context.Status)
						bg, ink := pal.Muted, pal.TextFaint
						if context.Status == "available" {
							bg, ink = pal.SuccessSoft, pal.SuccessText
						}
						if context.Status == "unavailable" {
							bg, ink = pal.WarningSoft, pal.WarningText
						}
						chip(c, dot, bg, ink, status)
					})
					if !context.Local && context.Endpoint != "" {
						ui.Text(c, locales.DockerRemoteNote()).FontSize(11).TextColor(pal.TextFaint)
					}
				})
				if context.Local {
					iconButton(c, "copy", locales.CopyPath()).OnClick(func() { c.WriteClipboard(context.Endpoint); c.Toast(locales.PathCopied()) })
				}
				if plan, ok := a.contextPlan(context.Name); ok {
					iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) {
						if menu.Item(locales.DockerUseContext()).Chosen() {
							a.Propose(plan)
						}
					})
				}
			})
		}
		if shown == 0 {
			cardNote(c, locales.DockerNoMatches())
		}
	})
}

func (a *App) dockerEngines(c *ui.Context, st inventory.ToolState) {
	pal := Colors(c.Theme())
	engines := st.Docker.Engines
	cli := inventory.TerminalCopy(st.Installations) >= 0
	listCard(c, locales.DockerEngineOptions(), dockerPlatform(), func() {
		for _, engine := range engines {
			caption := locales.DockerOpenSource()
			switch engine.Name {
			case "OrbStack":
				caption = locales.DockerOrbTerms()
			case "Docker Desktop":
				caption = locales.DockerDesktopTerms()
			case "Docker Engine":
				caption = locales.DockerEngineTerms()
			}
			listRow(c, engine.Name, false, caption, func() {
				if engine.Path != "" {
					chip(c, c.Theme().Success, pal.SuccessSoft, pal.SuccessText, locales.DockerInstalled())
					return
				}
				ui.Text(c, locales.DockerNotInstalled()).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
				if plan, ok := a.engineInstallPlan(engine.Name, cli); ok {
					iconButton(c.Key("install:"+engine.Name), "download", plan.Title).OnClick(func() { a.Propose(plan) })
				}
			})
		}
		ui.Row(c).Gap(8).Padding(10, 14).Children(func() {
			for _, engine := range engines {
				switch engine.Name {
				case "Docker Desktop":
					ui.Link(c, "Docker Desktop · "+locales.DockerTerms(), "https://docs.docker.com/subscription-billing/desktop-license/").FontSize(11)
				case "OrbStack":
					ui.Link(c, "OrbStack · "+locales.DockerTerms(), "https://orbstack.dev/pricing").FontSize(11)
				}
			}
		})
	})
}

func (a *App) dockerProjects() []int {
	var out []int
	for i, p := range a.Projects {
		for _, r := range p.Requirements {
			if r.Tool == "docker" {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

func (a *App) dockerProjectList(c *ui.Context) {
	listCard(c, locales.ProjectsTitle(), "", func() {
		rows := a.dockerProjects()
		for _, i := range rows {
			p := a.Projects[i]
			listRow(c, tilde(p.Dir, a.Home), false, "", func() {
				ui.Button(c, locales.Details()).OnClick(func() { a.router.Push(projectPath(p.Dir)) })
			})
		}
		if len(rows) == 0 {
			cardNote(c, locales.DockerNoProjects())
		}
	})
}

// engineStartPlan is the plan that starts the engine of the selected
// context when it did not answer: its own, or the only one installed.
// ok is false when the engine answered, refused this user, or is not
// known.
func (a *App) engineStartPlan() (ops.Plan, bool) {
	st, ok := a.Snapshot.Tool("docker")
	if !ok || st.Docker == nil {
		return ops.Plan{}, false
	}
	d := st.Docker
	i := slices.IndexFunc(d.Contexts, func(c inventory.DockerContext) bool { return c.Name == d.Selected })
	if i < 0 || d.Contexts[i].Status != "unavailable" || d.Contexts[i].Denied || !d.Contexts[i].Local {
		return ops.Plan{}, false
	}
	name := d.Contexts[i].Engine
	var installed []inventory.DockerEngine
	for _, e := range d.Engines {
		if e.Path != "" {
			installed = append(installed, e)
		}
	}
	// Linux's default socket is Docker Engine's.
	if name == "" && slices.ContainsFunc(installed, func(e inventory.DockerEngine) bool { return e.Name == "Docker Engine" }) {
		name = "Docker Engine"
	}
	if name == "" && len(installed) == 1 {
		name = installed[0].Name
	}
	for _, e := range installed {
		if e.Name == name {
			return ops.EngineStartPlan(ops.Engine{Name: e.Name, Path: e.Path}, d.Selected, a.detect("docker"))
		}
	}
	return ops.Plan{}, false
}

// contextPlan is the plan that makes name the Docker context, through
// the terminal's CLI; ok is false for the context already selected, or
// one an environment variable would override.
func (a *App) contextPlan(name string) (ops.Plan, bool) {
	st, ok := a.Snapshot.Tool("docker")
	if !ok || st.Docker == nil || st.Docker.Override != "" || name == st.Docker.Selected {
		return ops.Plan{}, false
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 || st.Installations[active].Source.Shim {
		return ops.Plan{}, false
	}
	return ops.ContextPlan(st.Installations[active].Path, st.Docker.Selected, name, a.detect("docker"))
}

// engineInstallPlan is the plan that installs engine through the first
// of the system's package managers that has it; cli reports a Docker CLI
// on the terminal's PATH.
func (a *App) engineInstallPlan(engine string, cli bool) (ops.Plan, bool) {
	st, ok := a.Snapshot.Tool("packages")
	if !ok {
		return ops.Plan{}, false
	}
	for _, m := range st.PackageManagers {
		if p, ok := ops.EngineInstallPlan(engine, m, cli, a.reread(m)); ok {
			return p, true
		}
	}
	return ops.Plan{}, false
}
