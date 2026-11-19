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
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	checkout := kube.Owner{Kind: "Deployment", Name: "checkout-api"}
	running := func(name, image string) kube.Container {
		return kube.Container{Name: name, Image: image, Ready: true, State: "running"}
	}
	api := running("api", "ghcr.io/acme/checkout:2.8.1")
	return kube.Cluster{Taken: now, Pods: []kube.Pod{
		{Namespace: "default", Name: "checkout-api-7d9f8c5b-m8q2z", UID: "p2", Phase: "Running", Ready: 1, Containers: []kube.Container{api},
			Node: "kind-worker2", Created: ago(2 * time.Hour), Owner: checkout},
		{Namespace: "default", Name: "checkout-api-7d9f8c5b-x2k4p", UID: "p1", Phase: "Running", Reason: "CrashLoopBackOff", Ready: 1,
			Containers: []kube.Container{
				{Name: "api", Image: "ghcr.io/acme/checkout:2.8.1", Restarts: 14, State: "waiting", Reason: "CrashLoopBackOff",
					Message: "back-off 5m0s restarting failed container=api pod=checkout-api-7d9f8c5b-x2k4p_default(p1)",
					Last:    &kube.Run{Reason: "Error", ExitCode: 1, Started: ago(4 * time.Minute), Finished: ago(4*time.Minute - 3*time.Second)}},
				running("envoy", "envoyproxy/envoy:v1.31.2"),
			},
			Restarts: 14, Node: "kind-worker", IP: "10.244.1.17", QoS: "Burstable", Account: "checkout",
			Labels: map[string]string{"app": "checkout-api", "pod-template-hash": "7d9f8c5b"}, Volumes: []string{"config", "tmp"},
			Created: ago(2 * time.Hour), Owner: checkout},
		{Namespace: "default", Name: "checkout-api-7d9f8c5b-p9w4d", UID: "p3", Phase: "Running", Ready: 1, Containers: []kube.Container{api},
			Node: "kind-worker", Created: ago(2 * time.Hour), Owner: checkout},
		{Namespace: "default", Name: "web-frontend-5c8d7f9b4-k2j9x", UID: "p4", Phase: "Pending", Reason: "ImagePullBackOff",
			Containers: []kube.Container{{Name: "web", Image: "ghcr.io/acme/web:1.4.2", State: "waiting", Reason: "ImagePullBackOff",
				Message: `Back-off pulling image "ghcr.io/acme/web:1.4.2": not found: manifest unknown`}},
			Node: "kind-worker2", Created: ago(6 * time.Minute), Owner: kube.Owner{Kind: "Deployment", Name: "web-frontend"}},
		{Namespace: "default", Name: "worker-6f7d8-aaaaa", UID: "p5", Phase: "Running", Ready: 1, Containers: []kube.Container{running("worker", "worker:1")},
			Node: "kind-worker", Created: ago(3 * time.Hour), Owner: kube.Owner{Kind: "Deployment", Name: "worker"}},
		{Namespace: "default", Name: "worker-6f7d8-bbbbb", UID: "p6", Phase: "Running", Ready: 1, Restarts: 2,
			Containers: []kube.Container{{Name: "worker", Image: "worker:1", Ready: true, State: "running", Restarts: 2}},
			Node:       "kind-worker2", Created: ago(3 * time.Hour), Owner: kube.Owner{Kind: "Deployment", Name: "worker"}},
		{Namespace: "default", Name: "db-migrate-29381-abcde", UID: "p7", Phase: "Succeeded",
			Containers: []kube.Container{{Name: "migrate", Image: "migrate:3", State: "terminated", Reason: "Completed"}},
			Created:    ago(40 * time.Minute), Owner: kube.Owner{Kind: "Job", Name: "db-migrate-29381"}},
		{Namespace: "default", Name: "debug", UID: "p8", Phase: "Running", Ready: 1, Containers: []kube.Container{running("debug", "busybox")},
			Node: "kind-worker", Created: ago(10 * time.Minute)},
	}, Deployments: []kube.Deployment{
		{Namespace: "default", Name: "checkout-api", UID: "d1", Replicas: 3, Pods: 3, Ready: 2, Updated: 3, Available: 2, Strategy: "RollingUpdate",
			Images: []string{"ghcr.io/acme/checkout:2.8.1"}, Revision: 7, Created: ago(12 * 24 * time.Hour), Revisions: []kube.Revision{
				{Number: 7, Name: "checkout-api-7d9f8c5b", UID: "r1", Images: []string{"ghcr.io/acme/checkout:2.8.1"}, Pods: 3, Ready: 2, Created: ago(2 * time.Hour)},
				{Number: 6, Name: "checkout-api-6b4f7d9c8", UID: "r0", Images: []string{"ghcr.io/acme/checkout:2.8.0"}, Created: ago(24 * time.Hour)},
			}},
		{Namespace: "default", Name: "web-frontend", UID: "d2", Replicas: 1, Pods: 1, Updated: 1, Created: ago(6 * time.Minute)},
		{Namespace: "default", Name: "worker", UID: "d3", Replicas: 2, Pods: 2, Ready: 2, Updated: 2, Available: 2, Created: ago(3 * time.Hour)},
		{Namespace: "default", Name: "search-indexer", UID: "d4", Paused: true, Created: ago(30 * 24 * time.Hour)},
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
	hasTexts(t, tt, "checkout-api-7d9f8c5b-x2k4p", "CrashLoopBackOff", "1/2", "14", "CPU", "MEMORY", "Running")
	screenshot(t, tt, "kube-cluster")
	// The pod in trouble comes first.
	texts := tt.Texts()
	if slices.Index(texts, "checkout-api-7d9f8c5b-x2k4p") > slices.Index(texts, "checkout-api-7d9f8c5b-m8q2z") {
		t.Error("the pod in trouble is not first")
	}
	a.cluster.kind = kindDeployments
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "checkout-api", "2 of 3 ready", "2/3", "12d")
	if got := w.running(); len(got) != 1 || got[0].Metrics || len(w.targets) != 2 || !w.stopped[0] {
		t.Fatalf("expected workload watch without metrics after leaving Pods: %+v", got)
	}

	// A later read fails: the data stays, saying it is old.
	w.got[1](kube.Cluster{}, &kube.Error{Access: detect.KubeAccess{State: detect.KubeUnreachable, Detail: "dial tcp: connection refused"}})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "not answering", "2/3")

	// Building the view again starts nothing more.
	tt.Frame()
	tt.Advance(time.Second)
	if len(w.targets) != 2 {
		t.Fatalf("watches = %d, want 2", len(w.targets))
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
	w.got[2](kube.Cluster{}, &kube.Error{Access: detect.KubeAccess{State: detect.KubeNeedsLogin, Detail: "error: You must be logged in to the server (Unauthorized)"}})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Sign-in needed", "error: You must be logged in to the server (Unauthorized)", "sign-in needed")
	screenshot(t, tt, "kube-cluster-login")
	if err := tt.Click("Check again"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(w.targets) != 4 || !w.stopped[2] || w.targets[3].Context != "production" {
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
