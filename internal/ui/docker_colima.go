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
)

func dockerManagementLabel(engine string) string {
	switch engine {
	case "Colima":
		return locales.ColimaManaged()
	case "":
		return ""
	case "Docker Engine", "Podman":
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
	if p, ok := dockerSelectedColima(d); ok {
		a.colimaCard(c, *d.Colima, p)
		return
	}
	for _, k := range d.Contexts {
		if k.Name != d.Selected || !k.Local || inventory.TerminalCopy(st.Installations) < 0 {
			continue
		}
		name := dockerContextEngine(d, k)
		if name != "" && name != "Colima" {
			a.dockerExternalCard(c, st, name)
			return
		}
	}
	if colima := d.Colima; colima != nil {
		if colima.Unreadable {
			listCard(c, "Colima", "", func() { cardNote(c, locales.ColimaUnavailable()) })
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
		if len(colima.Profiles) == 0 {
			a.colimaCard(c, *colima, inventory.ColimaProfile{Name: "default"})
			return
		}
	}
	if a.canInstallColima() && inventory.TerminalCopy(st.Installations) < 0 {
		listCard(c, locales.ColimaChoice(), "", func() {
			listRowWrapped(c, locales.ColimaEverything(), locales.ColimaWizard(), func() {})
			listRowWrapped(c, locales.ColimaPackages(), locales.ColimaPackagesHint(), func() {})
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
		if a.canInstallColima() && colimaNeedsSetup(st.Docker.Colima) {
			listRowWrapped(c, locales.ColimaOnlyHearth(name), locales.ColimaSeparateDataHint(), func() {
				ui.Button(c.Key("colima-migrate"), locales.ColimaMigrate()).OnClick(func() { a.installColima(true) })
			})
		} else if st.Docker.Colima != nil && !st.Docker.Colima.Unreadable {
			for _, p := range st.Docker.Colima.Profiles {
				if p.Name == "default" && p.Runtime == "docker" {
					listRow(c, "Colima", false, locales.ColimaManaged(), func() {
						if p.Status == "stopped" {
							ui.Button(c.Key("external-start-colima"), locales.ColimaStart()).Disabled(a.BuildPlan == nil).
								OnClick(func() { a.changeColima(*st.Docker.Colima, p, ops.ColimaStart, ops.ColimaLimits{}) })
						}
						if p.Status == "running" {
							for _, k := range st.Docker.Contexts {
								if profile, ok := dockerColimaProfile(st.Docker, k.Name); ok && profile.Name == p.Name && a.dockerOpenable(k.Name) {
									ui.Button(c.Key("external-view-colima"), locales.KubeClusterOpen()).OnClick(func() { a.router.Push(dockerEnginePath(k.Name)) })
									break
								}
							}
						}
					})
					break
				}
			}
		}
	})
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
		if p.CPU > 0 && p.Memory > 0 && p.Disk > 0 {
			listRow(c, locales.ColimaResourceSummary(strconv.Itoa(p.CPU), colimaGiB(p.Memory), colimaGiB(p.Disk)), false, caption, func() {
				disabled := a.BuildPlan == nil || (p.Status != "running" && p.Status != "stopped")
				ui.Button(c.Key("colima-resources"), locales.ColimaEdit()).Disabled(disabled).
					OnClick(func() { a.colimaResize = newColimaResize(colima, p) })
			})
		}
		if p.Name == "default" && colima.Brew != "" {
			title, hint, button, action := locales.ColimaAutostartOff(), "", locales.ColimaEnable(), ops.ColimaEnable
			switch colima.Autostart {
			case "on":
				title, hint, button, action = locales.ColimaAutostartOn(), locales.ColimaAutostartOnHint(), locales.ColimaDisable(), ops.ColimaDisable
			case "":
				title, hint = locales.ColimaAutostartUnknown(), ""
			}
			listRow(c, title, false, hint, func() {
				if colima.Autostart != "" {
					ui.Button(c.Key("colima-autostart"), button).Disabled(a.BuildPlan == nil).
						OnClick(func() { a.changeColima(colima, p, action, ops.ColimaLimits{}) })
				}
			})
		}
		status := locales.ColimaUnavailable()
		switch p.Status {
		case "":
			status = locales.ColimaNotStarted()
		case "stopped":
			status = locales.ColimaStopped()
		case "running":
			status = locales.ColimaRunning()
		}
		listRow(c, status, false, caption, func() {
			switch p.Status {
			case "", "stopped":
				ui.Button(c.Key("colima-card-start"), locales.ColimaStart()).Disabled(a.BuildPlan == nil).
					OnClick(func() { a.changeColima(colima, p, ops.ColimaStart, ops.ColimaLimits{}) })
			case "running":
				ui.Button(c.Key("colima-card-stop"), locales.ColimaStop()).Disabled(a.BuildPlan == nil).
					OnClick(func() { a.changeColima(colima, p, ops.ColimaStop, ops.ColimaLimits{}) })
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
		ui.Scroll(c).MaxHeight(440).Children(func() {
			ui.Column(c).Padding(0, 20, 12, 20).Gap(6).Children(func() {
				colimaLimit(c.Key("colima-cpu"), locales.DockerCpus(), &d.cpu, 1, float64(max(runtime.NumCPU(), d.profile.CPU)), 1)
				colimaLimit(c.Key("colima-memory"), locales.ColimaMemoryGiB(), &d.memory, min(1, memory), max(1024, memory), 0.5)
				colimaLimit(c.Key("colima-disk"), locales.ColimaDiskGiB(), &d.disk, minDisk, max(minDisk, 16384), 1)
			})
			limits := ops.ColimaLimits{CPU: int(d.cpu), Memory: d.memory, Disk: int64(d.disk)}
			plan, err = ops.ColimaPlan(run.Exec{}, nil, d.colima, d.profile, ops.ColimaResize, limits)
			if err == nil {
				a.operationPlan(c, plan)
			}
		})
		ui.Row(c).Gap(8).Padding(12, 20).Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			ui.Button(c.Key("colima-cancel"), locales.Cancel()).OnClick(func() { d.open = false })
			unchanged := int(d.cpu) == d.profile.CPU && d.memory == memory && d.disk == minDisk
			ui.PrimaryButton(c.Key("colima-apply"), locales.ColimaRestart()).
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
		ui.Button(c.Key("decrease"), "−").Size(28, 26).Label(locales.ColimaDecrease(label)).
			Disabled(*value <= lo).OnClick(func() { *value = max(lo, *value-step) })
		ui.Text(c, strconv.FormatFloat(*value, 'f', -1, 64)).Font(FontMono).FontSize(12).Width(50).TextAlign(ui.Center)
		ui.Button(c.Key("increase"), "+").Size(28, 26).Label(locales.ColimaIncrease(label)).
			Disabled(*value >= hi).OnClick(func() { *value = min(hi, *value+step) })
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
