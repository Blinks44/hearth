package ui

import (
	"cmp"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/aiquota"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
	"github.com/Blinks44/hearth/resources"
)

const (
	trayPopoverWidth            = 444
	trayPopoverScrollMaxH       = 240
	trayPopoverQuotasScrollMaxH = 232
)

// TrayPopoverActions carries live external data and callbacks from the host
// tray controller into the pure TrayPopoverView.
type TrayPopoverActions struct {
	Containers            []docker.Container
	Quotas                *aiquota.Snapshot
	HasColima             bool
	ColimaRunning         bool
	DockerContext         string
	DockerContexts        []string
	DockerOverride        string
	KubeContext           string
	KubeContexts          []string
	Switching             bool
	AppUpdateAvailable    string
	Refreshing            bool
	OnRefresh             func()
	OnOpenHearth          func()
	OnOpenSettings        func()
	OnOpenQuotas          func()
	OnCheckAppUpdate      func()
	OnOpenPorts           func()
	OnOpenDocker          func()
	OnOpenContainer       func(id, name string)
	OnOpenContainers      func()
	OnOpenKube            func()
	OnOpenLogs            func(dir, source, name string)
	OnStopContainer       func(docker.Container)
	OnRestartContainer    func(docker.Container)
	OnStopDatabase        func(inventory.Database)
	OnStopProcess         func(inventory.DatabaseListener)
	OnToggleColima        func(start bool)
	OnSwitchDockerContext func(name string)
	OnSwitchKubeContext   func(name string)
	OpenContextPickerKey  string
	OnOpenContextPicker   func(key string, anchor ui.Rect, current string, items []string, onSelect func(string))
}

type popoverServiceItem struct {
	key       string
	port      string
	tag       string
	tagBg     ui.Color
	tagFg     ui.Color
	title     string
	subtitle  string
	openURL   string
	onLogs    func()
	onRestart func()
	onStop    func()
	stopLabel string
	logsLabel string
}

