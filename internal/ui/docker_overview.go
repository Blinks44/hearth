package ui

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) dockerOverviewView(c *ui.Context, name string, e docker.Engine) {
	target, _ := a.engineTarget(name)
	o := e.Overview
	pal := Colors(c.Theme())
	var bytes, reclaimable int64
	for _, usage := range o.Disk.Usage {
		bytes += usage.Bytes
		reclaimable += usage.Reclaimable
	}
	total, reclaim := "-", locales.DockerOverviewDiskUnavailable()
	if o.Disk.Usage != nil {
		total, reclaim = formatBytes(bytes), locales.DockerOverviewReclaimable(formatBytes(reclaimable))
	}
	provider := cmp.Or(target.Provider, "Docker")
	engine := "Docker " + e.Version
	if provider != "Docker" {
		engine = provider + " · " + engine
	}
	ui.Row(c).Gap(12).AlignItems(ui.Stretch).Children(func() {
		overviewStat(c, locales.DockerViewContainers(), locales.DockerRunningOf(strconv.Itoa(e.Running), strconv.Itoa(e.Containers)), func() {
			dockerOverviewCaption(c, engine+" · "+e.Arch, pal.TextFaint)
			dockerOverviewCaption(c, locales.DockerStoppedPaused(strconv.Itoa(e.Stopped), strconv.Itoa(e.Paused)), pal.TextFaint)
		})
		overviewStat(c, locales.DockerOverviewDisk(), total, func() {
			dockerOverviewCaption(c, reclaim, pal.TextFaint)
		})
		dockerOverviewMetric(c, locales.DockerOverviewCPU(), o.Cores, float64(e.CPUs), o.CPURead,
			locales.KubeOverviewCores(o.Cores, float64(e.CPUs)), locales.DockerOverviewCPUCapacity(e.CPUs), Accent)
		dockerOverviewMetric(c, locales.DockerOverviewMemory(), float64(o.Memory), float64(e.Memory), o.MemoryRead,
			locales.DockerOverviewMemoryAmount(formatBytes(o.Memory), formatBytes(e.Memory)), formatBytes(e.Memory), pal.WarningText)
	})
	st, _ := a.Snapshot.Tool("docker")
	columns(c, 14, func() {
		a.dockerOverviewDisk(c, st.Docker, target, o.Disk, total, bytes)
		dockerOverviewEvents(c, o.Events)
	}, func() {
		a.dockerHealthCard(c, st, name, e.Health, true)
		if profile, ok := dockerColimaProfile(st.Docker, name); ok {
			a.colimaCard(c, *st.Docker.Colima, profile)
		}
	})
}

func dockerOverviewCaption(c *ui.Context, label string, color ui.Color) {
	ui.Text(c, label).FontSize(11.5).TextColor(color).Wrap()
}

func dockerOverviewMetric(c *ui.Context, label string, used, total float64, sampled bool, amount, capacity string, color ui.Color) {
	pal := Colors(c.Theme())
	if !sampled || total <= 0 {
		overviewStat(c, label, capacity, func() {
			dockerOverviewCaption(c, locales.DockerOverviewSampleUnavailable(), pal.TextFaint)
		})
		return
	}
	share := used / total
	overviewStat(c, label, requestPercent(share), func() {
		requestBar(c, share, color)
		dockerOverviewCaption(c, amount, pal.TextFaint)
	})
}

func (a *App) dockerProblemRows(items []docker.Container, context string) []dockerHealthRow {
	rows := make([]dockerHealthRow, 0, len(items))
	for _, item := range items {
		row := dockerHealthRow{
			key:    "container:" + item.ID,
			title:  item.Name,
			detail: cmp.Or(dockerContainerProblem(item), locales.DockerOverviewContainerFailed()),
			danger: item.State.Status != "restarting",
		}
		switch {
		case item.ConflictPort() != 0:
			row.button = locales.DockerOpenPorts()
			row.action = func() {
				a.portsFilter = strconv.Itoa(item.ConflictPort())
				a.router.Push(portsPage)
			}
		case item.State.Status == "restarting" && a.StreamDockerLogs != nil && a.WatchDockerContainers != nil:
			row.button = locales.KubeLogsOpen()
			row.action = func() {
				a.requestDockerContainer(context, item.ID, item.Name, true, true)
			}
		case a.WatchDockerContainers != nil:
			row.button = locales.KubeClusterOpen()
			row.action = func() { a.requestDockerContainer(context, item.ID, item.Name, true, false) }
		}
		rows = append(rows, row)
	}
	return rows
}

func (a *App) requestDockerContainer(context, id, name string, overview, logs bool) {
	target, problem := a.engineTarget(context)
	if problem != "" {
		return
	}
	a.engine.view = dockerContainers
	a.engine.containers.pending = &dockerContainerRequest{target: target, item: docker.LogTarget{ID: id, Name: name}, logs: logs}
	if !overview {
		a.router.Push(dockerEnginePath(context))
	}
}

func (a *App) selectedDockerTarget() (docker.Target, bool) {
	st, ok := a.Snapshot.Tool("docker")
	if !ok || st.Docker == nil || st.Docker.Selected == "" {
		return docker.Target{}, false
	}
	target, problem := a.engineTarget(st.Docker.Selected)
	return target, problem == ""
}

func (a *App) canOpenDockerContainer() bool {
	if a.WatchDocker == nil || a.WatchDockerContainers == nil {
		return false
	}
	_, ok := a.selectedDockerTarget()
	return ok
}

