package ui

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/ops"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) docker(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			st, _ := a.Snapshot.Tool("docker")
			if st.Docker != nil && inventory.TerminalCopy(st.Installations) < 0 && a.canInstallColima() {
				ui.PrimaryButton(c.Key("colima-install"), locales.ColimaInstall()).Height(controlHeight).OnClick(func() { a.installColima(false) })
			} else if plan, ok := a.nativeDockerInstallPlan(st); ok {
				ui.PrimaryButton(c.Key("docker-install"), locales.DockerEngineInstall()).Height(controlHeight).OnClick(func() { a.Propose(plan) })
			} else if profile, ok := dockerSelectedColima(st.Docker); ok && profile.Status == "stopped" {
				ui.PrimaryButton(c.Key("colima-start"), locales.TrayColimaStart()).Height(controlHeight).Disabled(a.BuildPlan == nil).
					OnClick(func() { a.changeColima(*st.Docker.Colima, profile, ops.ColimaStart, ops.ColimaLimits{}) })
			} else if plan, ok := a.engineStartPlan(); ok {
				ui.PrimaryButton(c.Key("engine-start"), plan.Title+"…").Height(controlHeight).OnClick(func() { a.Propose(plan) })
			} else if a.dockerAnswers() && a.dockerOpenable(st.Docker.Selected) {
				name := st.Docker.Selected
				ui.PrimaryButton(c.Key("engine-open"), locales.DockerOpenContext(name)).Height(controlHeight).OnClick(func() { a.router.Push(dockerEnginePath(name)) })
			}
		}})
		st, checked := a.Snapshot.Tool("docker")
		if !checked || st.Err != "" || st.Docker == nil {
			a.toolEmpty(c, "docker", "Docker")
			return
		}
		d := st.Docker
		active := inventory.TerminalCopy(st.Installations)
		if a.RefreshDocker != nil && active >= 0 {
			for _, ctx := range d.Contexts {
				if ctx.Name == d.Selected && ctx.Local && !ctx.Denied && ctx.Status == "unavailable" {
					now := time.Now()
					if a.dockerLastCheck.IsZero() || now.Sub(a.dockerLastCheck) >= 5*time.Second {
						a.dockerLastCheck = now
						a.RefreshDocker()
					}
					break
				}
			}
		}
		if !a.dockerTabsReady {
			a.dockerTabsReady = true
			if active < 0 {
				a.dockerTab = 1
			}
		}
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
				toolbar := ui.Row(c).Gap(12).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "docker-tabs", &a.dockerTab, []segment{
						{locales.DockerContexts(), strconv.Itoa(len(d.Contexts))},
						{locales.DockerEngines(), strconv.Itoa(len(d.Engines))},
						{locales.ProjectsTitle(), strconv.Itoa(len(a.dockerProjects()))},
						{locales.DockerPluginsTab(), strconv.Itoa(len(d.Plugins))},
						{"PATH", ""},
					})
					ui.Box(c).Grow(1)
					ui.SearchField(c.Key("docker-filter"), &a.dockerFilter).Bind(&a.filterField).Placeholder(locales.DockerFilter()).Height(controlHeight).Width(200).MinWidth(140).Shrink(1)
				})
				columns(c, 12, func() {
					switch a.dockerTab {
					case 0:
						a.dockerContexts(c, d)
						a.dockerQuickViewsCard(c, d)
						a.dockerDiskSummaryCard(c)
						toolNote(c, locales.DockerContextNote())
						if d.Override != "" {
							toolNote(c, locales.DockerOverrideNote(d.Override))
						}
					case 1:
						a.dockerEngines(c, st)
					case 2:
						a.dockerProjectList(c)
					case 3:
						plugins := filterDockerPlugins(d.Plugins, a.dockerFilter)
						emptyMsg := locales.DockerNoPlugins()
						if strings.TrimSpace(a.dockerFilter) != "" && len(d.Plugins) > 0 {
							emptyMsg = locales.DockerNoMatches()
						}
						globalsCard(c, locales.DockerPlugins(), emptyMsg, plugins, tilde(d.PluginDir, a.Home))
					case 4:
						insts, filteredActive := filterDockerInstalls(st.Installations, active, a.dockerFilter)
						if len(insts) == 0 && strings.TrimSpace(a.dockerFilter) != "" && len(st.Installations) > 0 {
							listCard(c, "PATH", "", func() {
								cardNote(c, locales.DockerNoMatches())
							})
						} else {
							pathCard(c, "docker", insts, filteredActive, a.Home)
						}
					}
				}, func() {
					a.dockerHealthCard(c, st, d.Selected, d.Health, false)
					a.dockerManagement(c, st)
					a.dockerRegistries(c, st, active)
					listCard(c, locales.DockerHow(), "", func() {
						listRow(c, locales.DockerHowScan(), false, locales.DockerHowScanCaption(), func() {})
						listRow(c, locales.DockerHowView(), false, locales.DockerHowViewCaption(), func() {})
						listRow(c, locales.DockerHowPlan(), false, locales.DockerHowPlanCaption(), func() {})
					})
				})
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