// TrayPopoverView renders the native menu-bar / system-tray live control center
// matching Sketch frame 158 (444px crisp floating panel, no top arrow).
// It is pure UI (zero I/O) and testable via ui.NewTester.
func (a *App) TrayPopoverView(c *ui.Context, act TrayPopoverActions) {
	c.SetTheme(Theme(c.Theme()))
	c.Root().Background(ui.Transparent)
	if act.OnOpenHearth != nil {
		c.OnShortcut(ui.Cmd, ui.KeyO, act.OnOpenHearth)
	}

	t := c.Theme()
	pal := Colors(t)
	now := c.Now()

	services := a.collectPopoverServices(pal, act)
	quotaSnap := act.Quotas
	if quotaSnap == nil {
		quotaSnap = a.Quotas
	}
	quotas := a.trayVisibleQuotas(quotaSnap)
	hasDockerCtx := act.DockerContext != "" || len(act.DockerContexts) > 0
	hasKubeCtx := act.KubeContext != "" || len(act.KubeContexts) > 0
	hasContexts := hasDockerCtx || hasKubeCtx

	showServices := !a.isTraySectionHidden(traySectionServices)
	showQuotas := !a.isTraySectionHidden(traySectionQuotas) && len(quotas) > 0
	showContexts := !a.isTraySectionHidden(traySectionContexts) && (hasContexts || act.HasColima)

	ui.Column(c.Key("tray-popover")).Width(trayPopoverWidth).
		Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Radius(14).Clip().Children(func() {
		// Header: Hearth logo + count + [Open ⌘O] + refresh + settings.
		logoOnce.Do(func() { logo, _ = ui.DecodeImage(resources.Logo) })
		ui.Row(c).Gap(8).Padding(12, 16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
			ui.Box(c).Size(22, 22).Radius(6).Clip().Shrink(0).Children(func() {
				ui.Image(c, logo).Size(22, 22)
			})
			ui.Text(c, "Hearth").FontSize(14).FontWeight(600).SingleLine()
			if len(services) > 0 {
				ui.Text(c, "·").FontSize(12).TextColor(pal.TextFaint).SingleLine()
				ui.Text(c, locales.TrayPopoverServicesCount(len(services))).FontSize(12).TextColor(t.TextMuted).SingleLine()
			}
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			if act.OnOpenHearth != nil {
				openBtn := ui.ButtonBase(c.Key("popover-open")).Row().Gap(6).Height(28).Padding(0, 10).Radius(7).
					AlignItems(ui.Center).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).
					Label(locales.TrayOpenHearth()).OnClick(act.OnOpenHearth)
				if openBtn.Hovered() {
					openBtn.Background(pal.Muted)
				}
				openBtn.Children(func() {
					ui.Text(c, locales.TrayPopoverOpenShort()).FontSize(12).FontWeight(500).TextColor(t.Text).SingleLine()
					ui.Text(c, cmdKey("O")).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
				})
			}
			if act.OnRefresh != nil {
				popoverToolbarIcon(c, "popover-refresh", "refresh-cw", locales.PortsRefresh(), act.Refreshing, act.Refreshing, act.OnRefresh)
			}
			if act.OnOpenSettings != nil {
				popoverToolbarIcon(c, "popover-settings", "settings", locales.TraySettings(), false, false, act.OnOpenSettings)
			}
		})

		// Render enabled sections in the user's configured order.
		renderedAny := false
		for _, sec := range a.traySectionOrder() {
			switch sec {
			case traySectionServices:
				if !showServices {
					continue
				}
				a.popoverServicesSection(c, services, showQuotas, renderedAny)
				renderedAny = true
			case traySectionQuotas:
				if !showQuotas {
					continue
				}
				a.popoverQuotasSection(c, quotas, act, showServices, renderedAny, now)
				renderedAny = true
			case traySectionContexts:
				if !showContexts {
					continue
				}
				a.popoverContextsSection(c, act, hasDockerCtx, hasKubeCtx, hasContexts, renderedAny)
				renderedAny = true
			}
		}

		// Bottom navigation & status bar.
		ui.Row(c.Key("popover-footer-links")).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).
			Padding(10, 16, 12, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			if act.OnOpenPorts != nil {
				link := ui.ButtonBase(c.Key("popover-all-services")).Padding(2, 0).Radius(4).
					Label(locales.TrayPopoverAllServices()).OnClick(act.OnOpenPorts)
				link.Children(func() {
					ui.Text(c, locales.TrayPopoverAllServices()).FontSize(12).FontWeight(500).TextColor(pal.AccentInk).SingleLine()
				})
			}
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			if act.AppUpdateAvailable != "" && act.OnCheckAppUpdate != nil {
				upd := ui.ButtonBase(c.Key("popover-update")).Padding(2, 0).Radius(4).
					Label(locales.AppUpdateAvailable(act.AppUpdateAvailable)).OnClick(act.OnCheckAppUpdate)
				upd.Children(func() {
					ui.Text(c, locales.AppUpdateAvailable(act.AppUpdateAvailable)).FontSize(11.5).FontWeight(500).TextColor(pal.AccentInk).SingleLine()
				})
			} else {
				ui.Text(c, locales.TrayPopoverRightClickHint()).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			}
		})
	})
}

func (a *App) popoverServicesSection(c *ui.Context, services []popoverServiceItem, hasQuotas, topDivider bool) {
	t := c.Theme()
	pal := Colors(t)
	col := ui.Column(c.Key("popover-sec-services"))
	if topDivider {
		col.BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	}
	col.Children(func() {
		if len(services) > 0 {
			ui.Row(c).Gap(8).Padding(10, 16, 6, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, strings.ToUpper(locales.TrayPopoverRunningNow())+" · "+strconv.Itoa(len(services))).
					FontSize(10.5).FontWeight(600).TextColor(pal.TextFaint).SingleLine()
				ui.Box(c).Grow(1).MinWidth(0).Height(1)
				ui.Text(c, locales.TrayPopoverRunningHint()).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			})

			maxH := float32(trayPopoverScrollMaxH)
			if !hasQuotas {
				maxH = 290
			}
			ui.Scroll(c.Key("popover-scroll")).MaxHeight(maxH).Children(func() {
				ui.Column(c).Children(func() {
					for _, item := range services {
						popoverServiceRow(c, item)
					}
				})
			})
			return
		}
		if hasQuotas {
			ui.Row(c.Key("popover-empty-compact")).Padding(10, 16).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon("layers")).Size(14, 14).TextColor(pal.TextFaint)
				ui.Text(c, locales.TrayPopoverEmpty()).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
			return
		}
		ui.Column(c.Key("popover-empty")).Padding(28, 24).Center().Gap(6).Children(func() {
			ui.Icon(c, Icon("layers")).Size(22, 22).TextColor(pal.TextFaint)
			ui.Text(c, locales.TrayPopoverEmpty()).FontSize(13).FontWeight(500).TextAlign(ui.Center)
			ui.Text(c, locales.TrayPopoverEmptyBody()).FontSize(11.5).TextColor(t.TextMuted).TextAlign(ui.Center).Wrap()
		})
	})
}

