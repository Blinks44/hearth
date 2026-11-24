package ui

import (
	"math"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
	"github.com/Blinks44/hearth/resources"
)

func dockerManagementLabel(engine string) string {
	switch {
	case engine == "Colima":
		return locales.ColimaManaged()
	case engine == "":
		return ""
	case dockerEngineIsService(engine):
		return locales.ColimaExternalService()
	default:
		return locales.ColimaExternalApp()
	}
}

func dockerContextEngine(d *inventory.DockerState, k inventory.DockerContext) string {
	if k.Engine != "" {
		return k.Engine
	}
	if k.Name == "default" && k.Local && d.Override != "DOCKER_HOST" {
		for _, engine := range d.Engines {
			if engine.Name == "Docker Engine" && engine.Path != "" {
				return engine.Name
			}
		}
	}
	return ""
}

func dockerSelectedColima(d *inventory.DockerState) (inventory.ColimaProfile, bool) {
	if d == nil {
		return inventory.ColimaProfile{}, false
	}
	return dockerColimaProfile(d, d.Selected)
}

// Match the managed VM by its reported socket, or the native Colima
// socket layout for a stopped profile. A context name alone can be reused.
func dockerColimaProfile(d *inventory.DockerState, contextName string) (inventory.ColimaProfile, bool) {
	if d == nil || d.Colima == nil {
		return inventory.ColimaProfile{}, false
	}
	for _, k := range d.Contexts {
		socket, ok := strings.CutPrefix(k.Endpoint, "unix://")
		if k.Name != contextName || !k.Local || !ok {
			continue
		}
		for _, p := range d.Colima.Profiles {
			if p.Socket == k.Endpoint || p.Socket == "" && k.Engine == "Colima" &&
				filepath.Base(socket) == "docker.sock" && filepath.Base(filepath.Dir(socket)) == p.Name {
				return p, true
			}
		}
	}
	return inventory.ColimaProfile{}, false
}

func (a *App) canInstallColima() bool {
	return a.BuildPlan != nil && recommendedDockerEngine == "Colima"
}

func (a *App) colimaBrew() inventory.PackageManager {
	for _, m := range a.packageManagers() {
		if m.ID == "brew" {
			return m
		}
	}
	return inventory.PackageManager{}
}

func (a *App) installColima(migrate bool) {
	brew := a.colimaBrew()
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.ColimaInstallPlan(brew, migrate, env, run.Exec{})
	}, a.proposeOrSay)
}

// A failed first boot can leave the formula and VM on disk before Colima
// has prepared the Docker runtime. Keep the installation plan available.
func colimaNeedsSetup(colima *inventory.ColimaState) bool {
	return colima == nil || !colima.Unreadable && !slices.ContainsFunc(colima.Profiles, func(p inventory.ColimaProfile) bool {
		return p.Name == "default" && p.Runtime != ""
	})
}

func (a *App) changeColima(colima inventory.ColimaState, profile inventory.ColimaProfile, action ops.ColimaAction, limits ops.ColimaLimits) {
	if a.BuildPlan == nil {
		return
	}
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.ColimaPlan(run.Exec{}, env, colima, profile, action, limits)
	}, a.proposeOrSay)
}

func hearthBadge(c *ui.Context, size float32, radius float32) {
	logoOnce.Do(func() { logo, _ = ui.DecodeImage(resources.Logo) })
	ui.Box(c).Size(size, size).Radius(radius).Clip().Shrink(0).
		Shadow(0, 2, 6, 0, ui.Hex("#C2410C").Alpha(0.22)).Children(func() {
		ui.Image(c, logo).Size(size, size)
	})
}

