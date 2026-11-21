package ui

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

type forwardPort struct {
	number, preferredLocal int
	label                  string
}

type forwardForm struct {
	target      ops.Forward
	open        bool
	local       string
	remote      string
	portLabel   string
	check       int
	checkedPort int
	suggestion  int
	checkErr    error
}

type portForward struct {
	target   ops.Forward
	stop     func()
	pod      kube.ForwardPod
	stopping bool
	ended    bool
	err      error
}

func (a *App) syncForwardForm() {
	if d := a.cluster.forward; d != nil && (!d.open || a.router.Path() != kubeClusterPage || a.cluster.logs != nil ||
		(d.target.Kind == "service") != (a.cluster.kind == kindServices) || d.target.Kind == "pod" && (a.cluster.kind != kindPods || a.cluster.pod != d.target.UID)) {
		a.cluster.forward = nil
	}
}

func parseForwardPort(value string) int {
	port, err := strconv.ParseUint(strings.TrimSpace(value), 10, 16)
	if err != nil {
		return 0
	}
	return int(port)
}

func (a *App) probeForwardPort(d *forwardForm) {
	d.check++
	check, port := d.check, parseForwardPort(d.local)
	d.checkedPort, d.suggestion, d.checkErr = 0, 0, nil
	if port == 0 {
		return
	}
	a.CheckForwardPort(port, func(suggestion int, err error) {
		if a.cluster.forward == d && check == d.check {
			d.checkedPort, d.suggestion, d.checkErr = port, suggestion, err
		}
	})
}

func (a *App) forwardBusy(port int) bool {
	return slices.ContainsFunc(a.forwards, func(f *portForward) bool { return f.target.Local == port && !f.ended })
}

func (a *App) startForward(d *forwardForm, remote int) {
	local := parseForwardPort(d.local)
	if !d.open || remote == 0 || local == 0 || d.checkedPort != local || d.checkErr != nil || a.forwardBusy(local) {
		return
	}
	target := d.target
	target.Local, target.Remote = local, remote
	d.open = false
	a.forwards = slices.DeleteFunc(a.forwards, func(f *portForward) bool { return f.target == target && f.ended })
	f := &portForward{target: target}
	a.forwards = append(a.forwards, f)
	f.stop = a.RunForward(target, func(pod kube.ForwardPod) { f.pod = pod }, func(err error) {
		f.ended, f.err, f.stop = true, err, nil
		if f.stopping {
			a.dismissForward(f)
		}
	})
}

func (a *App) dismissForward(f *portForward) {
	a.forwards = slices.DeleteFunc(a.forwards, func(other *portForward) bool { return other == f })
}

const podActionHeight = 28

func kubeActionButton(c *ui.Context, icon, label string, height float32) ui.Element {
	return ui.Button(c, "").Height(height).Label(label).Children(func() {
		ui.Icon(c, Icon(icon)).Size(14, 14)
		ui.Text(c, label).FontSize(12).SingleLine()
	})
}

func (a *App) forwardButton(c *ui.Context, uid, kind, namespace, name string, ports []forwardPort) {
	if a.CheckForwardPort == nil || a.RunForward == nil {
		return
	}
	height := float32(24)
	if kind == "pod" {
		height = podActionHeight
	}
	b := kubeActionButton(c.Key("forward-"+uid), "cable", locales.KubeForwardOpen(), height)
	b.OnClick(func() { a.openForward(uid, kind, namespace, name, ports) })
	if d := a.cluster.forward; d != nil && d.target.UID == uid {
		ui.PopoverBase(c.Key("forward-popup-"+uid), b, &d.open, func(panel ui.Element) {
			t := c.Theme()
			panel.AttachTo(b, ui.AnchorBottomEnd, ui.AnchorTopEnd).
				Width(360).Padding(14).Gap(10).Radius(12).Background(t.Background).
				BorderWidth(1).BorderColor(Colors(t).Divider).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.17))
			a.forwardPopover(c, d, ports)
		})
	}
}