// dockerContexts lists the contexts, local ones first, as the Kubernetes
// page lists its own: what each points to, the status of the one checked,
// and Open for an engine on this computer.
func (a *App) dockerContexts(c *ui.Context, d *inventory.DockerState) {
	pal := Colors(c.Theme())
	shown := 0
	ui.Column(c).Radius(12).Clip().Background(c.Theme().Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		for _, local := range []bool{true, false} {
			var rows []inventory.DockerContext
			for _, k := range d.Contexts {
				if k.Local == local && strings.Contains(strings.ToLower(k.Name+" "+k.Engine+" "+k.Endpoint), strings.ToLower(a.dockerFilter)) {
					rows = append(rows, k)
				}
			}
			if len(rows) == 0 {
				continue
			}
			title := locales.DockerRemote()
			if local {
				title = locales.DockerLocal()
			}
			cliGroupHead(c, title, len(rows))
			for _, k := range rows {
				a.dockerContextRow(c.Key("context:"+k.Name), d, k)
			}
			shown += len(rows)
		}
		if shown == 0 {
			cardNote(c, locales.DockerNoMatches())
		}
	})
}

// dockerContextRow is a context: its engine and address, whether the CLI
// uses it, what the scan found of its engine, and its actions; the row's
// right-click and its ⋯ open the same menu.
func (a *App) dockerContextRow(c *ui.Context, d *inventory.DockerState, k inventory.DockerContext) {
	pal := Colors(c.Theme())
	selected := k.Name == d.Selected
	plan, canUse := a.contextPlan(k.Name)
	removePlan, canRemove := a.contextRemovePlan(k.Name)
	menu := func(m *ui.Menu) {
		if canUse && m.Item(locales.DockerUseContext()).Chosen() {
			a.Propose(plan)
		}
		if k.Local && m.Item(locales.CopyPath()).Chosen() {
			c.WriteClipboard(k.Endpoint)
			c.Toast(locales.PathCopied())
		}
		if canRemove {
			if canUse || k.Local {
				m.Separator()
			}
			if m.Item(locales.DockerContextRemove()).Chosen() {
				a.Propose(removePlan)
			}
		}
	}
	row := ui.Row(c).Gap(10).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).ContextMenu(menu)
	if selected {
		row.Background(pal.AccentSoft.Alpha(0.4))
	}
	row.Children(func() {
		entityTile(c, dockerContextIcon(d, k), 28)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, k.Name).Font(FontMono).FontSize(13)
			caption := dockerContextCaption(k, a.Home)
			if label := dockerManagementLabel(dockerContextEngine(d, k)); k.Local && label != "" {
				caption += " · " + label
			}
			if selected {
				caption += " · " + locales.DockerInCli()
			}
			clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint).Tooltip(caption)
			if !k.Local && k.Endpoint != "" {
				clipped(c, locales.DockerRemoteNote()).FontSize(10.5).TextColor(pal.TextFaint)
			}
		})
		// Only the selected context is probed: saying the others were not
		// checked tells nothing.
		if selected || k.Status != "" {
			status, dot := dockerStatus(c, k.Status)
			bg, ink := pal.Muted, pal.TextFaint
			switch k.Status {
			case "available":
				bg, ink = pal.SuccessSoft, pal.SuccessText
			case "unavailable":
				bg, ink = pal.WarningSoft, pal.WarningText
			}
			chip(c, dot, bg, ink, status)
		}
		if a.dockerOpenable(k.Name) {
			ui.Button(c.Key("open"), locales.KubeClusterOpen()).OnClick(func() { a.router.Push(dockerEnginePath(k.Name)) })
		}
		if canUse || k.Local || canRemove {
			iconButton(c, "ellipsis", locales.MoreActions()).Menu(menu)
		}
	})
}

