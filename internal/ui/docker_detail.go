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
	shellOpening bool
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
	detailFrame(c, d.tab, false, func() {
		ui.Column(c).Gap(8).Padding(14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				status, bg, fg := dockerContainerStatus(c, item)
				ui.Icon(c, Icon("container")).Size(17, 17).TextColor(fg).Shrink(0)
				clipped(c, item.Name).Font(FontMono).FontSize(15).MinWidth(0).Tooltip(item.Name)
				aiTag(c, status, bg, fg).MaxWidthPercent(38).Tooltip(status)
				ui.Box(c).Grow(1)
				iconButton(c.Key("docker-close-detail"), "x", locales.KubeCloseDetails()).OnClick(s.closeDetail)
			})
			if data := d.data; data != nil {
				times := locales.KubeCreatedAgo(age(c, data.Created))
				if !item.Active() && !data.Finished.IsZero() {
					times += " · " + locales.DockerFinishedAgo(age(c, data.Finished))
				}
				ui.Text(c, times).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
			if item.Compose.Name != "" {
				a.dockerDetailProject(c, item)
			}
			a.dockerDetailActions(c, item)
		})
		ui.Column(c).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Padding(10, 16).AlignItems(ui.Start).Children(func() {
			count := ""
			if d.data != nil {
				count = strconv.Itoa(len(d.data.EnvKeys))
			}
			segments(c, "docker-detail-tab", &d.tab, []segment{{locales.KubeTabOverview(), ""}, {locales.DockerEnvTab(), count}, {locales.DockerInspectTab(), ""}})
		})
	}, func() {
		ui.Column(c).Gap(13).Padding(3, 16, 10, 16).Children(func() {
			if d.err != nil {
				warningRow(c, d.err.Error())
			}
			switch {
			case d.data == nil && d.err == nil:
				tableSkeleton(c, locales.DockerDetailReading())
			case d.data == nil:
			case d.tab == 1:
				a.dockerEnv(c, item.ID)
			case d.tab == 2:
				toolNote(c, locales.DockerInspectMasked())
				ui.Text(c, d.data.Inspect).Font(FontMono).FontSize(11.5).Wrap().Selectable()
			default:
				a.dockerDetailOverview(c, item, d.data)
			}
		})
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
			action, icon = ops.DockerStop, "circle-x"
		}
		if action.Applies(item) {
			kubeActionButton(c.Key("docker-detail-action"), icon, action.Label(), 28).
				Disabled(!a.canActOnDockerContainers()).OnClick(func() { a.planDockerContainers([]docker.Container{item}, action) })
		}
		kubeActionButton(c.Key("docker-detail-shell"), "terminal", locales.KubeShellAction(), 28).
			Disabled(a.OpenDockerShell == nil || item.State.Status != "running" || s.detail.shellOpening || s.err != nil).
			OnClick(func() {
				s.detail.shellOpening = true
				a.OpenDockerShell(s.target, item.ID, func(err error) {
					if s.detail.id == item.ID {
						s.detail.shellOpening = false
					}
					if err != nil {
						a.planErr = err.Error()
					}
				})
			})
		ui.Box(c).Grow(1)
		kubeActionButton(c.Key("docker-detail-remove"), "trash-2", ops.DockerRemove.Label()+"…", 28).
			TextColor(Colors(c.Theme()).DangerText).Disabled(!a.canActOnDockerContainers()).OnClick(func() { a.planDockerContainers([]docker.Container{item}, ops.DockerRemove) })
	})
}

