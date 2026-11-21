package ui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

type dockerDetailState struct {
	id           string
	tab          int
	key          string
	request      *byte
	stop         func()
	data         *docker.ContainerDetail
	err          error
	secret       secretView
	shellRequest *byte
}

func (s *dockerContainersState) selectContainer(id string) {
	s.closeDetail()
	s.detail.id = id
}

func (s *dockerContainersState) closeDetail() {
	d := &s.detail
	if d.stop != nil {
		d.stop()
	}
	d.secret.reset()
	*d = dockerDetailState{}
}

func (s *dockerContainersState) selectedContainer() (docker.Container, bool) {
	if i := slices.IndexFunc(s.items, func(c docker.Container) bool { return c.ID == s.detail.id }); i >= 0 {
		return s.items[i], true
	}
	return docker.Container{}, false
}

func (a *App) syncDockerDetail(c *ui.Context) {
	s := &a.engine.containers
	d := &s.detail
	if d.id == "" {
		return
	}
	item, ok := s.selectedContainer()
	if !ok || s.watching == "" {
		s.closeDetail()
		return
	}
	if d.tab != 1 || s.logs != nil || s.err != nil {
		d.secret.reset()
	} else {
		d.secret.tick(c)
	}
	if a.ReadDockerDetail == nil || s.logs != nil {
		return
	}
	status := ""
	if item.State.Health != nil {
		status = item.State.Health.Status
	}
	key := fmt.Sprintf("%s/%s/%s/%d/%s", s.watching, item.State.Status, item.State.StartedAt, item.RestartCount, status)
	if key == d.key {
		return
	}
	if d.stop != nil {
		d.stop()
	}
	req := new(byte)
	d.key, d.err, d.request = key, nil, req
	d.stop = a.ReadDockerDetail(s.target, item.ID, func(detail docker.ContainerDetail, err error) {
		if d.request != req {
			return
		}
		d.err = err
		if err == nil {
			d.data = &detail
		}
	})
}

func (a *App) dockerDetails(c *ui.Context, item docker.Container) {
	s := &a.engine.containers
	d := &s.detail
	t := c.Theme()
	pal := Colors(t)
	dot := pal.TextFaint.Alpha(0.45)
	switch {
	case item.Problem() && item.State.Status != "restarting":
		dot = t.Danger
	case item.State.Status == "restarting" || item.State.Status == "paused":
		dot = t.Warning
	case item.State.Status == "running":
		dot = t.Success
	}
	detailFrame(c, d.tab, false, func() {
		ui.Column(c).Gap(8).Padding(16, 16, 12, 16).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				status, bg, fg := dockerContainerStatus(c, item)
				dockerTechTile(c, item.Image, 30, &dot)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						clipped(c, item.Name).Font(FontMono).FontSize(15).FontWeight(500).MinWidth(0).Tooltip(item.Name)
						aiTag(c, status, bg, fg).MaxWidthPercent(40).Shrink(0).Tooltip(status)
					})
					if data := d.data; data != nil {
						times := locales.KubeCreatedAgo(age(c, data.Created))
						if !item.Active() && !data.Finished.IsZero() {
							times += " · " + locales.DockerFinishedAgo(age(c, data.Finished))
						}
						clipped(c, times).FontSize(11.5).TextColor(t.TextMuted).Tooltip(times)
					} else if item.Image != "" {
						clipped(c, item.Image).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted)
					}
				})
				iconButton(c.Key("docker-close-detail"), "x", locales.KubeCloseDetails()).OnClick(s.closeDetail)
			})
			if item.Compose.Name != "" {
				a.dockerDetailProject(c, item)
			}
			a.dockerDetailActions(c, item)
		})
		ui.Row(c).Padding(0, 16, 12, 16).Children(func() {
			count := ""
			if d.data != nil {
				count = strconv.Itoa(len(d.data.EnvKeys))
			}
			segments(c, "docker-detail-tab", &d.tab, []segment{
				{locales.KubeTabOverview(), ""},
				{locales.DockerEnvTab(), count},
				{locales.DockerInspectTab(), ""},
			})
		})
	}, func() {
		if d.err != nil {
			ui.Column(c).Padding(6, 16, 8, 16).Children(func() { warningRow(c, d.err.Error()) })
		}
		switch {
		case d.data == nil && d.err == nil:
			ui.Column(c).Padding(8, 16).Children(func() { tableSkeleton(c, locales.DockerDetailReading()) })
		case d.data == nil:
		case d.tab == 1:
			a.dockerEnv(c, item.ID)
		case d.tab == 2:
			ui.Column(c).Gap(10).Padding(4, 16, 12, 16).Children(func() {
				toolNote(c, locales.DockerInspectMasked())
				ui.Column(c).Padding(10, 12).Radius(8).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
					ui.Text(c, d.data.Inspect).Font(FontMono).FontSize(11.5).Wrap().Selectable()
				})
			})
		default:
			a.dockerDetailOverview(c, item, d.data)
		}
	})
}

