package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubePause(t *testing.T) {
	a, tt, w := openShop(t)
	builds, writes := 0, 0
	paused := false
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		builds++
		title, action := locales.KubePauseTitle("checkout-api"), locales.KubePauseAction()
		if paused {
			title, action = locales.KubeResumeTitle("checkout-api"), locales.KubeResumeAction()
		}
		done(ops.Plan{Title: title, Action: action}, nil)
	}
	a.RunPlan = func(_ ops.Plan, update func(ops.State)) func() {
		writes++
		update(ops.State{Phase: ops.Succeeded})
		return func() {}
	}
	a.cluster.kind, a.cluster.deployment = kindDeployments, "d1"
	buttonState := func(wantLabel string) ui.Rect {
		t.Helper()
		control, ok := tt.AccessByRole(ui.RoleButton, wantLabel)
		if !ok {
			t.Fatalf("button %q not found", wantLabel)
		}
		return control.Bounds
	}
	clickButton := func(wantLabel string) {
		t.Helper()
		b := buttonState(wantLabel)
		tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
		tt.Advance(time.Second)
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
	pauseBtn := locales.KubeRolloutPauseButton() + "…"
	resumeBtn := locales.KubeRolloutResumeButton() + "…"
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, locales.KubeRolloutActive(), locales.KubePauseHint())
	screenshot(t, tt, "kube-pause")
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	b := buttonState(pauseBtn)
	if b.W <= 0 || b.H != controlHeight || b.X+b.W > 1000 || b.Y+b.H > 680 {
		t.Fatalf("rollout button outside the card at minimum size: %+v", b)
	}
	screenshot(t, tt, "kube-pause-minimum")
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
	clickButton(pauseBtn)
	hasTexts(t, tt, "Pause the rollout for checkout-api?", "Cancel")
	click("Cancel")
	buttonState(pauseBtn)
	if builds != 1 || writes != 0 {
		t.Fatalf("after cancellation: %d plans, %d writes", builds, writes)
	}
	clickButton(pauseBtn)
	click(locales.KubePauseAction())
	click(locales.OpDone())
	buttonState(pauseBtn)
	paused = true
	cl := shopCluster()
	for i := range cl.Deployments {
		if cl.Deployments[i].UID == "d1" {
			cl.Deployments[i].Paused = true
			cl.Deployments[i].Updated = 1
		}
	}
	w.got[len(w.got)-1](cl, nil)
	tt.Advance(time.Second)
	buttonState(resumeBtn)
	hasTexts(t, tt, locales.KubeRolloutPaused(), locales.KubePausedNote())
	screenshot(t, tt, "kube-paused-minimum")
	clickButton(resumeBtn)
	hasTexts(t, tt, "Resume the rollout for checkout-api?", "Resume rollout")
	click(locales.KubeResumeAction())
	if builds != 3 || writes != 2 {
		t.Fatalf("after resuming: %d plans, %d writes", builds, writes)
	}
}
