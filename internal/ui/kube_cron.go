package ui

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const cronActionsWidth = 32 + 24 + 28 + 2*4

func (a *App) cronJobActions(c *ui.Context, job kube.CronJob, width float32) {
	ref := a.resourceRef("CronJob", job.Namespace, job.Name, job.UID)
	ui.Row(c).Width(width).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
		if a.BuildPlan == nil || ref.UID == "" || ref.Kubectl == "" {
			return
		}
		f := a.cluster.resources[ref]
		busy := (f != nil && f.busy) || a.op.running()
		enabled := !job.Suspended
		label := locales.KubeCronPauseAction()
		if job.Suspended {
			label = locales.KubeCronResumeAction()
		}
		toggle(c, "cron-enabled", &enabled).Size(32, 18).Shrink(0).
			Label(locales.KubeCronEnabled(job.Name)).Tooltip(label).Disabled(busy).
			OnClick(func() {
				a.planCronAction(ref, func(ctx context.Context, r run.Runner, env sysenv.Env, ref ops.KubeResource) (ops.Plan, error) {
					return ops.PlanKubeCronSuspend(ctx, r, env, ref, !job.Suspended)
				})
			})
		iconButton(c.Key("cron-run"), "play", locales.KubeCronRunAction()).Disabled(busy).
			OnClick(func() { a.planCronAction(ref, ops.PlanKubeCronRun) })
		a.batchResourceMenu(c, ref)
	})
}

func (a *App) planCronAction(ref ops.KubeResource, plan func(context.Context, run.Runner, sysenv.Env, ops.KubeResource) (ops.Plan, error)) {
	f := a.resourceForm(ref)
	if f.busy || a.op.running() {
		return
	}
	f.busy = true
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return plan(context.Background(), run.Exec{}, env, ref)
	}, func(p ops.Plan, err error) {
		f.busy = false
		s := &a.cluster
		if !a.resourceFormCurrent(f) || s.kind != kindCronJobs {
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