func (a *App) dockerDetailProject(c *ui.Context, item docker.Container) {
	name := "Compose " + item.Compose.Name
	if item.Compose.Dir != "" {
		name += " · " + tilde(item.Compose.Dir, a.Home)
	}
	if slices.ContainsFunc(a.Projects, func(p projects.Project) bool { return p.Dir == item.Compose.Dir }) {
		pal := Colors(c.Theme())
		b := ui.ButtonBase(c.Key("docker-detail-project")).Height(26).Radius(13).Padding(0, 8).Gap(6).
			AlignSelf(ui.Start).MaxWidthPercent(100).MinWidth(0).AlignItems(ui.Center).
			BorderWidth(1).BorderColor(pal.ControlBorder).Label(name).Tooltip(name).
			OnClick(func() { a.router.Push(projectPath(item.Compose.Dir)) })
		if b.Hovered() {
			b.Background(pal.Muted)
		}
		b.Children(func() {
			ui.Icon(c, Icon("folder-open")).Size(14, 14).Shrink(0)
			clipped(c, name).FontSize(12).MinWidth(0)
		})
		return
	}
	clipped(c, name).FontSize(12).TextColor(c.Theme().TextMuted).Tooltip(name)
}

func (a *App) dockerDetailActions(c *ui.Context, item docker.Container) {
	s := &a.engine.containers
	ui.Row(c).Gap(7).AlignItems(ui.Center).Wrap().Children(func() {
		kubeActionButton(c.Key("docker-detail-logs"), "scroll-text", locales.KubeLogsOpen(), 28).
			Disabled(a.StreamDockerLogs == nil).OnClick(func() { a.openDockerLogs(item.Name, []docker.LogTarget{{ID: item.ID}}, false) })
		action, icon := ops.DockerStart, "play"
		if item.Active() {
			action, icon = ops.DockerStop, "square"
		}
		if action.Applies(item) {
			kubeActionButton(c.Key("docker-detail-action"), icon, action.Label(), 28).
				Disabled(!a.canActOnDockerContainers()).OnClick(func() { a.planDockerContainers([]docker.Container{item}, action) })
		}
		kubeActionButton(c.Key("docker-detail-shell"), "terminal", locales.KubeShellAction(), 28).
			Disabled(a.OpenDockerShell == nil || item.State.Status != "running" || s.detail.shellRequest != nil || s.err != nil).
			OnClick(func() { a.openDockerShell(item) })
		ui.Box(c).Grow(1)
		kubeActionButton(c.Key("docker-detail-remove"), "trash-2", ops.DockerRemove.Label()+"…", 28).
			TextColor(Colors(c.Theme()).DangerText).Disabled(!a.canActOnDockerContainers()).OnClick(func() { a.planDockerContainers([]docker.Container{item}, ops.DockerRemove) })
	})
}

func (a *App) dockerDetailOverview(c *ui.Context, item docker.Container, d *docker.ContainerDetail) {
	t := c.Theme()
	pal := Colors(t)
	if problem := dockerContainerProblem(item); problem != "" {
		bg, fg, icon := pal.DangerSoft, pal.DangerText, "circle-alert"
		if item.State.Status == "restarting" {
			bg, fg, icon = pal.WarningSoft, pal.WarningText, "triangle-alert"
		}
		ui.Column(c).Gap(8).Margin(0, 16, 10, 16).Padding(12, 14).Radius(10).Background(bg).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
				ui.Icon(c, Icon(icon)).Size(15, 15).Shrink(0).TextColor(fg).Margin(1, 0, 0, 0)
				ui.Text(c, problem).FontSize(12.5).FontWeight(500).TextColor(fg).Grow(1).MinWidth(0).Wrap()
			})
			if port := item.ConflictPort(); port > 0 {
				ui.Row(c).Padding(0, 0, 0, 23).Children(func() {
					ui.Button(c.Key("docker-detail-conflict"), locales.DockerOpenPorts()).Height(24).OnClick(func() {
						a.portsFilter = strconv.Itoa(port)
						a.router.Push(portsPage)
					})
				})
			}
		})
	}
	if item.Active() && (item.CPU != "" || item.Memory != "") {
		ui.Row(c).Gap(10).Margin(0, 16, 10, 16).Children(func() {
			dockerDetailMetricTile(c, "CPU", cmp.Or(item.CPU, "—"))
			dockerDetailMetricTile(c, locales.KubeMetricsMemory(), cmp.Or(item.Memory, "—"))
		})
	}
	if len(d.Ports)+len(d.Mounts) != 0 {
		detailSection(c, locales.DockerPortsMounts())
		a.dockerPortsMounts(c, item, d)
	}
	detailSection(c, locales.Details())
	a.dockerDetailFactsCard(c, item, d)
	if len(d.Health) > 0 {
		detailSection(c, locales.DockerHealthChecks())
		dockerHealthChecksCard(c, d.Health)
	}
	if len(d.Networks) > 0 {
		detailSection(c, locales.DockerViewNetworks())
		dockerNetworksCard(c, d.Networks)
	}
	if len(d.Labels) > 0 {
		detailSection(c, locales.DockerLabels())
		dockerLabelsCard(c, d.Labels)
	}
}