// OpenDockerContainer navigates to the selected Docker context's container detail view.
func (a *App) OpenDockerContainer(id, name string) {
	a.openDockerContainer(id, name)
}

func (a *App) openDockerContainer(id, name string) {
	st, ok := a.Snapshot.Tool("docker")
	if !ok || st.Docker == nil || st.Docker.Selected == "" {
		return
	}
	a.requestDockerContainer(st.Docker.Selected, id, name, false, false)
}

func (a *App) dockerOverviewDisk(c *ui.Context, d *inventory.DockerState, target docker.Target, disk docker.OverviewDisk, total string, bytes int64) {
	pal := Colors(c.Theme())
	overviewPanel(c, locales.DockerOverviewDisk(), total, func() {
		if disk.Usage == nil {
			overviewMessage(c, locales.DockerOverviewDiskUnavailable())
		}
		for _, usage := range disk.Usage {
			label := usage.Type
			switch usage.Type {
			case "Images":
				label = locales.DockerViewImages()
			case "Containers":
				label = locales.DockerViewContainers()
			case "Local Volumes":
				label = locales.DockerViewVolumes()
			case "Build Cache":
				label = locales.DockerImageBuildCache()
			}
			ui.Column(c.Key(usage.Type)).Gap(5).Padding(8, 13).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).Grow(1).MinWidth(0).FontSize(12).Wrap()
					overviewCaption(c, formatBytes(usage.Bytes), pal.TextFaint)
					if usage.Reclaimable > 0 {
						overviewCaption(c, "· "+locales.DockerOverviewReclaimable(formatBytes(usage.Reclaimable)), pal.TextFaint)
					}
				})
				share := 0.0
				if bytes > 0 {
					share = float64(usage.Bytes) / float64(bytes)
				}
				requestBar(c, share, pal.InfoText)
			})
		}
		if disk.Usage != nil {
			var reclaimable int64
			for _, u := range disk.Usage {
				reclaimable += u.Reclaimable
			}
			navListRow(c, "docker-overview-cleanup", locales.ChangesOpenCleanup(), locales.DockerOverviewReclaimable(formatBytes(reclaimable)), locales.ChangesOpenCleanup(), func() {
				a.cleanupTab = slices.Index(cleanupGroups, cleanup.Docker) + 1
				a.cleanupFilter = ""
				a.router.Push(cleanupPage)
			})
		}
		switch target.Provider {
		case "Colima", "OrbStack", "Docker Desktop", "Podman":
			a.dockerVMDisk(c, d, target, disk.VM)
		}
	})
}

func (a *App) dockerVMDisk(c *ui.Context, d *inventory.DockerState, target docker.Target, disk docker.VMDisk) {
	pal := Colors(c.Theme())
	ui.Column(c).Gap(6).Padding(10, 13).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		if disk.Path == "" {
			dockerOverviewCaption(c, locales.DockerOverviewVMDiskUnavailable(), pal.TextFaint)
		} else {
			dockerOverviewCaption(c, locales.DockerOverviewVMDisk(formatBytes(disk.Allocated)), c.Theme().TextMuted)
			path := tilde(disk.Path, a.Home)
			dockerOverviewCaption(c, path, pal.TextFaint)
		}
		dockerOverviewCaption(c, locales.DockerOverviewPrune(), pal.TextFaint)
		switch target.Provider {
		case "Colima":
			profile, matched := dockerColimaProfile(d, target.Context)
			if matched && disk.Path != "" && profile.Status == "running" && a.canPlanDocker() {
				ui.Button(c.Key("trim"), locales.DockerOverviewTrim()).Height(24).OnClick(func() {
					a.changeColima(*d.Colima, profile, ops.ColimaTrim, ops.ColimaLimits{})
				})
			}
		case "OrbStack":
			dockerOverviewCaption(c, locales.DockerOverviewOrbReclaim(), pal.TextFaint)
		case "Docker Desktop":
			ui.Button(c.Key("desktop-disk"), locales.DockerOverviewDesktopHelp()).Height(24).OnClick(func() {
				c.OpenURL("https://docs.docker.com/desktop/settings-and-maintenance/settings/#advanced")
			})
		}
	})
}

func dockerOverviewEvents(c *ui.Context, events docker.OverviewEvents) {
	total := 0
	for _, event := range events.Items {
		total += event.Count
	}
	overviewPanel(c, locales.DockerOverviewEvents(), locales.DockerOverviewEventCount(strconv.Itoa(total)), func() {
		switch {
		case events.Err != nil:
			overviewMessage(c, locales.DockerOverviewEventsUnavailable())
		case len(events.Items) == 0:
			overviewMessage(c, locales.DockerOverviewNoEvents())
		}
		pal := Colors(c.Theme())
		for _, event := range events.Items {
			ui.Row(c).Gap(8).Padding(5, 13).AlignItems(ui.Center).Children(func() {
				ui.Text(c, event.Action).Font(FontMono).FontSize(10.5).TextColor(pal.WarningText)
				name := event.Name
				if code, err := strconv.Atoi(event.ExitCode); err == nil {
					name += " · " + locales.DockerContainerExit(code)
				}
				ui.Text(c, name).Font(FontMono).FontSize(11).Grow(1).Wrap()
				overviewCaption(c, "×"+strconv.Itoa(event.Count), pal.TextFaint)
			})
		}
	})
}