func (a *App) popoverQuotasSection(c *ui.Context, quotas []aiquota.ProviderQuota, act TrayPopoverActions, hasServices, topDivider bool, now time.Time) {
	pal := Colors(c.Theme())
	col := ui.Column(c.Key("popover-sec-quotas"))
	if topDivider {
		col.BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	}
	col.Children(func() {
		ui.Row(c).Gap(8).Padding(10, 16, 6, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, strings.ToUpper(locales.TrayPopoverQuotasHeader())+" · "+strconv.Itoa(len(quotas))).
				FontSize(10.5).FontWeight(600).TextColor(pal.TextFaint).SingleLine()
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			if act.OnOpenQuotas != nil {
				link := ui.ButtonBase(c.Key("popover-all-quotas")).Padding(2, 0).Radius(4).
					Label(locales.TrayPopoverAllQuotas()).OnClick(act.OnOpenQuotas)
				link.Children(func() {
					ui.Text(c, locales.TrayPopoverAllQuotas()).FontSize(11.5).FontWeight(500).TextColor(pal.AccentInk).SingleLine()
				})
			}
		})

		maxH := float32(trayPopoverQuotasScrollMaxH)
		if !hasServices {
			maxH = 290
		}
		ui.Scroll(c.Key("popover-quotas-scroll")).MaxHeight(maxH).Children(func() {
			ui.Column(c).Children(func() {
				for _, pq := range quotas {
					popoverQuotaRow(c, pq, act.OnOpenQuotas, now)
				}
			})
		})
	})
}

func popoverQuotaRow(c *ui.Context, pq aiquota.ProviderQuota, onOpen func(), now time.Time) {
	t := c.Theme()
	pal := Colors(t)
	info := aiquota.LookupProvider(pq.Provider)
	windows := aiquota.NormalizeWindows(pq.Windows)

	row := ui.ButtonBase(c.Key("pop-quota-"+string(pq.Provider))).Row().
		Padding(10, 16).Gap(10).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).
		Label(pq.Name)
	if onOpen != nil {
		row.OnClick(onOpen)
	}
	if row.Hovered() {
		row.Background(pal.Subtle)
	}

	// Group windows if provider has 3+ windows (e.g. Antigravity: Gemini 5h/week + Claude 5h/week).
	type winGroup struct {
		name    string
		windows []aiquota.QuotaWindow
	}
	var groups []winGroup
	if len(windows) >= 3 {
		byName := map[string]int{}
		for _, w := range windows {
			gn := w.Group
			if gn == "" {
				gn = w.Model
			}
			if idx, ok := byName[gn]; ok {
				groups[idx].windows = append(groups[idx].windows, w)
			} else {
				byName[gn] = len(groups)
				groups = append(groups, winGroup{name: gn, windows: []aiquota.QuotaWindow{w}})
			}
		}
		// If everything landed in 1 group with >2 windows, split in chunks of 2.
		if len(groups) == 1 && len(groups[0].windows) > 2 {
			ws := groups[0].windows
			groups = nil
			for i := 0; i < len(ws); i += 2 {
				end := min(i+2, len(ws))
				groups = append(groups, winGroup{windows: ws[i:end]})
			}
		}
	}

	row.Children(func() {
		if len(groups) >= 2 {
			// Compact 2-line layout matching Sketch 158 (Antigravity row).
			ui.Column(c).Width(104).Shrink(0).Gap(3).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					quotaProviderLogo(c, pq.Provider, 15)
					ui.Text(c, info.ShortName).FontSize(12.5).FontWeight(500).SingleLine()
				})
				ui.Text(c, locales.QuotasGroupWindowsMeta(len(windows), len(groups))).
					FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(9).Children(func() {
				limit := min(len(groups), 2)
				for gi := range limit {
					g := groups[gi]
					ui.Row(c.Key("g-" + strconv.Itoa(gi))).Gap(12).AlignItems(ui.Center).Children(func() {
						for wi := 0; wi < min(len(g.windows), 2); wi++ {
							w := g.windows[wi]
							lbl := quotaShortWindowLabel(w)
							if wi == 0 && g.name != "" {
								lbl = g.name + " " + lbl
							}
							popoverQuotaMiniBar(c.Key("mb-"+strconv.Itoa(wi)), lbl, w, now)
						}
					})
				}
			})
			return
		}

		// Standard 1-line compact row (1-2 windows).
		ui.Row(c).Width(104).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
			quotaProviderLogo(c, pq.Provider, 15)
			ui.Text(c, info.ShortName).FontSize(12.5).FontWeight(500).SingleLine()
		})
		ui.Row(c).Grow(1).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
			limit := min(len(windows), 2)
			for wi := range limit {
				w := windows[wi]
				popoverQuotaMiniBar(c.Key("mb-"+strconv.Itoa(wi)), quotaShortWindowLabel(w), w, now)
			}
		})
	})
}