func dockerContextIcon(d *inventory.DockerState, k inventory.DockerContext) string {
	if !k.Local {
		return "lucide/globe"
	}
	if engine := dockerContextEngine(d, k); engine != "" {
		return dockerEngineIcon(engine)
	}
	return "brand/docker"
}

func dockerEngineIcon(name string) string {
	switch name {
	case "Docker Desktop", "Docker Engine":
		return "brand/docker"
	case "Colima":
		return "brand/colima"
	case "OrbStack":
		return "lucide/layers"
	case "Podman", "Podman Desktop":
		return "brand/podman"
	case "Rancher Desktop":
		return "brand/rancher"
	case "Kiri":
		return "brand/kiri"
	default:
		return "lucide/container"
	}
}

func (a *App) dockerEngines(c *ui.Context, st inventory.ToolState) {
	pal := Colors(c.Theme())
	query := strings.ToLower(strings.TrimSpace(a.dockerFilter))
	engines := st.Docker.Engines
	cli := inventory.TerminalCopy(st.Installations) >= 0
	heroShown := query == "" && a.dockerHearthHeroCard(c, st)
	title := locales.DockerEngineOptions()
	if heroShown {
		title = locales.DockerHearthOtherEngines()
	}
	listCard(c, title, dockerPlatform(), func() {
		shown := 0
		for _, engine := range engines {
			if heroShown && engine.Name == recommendedDockerEngine {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(engine.Name+" "+engine.Path+" "+dockerManagementLabel(engine.Name)), query) {
				continue
			}
			shown++
			caption := locales.DockerOpenSource()
			switch engine.Name {
			case "OrbStack":
				caption = locales.DockerOrbTerms()
			case "Docker Desktop":
				caption = locales.DockerDesktopTerms()
			case "Docker Engine":
				caption = locales.DockerEngineTerms()
			}
			if label := dockerManagementLabel(engine.Name); label != "" {
				caption += " · " + label
			}
			listRowWithTile(c.Key("engine:"+engine.Name), dockerEngineIcon(engine.Name), engine.Name, false, caption, func() {
				if engine.Name == recommendedDockerEngine {
					chip(c, c.Theme().Success, pal.SuccessSoft, pal.SuccessText, locales.ColimaRecommended())
				}
				if engine.Name == "Colima" && engine.Path == "" && a.canInstallColima() {
					label := locales.ColimaInstall()
					migrate := cli
					if migrate {
						label = locales.ColimaMigrate()
					}
					ui.Button(c.Key("install-colima"), label).OnClick(func() { a.installColima(migrate) })
					return
				}
				if engine.Name == "Docker Engine" && engine.Path == "" && recommendedDockerEngine == "Docker Engine" {
					if plan, ok := a.dockerEngineInstallOrSwitchPlan(st, cli); ok {
						ui.Button(c.Key("install-docker-engine"), locales.DockerEngineInstall()).OnClick(func() { a.Propose(plan) })
						return
					}
				}
				if engine.Path != "" {
					chip(c, c.Theme().Success, pal.SuccessSoft, pal.SuccessText, locales.DockerInstalled())
					return
				}
				ui.Text(c, locales.DockerNotInstalled()).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
				if plan, ok := a.engineInstallPlan(engine.Name, cli); ok && engine.Name != "Colima" {
					iconButton(c.Key("install:"+engine.Name), "download", plan.Title).OnClick(func() { a.Propose(plan) })
				} else if m, ok := catalog.EngineMethod(engine.Name, "installer"); ok {
					// No system manager has it: its maker's page, not an install.
					site := strings.TrimSuffix(strings.TrimPrefix(m.Package, "https://"), "/")
					site, _, _ = strings.Cut(site, "/")
					iconButton(c.Key("site:"+engine.Name), "external-link", locales.DockerEngineSite(site)).OnClick(func() { c.OpenURL(m.Package) })
				}
			})
		}
		if shown == 0 {
			cardNote(c, locales.DockerNoMatches())
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

func (a *App) dockerQuickViewsCard(c *ui.Context, d *inventory.DockerState) {
	if !a.dockerAnswers() || !a.dockerOpenable(d.Selected) {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	selected := d.Selected
	listCard(c, locales.DockerQuickViews(), selected, func() {
		ui.Row(c).Gap(8).Padding(12, 14).Wrap().Children(func() {
			for i, v := range dockerViews() {
				view := i
				tile := ui.ButtonBase(c.Key("quick-view-"+strconv.Itoa(view))).Padding(8, 12).
					Radius(9).BorderWidth(1).BorderColor(pal.Line).Gap(7).AlignItems(ui.Center).
					Label(v.label).Transition(colorFade).OnClick(func() {
					a.engine.view = view
					a.router.Push(dockerEnginePath(selected))
				})
				if tile.Hovered() {
					tile.Background(pal.Subtle).BorderColor(pal.ControlBorder)
				} else {
					tile.Background(t.Background)
				}
				tile.Children(func() {
					ui.Icon(c, Icon(v.icon)).Size(15, 15).Shrink(0).TextColor(t.TextMuted)
					clipped(c, v.label).FontSize(12.5).FontWeight(500).MinWidth(0)
				})
			}
		})
	})
}

func (a *App) dockerDiskSummaryCard(c *ui.Context) {
	if a.Sizes == nil || len(a.Sizes.Docker) == 0 {
		if a.DockerCounting && a.dockerAnswers() {
			listCard(c, locales.DockerOverviewDisk(), "", func() {
				cardNote(c, locales.DockerDiskCounting())
			})
		}
		return
	}
	t := c.Theme()
	pal := Colors(t)
	var total, reclaimable int64
	for _, row := range a.Sizes.Docker {
		total += row.Bytes
		reclaimable += row.Reclaimable
	}
	if total <= 0 {
		return
	}
	listCardHeader(c, locales.DockerOverviewDisk(), func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.DockerCounting {
				chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.DockerDiskRefreshing())
			}
			ui.Text(c, formatBytes(total)).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
		})
	}, func() {
		for _, row := range a.Sizes.Docker {
			label := row.Type
			switch row.Type {
			case "Images":
				label = locales.DockerViewImages()
			case "Containers":
				label = locales.DockerViewContainers()
			case "Local Volumes":
				label = locales.DockerViewVolumes()
			case "Build Cache":
				label = locales.DockerImageBuildCache()
			}
			ui.Column(c.Key("disk-"+row.Type)).Gap(5).Padding(8, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).Grow(1).MinWidth(0).FontSize(12).SingleLine()
					ui.Text(c, formatBytes(row.Bytes)).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
					if row.Reclaimable > 0 {
						ui.Text(c, "· "+locales.DockerOverviewReclaimable(formatBytes(row.Reclaimable))).FontSize(11).TextColor(pal.TextFaint).SingleLine()
					}
				})
				requestBar(c, float64(row.Bytes)/float64(total), pal.InfoText)
			})
		}
		caption := locales.DockerOverviewReclaimable(formatBytes(reclaimable))
		if a.DockerCounting {
			caption += " · " + locales.DockerDiskCachedNote()
		}
		navListRow(c, "docker-disk-cleanup", locales.ChangesOpenCleanup(), caption, locales.ChangesOpenCleanup(), func() {
			a.cleanupTab = slices.Index(cleanupGroups, cleanup.Docker) + 1
			a.cleanupFilter = ""
			a.router.Push(cleanupPage)
		})
	})
}

