package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeSidebarResumesVerifiedCluster(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	a.WatchCluster = w.watch
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 820)
	if got := a.entryPath(kubePage); got != kubePage {
		t.Fatalf("no previous cluster: %s", got)
	}
	a.openCluster("kind-dev")
	tt.Advance(time.Second)
	if got := a.entryPath(kubePage); got != kubePage {
		t.Fatalf("unverified cluster: %s", got)
	}
	w.got[0](shopCluster(), nil)
	a.cluster.kind = kindDeployments
	tt.Advance(time.Second)
	w.got[1](shopCluster(), nil)
	if err := tt.Click(locales.BackToKubernetes()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != kubePage || len(w.running()) != 0 {
		t.Fatalf("back did not open the environment: %s, watches %+v", a.router.Path(), w.running())
	}
	a.router.Push(overview.path)
	tt.Advance(time.Second)
	if err := tt.Click("Kubernetes"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != kubeClusterPage || a.router.Query("context") != "kind-dev" || a.cluster.kind != kindDeployments {
		t.Fatalf("sidebar opened %s / %s, kind %d", a.router.Path(), a.router.Query("context"), a.cluster.kind)
	}
	w.got[len(w.got)-1](kube.Cluster{}, &kube.Error{Access: detect.KubeAccess{State: detect.KubeNeedsLogin}})
	tt.Advance(time.Second)
	if got := a.entryPath(kubePage); got != kubePage {
		t.Fatalf("opened a cluster needing sign-in: %s", got)
	}
	a.CheckKube = func(_ string, _ inventory.KubeContext, done func(detect.KubeAccess)) {
		done(detect.KubeAccess{State: detect.KubeReachable})
	}
	a.checkKube("/opt/homebrew/bin/kubectl", *a.kubeContext("kind-dev"))
	if got := a.entryPath(kubePage); got != kubeClusterPath("kind-dev") {
		t.Fatalf("successful recheck did not restore access: %s", got)
	}
	a.openCluster("production")
	tt.Advance(time.Second)
	if got := a.entryPath(kubePage); got != kubePage {
		t.Fatalf("opened a previous cluster after visiting another one: %s", got)
	}
}

func TestKubeSidebarRechecksChangedContext(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	a.WatchCluster = w.watch
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1280, 820)
	a.cluster.kind = kindJobs
	tt.Advance(time.Second)
	w.got[len(w.got)-1](kube.Cluster{Batch: &kube.Batch{}}, nil)
	if a.kubeEntryPath() != kubeClusterPath("kind-dev") {
		t.Fatal("a successful Jobs read did not verify access")
	}
	old := len(w.got) - 1
	a.kubeContext("kind-dev").File = "new-kubeconfig"
	tt.Advance(time.Second)
	if len(w.got) != old+2 || !w.stopped[old] {
		t.Fatal("read survived a change of kubeconfig")
	}
	w.got[old](kube.Cluster{Batch: &kube.Batch{}}, nil)
	if a.kubeEntryPath() != kubePage {
		t.Fatal("obsolete read verified the new context")
	}
	denied := &kube.Error{Access: detect.KubeAccess{State: detect.KubeNeedsLogin}}
	w.got[old+1](kube.Cluster{Batch: &kube.Batch{JobsErr: denied}}, nil)
	if a.kubeEntryPath() != kubePage {
		t.Fatal("failed Jobs read verified access")
	}
	w.got[old+1](kube.Cluster{Batch: &kube.Batch{}}, nil)
	if a.kubeEntryPath() != kubeClusterPath("kind-dev") {
		t.Fatal("successful read did not verify the new kubeconfig")
	}
}

func TestKubePodRowActions(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	var log kube.Logs
	a.WatchCluster = w.watch
	a.StreamLogs = func(target kube.Logs, _ func([]kube.LogLine), _ func(error)) func() {
		log = target
		return func() {}
	}
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1280, 820)
	cl := shopCluster()
	w.got[0](cl, nil)
	expandGroups(a, cl)
	pod := cl.Pods[1]
	logs := locales.KubeLogsOpen() + " · " + pod.Name
	menu := locales.MoreActions() + " · " + pod.Name
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		l, lOK := tt.Find(logs)
		m, mOK := tt.Find(menu)
		if !lOK || !mOK || l.Y != m.Y || l.X+l.W > m.X || m.X+m.W > float32(size[0]) {
			t.Fatalf("at %dx%d logs=%+v, menu=%+v", size[0], size[1], l, m)
		}
		screenshot(t, tt, "kube-pods-"+map[int]string{1280: "wide", 1000: "narrow"}[size[0]])
	}
	if err := tt.Click(menu); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.KubeCopyName()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(logs); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if log.Pod != pod.Name || log.Container != "api" {
		t.Fatalf("log target: %+v", log)
	}
}