func popoverQuotaMiniBar(c *ui.Context, label string, w aiquota.QuotaWindow, now time.Time) {
	t := c.Theme()
	pal := Colors(t)
	left := w.EffectiveLeft(now)
	barColor, pctColor := quotaBarColors(t, pal, w, now)
	valText := fmt.Sprintf("%.0f%%", math.Round(left))
	if w.DollarsUsed != nil && w.DollarsLimit != nil && *w.DollarsLimit > 0 {
		valText = fmt.Sprintf("$%.0f/$%.0f", *w.DollarsUsed, *w.DollarsLimit)
	}
	expectedLeft, hasTick := w.TimeLeft(now)
	resetAt := formatQuotaResetAt(w.ResetsAt, now)

	ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Gap(3).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted).SingleLine()
				if resetAt != "" {
					ui.Text(c, "· "+resetAt).FontSize(10).TextColor(pal.TextFaint).Shrink(1).MinWidth(0).SingleLine()
				}
			})
			ui.Text(c, valText).Font(FontMono).FontSize(10.5).FontWeight(600).TextColor(pctColor).Shrink(0).SingleLine()
		})
		quotaProgressBar(c, left, expectedLeft, hasTick, barColor, 5)
	})
}

func (a *App) popoverContextsSection(c *ui.Context, act TrayPopoverActions, hasDockerCtx, hasKubeCtx, hasContexts, topDivider bool) {
	pal := Colors(c.Theme())
	col := ui.Column(c.Key("popover-sec-contexts")).Background(pal.Subtle).Padding(12, 16).Gap(10)
	if topDivider {
		col.BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	}
	col.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if hasContexts {
				ui.Text(c, strings.ToUpper(locales.TrayPopoverContextsOneClick())).
					FontSize(10).FontWeight(600).TextColor(pal.TextFaint).SingleLine()
			}
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			if act.HasColima {
				popoverColimaStatus(c, act)
			}
		})

		if hasContexts {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				if hasDockerCtx {
					canSwitch := len(act.DockerContexts) > 1 && act.OnSwitchDockerContext != nil && act.DockerOverride == "" && !act.Switching
					popoverContextSelect(
						c, "pop-ctx-docker", &a.popoverDockerCtxBtn, "Docker",
						cmp.Or(act.DockerContext, "—"),
						act.DockerContexts, canSwitch, act.OpenContextPickerKey == "pop-ctx-docker",
						act.OnOpenContextPicker, act.OnSwitchDockerContext, act.OnOpenDocker,
					)
				}
				if hasKubeCtx {
					canSwitch := len(act.KubeContexts) > 1 && act.OnSwitchKubeContext != nil && !act.Switching
					popoverContextSelect(
						c, "pop-ctx-kube", &a.popoverKubeCtxBtn, "K8s",
						cmp.Or(act.KubeContext, "—"),
						act.KubeContexts, canSwitch, act.OpenContextPickerKey == "pop-ctx-kube",
						act.OnOpenContextPicker, act.OnSwitchKubeContext, act.OnOpenKube,
					)
				}
			})
		}
	})
}