func (a *App) dockerHearthHeroCard(c *ui.Context, st inventory.ToolState) bool {
	d := st.Docker
	if d == nil {
		return false
	}
	cli := inventory.TerminalCopy(st.Installations) >= 0

	var tag, body, ctaLabel, ctaHint string
	var features []string
	var onInstall func()

	switch {
	case a.canInstallColima() && colimaNeedsSetup(d.Colima):
		tag = locales.DockerHearthColimaTag()
		body = locales.DockerHearthHeroBody()
		ctaHint = locales.DockerHearthBrewHint()
		if cli {
			ctaLabel = locales.ColimaMigrate()
		} else {
			ctaLabel = locales.ColimaInstall()
		}
		features = []string{
			locales.DockerHearthFeaturePackages(),
			locales.DockerHearthFeatureResources(),
			locales.DockerHearthFeatureFree(),
		}
		onInstall = func() { a.installColima(cli) }
	case recommendedDockerEngine == "Docker Engine":
		installed := slices.ContainsFunc(d.Engines, func(e inventory.DockerEngine) bool {
			return e.Name == "Docker Engine" && e.Path != ""
		})
		if installed {
			return false
		}
		plan, ok := a.dockerEngineInstallOrSwitchPlan(st, cli)
		if !ok {
			return false
		}
		tag = locales.DockerHearthEngineTag()
		body = locales.DockerHearthLinuxHeroBody()
		ctaLabel = locales.DockerEngineInstall()
		ctaHint = locales.DockerHearthLinuxHint()
		features = []string{
			locales.DockerHearthLinuxFeaturePackages(),
			locales.DockerHearthLinuxFeatureNative(),
			locales.DockerHearthFeatureFree(),
		}
		onInstall = func() { a.Propose(plan) }
	default:
		return false
	}

	t := c.Theme()
	pal := Colors(t)
	ui.Column(c.Key("docker-hearth-hero")).Radius(12).Clip().
		LinearGradient(ui.LinearGradient{Angle: 150, Stops: []ui.GradientStop{
			{Color: pal.HeroFrom, At: 0}, {Color: pal.HeroMid, At: 1},
		}}).
		BorderWidth(1).BorderColor(pal.HeroBorder).Children(func() {
		ui.Row(c).Gap(14).Padding(16, 18, 14, 18).AlignItems(ui.Start).Children(func() {
			hearthBadge(c, 36, 9)
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Text(c, locales.DockerHearthTitle()).FontSize(14.5).FontWeight(700).SingleLine()
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ColimaRecommended())
				})
				ui.Text(c, tag).FontSize(11.5).FontWeight(500).TextColor(pal.AccentInk).Wrap()
				ui.Text(c, body).FontSize(12).TextColor(t.TextMuted).Wrap()
			})
			ui.Column(c).Shrink(0).Gap(5).AlignItems(ui.Center).Padding(4, 0, 0, 0).Children(func() {
				ui.PrimaryButton(c.Key("hearth-docker-install"), ctaLabel).OnClick(onInstall)
				ui.Text(c, ctaHint).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
			})
		})
		ui.Row(c).Gap(16).Padding(10, 18).Wrap().BorderWidth(1, 0, 0, 0).BorderColor(pal.HeroBorder).Children(func() {
			for i, feat := range features {
				ui.Row(c.Key("feat-" + strconv.Itoa(i))).Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, Icon("check")).Size(12, 12).Shrink(0).TextColor(pal.AccentInk)
					ui.Text(c, feat).FontSize(11).FontWeight(500).TextColor(t.TextMuted).SingleLine()
				})
			}
		})
	})
	return true
}

