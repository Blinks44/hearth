package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// clusterWatches records the cluster page's watches: what each reads,
// how to answer it, and whether it was stopped.
type clusterWatches struct {
	targets []kube.Target
	got     []func(kube.Cluster, error)
	stopped []bool
}

func (w *clusterWatches) watch(t kube.Target, got func(kube.Cluster, error)) func() {
	i := len(w.targets)
	w.targets = append(w.targets, t)
	w.got = append(w.got, got)
	w.stopped = append(w.stopped, false)
	return func() { w.stopped[i] = true }
}

// running are the watches not stopped.
func (w *clusterWatches) running() []kube.Target {
	var ts []kube.Target
	for i, t := range w.targets {
		if !w.stopped[i] {
			ts = append(ts, t)
		}
	}
	return ts
}

func shopCluster() kube.Cluster {
	now := time.Now()
	return kube.Cluster{Taken: now, Pods: []kube.Pod{
		{Namespace: "default", Name: "checkout-api-7d9f8c5b-m8q2z", UID: "p2", Phase: "Running", Ready: 1, Containers: 1, Node: "kind-worker2",
			Created: now.Add(-2 * time.Hour), Owner: kube.Owner{Kind: "Deployment", Name: "checkout-api"}},
		{Namespace: "default", Name: "checkout-api-7d9f8c5b-x2k4p", UID: "p1", Phase: "Running", Reason: "CrashLoopBackOff", Ready: 1, Containers: 2,
			Restarts: 14, Node: "kind-worker", Created: now.Add(-2 * time.Hour), Owner: kube.Owner{Kind: "Deployment", Name: "checkout-api"}},
	}, Deployments: []kube.Deployment{
		{Namespace: "default", Name: "checkout-api", UID: "d1", Replicas: 3, Ready: 2, Updated: 3, Created: now.Add(-12 * 24 * time.Hour)},
	}}
}

// The cluster opens from a context's row and is read only while its page
// shows; what it read lists, a failed read says why, and choosing another
// context reads it without touching kubectl's.
func TestKubeCluster(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	a.WatchCluster = w.watch
	a.ListNamespaces = func(kube.Target, func([]string, error)) {}
	plans := 0
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) { plans++ }
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if len(w.targets) != 0 {
		t.Fatal("the Kubernetes page read a cluster")
	}
	if err := tt.Click("Open"); err != nil { // kind-dev's
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	want := kube.Target{Kubectl: "/opt/homebrew/bin/kubectl", Context: "kind-dev", Namespaces: []string{"default"}}
	if got := w.running(); len(got) != 1 || got[0].Kubectl != want.Kubectl || got[0].Context != want.Context || !slices.Equal(got[0].Namespaces, want.Namespaces) {
		t.Fatalf("watches = %+v, want %+v", w.targets, want)
	}
	hasTexts(t, tt, "kind-dev", "default", "connecting", "Reading kind-dev…")

	w.got[0](shopCluster(), nil)
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "checkout-api-7d9f8c5b-x2k4p", "CrashLoopBackOff", "1/2", "14", "kind-worker", "2h", "Running")
	screenshot(t, tt, "kube-cluster")
	// The pod in trouble comes first.
	texts := tt.Texts()
	if slices.Index(texts, "checkout-api-7d9f8c5b-x2k4p") > slices.Index(texts, "checkout-api-7d9f8c5b-m8q2z") {
		t.Error("the pod in trouble is not first")
	}
	a.cluster.kind = kindDeployments
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "checkout-api", "1 not ready", "2/3", "12d")

	// A later read fails: the data stays, saying it is old.
	w.got[0](kube.Cluster{}, &kube.Error{Access: detect.KubeAccess{State: detect.KubeUnreachable, Detail: "dial tcp: connection refused"}})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "not answering", "2/3")

	// Building the view again starts nothing more.
	tt.Frame()
	tt.Advance(time.Second)
	if len(w.targets) != 1 {
		t.Fatalf("watches = %d, want 1", len(w.targets))
	}

	// Another context: the last watch stops, the new one reads its
	// namespace; kubectl's context is not changed.
	a.cluster.contextOpen = true
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Choosing here changes only what Hearth shows. kubectl in the terminal still uses staging.", "Make kind-dev current in kubectl…")
	screenshot(t, tt, "kube-cluster-context")
	if err := tt.Click("production"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := w.running(); len(got) != 1 || got[0].Context != "production" || !slices.Equal(got[0].Namespaces, []string{"prod"}) {
		t.Fatalf("watches = %+v", got)
	}
	if plans != 0 {
		t.Error("choosing a context planned to change kubectl's")
	}
	if tt.HasText("checkout-api") {
		t.Error("the last context's data shows")
	}

	// Nothing read: why, kubectl's words and a button to read again.
	w.got[1](kube.Cluster{}, &kube.Error{Access: detect.KubeAccess{State: detect.KubeNeedsLogin, Detail: "error: You must be logged in to the server (Unauthorized)"}})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Sign-in needed", "error: You must be logged in to the server (Unauthorized)", "sign-in needed")
	screenshot(t, tt, "kube-cluster-login")
	if err := tt.Click("Check again"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(w.targets) != 3 || !w.stopped[1] || w.targets[2].Context != "production" {
		t.Fatalf("read again: %+v %v", w.targets, w.stopped)
	}

	// Leaving the page stops reading.
	a.router.Push(kubePage)
	tt.Frame()
	tt.Advance(time.Second)
	if got := w.running(); len(got) != 0 {
		t.Fatalf("still reading after leaving: %+v", got)
	}

	// Closing the window stops it too.
	a.router.Back()
	tt.Frame()
	tt.Advance(time.Second)
	if got := w.running(); len(got) != 1 {
		t.Fatalf("back on the page: %+v", got)
	}
	a.StopCluster()
	if got := w.running(); len(got) != 0 {
		t.Fatalf("still reading after the window closed: %+v", got)
	}
}

// Namespaces chosen in the popover are read; all of them reads the whole
// cluster.
func TestKubeClusterNamespaces(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	a.WatchCluster = w.watch
	var listed []kube.Target
	var answer func([]string, error)
	a.ListNamespaces = func(t kube.Target, done func([]string, error)) { listed, answer = append(listed, t), done }
	a.router.Push(kubeClusterPath("kind-dev"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if err := tt.Click("default"); err != nil { // the namespace's select
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(listed) != 1 || listed[0].Context != "kind-dev" {
		t.Fatalf("listed = %+v", listed)
	}
	answer([]string{"default", "payments", "shop"}, nil)
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("shop"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := w.running(); len(got) != 1 || !slices.Equal(got[0].Namespaces, []string{"default", "shop"}) {
		t.Fatalf("watches = %+v", got)
	}
	if err := tt.Click("All namespaces"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := w.running(); len(got) != 1 || len(got[0].Namespaces) != 0 {
		t.Fatalf("watches = %+v", got)
	}

	// A cluster that lets no one list namespaces takes one typed.
	a.cluster.namespaceOpen = false
	tt.Frame()
	a.listNamespaces("kind-dev")
	answer(nil, &kube.Error{Access: detect.KubeAccess{State: detect.KubeForbidden}})
	a.cluster.namespaceOpen = true
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Could not list namespaces: no access. Type one instead.")
}