func (a *App) collectPopoverServices(pal Palette, act TrayPopoverActions) []popoverServiceItem {
	allPorts := a.portRows()
	var items []popoverServiceItem

	// 1. Managed project scripts.
	for _, run := range a.scriptRuns {
		if !run.running {
			continue
		}
		r := run
		sum := summarizeScriptListeners(a.scriptListeners(r, allPorts))
		tag := cmp.Or(sum.kind, "Dev")
		title := filepath.Base(r.dir)
		if r.script.Name != "" && !strings.EqualFold(r.script.Name, "dev") {
			title = title + " · " + r.script.Name
		}
		sub := formatCPUAndMem(sum.cpu, sum.memBytes)
		if sub == "" {
			sub = r.script.Source
		}
		var openURL string
		if sum.firstHTTPPort != "" {
			openURL = listenerURL(sum.firstHTTPPort)
		}
		var onLogs func()
		if act.OnOpenLogs != nil {
			onLogs = func() { act.OnOpenLogs(r.dir, r.script.Source, r.script.Name) }
		}
		items = append(items, popoverServiceItem{
			key:       "run:" + r.key,
			port:      sum.firstPort,
			tag:       tag,
			tagBg:     pal.AccentSoft,
			tagFg:     pal.AccentInk,
			title:     title,
			subtitle:  sub,
			openURL:   openURL,
			onLogs:    onLogs,
			logsLabel: locales.ProjectScriptsLogs(),
			onRestart: func() { a.restartScript(r) },
			onStop:    func() { a.stopScript(r) },
			stopLabel: locales.ProjectScriptsStop(),
		})
	}

	// 2. External dev servers (excluding managed script listeners).
	for _, row := range allPorts {
		if a.scriptRunForListener(row.DatabaseListener) != nil {
			continue
		}
		cls := inventory.ClassifyDevServer(row.DatabaseListener)
		if cls.Role != inventory.RoleDevServer {
			continue
		}
		r := row
		tag := cmp.Or(cls.Kind, "Dev")
		title := r.Process
		if r.Project != "" {
			title = filepath.Base(r.Project)
		} else if title == "" {
			title = tag
		}
		sub := formatCPUAndMem(r.CPUPercent, r.MemoryBytes)
		if sub == "" {
			sub = cmp.Or(r.Process, "localhost")
		}
		var openURL string
		if cls.HTTPOpenable && !nonHTTPPorts[r.Port] {
			openURL = listenerURL(r.Port)
		}
		var onStop func()
		if r.Own && act.OnStopProcess != nil {
			onStop = func() { act.OnStopProcess(r.DatabaseListener) }
		}
		items = append(items, popoverServiceItem{
			key:       "dev:" + r.Port + ":" + r.PID,
			port:      r.Port,
			tag:       tag,
			tagBg:     pal.AccentSoft,
			tagFg:     pal.AccentInk,
			title:     title,
			subtitle:  sub,
			openURL:   openURL,
			onStop:    onStop,
			stopLabel: locales.PortsStop(),
		})
	}

	// 3. Running Docker containers.
	for _, container := range act.Containers {
		ct := container
		name := strings.TrimPrefix(ct.Name, "/")
		if name == "" {
			name = ct.ID
		}
		port := docker.FirstPublishedPort(ct.Ports)
		sub := ct.Image
		var onLogs, onRestart, onStop func()
		if act.OnOpenContainer != nil && ct.ID != "" {
			ctID, ctName := ct.ID, name
			onLogs = func() { act.OnOpenContainer(ctID, ctName) }
		} else if act.OnOpenContainers != nil {
			onLogs = act.OnOpenContainers
		}
		if act.OnRestartContainer != nil && ops.DockerRestart.Applies(ct) {
			onRestart = func() { act.OnRestartContainer(ct) }
		}
		if act.OnStopContainer != nil && ops.DockerStop.Applies(ct) {
			onStop = func() { act.OnStopContainer(ct) }
		}
		items = append(items, popoverServiceItem{
			key:       "ct:" + ct.ID,
			port:      port,
			tag:       "Docker",
			tagBg:     pal.InfoSoft,
			tagFg:     pal.InfoText,
			title:     name,
			subtitle:  sub,
			onLogs:    onLogs,
			logsLabel: locales.PortsOpenLogs(),
			onRestart: onRestart,
			onStop:    onStop,
			stopLabel: locales.DockerContainerStop(),
		})
	}

	// 4. Local running non-Docker databases.
	for _, db := range inventory.DatabaseServers(a.Snapshot) {
		if db.Source == "docker" || db.Status != "running" {
			continue
		}
		d := db
		port := ""
		if len(d.Ports) > 0 {
			port = d.Ports[0]
		}
		title := cmp.Or(d.Kind, d.Service)
		if d.Version != "" && !strings.Contains(title, d.Version) {
			major, _, _ := strings.Cut(d.Version, ".")
			if major != "" {
				title = title + " " + major
			}
		}
		sub := cmp.Or(d.Service, d.Source)
		for _, r := range allPorts {
			matched := port != "" && r.Port == port
			if !matched {
				if svc, ok := a.portService(r); ok && svc.Service == d.Service && svc.Kind == d.Kind {
					matched = true
					if port == "" {
						port = r.Port
					}
				}
			}
			if matched {
				if m := formatCPUAndMem(r.CPUPercent, r.MemoryBytes); m != "" {
					sub = m
				}
				break
			}
		}
		var onStop func()
		if act.OnStopDatabase != nil && d.Service != "" {
			onStop = func() { act.OnStopDatabase(d) }
		}
		items = append(items, popoverServiceItem{
			key:       "db:" + d.Source + ":" + cmp.Or(d.Service, d.Kind) + ":" + port,
			port:      port,
			tag:       locales.PortsDbTag(),
			tagBg:     pal.SuccessSoft,
			tagFg:     pal.SuccessText,
			title:     title,
			subtitle:  sub,
			onStop:    onStop,
			stopLabel: locales.PortsStop(),
		})
	}

	// 5. Active Kubernetes port-forwards.
	for _, fwd := range a.forwards {
		if fwd.ended {
			continue
		}
		f := fwd
		portStr := strconv.Itoa(f.target.Local)
		title := f.target.Name
		if f.target.Context != "" {
			title = f.target.Context + " · " + f.target.Name
		}
		sub := "localhost:" + portStr + " → :" + strconv.Itoa(f.target.Remote)
		var openURL string
		if !nonHTTPPorts[portStr] {
			openURL = listenerURL(portStr)
		}
		var onStop func()
		if f.stop != nil {
			onStop = func() {
				f.stopping = true
				f.stop()
			}
		}
		items = append(items, popoverServiceItem{
			key:       "fwd:" + portStr,
			port:      portStr,
			tag:       "k8s",
			tagBg:     pal.WarningSoft,
			tagFg:     pal.WarningText,
			title:     title,
			subtitle:  sub,
			openURL:   openURL,
			onStop:    onStop,
			stopLabel: locales.ProjectScriptsStop(),
		})
	}

	return items
}