func (a *App) dockerProjects() []int {
	var out []int
	for i, p := range a.Projects {
		if _, ok := composeProject(p); ok {
			out = append(out, i)
			continue
		}
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
		query := strings.ToLower(strings.TrimSpace(a.dockerFilter))
		shown := 0
		for _, i := range rows {
			p := a.Projects[i]
			dir := p.Dir
			if query != "" && !strings.Contains(strings.ToLower(dir+" "+tilde(dir, a.Home)), query) {
				continue
			}
			shown++
			navListRow(c, "docker-proj:"+dir, tilde(dir, a.Home), "", locales.Details(), func() {
				a.router.Push(projectPath(dir))
			})
		}
		switch {
		case len(rows) == 0:
			cardNote(c, locales.DockerNoProjects())
		case shown == 0:
			cardNote(c, locales.DockerNoMatches())
		}
	})
}

func filterDockerPlugins(plugins []inventory.Package, filter string) []inventory.Package {
	query := strings.ToLower(strings.TrimSpace(filter))
	if query == "" {
		return plugins
	}
	var out []inventory.Package
	for _, p := range plugins {
		if strings.Contains(strings.ToLower(p.Name+" "+p.Version+" "+strings.Join(p.Commands, " ")), query) {
			out = append(out, p)
		}
	}
	return out
}

