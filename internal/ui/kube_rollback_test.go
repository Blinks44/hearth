package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeRollback(t *testing.T) {
	a, tt, w := openShop(t)
	builds, writes := 0, 0
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		builds++
		done(ops.Plan{Title: locales.KubeRollbackTitle("checkout-api", 6), Action: locales.KubeRollbackAction()}, nil)
	}
	a.RunPlan = func(_ ops.Plan, update func(ops.State)) func() {
		writes++
		update(ops.State{Phase: ops.Succeeded})
		return func() {}
	}
	a.cluster.kind, a.cluster.deployment = kindDeployments, "d1"
	label := locales.KubeRollbackRevision(6)
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
	if _, ok := tt.AccessByRole(ui.RoleButton, locales.KubeRollbackRevision(7)); ok {
		t.Fatal("the current revision offered rollback")
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
	screenshot(t, tt, "kube-rollback")
	click(label)
	hasTexts(t, tt, "Roll back checkout-api to revision 6?", "Cancel")
	click("Cancel")
	if builds != 1 || writes != 0 {
		t.Fatalf("after cancellation: %d plans, %d writes", builds, writes)
	}
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	tt.Scroll(810, 470, 0, 160)
	b, ok := tt.AccessByRole(ui.RoleButton, label)
	if !ok || b.Disabled || b.Bounds.H != 24 || b.Bounds.Y < 250 || b.Bounds.Y+b.Bounds.H > 680 || b.Bounds.X+b.Bounds.W > 1000 {
		t.Fatalf("rollback button outside the detail pane at minimum size: %+v", b)
	}
	screenshot(t, tt, "kube-rollback-minimum")
	click(label)
	click(locales.KubeRollbackAction())
	click(locales.OpDone())
	if builds != 2 || writes != 1 {
		t.Fatalf("after confirmation: %d plans, %d writes", builds, writes)
	}
	cl := shopCluster()
	cl.Deployments[0].Paused = true
	w.got[len(w.got)-1](cl, nil)
	tt.Advance(time.Second)
	if b, ok := tt.AccessByRole(ui.RoleButton, label); !ok || !b.Disabled {
		t.Fatalf("rollback enabled on a paused deployment: %+v", b)
	}
}