func popoverServiceRow(c *ui.Context, item popoverServiceItem) {
	t := c.Theme()
	pal := Colors(t)
	portText := "—"
	if item.port != "" {
		portText = ":" + item.port
	}
	ui.Row(c.Key("pop-svc:"+item.key)).Gap(10).Padding(10, 16).MinHeight(52).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Text(c, portText).Font(FontMono).FontSize(12.5).TextColor(t.Text).Width(50).Shrink(0).SingleLine()
		ui.Box(c).Width(60).Shrink(0).Padding(3, 6).Radius(6).Center().Background(item.tagBg).Children(func() {
			clipped(c, item.tag).FontSize(11).FontWeight(500).TextColor(item.tagFg)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, item.title).FontSize(13).FontWeight(500)
			if item.subtitle != "" {
				clipped(c, item.subtitle).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			}
		})
		ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
			if item.openURL != "" {
				u := item.openURL
				subtleSquareIconButton(c, "pop-open:"+item.key, "external-link", locales.PortsOpenBrowser(), 26, func() {
					c.OpenURL(u)
				})
			}
			if item.onLogs != nil {
				subtleSquareIconButton(c, "pop-logs:"+item.key, "scroll-text", cmp.Or(item.logsLabel, locales.ProjectScriptsLogs()), 26, item.onLogs)
			}
			if item.onRestart != nil {
				subtleSquareIconButton(c, "pop-restart:"+item.key, "refresh-cw", locales.ProjectScriptsRestart(), 26, item.onRestart)
			}
			if item.onStop != nil {
				dangerIconButton(c, "pop-stop:"+item.key, "square", cmp.Or(item.stopLabel, locales.PortsStop()), 26, item.onStop)
			}
		})
	})
}