// openForward opens the forward popover of the pod or Service, by its
// button.
func (a *App) openForward(uid, kind, namespace, name string, ports []forwardPort) {
	in, ok := cliCopy(a.cliState(), "kubectl")
	if !ok {
		return
	}
	d := &forwardForm{
		target: ops.Forward{Kubectl: in.Path, Context: a.router.Query("context"), Namespace: namespace, Kind: kind, Name: name, UID: uid},
		open:   true,
	}
	if len(ports) > 0 {
		d.portLabel, d.local = ports[0].label, strconv.Itoa(ports[0].preferredLocal)
	}
	a.cluster.forward = d
	a.probeForwardPort(d)
}

func (a *App) forwardPopover(c *ui.Context, d *forwardForm, ports []forwardPort) {
	t := c.Theme()
	ui.Text(c, locales.KubeForwardTitle(d.target.Name)).FontSize(14).FontWeight(600).Wrap()
	remote := parseForwardPort(d.remote)
	label := locales.KubeForwardPodPort()
	if d.target.Kind == "service" {
		label = locales.KubeForwardServicePort()
	}
	forwardField(c, label, func() {
		if len(ports) == 0 {
			ui.TextInput(c.Key("forward-remote"), &d.remote).Label(label).Width(112).Height(podActionHeight).Font(FontMono).FontSize(12)
			return
		}
		labels := make([]string, 0, len(ports))
		for _, p := range ports {
			labels = append(labels, p.label)
			if p.label == d.portLabel {
				remote = p.number
			}
		}
		ui.Select(c.Key("forward-remote-select"), &d.portLabel, labels).Label(label).Width(145).Height(podActionHeight).FontSize(12)
	})
	local := parseForwardPort(d.local)
	blocked := local != 0 && a.forwardBusy(local)
	conflict := blocked || local != 0 && d.checkedPort == local && errors.Is(d.checkErr, ops.ErrForwardPortBusy)
	forwardField(c, locales.KubeForwardLocalPort(), func() {
		input := ui.TextInput(c.Key("forward-local"), &d.local).Label(locales.KubeForwardLocalPort()).
			Width(88).Height(podActionHeight).Font(FontMono).FontSize(12).OnChange(func() { a.probeForwardPort(d) })
		if conflict {
			input.BorderColor(t.Warning)
		}
	})
	switch {
	case conflict:
		pal := Colors(t)
		ui.Column(c).Gap(4).Padding(8, 10).Radius(8).Background(pal.WarningSoft).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon("triangle-alert")).Size(14, 14).Shrink(0).TextColor(t.Warning)
				ui.Text(c, locales.KubeForwardBusy(strconv.Itoa(local))).FontSize(12).TextColor(pal.WarningText).Wrap().Grow(1)
			})
			if d.suggestion != 0 {
				ui.Button(c.Key("forward-suggest"), locales.KubeForwardSuggestion(strconv.Itoa(d.suggestion))).
					AlignSelf(ui.End).Height(podActionHeight).OnClick(func() {
					d.local = strconv.Itoa(d.suggestion)
					a.probeForwardPort(d)
				})
			}
		})
	case local == 0 || (len(ports) == 0 && remote == 0):
		ui.Text(c, locales.KubeForwardRange()).FontSize(11.5).TextColor(t.TextMuted).Wrap()
	case d.checkedPort != local:
		ui.Text(c, locales.KubeForwardChecking()).FontSize(11.5).TextColor(t.TextMuted).Wrap()
	case d.checkErr != nil:
		warningRow(c, d.checkErr.Error())
	}
	if local != 0 {
		forwardField(c, locales.KubeForwardAddress(), func() {
			ui.Text(c, (ops.Forward{Local: local}).Address()).Font(FontMono).FontSize(12).TextColor(Colors(t).AccentInk).Selectable()
		})
	}
	forwardField(c, locales.KubeForwardDestination(), func() {
		destination := d.target.Namespace + "/" + d.target.Name
		if d.target.Kind == "service" {
			destination = locales.KubeForwardSelectedPod()
		}
		ui.Text(c, destination).FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1).MinWidth(0)
	})
	ui.Text(c, locales.KubeForwardLifetime()).FontSize(11.5).TextColor(Colors(t).TextFaint).Wrap()
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		ui.Button(c.Key("forward-cancel"), locales.Cancel()).Height(podActionHeight).OnClick(func() { d.open = false })
		ui.PrimaryButton(c.Key("forward-start"), locales.KubeForwardStart()).Height(podActionHeight).
			Disabled(remote == 0 || local == 0 || d.checkedPort != local || d.checkErr != nil || blocked).
			OnClick(func() { a.startForward(d, remote) })
	})
}