func dockerDetailMetricTile(c *ui.Context, label, value string) {
	pal := Colors(c.Theme())
	ui.Column(c).Grow(1).MinWidth(0).Gap(3).Padding(9, 12).Radius(9).
		Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Text(c, strings.ToUpper(label)).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
		ui.Text(c, value).Font(FontMono).FontSize(15).FontWeight(600).SingleLine()
	})
}

func (a *App) dockerDetailFactsCard(c *ui.Context, item docker.Container, d *docker.ContainerDetail) {
	t := c.Theme()
	pal := Colors(t)
	command := strings.TrimSpace(strings.Join(append([]string{d.Path}, d.Args...), " "))
	type factRow struct {
		label, display, copyVal, tooltip string
		chip                             bool
		chipBg, chipFg                   ui.Color
	}
	var rows []factRow
	if d.Image != "" {
		rows = append(rows, factRow{label: locales.DockerImage(), display: d.Image, copyVal: d.Image, tooltip: d.Image})
	}
	if d.ImageID != "" {
		rows = append(rows, factRow{label: locales.DockerImageID(), display: shortDockerHash(d.ImageID), copyVal: d.ImageID, tooltip: d.ImageID})
	}
	for _, digest := range d.Digests {
		rows = append(rows, factRow{label: locales.DockerImageDigest(), display: shortDockerHash(digest), copyVal: digest, tooltip: digest})
	}
	if command != "" {
		rows = append(rows, factRow{label: locales.DockerCommand(), display: command, copyVal: command, tooltip: command})
	}
	if d.Platform != "" {
		rows = append(rows, factRow{label: locales.DockerPlatform(), display: d.Platform})
	}
	if d.RestartPolicy != "" {
		rows = append(rows, factRow{label: locales.DockerRestartPolicy(), display: d.RestartPolicy})
	}
	if item.State.Health != nil && item.Active() {
		hl, hbg, hfg := dockerHealthBadge(c, item.State.Health.Status)
		rows = append(rows, factRow{label: locales.DockerHealth(), display: hl, chip: true, chipBg: hbg, chipFg: hfg})
	}
	if len(rows) == 0 {
		return
	}
	ui.Column(c).Margin(0, 16, 6, 16).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
		for i, r := range rows {
			row := ui.Row(c.Key(fmt.Sprintf("fact-%d", i))).Padding(7, 12).Gap(10).AlignItems(ui.Center)
			if i < len(rows)-1 {
				row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
			}
			row.Children(func() {
				ui.Text(c, r.label).Width(116).Shrink(0).FontSize(12).TextColor(t.TextMuted).Wrap()
				if r.chip {
					ui.Row(c).Grow(1).MinWidth(0).Children(func() {
						aiTag(c, r.display, r.chipBg, r.chipFg)
					})
				} else {
					val := ui.Text(c, r.display).Grow(1).MinWidth(0).Font(FontMono).FontSize(11.5).Wrap().Selectable()
					if r.tooltip != "" {
						val.Tooltip(r.tooltip)
					}
				}
				if r.copyVal != "" {
					val := r.copyVal
					iconButton(c.Key("copy"), "copy", locales.KubeCopyName()).OnClick(func() { copyName(c, val) })
				}
			})
		}
	})
}

func shortDockerHash(s string) string {
	if prefix, hex, ok := strings.Cut(s, "sha256:"); ok && len(hex) > 12 {
		return prefix + "sha256:" + hex[:12] + "…"
	}
	return s
}

