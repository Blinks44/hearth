package ui

import (
	"cmp"
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) deploymentRestart(c *ui.Context, ref ops.KubeResource, paused bool) {
	if ref.UID == "" || ref.Kubectl == "" {
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

func (a *App) restartStillOpen(ref ops.KubeResource) bool {
	s := &a.cluster
	switch ref.Kind {
	case "Deployment":
		return s.kind == kindDeployments && s.deployment == ref.UID
	case "StatefulSet":
		return s.kind == kindStatefulSets
	case "DaemonSet":
		return s.kind == kindDaemonSets
	}
	return false
}

func (a *App) planRestart(f *kubeResourceForm) {
	if f.busy || a.op.running() || f.ref.Kubectl == "" {
		return
	}
	f.busy = true
	ref := f.ref
	runner := cmp.Or[run.Runner](a.Runner, run.Exec{})
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanKubeRestart(context.Background(), runner, env, ref)
	}, func(p ops.Plan, err error) {
		f.busy = false
		s := &a.cluster
		if !a.resourceFormCurrent(f) || !a.restartStillOpen(ref) {
			return
		}
		a.proposeThen(func() {
			if a.resourceFormCurrent(f) {
				s.refresh++
			}
		})(p, err)
	})
}
