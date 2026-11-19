package ui

import (
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeScale(t *testing.T) {
	a, tt, _ := openShop(t)
	builds, runs := 0, 0
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		builds++
		done(ops.Plan{Title: locales.KubeScaleTitle("checkout-api"), Action: locales.KubeResourceApply()}, nil)
	}
	a.RunPlan = func(_ ops.Plan, update func(ops.State)) func() {
		runs++
		update(ops.State{Phase: ops.Succeeded, Verified: locales.KubeScaleVerified(4)})
		return func() {}
	}
	a.cluster.kind, a.cluster.deployment = kindDeployments, "d1"
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Now 3", "Apply…")
	_ = tt.Click("Apply…")
	if builds != 0 {
		t.Fatal("an unchanged replica count opened a plan")
	}

	button, ok := tt.Find("Increase replicas")
	if !ok {
		t.Fatal("increase button not found")
	}
	if err := tt.Click("Increase replicas"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "was 3", "Apply…")
	if area, ok := tt.Find("Increase replicas"); !ok || button != area {
		t.Fatalf("increase button moved from %+v to %+v when the draft appeared", button, area)
	}
	screenshot(t, tt, "kube-scale")
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	if area, ok := tt.Find("Apply…"); !ok || area.X+area.W > 1000 || area.Y+area.H > 680 {
		t.Fatalf("Apply button outside the minimum window: %+v, found %t", area, ok)
	}
	screenshot(t, tt, "kube-scale-minimum")

	if err := tt.Click("Apply…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if builds != 1 || runs != 0 {
		t.Fatalf("before confirmation: %d plans, %d writes", builds, runs)
	}
	if a.cluster.scale.want != 4 {
		t.Fatalf("target is %d, want 4", a.cluster.scale.want)
	}
	hasTexts(t, tt, "Scale checkout-api?", "Cancel", "Apply")
	if err := tt.Click("Apply"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if runs != 1 || a.cluster.scale != nil {
		t.Fatalf("after confirmation: %d writes, draft %+v", runs, a.cluster.scale)
	}
}