func dockerHealthBadge(c *ui.Context, status string) (string, ui.Color, ui.Color) {
	t := c.Theme()
	pal := Colors(t)
	switch status {
	case "healthy":
		return locales.DockerHealthHealthy(), pal.SuccessSoft, pal.SuccessText
	case "unhealthy":
		return locales.DockerHealthUnhealthy(), pal.DangerSoft, pal.DangerText
	case "starting":
		return locales.DockerHealthStarting(), pal.WarningSoft, pal.WarningText
	default:
		return status, pal.Muted, t.TextMuted
	}
}

func dockerHealthChecksCard(c *ui.Context, checks []docker.HealthCheck) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Margin(0, 16, 6, 16).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
		total := len(checks)
		for idx, check := range slices.Backward(checks) {
			row := ui.Row(c.Key(fmt.Sprintf("hc-%d", idx))).Padding(8, 12).Gap(10).AlignItems(ui.Start)
			if idx > 0 && total > 1 {
				row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
			}
			ok := check.ExitCode == 0
			icon, color, badgeBg, badgeFg, badgeText := "circle-x", t.Danger, pal.DangerSoft, pal.DangerText, locales.DockerContainerExit(check.ExitCode)
			if ok {
				icon, color, badgeBg, badgeFg, badgeText = "circle-check", t.Success, pal.SuccessSoft, pal.SuccessText, locales.DockerHealthPassed()
			}
			row.Children(func() {
				ui.Icon(c, Icon(icon)).Size(14, 14).Shrink(0).TextColor(color).Margin(2, 0, 0, 0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						aiTag(c, badgeText, badgeBg, badgeFg).Shrink(0)
						ui.Text(c, check.End.Local().Format("15:04:05")).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
					})
					if output := strings.TrimSpace(check.Output); output != "" {
						ui.Text(c, output).Font(FontMono).FontSize(11).TextColor(t.TextMuted).Wrap().Selectable()
					}
				})
			})
		}
	})
}

func dockerNetworksCard(c *ui.Context, networks map[string]string) {
	pal := Colors(c.Theme())
	names := slices.Sorted(maps.Keys(networks))
	ui.Column(c).Margin(0, 16, 6, 16).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
		for i, name := range names {
			ip := networks[name]
			row := ui.Row(c.Key("net-"+name)).Padding(7, 12).Gap(10).AlignItems(ui.Center)
			if i < len(names)-1 {
				row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
			}
			row.Children(func() {
				ui.Icon(c, Icon("network")).Size(13, 13).Shrink(0).TextColor(pal.TextFaint)
				clipped(c, name).Font(FontMono).FontSize(12).Grow(1).MinWidth(0).Tooltip(name)
				if ip != "" {
					aiTag(c, ip, pal.Muted, pal.MonoText).Font(FontMono).Shrink(0)
					iconButton(c.Key("copy-ip"), "copy", locales.KubeCopyName()).OnClick(func() { copyName(c, ip) })
				}
			})
		}
	})
}

func dockerLabelsCard(c *ui.Context, labels map[string]string) {
	t := c.Theme()
	pal := Colors(t)
	keys := slices.Sorted(maps.Keys(labels))
	ui.Column(c).Margin(0, 16, 6, 16).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
		for i, key := range keys {
			row := ui.Column(c.Key("lbl-"+key)).Padding(7, 12).Gap(2)
			if i < len(keys)-1 {
				row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
			}
			row.Children(func() {
				ui.Text(c, key).Font(FontMono).FontSize(10.5).TextColor(t.TextMuted).Wrap().Selectable()
				ui.Text(c, cmp.Or(labels[key], "—")).Font(FontMono).FontSize(11.5).Wrap().Selectable()
			})
		}
	})
}