func forwardField(c *ui.Context, label string, field func()) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(108).Shrink(0).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
		field()
	})
}

// forwardList shows sessions in the current context, or across contexts
// on the Ports page.
// forwardList is the port forwards of the context, those of every context
// for "" (as the Ports and Kubernetes pages list them), each with the
// cluster it leads to there.
func (a *App) forwardList(c *ui.Context, context string) {
	onPorts := context == ""
	shown := slices.ContainsFunc(a.forwards, func(f *portForward) bool { return onPorts || f.target.Context == context })
	if !shown {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c.Key("kube-forwards")).Gap(8).Padding(10, 14).Radius(10).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, Icon("cable")).Size(16, 16).TextColor(t.TextMuted)
			ui.Text(c, locales.KubeForwardList()).FontSize(13).FontWeight(600).Grow(1)
			if a.router.Path() != portsPage {
				ui.Button(c.Key("forward-to-ports"), locales.PortsTitle()+" ›").OnClick(func() { a.router.Push(portsPage) })
			}
		})
		for _, f := range a.forwards {
			if onPorts || f.target.Context == context {
				a.forwardRow(c, f, onPorts)
			}
		}
	})
}

func (a *App) forwardRow(c *ui.Context, f *portForward, onPorts bool) {
	t := c.Theme()
	pal := Colors(t)
	label, bg, fg := locales.KubeForwardStarting(), pal.InfoSoft, pal.InfoText
	switch {
	case f.ended && f.pod.Name != "":
		label, bg, fg = locales.KubeForwardDisconnected(), pal.WarningSoft, pal.WarningText
	case f.ended:
		label, bg, fg = locales.KubeForwardFailed(), pal.DangerSoft, pal.DangerText
	case f.stopping:
		label = locales.KubeForwardStopping()
	case f.pod.Name != "":
		label, bg, fg = locales.KubeForwardActive(), pal.SuccessSoft, pal.SuccessText
	}
	target := f.target
	key := strings.Join([]string{target.Context, target.Namespace, target.Kind, target.UID, target.Kubectl, target.Address(), strconv.Itoa(target.Remote)}, "/")
	ui.Column(c.Key(key)).Gap(5).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Padding(6, 0, 0, 0).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, target.Address()).Font(FontMono).FontSize(12).Selectable()
			destination := target.Namespace + "/" + target.Kind + "/" + target.Name + ":" + strconv.Itoa(target.Remote)
			if target.Kind == "service" && f.pod.Name != "" {
				destination += " → pod/" + f.pod.Name + ":" + strconv.Itoa(f.pod.Port)
			}
			ui.Text(c, "→ "+destination).FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1).MinWidth(130)
			if onPorts {
				ui.Button(c.Key("forward-context"), target.Context+" ›").OnClick(func() {
					a.openCluster(target.Context)
					if target.Kind == "service" {
						a.cluster.kind = kindServices
					} else {
						a.cluster.kind = kindPods
					}
				})
			}
			aiTag(c, label, bg, fg)
			if f.ended {
				iconButton(c, "x", locales.KubeForwardDismiss()).OnClick(func() { a.dismissForward(f) })
			} else {
				if f.pod.Name != "" && !f.stopping {
					iconButton(c, "copy", locales.KubeForwardCopy()).OnClick(func() { c.WriteClipboard(target.Address()) })
				}
				ui.Button(c.Key("forward-stop"), locales.KubeForwardStop()).Disabled(f.stopping).OnClick(func() {
					f.stopping = true
					f.stop()
				})
			}
		})
		if f.ended && f.err != nil {
			msg := strings.TrimSpace(f.err.Error())
			if errors.Is(f.err, ops.ErrForwardPortBusy) {
				msg = locales.KubeForwardBusy(strconv.Itoa(target.Local))
			} else if errors.Is(f.err, ops.ErrForwardPodEnded) {
				msg = locales.KubeForwardPodEnded()
			} else if errors.Is(f.err, ops.ErrForwardTimeout) {
				msg = locales.KubeForwardTimeout()
			}
			ui.Text(c, msg).FontSize(11.5).TextColor(pal.WarningText).Wrap().Selectable()
		}
	})
}
