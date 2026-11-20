package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestProjectManifestPreview(t *testing.T) {
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			if size[0] == 1000 {
				i18n.SetLocaleFunc(func() string { return "ru" })
				t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			}
			a := kubeProjectApp()
			p := a.Projects[0]
			a.PickManifest = func(string, bool, func(string, error)) {}
			source := ops.KubeManifestSource{Path: filepath.Join(p.Dir, "k8s"), Context: "kind-dev", Namespace: "shop"}
			preview := &ops.KubeManifestPreview{
				KubeManifestSource: source,
				Kustomize:          true,
				Created:            []string{"Service shop/checkout-metrics", "ConfigMap shop/checkout-flags"},
				Unchanged:          []string{"Service shop/checkout"},
				Changed: []ops.KubeManifestDiff{{Resource: "Deployment shop/checkout", Lines: []ops.DiffLine{
					{Line: 21, Text: "    image: checkout:2.8.1"},
					{Line: 21, Text: "    image: checkout:2.9.0", Added: true},
				}}},
			}
			builds, writes := 0, 0
			var finish func(ops.Plan, error)
			a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
				builds++
				finish = done
			}
			a.RunPlan = func(ops.Plan, func(ops.State)) func() {
				writes++
				return func() {}
			}
			a.router.Push(projectPath(p.Dir))
			tt := ui.NewTester(a.View, size[0], size[1])
			tt.Advance(time.Second)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Advance(time.Second)
			}
			click(locales.KubeApplyOpen())
			if a.kubeApply.source != preview.KubeManifestSource {
				t.Fatalf("wrong project target: %+v", a.kubeApply.source)
			}
			click(locales.KubeResourceCheck())
			if builds != 1 || finish == nil || writes != 0 {
				t.Fatalf("builds=%d writes=%d", builds, writes)
			}
			finish(ops.Plan{
				Title: locales.KubeApplyTitle("shop"), Why: locales.KubeApplyReviewed("kind-dev", "shop"),
				Action: locales.KubeApplyConfirm(3), Steps: []ops.Step{{Kube: &ops.KubeChange{Manifests: preview}}},
			}, nil)
			tt.Frame()
			tt.Advance(time.Second)
			hasTexts(t, tt, strings.ToUpper(locales.KubeApplySummary(1, 2, 1)), "Deployment shop/checkout",
				"Service shop/checkout-metrics", locales.KubeApplyConfirm(3))
			button, found := tt.Find(locales.KubeApplyConfirm(3))
			if !found || button.Y < 0 || button.Y+button.H > float32(size[1]) {
				t.Fatalf("confirmation offscreen: %+v", button)
			}
			screenshot(t, tt, fmt.Sprintf("kube-apply-%d", size[0]))
			click(locales.Cancel())
			if writes != 0 || a.op.open {
				t.Fatalf("cancel left operation open or wrote to cluster: %d", writes)
			}
		})
	}
}

func TestClusterManifestSource(t *testing.T) {
	a := kubeApp()
	var watches clusterWatches
	a.WatchCluster = watches.watch
	a.PickManifest = func(string, bool, func(string, error)) {}
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)

	if err := tt.Click(locales.KubeApplyOpen()); err != nil {
		t.Fatal(err)
	}
	d := a.kubeApply
	if d == nil || !d.open || d.source.Context != "kind-dev" || d.source.Namespace != "default" {
		t.Fatalf("manifest dialog = %+v", d)
	}
}
