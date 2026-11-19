package ui

import (
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeRestart(t *testing.T) {
	a, tt, w := openShop(t)
	builds, writes := 0, 0
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		builds++
		done(ops.Plan{Title: locales.KubeRestartTitle("checkout-api"), Action: locales.KubeRestartAction()}, nil)
	}
	a.RunPlan = func(_ ops.Plan, update func(ops.State)) func() {
		writes++
		update(ops.State{Phase: ops.Succeeded, Verified: locales.KubeRestartVerified()})
		return func() {}
	}
	a.cluster.kind, a.cluster.deployment = kindDeployments, "d1"
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "kube-restart")
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	screenshot(t, tt, "kube-restart-minimum")
	restart, found := tt.Find("Restart…")
	edit, editFound := tt.Find("Edit YAML")
	if !found || !editFound || restart.H != edit.H || restart.Y != edit.Y || restart.X+restart.W >= edit.X {
		t.Fatalf("restart and edit actions do not align: %+v, %+v", restart, edit)
	}
	cl := shopCluster()
	for i := range cl.Deployments {
		if cl.Deployments[i].UID == "d1" {
			cl.Deployments[i].Paused = true
		}
	}
	w.got[len(w.got)-1](cl, nil)
	tt.Advance(time.Second)
	_ = tt.Click("Restart…")
	if builds != 0 {
		t.Fatal("a paused Deployment opened a restart plan")
	}
	w.got[len(w.got)-1](shopCluster(), nil)
	tt.Advance(time.Second)
	if err := tt.Click("Restart…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if builds != 1 || writes != 0 {
		t.Fatalf("before confirmation: %d plans, %d writes", builds, writes)
	}
	hasTexts(t, tt, "Restart checkout-api?", "Cancel", "Restart")
	if err := tt.Click("Restart"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if writes != 1 {
		t.Fatalf("after confirmation: %d writes", writes)
	}
}
