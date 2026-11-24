package ui

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

// dockerPage is Docker: the CLI, its contexts, the engines and projects.
const dockerPage = "/tools/docker"

// dockerEnginePage is a local context's engine: its overview, containers,
// images, volumes and networks, read while the page shows.
const dockerEnginePage = dockerPage + "/engine"

// dockerEnginePath is the engine page of the context name.
func dockerEnginePath(name string) string {
	return dockerEnginePage + "?context=" + url.QueryEscape(name)
}

// The views of an engine, in their chips' order.
const (
	dockerOverview = iota
	dockerContainers
	dockerImages
	dockerVolumes
	dockerNetworks
)

type dockerView struct{ label, icon string }

func dockerViews() []dockerView {
	return []dockerView{
		{locales.DockerViewOverview(), "layout-dashboard"},
		{locales.DockerViewContainers(), "container"},
		{locales.DockerViewImages(), "layers"},
		{locales.DockerViewVolumes(), "hard-drive"},
		{locales.DockerViewNetworks(), "network"},
	}
}

// dockerEngineState is the engine page's: the view shown, what it reads
// and what it read.
type dockerEngineState struct {
	// view stays as the user left it, so the sidebar comes back to it.
	view                   int
	contextOpen, viewsOpen bool
	// target is the engine the data was read from; running is what the
	// watch reads, with refresh, and stop ends it.
	target, running string
	refresh, watch  int
	stop            func()
	data            *docker.Engine
	err             error
	// reading is true from a read the user waits for to its answer.
	reading      bool
	problemCount int
	containers   dockerContainersState
	images       dockerImagesState
	resources    dockerResourcesState
}