func filterDockerInstalls(insts []inventory.Installation, active int, filter string) ([]inventory.Installation, int) {
	query := strings.ToLower(strings.TrimSpace(filter))
	if query == "" {
		return insts, active
	}
	var out []inventory.Installation
	filteredActive := -1
	for i, in := range insts {
		if !strings.Contains(strings.ToLower(in.Path+" "+in.Version+" "+sourceName(in.Source)), query) {
			continue
		}
		if i == active {
			filteredActive = len(out)
		}
		out = append(out, in)
	}
	return out, filteredActive
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
		if e.Name == name && e.Name != "Colima" {
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

// contextRemovePlan is the plan that removes an inactive non-default Docker
// CLI context through the terminal's CLI.
func (a *App) contextRemovePlan(name string) (ops.Plan, bool) {
	st, ok := a.Snapshot.Tool("docker")
	if !ok || st.Docker == nil || name == "default" || name == st.Docker.Selected {
		return ops.Plan{}, false
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 || st.Installations[active].Source.Shim {
		return ops.Plan{}, false
	}
	return ops.ContextRemovePlan(st.Installations[active].Path, name, a.detect("docker"))
}

// credentialHelperInstallPlan returns a package manager installation plan for
// a missing Docker credential helper when a supported package manager is found.
func (a *App) credentialHelperInstallPlan(helper string) (ops.Plan, bool) {
	st, ok := a.Snapshot.Tool("packages")
	if !ok {
		return ops.Plan{}, false
	}
	for _, m := range st.PackageManagers {
		if p, ok := ops.DockerCredentialHelperInstallPlan(helper, m, a.reread(m)); ok {
			return p, true
		}
	}
	return ops.Plan{}, false
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

func (a *App) engineSwitchPlan(engine string, cli bool) (ops.Plan, bool) {
	st, ok := a.Snapshot.Tool("packages")
	if !ok {
		return ops.Plan{}, false
	}
	for _, m := range st.PackageManagers {
		if p, ok := ops.EngineSwitchPlan(engine, m, cli, a.reread(m)); ok {
			return p, true
		}
	}
	return ops.Plan{}, false
}

func (a *App) dockerEngineUninstalled(st inventory.ToolState) bool {
	return recommendedDockerEngine == "Docker Engine" && st.Docker != nil &&
		slices.ContainsFunc(st.Docker.Engines, func(e inventory.DockerEngine) bool {
			return e.Name == recommendedDockerEngine && e.Path == ""
		})
}

func (a *App) nativeDockerInstallPlan(st inventory.ToolState) (ops.Plan, bool) {
	if a.dockerEngineUninstalled(st) && inventory.TerminalCopy(st.Installations) < 0 {
		return a.engineInstallPlan(recommendedDockerEngine, false)
	}
	return ops.Plan{}, false
}

func (a *App) dockerEngineSwitchPlan(st inventory.ToolState) (ops.Plan, bool) {
	if a.dockerEngineUninstalled(st) {
		return a.engineSwitchPlan(recommendedDockerEngine, inventory.TerminalCopy(st.Installations) >= 0)
	}
	return ops.Plan{}, false
}

func (a *App) dockerEngineInstallOrSwitchPlan(st inventory.ToolState, hasCLI bool) (ops.Plan, bool) {
	if !a.dockerEngineUninstalled(st) {
		return ops.Plan{}, false
	}
	if hasCLI {
		return a.engineSwitchPlan(recommendedDockerEngine, true)
	}
	return a.engineInstallPlan(recommendedDockerEngine, false)
}
