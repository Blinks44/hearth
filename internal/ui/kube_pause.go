package ui

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) deploymentPause(c *ui.Context, ref ops.KubeResource, paused bool) {
	t := c.Theme()
	pal := Colors(t)
	if a.BuildPlan == nil || ref.UID == "" || ref.Kubectl == "" {
		if paused {
			ui.Text(c, locales.KubePausedNote()).FontSize(12).TextColor(pal.WarningText).Padding(0, 16, 6, 16).Wrap()
		}
		return
	}
	note, color := locales.KubePauseHint(), t.TextMuted
	if paused {
		note, color = locales.KubePausedNote(), pal.WarningText
	}
	f := a.cluster.resources[ref]
	label := locales.KubePauseAction()
	detailSection(c, locales.KubeResourceRollout())
	ui.Row(c.Key("deployment-rollout")).Gap(10).AlignItems(ui.Center).Margin(0, 16, 6, 16).Padding(10, 12).
		Radius(10).BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(13).FontWeight(500)
			ui.Text(c, note).FontSize(12).TextColor(color).Wrap()
		})
		on := paused
		toggle(c, "deployment-pause", &on).Size(32, 18).Shrink(0).Label(label).
			Disabled((f != nil && f.busy) || a.op.running()).
			OnClick(func() { a.planDeploymentPause(a.resourceForm(ref), !paused) })
	})
}

func (a *App) planDeploymentPause(f *kubeResourceForm, pause bool) {
	if f.busy || a.op.running() {
		return
	}
	f.busy = true
	ref := f.ref
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanKubePause(context.Background(), run.Exec{}, env, ref, pause)
	}, func(p ops.Plan, err error) {
		f.busy = false
		s := &a.cluster
		if !a.resourceFormCurrent(f) || s.kind != kindDeployments || s.deployment != ref.UID || s.tab != 0 {
			return
		}
		if err != nil {
			s.refresh++
		}
		a.proposeThen(func() {
			if a.resourceFormCurrent(f) {
				s.refresh++
			}
		})(p, err)
	})
}