// engineTarget is the engine of the context name, or why Hearth cannot
// read it.
func (a *App) engineTarget(name string) (docker.Target, string) {
	st, ok := a.Snapshot.Tool("docker")
	if !ok || st.Docker == nil {
		return docker.Target{}, locales.DockerEngineMissing(name)
	}
	var context *inventory.DockerContext
	for i, k := range st.Docker.Contexts {
		if k.Name == name {
			context = &st.Docker.Contexts[i]
		}
	}
	switch {
	case context == nil:
		return docker.Target{}, locales.DockerEngineMissing(name)
	case !context.Local:
		return docker.Target{}, locales.DockerEngineRemote(name)
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 {
		return docker.Target{}, locales.DockerEngineNoCli()
	}
	target := docker.Target{Docker: st.Installations[active].Path, Context: name, Endpoint: context.Endpoint, Provider: context.Engine}
	if profile, ok := dockerColimaProfile(st.Docker, name); ok {
		target.Provider, target.ColimaProfile = "Colima", profile.Name
	}
	return target, ""
}

// dockerOpenable reports that the context's engine has a page: a local
// context, with a CLI to read it.
func (a *App) dockerOpenable(name string) bool {
	_, problem := a.engineTarget(name)
	return problem == "" && a.WatchDocker != nil
}

// entryPath resolves the sidebar destination for the current session.
func (a *App) entryPath(path string) string {
	if path == kubePage {
		return a.kubeEntryPath()
	}
	if path != dockerPage || !a.dockerAnswers() {
		return path
	}
	st, _ := a.Snapshot.Tool("docker")
	if !a.dockerOpenable(st.Docker.Selected) {
		return path
	}
	return dockerEnginePath(st.Docker.Selected)
}

// syncDocker keeps the watch of the engine page's engine running while
// the page shows and stops it once it does not, as syncCluster does for
// clusters.
func (a *App) syncDocker() {
	s := &a.engine
	var t docker.Target
	target := ""
	if a.router.Path() == dockerEnginePage && a.WatchDocker != nil && a.Snapshot != nil {
		if tt, problem := a.engineTarget(a.router.Query("context")); problem == "" {
			t = tt
			target = fmt.Sprintf("%q", []string{t.Docker, t.Context, t.Endpoint, t.Provider, t.ColimaProfile})
		}
	}
	running := ""
	if target != "" {
		running = fmt.Sprintf("%s#%d", target, s.refresh)
	}
	a.syncDockerContainers(t, running)
	a.syncDockerImages(t, running)
	a.syncDockerResources(t, running)
	overview := s.view == dockerOverview
	if running != "" && overview {
		running += "/overview"
	}
	if running == s.running {
		return
	}
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	s.running = running
	s.watch++
	if target != s.target {
		// Another engine: what was read is not its.
		s.target, s.data, s.err, s.problemCount = target, nil, nil, 0
	}
	if running == "" {
		s.reading = false
		return
	}
	watch := s.watch
	s.reading = true
	s.stop = a.WatchDocker(t, overview, func(e docker.Engine, err error) {
		if s.watch != watch {
			return
		}
		s.reading = false
		if err != nil {
			s.err = err
			return
		}
		if overview {
			s.problemCount = len(e.Overview.Problems)
		}
		s.data, s.err = &e, nil
	})
}

// StopDocker ends the engine page's reads, as the window closes.
func (a *App) StopDocker() {
	s := &a.engine
	s.containers.stopWatch()
	s.containers.pending = nil
	s.containers.closeLogs()
	s.containers.closeDetail()
	s.images.stopWatch()
	s.resources.stopWatch()
	if s.stop != nil {
		s.stop()
		s.stop, s.running, s.reading = nil, "", false
		s.watch++
	}
}

func (a *App) toggleDockerViews() {
	s := &a.engine
	s.viewsOpen = !s.viewsOpen
	if s.viewsOpen {
		s.contextOpen = false
	}
}

func (a *App) dockerEngineView(c *ui.Context, name string) {
	s := &a.engine
	pal := Colors(c.Theme())
	ui.Column(c).Fill().Children(func() {
		a.dockerEngineHeader(c, name)
		if l := s.containers.logs; l != nil {
			a.dockerLogsView(c, l)
			return
		}
		ui.Column(c).Grow(1).MinHeight(0).Gap(14).Padding(20, 28, 24, 28).Children(func() {
			ui.Row(c).Gap(6).Wrap().Shrink(0).Children(func() {
				for i, v := range dockerViews() {
					count := 0
					if i == dockerContainers {
						switch {
						case s.containers.loaded:
							for _, item := range s.containers.items {
								if item.Problem() {
									count++
								}
							}
						case s.view == dockerOverview && s.data != nil:
							count = len(s.data.Overview.Problems)
						default:
							count = s.problemCount
						}
					}
					kindChip(c, &s.view, i, v.icon, v.label, count, nil)
				}
			})
			_, problem := a.engineTarget(name)
			switch {
			case a.Snapshot == nil:
				toolNote(c, locales.CleanupWaiting())
			case problem != "":
				toolNote(c, problem)
			case s.data == nil && s.err != nil:
				a.dockerEngineDown(c, name, s.err)
			case s.data == nil:
				tableSkeleton(c, locales.DockerEngineReading(name))
			default:
				if s.err != nil {
					ui.Row(c).Gap(10).Padding(10, 14).Radius(10).AlignItems(ui.Center).Background(pal.WarningSoft).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
						ui.Icon(c, Icon("triangle-alert")).Size(15, 15).Shrink(0).TextColor(c.Theme().Warning)
						ui.Text(c, locales.KubeClusterStale(s.err.Error())).FontSize(12).TextColor(pal.WarningText).Grow(1).MinWidth(0).Wrap()
						st, _ := a.Snapshot.Tool("docker")
						if profile, ok := dockerColimaProfile(st.Docker, name); ok && (profile.Status == "stopped" || profile.Status == "") {
							ui.Button(c.Key("engine-stale-start-colima"), locales.TrayColimaStart()).Height(controlHeight).Shrink(0).Disabled(a.BuildPlan == nil).
								OnClick(func() { a.changeColima(*st.Docker.Colima, profile, ops.ColimaStart, ops.ColimaLimits{}) })
						} else if plan, ok := a.engineStartPlan(); ok && st.Docker != nil && st.Docker.Selected == name {
							ui.Button(c.Key("engine-stale-start"), plan.Title+"…").Height(controlHeight).Shrink(0).OnClick(func() { a.Propose(plan) })
						}
					})
				}
				switch s.view {
				case dockerOverview:
					ui.Scroll(c).Grow(1).Children(func() {
						ui.Column(c).Gap(14).Children(func() {
							a.dockerOverviewView(c, name, *s.data)
						})
					})
				case dockerContainers:
					a.dockerContainersView(c)
				case dockerImages:
					a.dockerImagesView(c)
				case dockerVolumes, dockerNetworks:
					a.dockerResourcesView(c)
				}
			}
		})
	})
}