func (a *App) dockerPortsMounts(c *ui.Context, item docker.Container, d *docker.ContainerDetail) {
	pal := Colors(c.Theme())
	type entry struct {
		key, icon, name, info, url, button string
	}
	var entries []entry
	for _, port := range slices.Sorted(maps.Keys(d.Ports)) {
		if len(d.Ports[port]) == 0 {
			entries = append(entries, entry{key: "port:" + port, icon: "network", name: port, info: locales.DockerUnpublishedPort()})
		}
		for _, binding := range d.Ports[port] {
			address, button := binding.URL(port), ""
			if item.State.Status == "running" && address != "" {
				button = locales.OpOpenLink()
			}
			info := cmp.Or(address, strings.Trim(binding.HostIP+":"+binding.HostPort, ":"))
			if item.State.Status != "running" {
				info = strings.TrimSpace(info + " · " + locales.DockerContainerStopped())
			}
			entries = append(entries, entry{
				key:    binding.HostIP + ":" + binding.HostPort + "/" + port,
				icon:   "network",
				name:   binding.HostPort + " → " + port,
				info:   info,
				url:    address,
				button: button,
			})
		}
	}
	for i, mount := range d.Mounts {
		access := locales.DockerMountReadOnly()
		if mount.RW {
			access = locales.DockerMountWrite()
		}
		source := cmp.Or(mount.Name, tilde(mount.Source, a.Home), mount.Type)
		entries = append(entries, entry{
			key:  fmt.Sprintf("mount:%d", i),
			icon: "hard-drive",
			name: source + " → " + mount.Destination,
			info: mount.Type + " · " + access,
		})
	}
	ui.Column(c).Margin(0, 16, 6, 16).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
		for i, e := range entries {
			row := ui.Row(c.Key(e.key)).Padding(8, 12).Gap(10).AlignItems(ui.Center)
			if i < len(entries)-1 {
				row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
			}
			row.Children(func() {
				ui.Icon(c, Icon(e.icon)).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, e.name).Font(FontMono).FontSize(12).Wrap().Selectable()
					ui.Text(c, e.info).FontSize(11).TextColor(pal.TextFaint).Wrap()
				})
				if e.button != "" {
					url := e.url
					kubeActionButton(c.Key("open-port"), "external-link", e.button, 24).Tooltip(url).OnClick(func() { c.OpenURL(url) })
				}
			})
		}
	})
}

func (a *App) dockerEnv(c *ui.Context, id string) {
	s := &a.engine.containers
	v := &s.detail.secret
	pal := Colors(c.Theme())
	ui.Column(c).Gap(10).Padding(4, 16, 12, 16).Children(func() {
		toolNote(c, locales.DockerInspectMasked())
		if v.err != "" {
			warningRow(c, v.err)
		}
		if len(s.detail.data.EnvKeys) == 0 {
			toolNote(c, locales.KubeSecretNoKeys())
			return
		}
		services := c.Services()
		canRead := a.ReadDockerEnv != nil && a.SecretTimer != nil && s.err == nil
		ui.Column(c).Radius(9).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
			for i, key := range s.detail.data.EnvKeys {
				row := ui.Row(c.Key(key)).Gap(6).AlignItems(ui.Center).Padding(7, 12)
				if i < len(s.detail.data.EnvKeys)-1 {
					row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
				}
				row.Children(func() {
					reveal := v.revealed
					shown := reveal != nil && reveal.key == key
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						clipped(c, key).Font(FontMono).FontSize(12).Tooltip(key)
						switch {
						case !shown:
							ui.Text(c, "••••••••").Font(FontMono).FontSize(11).TextColor(c.Theme().TextMuted)
						case len(reveal.value) == 0:
							ui.Text(c, locales.KubeSecretEmptyValue()).FontSize(11).TextColor(c.Theme().TextMuted)
						default:
							secretValue(c, reveal)
						}
					})
					if shown {
						iconButton(c, "eye-off", locales.KubeSecretHide(key)).OnClick(v.hide)
					} else {
						iconButton(c, "eye", locales.KubeSecretShow(key)).Disabled(!canRead).
							OnClick(func() { a.loadDockerEnv(services, id, key, false) })
					}
					iconButton(c, "copy", locales.KubeSecretCopy(key)).Disabled(!canRead).
						OnClick(func() { a.loadDockerEnv(services, id, key, true) })
				})
			}
		})
	})
}

func (a *App) loadDockerEnv(services ui.Services, id, key string, copyValue bool) {
	s := &a.engine.containers
	v := &s.detail.secret
	v.cancel()
	if !copyValue {
		v.hide()
	}
	v.err = ""
	req := &secretRequest{key: key}
	v.request = req
	target := s.target
	req.stop = a.ReadDockerEnv(target, id, key, func(value []byte, err error) {
		if v.request != req || s.detail.tab != 1 || !a.isWatchingDockerContainer(target) {
			clear(value)
			return
		}
		v.cancel()
		if err != nil {
			clear(value)
			v.err = err.Error()
			return
		}
		a.presentSecret(services, v, key, value, copyValue)
	})
}

func (a *App) isWatchingDockerContainer(target docker.Target) bool {
	if a.engine.view != dockerContainers || a.router.Path() != dockerEnginePage {
		return false
	}
	current, problem := a.engineTarget(a.router.Query("context"))
	return problem == "" && current == target
}
