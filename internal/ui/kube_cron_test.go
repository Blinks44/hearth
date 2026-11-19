package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeCronActions(t *testing.T) {
	a, tt, w := openShop(t)
	a.cluster.kind = kindCronJobs
	job := kube.CronJob{Namespace: "work", Name: "a-very-long-cronjob-name-for-the-minimum-window", UID: "cron", Schedule: "0 3 * * *"}
	action := locales.KubeCronPauseAction()
	title := locales.KubeCronPauseTitle(job.Name)
	writes := 0
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(ops.Plan{Title: title, Action: action}, nil)
	}
	a.RunPlan = func(_ ops.Plan, update func(ops.State)) func() {
		writes++
		update(ops.State{Phase: ops.Succeeded})
		return func() {}
	}
	tt.Advance(time.Second)
	show := func() {
		t.Helper()
		cl := shopCluster()
		cl.Batch = &kube.Batch{CronJobs: []kube.CronJob{job}}
		w.got[len(w.got)-1](cl, nil)
		tt.Advance(time.Second)
	}
	show()
	switchState := func(want bool) ui.Rect {
		t.Helper()
		e, ok := tt.AccessByRole(ui.RoleSwitch, locales.KubeCronEnabled(job.Name))
		if !ok || e.Checked != want {
			t.Fatalf("switch = %+v, want checked %t", e, want)
		}
		return e.Bounds
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "kube-cron-actions")
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	b := switchState(true)
	run, ok := tt.AccessByRole(ui.RoleButton, locales.KubeCronRunAction())
	menu, menuOK := tt.AccessByRole(ui.RoleMenuButton, locales.MoreActions())
	if !ok || !menuOK || b.W != 32 || b.H != 18 || b.X+b.W > run.Bounds.X ||
		run.Bounds.W != 24 || run.Bounds.X+run.Bounds.W > menu.Bounds.X || menu.Bounds.X+menu.Bounds.W > 1000 {
		t.Fatalf("CronJob controls overlap or clip at minimum window size: switch=%+v, run=%+v, menu=%+v", b, run, menu)
	}
	screenshot(t, tt, "kube-cron-actions-minimum")
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	tt.Advance(time.Second)
	hasTexts(t, tt, title, "Cancel")
	click("Cancel")
	switchState(true)
	if writes != 0 {
		t.Fatal("cancelling the plan wrote to the cluster")
	}
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	tt.Advance(time.Second)
	click(action)
	click(locales.OpDone())
	switchState(true)
	job.Suspended = true
	show()
	switchState(false)

	action, title = locales.KubeCronRunAction(), locales.KubeCronRunTitle(job.Name)
	click(action)
	hasTexts(t, tt, title, "Cancel")
	click(action)
	click(locales.OpDone())
	switchState(false)
	if writes != 2 {
		t.Fatalf("got %d operations, want pause and manual run", writes)
	}
}