// dockerEngineHeader is the page's path: the context and the view, each a
// select; how fresh the data is, and a button reading it again.
func (a *App) dockerEngineHeader(c *ui.Context, name string) {
	s := &a.engine
	t := c.Theme()
	pal := Colors(t)
	right := float32(20)
	if bar := c.TitleBar(); bar.Right > 0 {
		right = bar.Right + 12
	}
	ui.Row(c).Height(56).Shrink(0).AlignItems(ui.Center).Gap(6).Padding(0, right, 0, 24).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.Divider).DragWindow().Children(func() {
		if s.containers.logs != nil {
			headerBackButton(c, locales.DockerBackToContainers(), s.containers.closeLogs)
		} else {
			headerBackButton(c, locales.BackToDocker(), func() { a.router.Push(dockerPage) })
		}
		anchor := pathButton(c, "engine-context", "brand/docker", name, 0)
		if anchor.Clicked() {
			s.contextOpen = !s.contextOpen
			s.viewsOpen = false
		}
		ui.Popover(c, anchor, &s.contextOpen, func() { a.dockerContextPopover(c, name) })
		if st, _ := a.Snapshot.Tool("docker"); st.Docker != nil && s.containers.logs == nil {
			for _, ctx := range st.Docker.Contexts {
				if ctx.Name != name {
					continue
				}
				engine := dockerContextEngine(st.Docker, ctx)
				if label := dockerManagementLabel(engine); label != "" {
					bg, ink := pal.Muted, t.TextMuted
					if engine == "Colima" {
						bg, ink = pal.InfoSoft, pal.InfoText
					}
					aiTag(c, label, bg, ink).Shrink(0)
				}
				break
			}
		}
		pathSlash(c)
		v := dockerViews()[s.view]
		anchor = pathButton(c, "engine-view", "lucide/"+v.icon, v.label, 0)
		anchor.OnClick(a.toggleDockerViews)
		ui.Popover(c, anchor, &s.viewsOpen, func() { a.dockerViewPicker(c) })
		if l := s.containers.logs; l != nil {
			pathSlash(c)
			clipped(c, l.name).Font(FontMono).FontSize(12).MinWidth(0).Shrink(1).Tooltip(l.name)
			aiTag(c, locales.KubeLogsTag(), pal.Muted, t.TextMuted).Shrink(0)
			ui.Box(c).Grow(1)
			a.logActions(c, &l.logView, l.name)
			return
		}
		ui.Box(c).Grow(1).Height(1)
		a.dockerFreshness(c)
		refresh := ui.ButtonBase(c.Key(locales.KubeClusterRefresh())).Size(24, 24).Radius(6).Center().
			Label(locales.KubeClusterRefresh()).Tooltip(locales.KubeClusterRefresh()).OnClick(func() { s.refresh++ })
		if refresh.Hovered() {
			refresh.Background(pal.Muted)
		}
		refresh.Children(func() { spinIcon(c, "refresh-cw", 14, t.TextMuted, s.reading) })
	})
}

// dockerFreshness says when the engine was read: how long ago while reads
// keep coming, and why after one failed.
func (a *App) dockerFreshness(c *ui.Context) {
	s := &a.engine
	t := c.Theme()
	dot, text := Colors(t).TextFaint, locales.KubeClusterConnecting()
	switch {
	case s.err != nil:
		dot, text = t.Warning, locales.DockerUnavailable()
	case s.data != nil:
		since := max(c.Now().Sub(s.data.Taken), 0)
		c.After(time.Second - since%time.Second)
		dot, text = t.Success, locales.KubeClusterFresh(int(since/time.Second))
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, text).FontSize(11.5).TextColor(t.TextMuted).SingleLine().MaxWidth(260)
	})
}

