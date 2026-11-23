package main

import (
	"context"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	hearthui "github.com/Blinks44/hearth/internal/ui"
	"github.com/Blinks44/hearth/locales"
)

const trayMaxContainers = 8

var (
	trayRunningDotOnce sync.Once
	trayRunningDotIcon *mygo.MenuIcon
)

func trayRunningIcon() *mygo.MenuIcon {
	trayRunningDotOnce.Do(func() {
		svg := ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="4" fill="#2E9A5C"/></svg>`))
		icon, err := mygo.NewMenuIconSVG(svg, false)
		if err != nil {
			log.Printf("tray running icon: %v", err)
			return
		}
		trayRunningDotIcon = icon
	})
	return trayRunningDotIcon
}

type trayActions struct {
	openHearth      func()
	openContainers  func()
	openDocker      func()
	startColima     func()
	stopColima      func()
	switchDockerCtx func(name string)
	switchKubeCtx   func(name string)
	checkUpdates    func()
	checkAppUpdate  func()
	openSettings    func()
	quit            func()
}

type trayController struct {
	win        *mygo.Window
	app        *hearthui.App
	env        func() sysenv.Env
	onQuit     func()
	tray       *mygo.Tray
	containers []docker.Container
	cancelRead context.CancelFunc
	switching  bool
}

func newTrayController(win *mygo.Window, a *hearthui.App, env func() sysenv.Env, onQuit func()) *trayController {
	return &trayController{
		win:    win,
		app:    a,
		env:    env,
		onQuit: onQuit,
	}
}

// syncDockIcon applies the macOS activation policy for Hearth's Dock icon.
// It is a no-op on Windows and Linux. When ShowInDock is off and the menu bar
// icon is enabled, the Dock icon stays visible while the window is open (so
// Cmd+Tab works) and hides once the window is closed to the menu bar.
func syncDockIcon(trayEnabled, showInDock, windowVisible bool) {
	if trayEnabled && !showInDock && !windowVisible {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
		return
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular)
}

func (t *trayController) sync(enabled bool) {
	if !enabled {
		t.destroy()
		return
	}
	if t.tray == nil {
		icon, template := trayIcon()
		tr, err := mygo.NewTray(mygo.TrayOptions{
			ID:             "hearth-tray",
			Icon:           icon,
			IconIsTemplate: template,
			ToolTip:        "Hearth",
			Menu:           t.buildMenu(),
		})
		if err != nil {
			log.Printf("create tray: %v", err)
			return
		}
		tr.OnClick(func() {
			t.refreshContainers()
		})
		t.tray = tr
	}
	t.refresh()
	t.refreshContainers()
}

func (t *trayController) destroy() {
	if t.cancelRead != nil {
		t.cancelRead()
		t.cancelRead = nil
	}
	if t.tray != nil {
		t.tray.Destroy()
		t.tray = nil
	}
}

func (t *trayController) refresh() {
	if t.tray == nil {
		return
	}
	t.tray.SetMenu(t.buildMenu())
}

func (t *trayController) showWindow() {
	syncDockIcon(t.app.TrayEnabled, t.app.ShowInDock, true)
	t.win.Show()
	t.win.Focus()
}

func (t *trayController) buildMenu() *mygo.Menu {
	items := buildTrayMenuItems(t.app.Snapshot, t.containers, t.switching, t.app.AppUpdateAvailable, trayActions{
		openHearth: func() {
			t.showWindow()
		},
		openContainers: func() {
			t.app.OpenDockerContainers()
			t.showWindow()
		},
		openDocker: func() {
			t.app.OpenDocker()
			t.showWindow()
		},
		startColima: func() {
			t.app.TriggerColima(true)
			t.showWindow()
		},
		stopColima: func() {
			t.app.TriggerColima(false)
			t.showWindow()
		},
		switchDockerCtx: t.switchDockerContext,
		switchKubeCtx:   t.switchKubeContext,
		checkUpdates: func() {
			t.app.OpenUpdates()
			if t.app.CheckUpdates != nil {
				t.app.CheckUpdates()
			}
			t.showWindow()
		},
		checkAppUpdate: func() {
			if t.app.CheckAppUpdate != nil {
				t.app.CheckAppUpdate()
			}
		},
		openSettings: func() {
			t.app.OpenSettings()
			t.showWindow()
		},
		quit: func() {
			if t.onQuit != nil {
				t.onQuit()
			}
		},
	})
	return mygo.NewMenu(items)
}

func (t *trayController) refreshContainers() {
	if t.tray == nil || t.app.Snapshot == nil {
		return
	}
	target, ok := trayDockerTarget(t.app.Snapshot)
	if !ok {
		if len(t.containers) > 0 {
			t.containers = nil
			t.refresh()
		}
		return
	}
	if t.cancelRead != nil {
		t.cancelRead()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.cancelRead = cancel
	env := slices.Clone(t.env())
	go func() {
		defer cancel()
		items, err := docker.ReadContainers(ctx, run.Exec{}, env, target)
		if ctx.Err() != nil {
			return
		}
		var active []docker.Container
		if err == nil {
			for _, c := range items {
				if c.Active() {
					active = append(active, c)
				}
			}
		}
		t.win.Update(func() {
			if ctx.Err() != nil || t.tray == nil {
				return
			}
			t.containers = active
			t.refresh()
		})
	}()
}

func (t *trayController) switchDockerContext(to string) {
	if t.switching || t.app.Snapshot == nil {
		return
	}
	st, ok := t.app.Snapshot.Tool("docker")
	if !ok || st.Docker == nil || st.Docker.Selected == to {
		return
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 {
		return
	}
	cli := st.Installations[active].Path
	plan, ok := ops.ContextPlan(cli, st.Docker.Selected, to, nil)
	if !ok || len(plan.Steps) == 0 {
		return
	}
	t.runQuickSwitch(plan.Steps[0].Argv)
}

func (t *trayController) switchKubeContext(to string) {
	if t.switching || t.app.Snapshot == nil {
		return
	}
	st, ok := t.app.Snapshot.Tool("cli")
	if !ok || st.Cli == nil {
		return
	}
	from := trayKubeCurrent(*st.Cli)
	if from == to {
		return
	}
	cli := trayKubectlPath(*st.Cli)
	if cli == "" {
		return
	}
	env := slices.Clone(t.env())
	plan, err := ops.KubeContextPlan(cli, from, to, env, run.Exec{})
	if err != nil || len(plan.Steps) == 0 {
		return
	}
	t.runQuickSwitch(plan.Steps[0].Argv)
}

func (t *trayController) runQuickSwitch(argv []string) {
	t.switching = true
	t.refresh()
	env := slices.Clone(t.env())
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := (run.Exec{}).Run(ctx, run.Cmd{Args: argv, Env: env, Timeout: 15 * time.Second})
		if err != nil || res.ExitCode != 0 {
			log.Printf("tray context switch %v failed: err=%v exit=%d stderr=%s", argv, err, res.ExitCode, strings.TrimSpace(string(res.Stderr)))
			t.win.Update(func() {
				t.switching = false
				t.refresh()
			})
			return
		}
		t.win.Update(func() {
			t.switching = false
			if t.app.OnScan != nil {
				t.app.OnScan()
			}
			t.refresh()
		})
	}()
}

func buildTrayMenuItems(snap *inventory.Snapshot, containers []docker.Container, switching bool, appUpdateAvailable string, act trayActions) []*mygo.MenuItem {
	items := []*mygo.MenuItem{
		{
			ID:          "open",
			Label:       locales.TrayOpenHearth(),
			Accelerator: "CmdOrCtrl+O",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if act.openHearth != nil {
					act.openHearth()
				}
			},
		},
	}

	// Containers section (when Docker CLI or contexts are present).
	var dockerState *inventory.DockerState
	var dockerHasCLI bool
	if snap != nil {
		if st, ok := snap.Tool("docker"); ok && st.Docker != nil {
			dockerState = st.Docker
			dockerHasCLI = inventory.TerminalCopy(st.Installations) >= 0
		}
	}
	if dockerState != nil && (dockerHasCLI || len(dockerState.Contexts) > 0) {
		items = append(items, mygo.Separator())
		header := &mygo.MenuItem{
			ID:       "containers-header",
			Label:    locales.TrayContainers(),
			Disabled: true,
		}
		if len(containers) > 0 {
			header.Badge = locales.TrayContainersRunning(strconv.Itoa(len(containers)))
		}
		items = append(items, header)
		if len(containers) == 0 {
			items = append(items, &mygo.MenuItem{
				ID:       "containers-none",
				Label:    locales.TrayContainersNone(),
				Disabled: true,
			})
		} else {
			dot := trayRunningIcon()
			limit := min(len(containers), trayMaxContainers)
			for i := range limit {
				c := containers[i]
				label := strings.TrimPrefix(c.Name, "/")
				if label == "" {
					label = c.ID
				}
				items = append(items, &mygo.MenuItem{
					ID:    "container-" + c.ID,
					Label: label,
					Icon:  dot,
					Badge: trayFirstPort(c.Ports),
					Click: func(*mygo.MenuItem, *mygo.Window) {
						if act.openContainers != nil {
							act.openContainers()
						}
					},
				})
			}
		}
		items = append(items, &mygo.MenuItem{
			ID:    "containers-all",
			Label: locales.TrayAllContainers(),
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if act.openContainers != nil {
					act.openContainers()
				}
			},
		})
	}

	// Colima control submenu when default Colima docker profile exists.
	if p, ok := trayColimaProfile(dockerState); ok {
		items = append(items, mygo.Separator())
		running := p.Status == "running"
		title := locales.TrayColimaStopped()
		actionLabel := locales.TrayColimaStart()
		actionFn := act.startColima
		if running {
			title = locales.TrayColimaRunning()
			actionLabel = locales.TrayColimaStop()
			actionFn = act.stopColima
		}
		items = append(items, &mygo.MenuItem{
			ID:    "colima",
			Label: title,
			Submenu: []*mygo.MenuItem{
				{
					ID:    "colima-toggle",
					Label: actionLabel,
					Click: func(*mygo.MenuItem, *mygo.Window) {
						if actionFn != nil {
							actionFn()
						}
					},
				},
				{
					ID:    "colima-settings",
					Label: locales.TrayColimaSettings(),
					Click: func(*mygo.MenuItem, *mygo.Window) {
						if act.openDocker != nil {
							act.openDocker()
						}
					},
				},
			},
		})
	}

	// Contexts submenu (Docker + Kubernetes, 1-click switching).
	var ctxSub []*mygo.MenuItem
	if dockerState != nil && dockerHasCLI && len(dockerState.Contexts) > 0 {
		ctxSub = append(ctxSub, &mygo.MenuItem{ID: "ctx-docker-hdr", Label: "Docker", Disabled: true})
		for _, k := range dockerState.Contexts {
			name := k.Name
			ctxSub = append(ctxSub, &mygo.MenuItem{
				ID:       "ctx-docker-" + name,
				Label:    name,
				Type:     mygo.MenuItemRadio,
				Checked:  name == dockerState.Selected,
				Disabled: switching,
				Click: func(*mygo.MenuItem, *mygo.Window) {
					if act.switchDockerCtx != nil {
						act.switchDockerCtx(name)
					}
				},
			})
		}
	}
	if snap != nil {
		if st, ok := snap.Tool("cli"); ok && st.Cli != nil && trayKubectlPath(*st.Cli) != "" {
			kubeNames := trayKubeContexts(*st.Cli)
			if len(kubeNames) > 0 {
				if len(ctxSub) > 0 {
					ctxSub = append(ctxSub, mygo.Separator())
				}
				current := trayKubeCurrent(*st.Cli)
				ctxSub = append(ctxSub, &mygo.MenuItem{ID: "ctx-kube-hdr", Label: "Kubernetes", Disabled: true})
				for _, name := range kubeNames {
					ctxName := name
					ctxSub = append(ctxSub, &mygo.MenuItem{
						ID:       "ctx-kube-" + ctxName,
						Label:    ctxName,
						Type:     mygo.MenuItemRadio,
						Checked:  ctxName == current,
						Disabled: switching,
						Click: func(*mygo.MenuItem, *mygo.Window) {
							if act.switchKubeCtx != nil {
								act.switchKubeCtx(ctxName)
							}
						},
					})
				}
			}
		}
	}
	if len(ctxSub) > 0 {
		items = append(items, mygo.Separator(), &mygo.MenuItem{
			ID:      "contexts",
			Label:   locales.TrayContexts(),
			Submenu: ctxSub,
		})
	}

	appUpdateLabel := locales.AppUpdateCheck()
	if appUpdateAvailable != "" {
		appUpdateLabel = locales.AppUpdateAvailable(appUpdateAvailable)
	}
	items = append(items,
		mygo.Separator(),
		&mygo.MenuItem{
			ID:    "updates",
			Label: locales.TrayCheckUpdates(),
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if act.checkUpdates != nil {
					act.checkUpdates()
				}
			},
		},
		&mygo.MenuItem{
			ID:    "app-update",
			Label: appUpdateLabel,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if act.checkAppUpdate != nil {
					act.checkAppUpdate()
				}
			},
		},
		&mygo.MenuItem{
			ID:          "settings",
			Label:       locales.TraySettings(),
			Accelerator: "CmdOrCtrl+,",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if act.openSettings != nil {
					act.openSettings()
				}
			},
		},
		mygo.Separator(),
		&mygo.MenuItem{
			ID:          "quit",
			Label:       locales.TrayQuit(),
			Accelerator: "CmdOrCtrl+Q",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if act.quit != nil {
					act.quit()
				}
			},
		},
	)
	return items
}

func trayDockerTarget(snap *inventory.Snapshot) (docker.Target, bool) {
	st, ok := snap.Tool("docker")
	if !ok || st.Docker == nil {
		return docker.Target{}, false
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 {
		return docker.Target{}, false
	}
	for _, k := range st.Docker.Contexts {
		if k.Name == st.Docker.Selected && k.Local && k.Status == "available" && k.Endpoint != "" {
			return docker.Target{
				Docker:   st.Installations[active].Path,
				Endpoint: k.Endpoint,
			}, true
		}
	}
	return docker.Target{}, false
}

func trayColimaProfile(d *inventory.DockerState) (inventory.ColimaProfile, bool) {
	if d == nil || d.Colima == nil || d.Colima.Unreadable {
		return inventory.ColimaProfile{}, false
	}
	for _, p := range d.Colima.Profiles {
		if p.Name == "default" && p.Runtime == "docker" {
			return p, true
		}
	}
	return inventory.ColimaProfile{}, false
}

func trayKubectlPath(cli inventory.CliState) string {
	var installs []inventory.Installation
	for _, in := range cli.Tools {
		if in.Tool == "kubectl" {
			installs = append(installs, in)
		}
	}
	if i := inventory.TerminalCopy(installs); i >= 0 {
		return installs[i].Path
	}
	return ""
}

func trayKubeCurrent(cli inventory.CliState) string {
	for _, c := range cli.Contexts {
		if c.Tool == "kubectl" {
			return c.Name
		}
	}
	return ""
}

func trayKubeContexts(cli inventory.CliState) []string {
	var out []string
	for _, k := range cli.KubeContexts {
		if !k.Shadowed && k.Name != "" {
			out = append(out, k.Name)
		}
	}
	return out
}

func trayFirstPort(raw string) string {
	for item := range strings.SplitSeq(raw, ",") {
		host, _, ok := strings.Cut(strings.TrimSpace(item), "->")
		if !ok {
			continue
		}
		if _, port, ok := strings.CutLast(host, ":"); ok {
			port = strings.TrimSpace(port)
			if port != "" {
				return ":" + port
			}
		}
	}
	return ""
}
