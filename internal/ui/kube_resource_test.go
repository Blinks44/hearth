package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestKubeResourceDraft(t *testing.T) {
	a, tt, _ := openShop(t)
	a.cluster.kind, a.cluster.deployment = kindDeployments, "d1"
	ref := a.resourceRef("Deployment", "default", "checkout-api", "d1")
	f := a.resourceForm(ref)
	// Supply the manifest that the background reader decoded.
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(ops.Plan{}, nil)
	}
	a.openResourceEditor(ref, 3)
	f.draft.Version = "48213"
	f.editor.Buffer.Reset("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: checkout-api\nspec:\n  replicas: 3\n")
	f.editor.DiffBase.Reset(f.editor.Buffer.String())
	f.editor.Buffer.Set("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: checkout-api\nspec:\n  replicas: 4\n")
	a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(ops.Plan{}, errors.New("resource changed on the server"))
	}
	tt.SetSize(1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Draft · unsaved changes", "Check…")
	screenshot(t, tt, "kube-resource-yaml")
	if err := tt.Click("Check…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "resource changed on the server", "Draft · unsaved changes")
	if f.editor.ReadOnly || !f.editor.Buffer.Modified() {
		t.Fatal("the failed check did not return the draft to the editor")
	}
	tt.SetSize(1000, 680)
	tt.Advance(time.Second)
	if r, ok := tt.Find("Check…"); !ok || r.X+r.W > 1000 || r.Y+r.H > 680 {
		t.Fatalf("check button outside the minimum window: %+v, found %t", r, ok)
	}
	screenshot(t, tt, "kube-resource-yaml-minimum")
}