// dockerContextPopover chooses the local engine the page shows; it does
// not change the CLI's context.
func (a *App) dockerContextPopover(c *ui.Context, name string) {
	t := c.Theme()
	pal := Colors(t)
	st, _ := a.Snapshot.Tool("docker")
	ui.Column(c).Width(400).Gap(2).Padding(6).Children(func() {
		if st.Docker != nil {
			for _, k := range st.Docker.Contexts {
				if !k.Local {
					continue
				}
				row := ui.ButtonBase(c.Key("engine-context-"+k.Name)).Padding(7, 10).Radius(7).AlignItems(ui.Center).Gap(10).Label(k.Name)
				if k.Name == name {
					row.Background(pal.ActiveRow)
				} else if row.Hovered() {
					row.Background(pal.Muted)
				}
				row.OnClick(func() {
					a.engine.contextOpen = false
					if k.Name != name {
						a.router.Replace(dockerEnginePath(k.Name))
					}
				}).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							clipped(c, k.Name).FontSize(13).FontWeight(500)
							if k.Name == st.Docker.Selected {
								aiTag(c, locales.DockerInCli(), pal.InfoSoft, pal.InfoText)
							}
						})
						clipped(c, dockerContextCaption(k, a.Home)).FontSize(10.5).TextColor(pal.TextFaint)
					})
				})
			}
		}
		ui.Box(c).Height(1).Background(pal.Line).Margin(4, 0)
		ui.Row(c).Gap(8).Padding(4, 10).Children(func() {
			ui.Icon(c, Icon("info")).Size(14, 14).TextColor(pal.Info)
			ui.Text(c, locales.DockerPickNote()).FontSize(11.5).TextColor(t.TextMuted).Wrap().Grow(1)
		})
		ui.Row(c).Padding(4, 10, 6, 10).Children(func() {
			ui.Box(c).Grow(1)
			ui.Button(c.Key("engine-all"), locales.KubeClusterAllContexts()+" ›").OnClick(func() {
				a.engine.contextOpen = false
				a.router.Push(dockerPage)
			})
		})
	})
}

// dockerViewPicker chooses what the engine page shows (⌘J).
func (a *App) dockerViewPicker(c *ui.Context) {
	s := &a.engine
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Width(260).Gap(2).Padding(6).Children(func() {
		ui.Text(c, locales.DockerViews()).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).Padding(6, 10, 4, 10)
		for i, v := range dockerViews() {
			row := ui.ButtonBase(c.Key("engine-view-"+strconv.Itoa(i))).Padding(7, 10).Radius(7).AlignItems(ui.Center).Gap(10).Label(v.label)
			if i == s.view {
				row.Background(pal.ActiveRow)
			} else if row.Hovered() {
				row.Background(pal.Muted)
			}
			row.OnClick(func() {
				s.view, s.viewsOpen = i, false
			}).Children(func() {
				ui.Icon(c, Icon(v.icon)).Size(14, 14).TextColor(t.TextMuted)
				ui.Text(c, v.label).FontSize(13).Grow(1)
			})
		}
	})
}

// dockerEngineDown is an engine that did not answer: what Docker said, and
// the plan that starts it when it is the selected context's.
func (a *App) dockerEngineDown(c *ui.Context, name string, err error) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(8).Padding(14, 16).Radius(12).Background(pal.WarningSoft).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, Icon("triangle-alert")).Size(15, 15).TextColor(t.Warning)
			ui.Text(c, locales.DockerEngineDown(name)).FontSize(13.5).FontWeight(600).TextColor(pal.WarningText)
		})
		ui.Text(c, err.Error()).Font(FontMono).FontSize(11).TextColor(pal.WarningText).Wrap()
		ui.Text(c, locales.DockerEngineDownHint()).FontSize(12).TextColor(t.TextMuted).Wrap()
		ui.Row(c).Gap(8).Padding(4, 0, 0, 0).Children(func() {
			st, _ := a.Snapshot.Tool("docker")
			if profile, ok := dockerColimaProfile(st.Docker, name); ok && profile.Status == "stopped" {
				ui.PrimaryButton(c.Key("engine-start-colima"), locales.TrayColimaStart()).Height(controlHeight).Disabled(a.BuildPlan == nil).
					OnClick(func() { a.changeColima(*st.Docker.Colima, profile, ops.ColimaStart, ops.ColimaLimits{}) })
			} else if plan, ok := a.engineStartPlan(); ok && st.Docker != nil && st.Docker.Selected == name {
				ui.PrimaryButton(c.Key("engine-start"), plan.Title+"…").Height(controlHeight).OnClick(func() { a.Propose(plan) })
			}
			ui.Button(c.Key("engine-contexts"), locales.KubeClusterAllContexts()).Height(controlHeight).OnClick(func() { a.router.Push(dockerPage) })
		})
	})
}

// dockerContextCaption is what a context row says under its name: its
// engine and address.
func dockerContextCaption(k inventory.DockerContext, home string) string {
	text := endpointText(k.Endpoint, home)
	if k.Engine != "" {
		text = k.Engine + " · " + text
	}
	return text
}