func popoverColimaStatus(c *ui.Context, act TrayPopoverActions) {
	t := c.Theme()
	pal := Colors(t)
	dot := pal.TextFaint
	textColor := t.TextMuted
	label := locales.TrayPopoverColimaStoppedShort()
	actionLabel := locales.TrayColimaStart()
	if act.ColimaRunning {
		dot = t.Success
		textColor = pal.SuccessText
		label = locales.TrayPopoverColimaRunningShort()
		actionLabel = locales.TrayColimaStop()
	}
	onClick := func() {
		if act.OnToggleColima != nil {
			act.OnToggleColima(!act.ColimaRunning)
		} else if act.OnOpenDocker != nil {
			act.OnOpenDocker()
		}
	}
	btn := ui.ButtonBase(c.Key("pop-colima-status")).Row().Gap(6).Padding(2, 6).Radius(6).AlignItems(ui.Center).
		Label(actionLabel).Tooltip(actionLabel).OnClick(onClick)
	if btn.Hovered() {
		btn.Background(pal.Muted)
	}
	btn.Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot).Shrink(0)
		ui.Text(c, label).FontSize(11.5).FontWeight(500).TextColor(textColor).SingleLine()
	})
}

func popoverContextSelect(
	c *ui.Context,
	key string,
	handle *ui.Handle,
	prefix, current string,
	items []string,
	canSwitch, open bool,
	onOpenPicker func(key string, anchor ui.Rect, current string, items []string, onSelect func(string)),
	onSelect func(string),
	onOpen func(),
) {
	t := c.Theme()
	pal := Colors(t)
	label := prefix + ": " + current

	if canSwitch {
		btn := ui.ButtonBase(c.Key(key)).Row().Grow(1).Basis(0).MinWidth(0).Height(34).Padding(0, 12).Radius(8).
			Gap(6).AlignItems(ui.Center).Justify(ui.Start).
			Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Label(label)
		if open || btn.Hovered() {
			btn.BorderColor(pal.TextFaint)
		}
		if onOpenPicker != nil {
			if handle != nil {
				btn.Bind(handle)
			}
			btn.OnClick(func() {
				var bounds ui.Rect
				if handle != nil {
					bounds = handle.Bounds(c)
				}
				onOpenPicker(key, bounds, current, items, onSelect)
			})
		} else {
			btn.Menu(func(m *ui.Menu) {
				for _, item := range items {
					name := item
					if m.Item(name).Checked(name == current).Chosen() && name != current && onSelect != nil {
						onSelect(name)
					}
				}
			})
		}
		chev := "chevron-down"
		if open {
			chev = "chevron-up"
		}
		btn.Children(func() {
			ui.Text(c, prefix+":").FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(0)
			clipped(c, current).Font(FontMono).FontSize(12).FontWeight(500).TextColor(t.Text).Grow(1)
			ui.Icon(c, Icon(chev)).Size(13, 13).TextColor(pal.TextFaint).Shrink(0)
		})
		return
	}

	btn := ui.ButtonBase(c.Key(key)).Row().Grow(1).Basis(0).MinWidth(0).Height(34).Padding(0, 12).Radius(8).
		Gap(6).AlignItems(ui.Center).Justify(ui.Start).
		Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Label(label)
	if onOpen != nil {
		btn.OnClick(onOpen)
		if btn.Hovered() {
			btn.BorderColor(pal.TextFaint)
		}
	}
	btn.Children(func() {
		ui.Text(c, prefix+":").FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(0)
		clipped(c, current).Font(FontMono).FontSize(12).FontWeight(500).TextColor(t.Text).Grow(1)
		if onOpen != nil {
			ui.Icon(c, Icon("chevron-right")).Size(13, 13).TextColor(pal.TextFaint).Shrink(0)
		}
	})
}