func (a *App) dockerHearthSwitchCard(c *ui.Context, st inventory.ToolState, foreignEngine string) {
	if a.DockerPromoHidden || st.Docker == nil {
		return
	}
	var tag, body, ctaLabel, hint string
	var onAction func()

	switch {
	case a.canInstallColima() && colimaNeedsSetup(st.Docker.Colima):
		tag = locales.DockerHearthColimaShortTag()
		body = locales.ColimaSwitchHint(foreignEngine)
		ctaLabel = locales.ColimaMigrate()
		hint = locales.DockerHearthColimaHint()
		onAction = func() { a.installColima(true) }
	case recommendedDockerEngine == "Docker Engine" && foreignEngine != "Docker Engine":
		plan, ok := a.dockerEngineSwitchPlan(st)
		if !ok {
			return
		}
		tag = locales.DockerHearthEngineShortTag()
		body = locales.DockerEngineSwitchHint(foreignEngine)
		ctaLabel = locales.DockerEngineInstall()
		hint = locales.DockerHearthLinuxSwitchHint()
		onAction = func() { a.Propose(plan) }
	default:
		return
	}

	t := c.Theme()
	pal := Colors(t)
	ui.Column(c.Key("docker-hearth-switch")).Radius(12).Clip().
		LinearGradient(ui.LinearGradient{Angle: 150, Stops: []ui.GradientStop{
			{Color: pal.HeroFrom, At: 0}, {Color: pal.HeroMid, At: 1},
		}}).
		BorderWidth(1).BorderColor(pal.HeroBorder).Padding(14).Gap(10).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
			hearthBadge(c, 26, 7)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				clipped(c, locales.DockerHearthTitle()).FontSize(13).FontWeight(700)
				clipped(c, tag).FontSize(11).FontWeight(500).TextColor(pal.AccentInk)
			})
			iconButton(c.Key("docker-promo-dismiss"), "x", locales.DockerHearthDismiss()).OnClick(func() {
				a.DockerPromoHidden = true
				if a.OnSettings != nil {
					a.OnSettings()
				}
			})
		})
		ui.Text(c, body).FontSize(11.5).TextColor(t.TextMuted).Wrap()
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Button(c.Key("hearth-docker-switch"), ctaLabel).OnClick(onAction)
			ui.Text(c, hint).FontSize(10.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0).Wrap()
		})
	})
}

func (a *App) dockerManagement(c *ui.Context, st inventory.ToolState) {
	d := st.Docker
	if a.dockerTab == 1 && d.Colima != nil && len(d.Colima.Profiles) > 0 {
		shown := false
		for _, p := range d.Colima.Profiles {
			if p.Runtime == "docker" {
				a.colimaCard(c.Key("profile:"+p.Name), *d.Colima, p)
				shown = true
			}
		}
		if shown {
			return
		}
	}
	if a.dockerTab == 1 && d.Service != nil {
		a.dockerServiceCard(c, *d.Service)
		return
	}
	if p, ok := dockerSelectedColima(d); ok {
		a.colimaCard(c, *d.Colima, p)
		return
	}
	for _, k := range d.Contexts {
		if k.Name != d.Selected || !k.Local || inventory.TerminalCopy(st.Installations) < 0 {
			continue
		}
		name := dockerContextEngine(d, k)
		if name == "Docker Engine" && d.Service != nil {
			a.dockerServiceCard(c, *d.Service)
			return
		}
		if name != "" && name != "Colima" {
			a.dockerExternalCard(c, st, name)
			a.dockerHearthSwitchCard(c, st, name)
			a.dockerColimaCompanionCard(c, st)
			return
		}
	}
	if colima := d.Colima; colima != nil {
		if colima.Unreadable {
			listCard(c, "Colima", "", func() { cardNote(c, locales.ColimaUnavailable()) })
			return
		}
		if len(colima.Profiles) == 0 && colima.Path != "" {
			a.colimaCard(c, *colima, inventory.ColimaProfile{Name: "default"})
			return
		}
		if colimaNeedsSetup(colima) && a.canInstallColima() {
			listCard(c, "Colima", "", func() {
				listRowWrapped(c, locales.ColimaUnavailable(), locales.ColimaPackagesHint(), func() {
					ui.Button(c.Key("colima-setup"), locales.ColimaInstall()).OnClick(func() { a.installColima(false) })
				})
			})
			return
		}
		for _, p := range colima.Profiles {
			if p.Name == "default" && p.Runtime == "docker" {
				a.colimaCard(c, *colima, p)
				return
			}
		}
	}
	if a.canInstallColima() && inventory.TerminalCopy(st.Installations) < 0 {
		listCard(c, locales.DockerHearthNeedToKnow(), "", func() {
			listRowWrapped(c, locales.ColimaPackages(), locales.ColimaPackagesHint(), func() {})
			listRowWrapped(c, locales.ColimaOtherEngine(), locales.ColimaOtherEngineHint(), func() {})
		})
	} else if _, ok := a.nativeDockerInstallPlan(st); ok {
		listCard(c, locales.DockerHearthNeedToKnow(), "", func() {
			listRowWrapped(c, locales.DockerHearthLinuxFeaturePackages(), locales.DockerLinuxWizard(), func() {})
			listRowWrapped(c, locales.ColimaOtherEngine(), locales.ColimaOtherEngineHint(), func() {})
		})
	}
}

