package ui

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) deploymentRestart(c *ui.Context, ref ops.KubeResource, paused bool) {
	if ref.UID == "" {
		return
	}
	f := a.cluster.resources[ref]
	button := kubeActionButton(c.Key("deployment-restart"), "refresh-cw", locales.KubeRestartOpen(), podActionHeight).
		Disabled(paused || (f != nil && f.busy) || a.op.running()).
		OnClick(func() { a.planRestart(a.resourceForm(ref)) })
	if paused {
		button.Tooltip(locales.KubeRestartPaused())
	}
}

func (a *App) planRestart(f *kubeResourceForm) {
	f.busy = true
	ref := f.ref
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanKubeRestart(context.Background(), run.Exec{}, env, ref)
	}, func(p ops.Plan, err error) {
		f.busy = false
		s := &a.cluster
		if !a.resourceFormCurrent(f) || s.kind != kindDeployments || s.deployment != ref.UID {
			return
		}
		a.proposeThen(func() {
			if a.resourceFormCurrent(f) {
				s.refresh++
			}
		})(p, err)
	})
}
