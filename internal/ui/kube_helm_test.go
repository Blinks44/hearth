package ui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/kube"
)

func TestKubeHelm(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	a.WatchCluster = w.watch
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	tt.Key(ui.Cmd, ui.KeyJ)
	tt.Type("helm")
	tt.Key(0, ui.KeyEnter)
	running := w.running()
	if len(running) != 1 || running[0].Helm != "/opt/homebrew/bin/helm" ||
		running[0].Context != "kind-dev" || !slices.Equal(running[0].Namespaces, []string{"default"}) {
		t.Fatalf("watches = %+v", running)
	}
	w.got[1](kube.Cluster{}, &kube.Error{Access: detect.KubeAccess{State: detect.KubeForbidden, Detail: "secrets is forbidden"}})
	tt.Frame()
	hasTexts(t, tt, "Could not read Helm releases", "secrets is forbidden")
	w.got[1](kube.Cluster{Taken: time.Now(), HelmReleases: []kube.HelmRelease{}}, nil)
	tt.Frame()
	hasTexts(t, tt, "No Helm releases in the selected namespaces.")

	w.got[1](kube.Cluster{Taken: time.Now(), HelmReleases: helmTestReleases()}, nil)
	tt.Frame()
	hasTexts(t, tt, "helm-z", "shop", "web-1.0.3", "2.4", "12", "The deployment timed out", "pending-upgrade", "deployed")
	screenshot(t, tt, "kube-helm")
	if err := tt.Click("In progress"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("helm-z") || !tt.HasText("helm-pending") {
		t.Fatalf("pending filter shows %v", tt.Texts())
	}
	a.cluster.query = "other-namespace"
	tt.Frame()
	hasTexts(t, tt, "No releases match the filters.")
	if err := tt.Click("Pods"); err != nil {
		t.Fatal(err)
	}
	if running := w.running(); len(running) != 1 || running[0].Helm != "" || !w.stopped[1] {
		t.Fatalf("watches = %+v", running)
	}
	w.got[1](kube.Cluster{Taken: time.Now(), HelmReleases: helmTestReleases()}, nil)
	tt.Frame()
	if tt.HasText("helm-z") {
		t.Fatal("published data from a stopped Helm watch")
	}
}

func TestKubeHelmMissing(t *testing.T) {
	a := kubeApp()
	cli := a.Snapshot.Tools[0].Cli
	cli.Tools = slices.DeleteFunc(cli.Tools, func(in inventory.Installation) bool { return in.Tool == "helm" })
	var w clusterWatches
	a.WatchCluster = w.watch
	a.cluster.kind = kindHelm
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Helm unavailable", "Hearth couldn’t find Helm")
	if len(w.targets) != 0 {
		t.Fatalf("watches = %+v", w.targets)
	}
}

func TestKubeHelmTable(t *testing.T) {
	for _, width := range []int{1000, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := kubeApp()
			var w clusterWatches
			a.WatchCluster = w.watch
			a.cluster.kind = kindHelm
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, width, 680)
			tt.Advance(time.Second)
			releases := helmTestReleases()
			for i := range 40 {
				releases = append(releases, kube.HelmRelease{Name: fmt.Sprintf("release-%02d", i), Namespace: "a-long-namespace-used-for-the-test", Chart: "long-chart-name-with-a-version-1.0.0", Status: "deployed"})
			}
			w.got[0](kube.Cluster{Taken: time.Now(), HelmReleases: releases}, nil)
			tt.Frame()
			find := func(label string) ui.Rect {
				t.Helper()
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing %q", label)
				}
				return r
			}
			for _, pair := range [][2]string{{"RELEASE", "helm-z"}, {"CHART", "web-1.0.3"}, {"APP VERSION", "2.4"}, {"REVISION", "12"}} {
				if head, value := find(pair[0]), find(pair[1]); head.X-value.X > 0.1 || value.X-head.X > 0.1 {
					t.Errorf("%s and %s start at %g and %g", pair[0], pair[1], head.X, value.X)
				}
			}
			head, start := find("RELEASE"), find("helm-z")
			if start.Y >= find("helm-a").Y {
				t.Fatal("failed releases should appear first")
			}
			tt.Scroll(start.X+10, start.Y+8, 0, 190)
			if header := find("RELEASE"); header.Y != head.Y {
				t.Errorf("table header scrolled from %g to %g", head.Y, header.Y)
			}
			if label, ok := tt.Find("helm-z"); ok && label.Y == start.Y {
				t.Error("Helm rows did not scroll")
			}
			screenshot(t, tt, fmt.Sprintf("kube-helm-table-%d", width))
		})
	}
}

func helmTestReleases() []kube.HelmRelease {
	return []kube.HelmRelease{
		{Name: "helm-a", Namespace: "shop", Revision: "2", Chart: "checkout-0.14.2", AppVersion: "2.8.1", Status: "deployed"},
		{Name: "helm-z", Namespace: "shop", Revision: "12", Chart: "web-1.0.3", AppVersion: "2.4", Status: "failed", Description: "The deployment timed out"},
		{Name: "helm-pending", Namespace: "tools", Revision: "4", Chart: "bitnami-redis-20.2.1", AppVersion: "7.4.1", Status: "pending-upgrade"},
	}
}

func TestKubeHelmDetails(t *testing.T) {
	for _, width := range []int{1000, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := kubeApp()
			var w clusterWatches
			a.WatchCluster = w.watch
			a.cluster.kind = kindHelm
			a.openCluster("kind-dev")
			history := []kube.HelmRevision{
				{Revision: 12, Chart: "web-1.0.3", Status: "failed", Description: "The deployment timed out"},
				{Revision: 11, Chart: "web-1.0.2", Status: "deployed"},
			}
			var requests, stopped int
			var values func(string, error)
			a.ReadHelmHistory = func(target kube.HelmTarget, done func([]kube.HelmRevision, error)) func() {
				if target.Namespace != "shop" || target.Name != "helm-z" || target.Context != "kind-dev" {
					t.Fatalf("history target = %+v", target)
				}
				done(history, nil)
				return func() { stopped++ }
			}
			a.ReadHelmContent = func(_ kube.HelmTarget, revision int, manifest bool, done func(string, error)) func() {
				requests++
				if revision != 12 {
					t.Fatalf("content revision = %d", revision)
				}
				if manifest {
					done("kind: ConfigMap", nil)
				} else {
					values = done
				}
				return func() { stopped++ }
			}
			tt := ui.NewTester(a.View, width, 680)
			tt.Advance(time.Second)
			w.got[0](kube.Cluster{Taken: time.Now(), HelmReleases: helmTestReleases()}, nil)
			tt.Frame()
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			click("shop/helm-z")
			hasTexts(t, tt, "Roll back to 11…", "Uninstall release…", "The deployment timed out", "#11")
			if requests != 0 {
				t.Fatal("content requested on overview")
			}
			screenshot(t, tt, fmt.Sprintf("helm-details-%d", width))
			click("Values")
			if values == nil {
				t.Fatal("values were not requested")
			}
			click("Manifest")
			values("late: response", nil)
			tt.Frame()
			hasTexts(t, tt, "kind: ConfigMap")
			if tt.HasText("late: response") || requests != 2 || stopped != 1 {
				t.Fatalf("tab change: %v, %d requests, %d stopped", tt.Texts(), requests, stopped)
			}
			click("Pods")
			if a.cluster.helmDetail != nil || stopped != 3 {
				t.Fatal("leaving Helm did not clear its selected release")
			}
		})
	}
}