func (a *App) dockerExternalCard(c *ui.Context, st inventory.ToolState, name string) {
	path := ""
	for _, e := range st.Docker.Engines {
		if e.Name == name {
			path = e.Path
			break
		}
	}
	listCard(c, name, "", func() {
		if dockerManagementLabel(name) == locales.ColimaExternalApp() {
			listRow(c, locales.ColimaExternalResources(), false, locales.ColimaExternalResourcesHint(name), func() {
				switch {
				case a.Shell != nil && strings.HasSuffix(strings.ToLower(path), ".app"):
					ui.Button(c.Key("external-open"), locales.KubeClusterOpen()).OnClick(func() {
						if err := a.Shell.OpenPath(path); err != nil {
							c.Toast(locales.OpenFailed(tilde(path, a.Home), err.Error()))
						}
					})
				case goos == "windows":
					if p, ok := ops.EngineStartPlan(ops.Engine{Name: name, Path: path}, st.Docker.Selected, nil); ok {
						ui.Button(c.Key("external-open"), locales.KubeClusterOpen()).OnClick(func() { a.Propose(p) })
					}
				}
			})
		}
		listRow(c, locales.ColimaDockerResources(), false, locales.ColimaDockerResourcesHint(), func() {})
	})
}

func (a *App) dockerColimaCompanionCard(c *ui.Context, st inventory.ToolState) {
	if st.Docker == nil || st.Docker.Colima == nil || st.Docker.Colima.Unreadable {
		return
	}
	for _, p := range st.Docker.Colima.Profiles {
		if p.Name == "default" && p.Runtime == "docker" {
			listCard(c, "Colima", "", func() {
				status := colimaProfileStatus(p.Status)
				listRow(c, status, false, locales.ColimaManaged(), func() {
					if p.Status == "stopped" {
						ui.Button(c.Key("external-start-colima"), locales.SetupStart()).Shrink(0).Disabled(a.BuildPlan == nil).
							OnClick(func() { a.changeColima(*st.Docker.Colima, p, ops.ColimaStart, ops.ColimaLimits{}) })
					}
					if p.Status == "running" {
						for _, k := range st.Docker.Contexts {
							if profile, ok := dockerColimaProfile(st.Docker, k.Name); ok && profile.Name == p.Name && a.dockerOpenable(k.Name) {
								ui.Button(c.Key("external-view-colima"), locales.KubeClusterOpen()).Shrink(0).OnClick(func() { a.router.Push(dockerEnginePath(k.Name)) })
								break
							}
						}
					}
				})
			})
			return
		}
	}
}

func colimaProfileStatus(status string) string {
	switch status {
	case "":
		return locales.ColimaNotStarted()
	case "stopped":
		return locales.ColimaStopped()
	case "running":
		return locales.ColimaRunning()
	default:
		return locales.ColimaUnavailable()
	}
}