// TrayContextPickerView renders the full-width downward dropdown list for a
// Docker or Kubernetes context selector inside a child flyout anchored directly
// below the trigger button.
func TrayContextPickerView(c *ui.Context, current string, items []string, onChoose func(string)) {
	c.SetTheme(Theme(c.Theme()))
	c.Root().Background(ui.Transparent)

	t := c.Theme()
	pal := Colors(t)

	ui.Column(c.Key("tray-context-picker")).FillWidth().
		Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Radius(10).Clip().Padding(4).Children(func() {
		ui.Scroll(c.Key("tray-context-picker-scroll")).FillWidth().MaxHeight(252).Vertical().Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for _, item := range items {
					name := item
					selected := name == current
					row := ui.ButtonBase(c.Key("ctx-opt:"+name)).Row().FillWidth().Height(30).Padding(0, 10).Radius(6).
						Gap(8).AlignItems(ui.Center).Justify(ui.Start).Label(name).OnClick(func() {
						if onChoose != nil {
							onChoose(name)
						}
					})
					switch {
					case row.Hovered():
						row.Background(pal.Muted)
					case selected:
						row.Background(pal.Subtle)
					}
					row.Children(func() {
						ui.Box(c).Size(14, 14).Center().Shrink(0).Children(func() {
							if selected {
								ui.Icon(c, Icon("check")).Size(13, 13).TextColor(pal.AccentInk)
							}
						})
						weight := 400
						if selected {
							weight = 600
						}
						clipped(c, name).Font(FontMono).FontSize(12).FontWeight(weight).TextColor(t.Text).Grow(1)
					})
				}
			})
		})
	})
}

func popoverToolbarIcon(c *ui.Context, key, icon, label string, disabled, spinning bool, onClick func()) {
	t := c.Theme()
	pal := Colors(t)
	b := ui.ButtonBase(c.Key(key)).Size(28, 28).Center().Radius(7).Shrink(0).
		Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).
		Label(label).Tooltip(label).Disabled(disabled).OnClick(onClick)
	if !disabled && b.Hovered() {
		b.Background(pal.Muted)
	}
	b.Children(func() {
		if spinning {
			spinIcon(c, icon, 14, t.Text, true)
		} else {
			ui.Icon(c, Icon(icon)).Size(14, 14).TextColor(t.Text)
		}
	})
}

// ProposeStopProcess builds a confirmation plan to stop a listening process.
func (a *App) ProposeStopProcess(l inventory.DatabaseListener) bool {
	if a.op.running() {
		return false
	}
	plan, ok := ops.ProcessPlan(l, a.detect("databases"))
	if !ok {
		return false
	}
	a.Propose(plan)
	return true
}

// ProposeStopDatabase builds a confirmation plan to stop a local database service.
func (a *App) ProposeStopDatabase(db inventory.Database) bool {
	if a.op.running() {
		return false
	}
	plan, ok := ops.ServicePlan(ops.ServiceStop, db, a.detect("databases"))
	if !ok {
		return false
	}
	a.Propose(plan)
	return true
}

// ProposeDockerContainer builds a confirmation plan to stop or restart a Docker container.
func (a *App) ProposeDockerContainer(ct docker.Container, action ops.DockerAction) {
	if a.BuildPlan == nil || a.op.running() {
		return
	}
	target, ok := a.selectedDockerTarget()
	if !ok {
		return
	}
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerContainers(target, env, []docker.Container{ct}, action)
	}, func(p ops.Plan, err error) {
		if err != nil {
			a.planErr = err.Error()
			return
		}
		a.Propose(p)
	})
}
