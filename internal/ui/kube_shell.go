package ui

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

type shellForm struct {
	uid, container  string
	open, launching bool
	err             error
}

func (a *App) shellButton(c *ui.Context, pod kube.Pod, preferred string) {
	if a.OpenKubeShell == nil || len(pod.Containers) == 0 {
		return
	}
	button := kubeActionButton(c.Key("pod-shell"), "terminal", locales.KubeShellAction(), podActionHeight)
	button.OnClick(func() {
		if preferred == "" {
			preferred = pod.Containers[0].Name
		}
		a.cluster.shell = &shellForm{uid: pod.UID, container: preferred, open: true}
	})
	form := a.cluster.shell
	if form == nil || form.uid != pod.UID {
		return
	}
	ui.PopoverBase(c.Key("pod-shell-popup"), button, &form.open, func(panel ui.Element) {
		t := c.Theme()
		panel.AttachTo(button, ui.AnchorBottom, ui.AnchorTop).
			Width(460).Padding(16, 20).Gap(10).Radius(12).Background(t.Background).
			BorderWidth(1).BorderColor(Colors(t).Divider).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.17))
		a.shellPopover(c, pod, form)
	})
}

func (a *App) shellPopover(c *ui.Context, pod kube.Pod, form *shellForm) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, locales.KubeShellTitle()).FontSize(16).FontWeight(600)
		ui.Text(c, locales.KubeShellDescription()).FontSize(12).TextColor(t.TextMuted).Wrap()
	})
	names := make([]string, 0, len(pod.Containers))
	for _, ct := range pod.Containers {
		names = append(names, ct.Name)
	}
	index := max(0, slices.Index(names, form.container))
	container := pod.Containers[index]
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		label := locales.KubeShellContainer()
		ui.Select(c.Key("shell-container"), &form.container, names).Label(label).
			Height(28).Width(150).FontSize(12).Disabled(form.launching)
		clipped(c, container.Image).Grow(1).MinWidth(0).FontSize(12).TextColor(t.TextMuted)
	})
	in, ok := cliCopy(a.cliState(), "kubectl")
	target := kube.Shell{Kubectl: in.Path, Context: a.router.Query("context"), Namespace: pod.Namespace, Pod: pod.Name, Container: container.Name}
	command := run.TerminalCommand(target.Args())
	detailSection(c, locales.KubeShellCommand())
	ui.Column(c).Padding(8, 12).Radius(8).Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Text(c, command).Font(FontMono).FontSize(11.5).Wrap().Selectable()
	})
	detailSection(c, locales.KubeShellNotes())
	if container.State == "waiting" && container.Reason == "CrashLoopBackOff" {
		ui.Text(c, locales.KubeShellRestarting(container.Name, container.Restarts)).FontSize(12).TextColor(pal.WarningText).Wrap()
	}
	ui.Text(c, locales.KubeShellMissingSh()).FontSize(12).TextColor(t.TextMuted).Wrap()
	if form.err != nil {
		warningRow(c, locales.KubeShellLaunchFailed(form.err.Error()))
	}
	ui.Row(c).Gap(8).Justify(ui.End).Wrap().Children(func() {
		ui.Button(c.Key("shell-copy"), locales.KubeShellCopy()).Height(podActionHeight).Disabled(!ok).
			OnClick(func() { c.WriteClipboard(command) })
		launch := locales.KubeShellLaunch()
		if slices.Contains(a.Terminals, a.Terminal) {
			launch = locales.KubeShellLaunchIn(a.Terminal)
		}
		ui.PrimaryButton(c.Key("shell-launch"), launch).Height(podActionHeight).Disabled(!ok || form.launching).
			OnClick(func() {
				form.launching, form.err = true, nil
				a.OpenKubeShell(target, func(err error) {
					form.launching, form.err = false, err
					if err == nil {
						form.open = false
					}
				})
			})
	})
}