func (a *App) colimaCard(c *ui.Context, colima inventory.ColimaState, p inventory.ColimaProfile) {
	if p.Runtime != "" && p.Runtime != "docker" {
		return
	}
	listCard(c, "Colima", "", func() {
		details := []string{locales.ColimaProfile(p.Name)}
		for _, detail := range []string{p.Driver, p.Mount} {
			if detail != "" {
				details = append(details, detail)
			}
		}
		caption := strings.Join(details, " · ")
		hasResources := p.CPU > 0 && p.Memory > 0 && p.Disk > 0
		if hasResources {
			listRow(c, locales.ColimaResourceSummary(strconv.Itoa(p.CPU), colimaGiB(p.Memory), colimaGiB(p.Disk)), false, caption, func() {
				disabled := a.BuildPlan == nil || (p.Status != "running" && p.Status != "stopped")
				ui.Button(c.Key("colima-resources"), locales.ColimaEdit()).Shrink(0).Disabled(disabled).
					OnClick(func() { a.colimaResize = newColimaResize(colima, p) })
			})
		}
		if p.Name == "default" && colima.Brew != "" {
			title, hint, label, action := locales.ColimaAutostartOff(), "", locales.ColimaEnable(), ops.ColimaEnable
			on := colima.Autostart == "on"
			switch colima.Autostart {
			case "on":
				title, hint, label, action = locales.ColimaAutostartOn(), locales.ColimaAutostartOnHint(), locales.ColimaDisable(), ops.ColimaDisable
			case "":
				title, hint = locales.ColimaAutostartUnknown(), ""
			}
			listRow(c, title, false, hint, func() {
				if colima.Autostart != "" {
					if toggle(c, "colima-autostart", &on).Label(label).Tooltip(label).Disabled(a.BuildPlan == nil || a.op.running()).Changed() {
						a.changeColima(colima, p, action, ops.ColimaLimits{})
					}
				}
			})
		}
		statusCaption := ""
		if !hasResources {
			statusCaption = caption
		}
		listRow(c, colimaProfileStatus(p.Status), false, statusCaption, func() {
			switch p.Status {
			case "", "stopped":
				ui.Button(c.Key("colima-card-start"), locales.SetupStart()).Shrink(0).Disabled(a.BuildPlan == nil).
					OnClick(func() { a.changeColima(colima, p, ops.ColimaStart, ops.ColimaLimits{}) })
			case "running":
				ui.Button(c.Key("colima-card-stop"), locales.PortsStop()).Shrink(0).Disabled(a.BuildPlan == nil).
					OnClick(func() { a.changeColima(colima, p, ops.ColimaStop, ops.ColimaLimits{}) })
			}
		})
	})
}

func dockerServiceStatus(status string) string {
	switch status {
	case "running":
		return locales.DockerServiceRunning()
	case "stopped":
		return locales.DockerServiceStopped()
	case "error":
		return locales.DockerServiceError()
	default:
		return locales.ColimaUnavailable()
	}
}

func (a *App) dockerServiceCard(c *ui.Context, svc inventory.DockerServiceState) {
	listCard(c, "Docker Engine", svc.Unit, func() {
		title, hint, label, action := locales.ColimaAutostartOff(), "", locales.ColimaEnable(), ops.DockerServiceEnable
		on := svc.Autostart == "on"
		switch svc.Autostart {
		case "on":
			title, hint, label, action = locales.ColimaAutostartOn(), locales.DockerServiceAutostartOnHint(), locales.ColimaDisable(), ops.DockerServiceDisable
		case "":
			title, hint = locales.ColimaAutostartUnknown(), ""
		}
		listRow(c, title, false, hint, func() {
			if svc.Autostart != "" {
				if plan, ok := ops.DockerServicePlan(action, svc, a.detect("docker")); ok {
					if toggle(c, "docker-service-autostart", &on).Label(label).Tooltip(label).Disabled(a.op.running()).Changed() {
						a.Propose(plan)
					}
				}
			}
		})
		listRow(c, dockerServiceStatus(svc.Status), false, "", func() {
			switch svc.Status {
			case "stopped", "error":
				if plan, ok := ops.DockerServicePlan(ops.DockerServiceStart, svc, a.detect("docker")); ok {
					ui.Button(c.Key("docker-service-start"), locales.SetupStart()).Shrink(0).Disabled(a.op.running()).
						OnClick(func() { a.Propose(plan) })
				}
			case "running":
				if plan, ok := ops.DockerServicePlan(ops.DockerServiceStop, svc, a.detect("docker")); ok {
					ui.Button(c.Key("docker-service-stop"), locales.PortsStop()).Shrink(0).Disabled(a.op.running()).
						OnClick(func() { a.Propose(plan) })
				}
			}
		})
	})
}

func colimaGiB(bytes int64) string {
	return strconv.FormatFloat(float64(bytes)/float64(docker.ColimaGiB), 'f', -1, 64)
}

type colimaResizeForm struct {
	open              bool
	colima            inventory.ColimaState
	profile           inventory.ColimaProfile
	cpu, memory, disk float64
}

func newColimaResize(colima inventory.ColimaState, p inventory.ColimaProfile) colimaResizeForm {
	return colimaResizeForm{true, colima, p, float64(p.CPU), float64(p.Memory) / float64(docker.ColimaGiB), math.Ceil(float64(p.Disk) / float64(docker.ColimaGiB))}
}