func (a *App) dockerDetailOverview(c *ui.Context, item docker.Container, d *docker.ContainerDetail) {
	if problem := dockerContainerProblem(item); problem != "" {
		pal := Colors(c.Theme())
		ui.Column(c).Gap(8).Padding(11, 12).Radius(9).Background(pal.DangerSoft).Children(func() {
			warningRow(c, problem)
			if port := item.ConflictPort(); port > 0 {
				ui.Button(c.Key("docker-detail-conflict"), locales.DockerOpenPorts()).Height(24).OnClick(func() {
					a.portsFilter = strconv.Itoa(port)
					a.router.Push(portsPage)
				})
			}
		})
	}
	if len(d.Ports)+len(d.Mounts) != 0 {
		detailSection(c, locales.DockerPortsMounts())
		a.dockerPortsMounts(c, item, d)
	}
	detailSection(c, locales.Details())
	dockerDetailField(c, locales.DockerImage(), d.Image)
	dockerDetailField(c, locales.DockerImageID(), d.ImageID)
	for _, digest := range d.Digests {
		dockerDetailField(c, locales.DockerImageDigest(), digest)
	}
	dockerDetailField(c, locales.DockerCommand(), strings.Join(append([]string{d.Path}, d.Args...), " "))
	dockerDetailField(c, locales.DockerPlatform(), d.Platform)
	dockerDetailField(c, locales.DockerRestartPolicy(), d.RestartPolicy)
	if item.State.Health != nil {
		dockerDetailField(c, locales.DockerHealth(), item.State.Health.Status)
	}
	if len(d.Health) > 0 {
		detailSection(c, locales.DockerHealthChecks())
		for _, check := range slices.Backward(d.Health) {
			result := locales.DockerContainerExit(check.ExitCode)
			if output := strings.TrimSpace(check.Output); output != "" {
				result += " · " + output
			}
			dockerDetailField(c, check.End.Local().Format("15:04:05"), result)
		}
	}
	if len(d.Networks) > 0 {
		detailSection(c, locales.DockerViewNetworks())
		for _, name := range slices.Sorted(maps.Keys(d.Networks)) {
			dockerDetailField(c, name, d.Networks[name])
		}
	}
	if len(d.Labels) > 0 {
		detailSection(c, locales.DockerLabels())
		for _, key := range slices.Sorted(maps.Keys(d.Labels)) {
			dockerDetailField(c, key, d.Labels[key])
		}
	}
}

func (a *App) dockerPortsMounts(c *ui.Context, item docker.Container, d *docker.ContainerDetail) {
	pal := Colors(c.Theme())
	ui.Column(c).Radius(8).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
		for _, port := range slices.Sorted(maps.Keys(d.Ports)) {
			if len(d.Ports[port]) == 0 {
				dockerDetailResource(c, port, locales.DockerUnpublishedPort(), "", "")
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
				dockerDetailResource(c.Key(binding.HostIP+":"+binding.HostPort+"/"+port), binding.HostPort+" → "+port, info, address, button)
			}
		}
		for _, mount := range d.Mounts {
			access := locales.DockerMountReadOnly()
			if mount.RW {
				access = locales.DockerMountWrite()
			}
			source := cmp.Or(mount.Name, tilde(mount.Source, a.Home), mount.Type)
			dockerDetailResource(c, source+" → "+mount.Destination, mount.Type+" · "+access, "", "")
		}
	})
}

func dockerDetailResource(c *ui.Context, name, info, url, button string) {
	pal := Colors(c.Theme())
	ui.Row(c).Padding(7, 10).Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, name).Font(FontMono).FontSize(12).Wrap().Selectable()
			ui.Text(c, info).FontSize(11).TextColor(pal.TextFaint).Wrap()
		})
		if button != "" {
			kubeActionButton(c.Key("open-port"), "external-link", button, 24).Tooltip(url).OnClick(func() { c.OpenURL(url) })
		}
	})
}

func dockerDetailField(c *ui.Context, name, value string) {
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Text(c, name).Width(105).Shrink(0).FontSize(11.5).TextColor(c.Theme().TextMuted).Wrap().Selectable()
		ui.Text(c, cmp.Or(value, "—")).Grow(1).MinWidth(0).Font(FontMono).FontSize(11.5).Wrap().Selectable()
	})
}

func (a *App) dockerEnv(c *ui.Context, id string) {
	s := &a.engine.containers
	v := &s.detail.secret
	toolNote(c, locales.DockerInspectMasked())
	if v.err != "" {
		warningRow(c, v.err)
	}
	if len(s.detail.data.EnvKeys) == 0 {
		toolNote(c, locales.KubeSecretNoKeys())
	}
	services := c.Services()
	canRead := a.ReadDockerEnv != nil && a.SecretTimer != nil && s.err == nil
	for _, key := range s.detail.data.EnvKeys {
		ui.Row(c.Key(key)).Gap(6).AlignItems(ui.Center).Padding(6, 8).BorderWidth(0, 0, 1, 0).
			BorderColor(Colors(c.Theme()).Line).Children(func() {
			reveal := v.revealed
			shown := reveal != nil && reveal.key == key
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
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
	req.stop = a.ReadDockerEnv(s.target, id, key, func(value []byte, err error) {
		if v.request != req || s.detail.tab != 1 || a.router.Path() != dockerEnginePage || a.router.Query("context") != s.target.Context {
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
