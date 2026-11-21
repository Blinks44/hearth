package ui

import (
	"path/filepath"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type dockerHealthRow struct {
	key, title, detail string
	action             func()
	button             string
	danger             bool
}

func (a *App) dockerHealthCard(c *ui.Context, st inventory.ToolState, context string, health inventory.DockerHealth, overview bool) {
	d := st.Docker
	var rows []dockerHealthRow
	for _, issue := range d.CredentialIssues {
		title := locales.DockerHelperTitle()
		if issue.Registry != "" {
			title = locales.DockerHelperRegistry(issue.Registry)
		}
		detail := locales.DockerHelperInvalid()
		if issue.Name != "" {
			detail = locales.DockerHelperMissing("docker-credential-" + issue.Name)
		}
		row := dockerHealthRow{key: "helper:" + issue.Key, title: title, detail: detail, danger: true}
		if issue.Replacement != "" && a.canPlanDocker() {
			row.button = locales.DockerHelperFix()
			row.action = func() {
				a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
					return ops.DockerCredentialPlan(env, d.ConfigDir, issue)
				}, a.proposeOrSay)
			}
		} else if a.Shell != nil {
			path := filepath.Join(d.ConfigDir, "config.json")
			row.button = locales.OpOpenFile()
			row.action = func() { a.openFile(c, path) }
		}
		rows = append(rows, row)
	}
	for _, file := range d.BrokenPlugins {
		row := dockerHealthRow{key: "plugin:" + file, title: locales.DockerBrokenPlugin(file), detail: tilde(d.PluginDir, a.Home)}
		if a.Shell != nil {
			row.button = locales.DockerOpenFolder()
			row.action = func() {
				if err := a.Shell.OpenPath(d.PluginDir); err != nil {
					c.Toast(locales.OpenFailed(tilde(d.PluginDir, a.Home), err.Error()))
				}
			}
		}
		rows = append(rows, row)
	}
	if d.DefaultSocketIssue != "" {
		row := dockerHealthRow{key: "socket", title: locales.DockerDefaultSocket(), detail: locales.DockerSocketDifferent(d.Selected)}
		switch d.DefaultSocketIssue {
		case "missing":
			row.detail = locales.DockerSocketMissing(d.Selected)
		case "unavailable":
			row.detail = locales.DockerSocketUnavailable()
		}
		if d.DefaultSocketIssue != "unavailable" || a.dockerAnswers() {
			row.button = locales.KubeShellCopy()
			command := run.TerminalCommand([]string{"docker", "--context", d.Selected, "ps"})
			row.action = func() { c.WriteClipboard(command) }
		} else if plan, ok := a.engineStartPlan(); ok && !a.op.running() {
			row.button = plan.Title
			row.action = func() { a.Propose(plan) }
		}
		rows = append(rows, row)
	}
	if profile, ok := dockerColimaProfile(d, context); ok && health.Memory > 0 &&
		float64(health.ContainerMemory)/float64(health.Memory) >= 0.8 {
		row := dockerHealthRow{
			key: "low-memory", title: locales.DockerLowMemory(),
			detail: locales.DockerLowMemoryDetail(formatBytes(health.ContainerMemory), formatBytes(health.Memory)),
			danger: true,
		}
		if profile.Status == "running" && profile.CPU > 0 && profile.Memory > 0 && profile.Disk > 0 && a.canPlanDocker() {
			row.button = locales.ColimaEdit()
			row.action = func() { a.colimaResize = newColimaResize(*d.Colima, profile) }
		}
		rows = append(rows, row)
	}
	for _, item := range health.Emulated {
		row := dockerHealthRow{
			key: "emulated:" + item.ID, title: locales.DockerEmulatedContainer(item.Name),
			detail: locales.DockerEmulatedDetail(item.ImageArch, item.EngineArch),
		}
		if a.dockerOpenable(context) && a.WatchDockerContainers != nil {
			row.button = locales.KubeClusterOpen()
			row.action = func() {
				a.engine.view = dockerContainers
				a.engine.containers.filter, a.engine.containers.query = dockerAll, item.Name
				a.engine.containers.openID = item.ID
				if !overview {
					a.router.Push(dockerEnginePath(context))
				}
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return
	}
	body := func() {
		for i, row := range rows {
			if overview {
				dockerOverviewHealthRow(c.Key(row.key), row, i > 0)
			} else {
				listRowWrapped(c.Key(row.key), row.title, row.detail, func() { dockerHealthButton(c, row) })
			}
		}
	}
	if overview {
		overviewPanel(c, locales.DockerAttention(), strconv.Itoa(len(rows)), body)
	} else {
		listCard(c, locales.DockerAttention(), "", body)
	}
}

func dockerOverviewHealthRow(c *ui.Context, row dockerHealthRow, divider bool) {
	pal := Colors(c.Theme())
	r := ui.Row(c).Gap(10).Padding(8, 13).AlignItems(ui.Center)
	if divider {
		r.BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	}
	r.Children(func() {
		icon, color := "triangle-alert", pal.WarningText
		if row.danger {
			icon, color = "circle-alert", pal.DangerText
		}
		ui.Icon(c, Icon(icon)).Size(15, 15).Shrink(0).TextColor(color)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, row.title).FontSize(13).Wrap()
			ui.Text(c, row.detail).FontSize(11.5).TextColor(c.Theme().TextMuted).Wrap()
		})
		dockerHealthButton(c, row)
	})
}

func dockerHealthButton(c *ui.Context, row dockerHealthRow) {
	if row.action != nil {
		ui.Button(c.Key("action"), row.button).Height(24).Shrink(0).OnClick(row.action)
	}
}

func (a *App) dockerRegistries(c *ui.Context, st inventory.ToolState, active int) {
	d := st.Docker
	if len(d.Registries) == 0 {
		return
	}
	listCard(c, locales.DockerRegistries(), strconv.Itoa(len(d.Registries)), func() {
		for _, registry := range d.Registries {
			detail := locales.DockerRegistryConfigured()
			if registry.MissingHelper {
				detail = locales.DockerRegistryBlocked()
			}
			listRowWrapped(c, registry.Name, detail, func() {
				if a.OpenDockerLogin == nil || active < 0 || st.Installations[active].Source.Shim || !filepath.IsAbs(d.ConfigDir) {
					return
				}
				ui.Button(c.Key("docker-login:"+registry.Name), locales.DockerRegistryLogin()).Height(24).Shrink(0).
					Disabled(registry.MissingHelper).OnClick(func() {
					argv := []string{st.Installations[active].Path, "--config", d.ConfigDir, "login"}
					if registry.Name != docker.HubRegistry {
						argv = append(argv, registry.Name)
					}
					a.OpenDockerLogin(argv, func(err error) {
						if err != nil {
							a.planErr = locales.KubeShellLaunchFailed(err.Error())
						}
					})
				})
			})
		}
	})
}