func (a *App) colimaResizeDialog(c *ui.Context) {
	d := &a.colimaResize
	if !d.open {
		return
	}
	t, pal := c.Theme(), Colors(c.Theme())
	dialogBase(c.Key("colima-resize"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Padding(20, 20, 12, 20).Gap(4).Children(func() {
			ui.Text(c, locales.ColimaResources()).FontSize(18).FontWeight(600)
			ui.Text(c, locales.ColimaResizeWhy(d.profile.Name)).FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		minDisk := math.Ceil(float64(d.profile.Disk) / float64(docker.ColimaGiB))
		memory := float64(d.profile.Memory) / float64(docker.ColimaGiB)
		var plan ops.Plan
		var err error
		maxMemory := float64(1024)
		if total := sysenv.TotalMemoryBytes(); total > 0 {
			hostGiB := math.Floor(float64(total)/float64(docker.ColimaGiB)*2) / 2
			if hostGiB >= 1 {
				maxMemory = min(maxMemory, hostGiB)
			}
		}
		ui.Scroll(c).MaxHeight(440).Children(func() {
			ui.Column(c).Padding(0, 20, 12, 20).Gap(6).Children(func() {
				colimaLimit(c.Key("colima-cpu"), locales.DockerCpus(), &d.cpu, 1, float64(max(runtime.NumCPU(), d.profile.CPU)), 1)
				colimaLimit(c.Key("colima-memory"), locales.ColimaMemoryGiB(), &d.memory, min(1, memory), max(maxMemory, memory), 0.5)
				colimaLimit(c.Key("colima-disk"), locales.ColimaDiskGiB(), &d.disk, minDisk, max(minDisk, 16384), 1)
			})
			limits := ops.ColimaLimits{CPU: int(d.cpu), Memory: d.memory, Disk: int64(d.disk)}
			plan, err = ops.ColimaPlan(run.Exec{}, nil, d.colima, d.profile, ops.ColimaResize, limits)
			if err == nil {
				a.operationPlan(c, plan)
			}
		})
		ui.Row(c).Gap(8).Padding(12, 20).Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			ui.Button(c.Key("colima-cancel"), locales.Cancel()).Height(controlHeight).OnClick(func() { d.open = false })
			unchanged := int(d.cpu) == d.profile.CPU && d.memory == memory && d.disk == minDisk
			applyLabel := locales.ColimaRestart()
			if d.profile.Status == "stopped" {
				applyLabel = locales.ColimaStart()
			}
			ui.PrimaryButton(c.Key("colima-apply"), applyLabel).Height(controlHeight).
				Disabled(err != nil || unchanged || a.BuildPlan == nil || a.RunPlan == nil || a.op.running()).
				OnClick(func() {
					d.open = false
					a.applyColimaResize(d.colima, d.profile, ops.ColimaLimits{CPU: int(d.cpu), Memory: d.memory, Disk: int64(d.disk)})
				})
		})
	})
}

func colimaLimit(c *ui.Context, label string, value *float64, lo, hi, step float64) {
	ui.Row(c).MinHeight(34).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, label).FontSize(13).Grow(1).Wrap()
		stepper(c, strconv.FormatFloat(*value, 'f', -1, 64), 52,
			"decrease", "increase",
			locales.ColimaDecrease(label), locales.ColimaIncrease(label),
			*value <= lo, *value >= hi,
			func() { *value = max(lo, *value-step) },
			func() { *value = min(hi, *value+step) })
	})
}

// The resize dialog already shows and confirms these exact commands.
// BuildPlan supplies the terminal environment to the confirmed operation.
func (a *App) applyColimaResize(colima inventory.ColimaState, profile inventory.ColimaProfile, limits ops.ColimaLimits) {
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.ColimaPlan(run.Exec{}, env, colima, profile, ops.ColimaResize, limits)
	}, func(plan ops.Plan, err error) {
		if err != nil || a.op.running() {
			a.proposeOrSay(plan, err)
			return
		}
		a.Propose(plan)
		a.startOperation()
	})
}
