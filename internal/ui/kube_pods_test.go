package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
)

// What is going on with a pod says only what Kubernetes reported, keeps
// what Hearth infers apart, and leaves a reason it does not know unknown
// with Kubernetes' own words.
func TestStoryOf(t *testing.T) {
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	created := now.Add(-41 * time.Minute)
	crash := func(code int, reason string) kube.Pod {
		return kube.Pod{Phase: "Running", Reason: "CrashLoopBackOff", Created: created, Containers: []kube.Container{{
			Name: "api", Restarts: 14, State: "waiting", Reason: "CrashLoopBackOff",
			Message: "back-off 5m0s restarting failed container=api pod=x_shop(p1)",
			Last:    &kube.Run{Reason: reason, ExitCode: code, Started: now.Add(-time.Minute), Finished: now.Add(-time.Minute + 3*time.Second)},
		}}}
	}
	pull := func(message string) kube.Pod {
		return kube.Pod{Phase: "Pending", Reason: "ImagePullBackOff", Containers: []kube.Container{{
			Name: "web", Image: "ghcr.io/acme/web:1.4.2", State: "waiting", Reason: "ImagePullBackOff", Message: message,
		}}}
	}
	tests := []struct {
		name string
		pod  kube.Pod
		// events are the pod's, newest first.
		events []kube.Event
		want   podStory
	}{
		{"crash", crash(1, "Error"), nil, podStory{title: "Container api keeps failing after it starts", known: true, facts: []string{
			"Its last run ended with exit code 1 (Error) 3s after it started", "14 restarts in 41m", "Kubernetes waits up to 5m before the next try",
		}}},
		{"crash, command not found", crash(127, "Error"), nil, podStory{title: "Container api keeps failing after it starts", known: true, facts: []string{
			"Its last run ended with exit code 127 (Error) 3s after it started", "14 restarts in 41m", "Kubernetes waits up to 5m before the next try",
		}, guesses: []string{"Exit code 127 usually means the container's command was not found or cannot run"}}},
		{"crash, out of memory", crash(137, "OOMKilled"), nil, podStory{title: "Container api keeps failing after it starts", known: true, facts: []string{
			"Its last run ended with exit code 137 (OOMKilled) 3s after it started", "Kubernetes stopped it for going over its memory limit",
			"14 restarts in 41m", "Kubernetes waits up to 5m before the next try",
		}}},
		{"image not found", pull(`Back-off pulling image "ghcr.io/acme/web:1.4.2": not found: manifest unknown`), nil, podStory{
			title: "Image ghcr.io/acme/web:1.4.2 does not pull", known: true, facts: []string{"Registry ghcr.io has no such image or tag"},
			detail: `Back-off pulling image "ghcr.io/acme/web:1.4.2": not found: manifest unknown`}},
		{"image, words unknown", pull(`Back-off pulling image "ghcr.io/acme/web:1.4.2"`), nil, podStory{
			title: "Image ghcr.io/acme/web:1.4.2 does not pull", detail: `Back-off pulling image "ghcr.io/acme/web:1.4.2"`}},
		{"unscheduled", kube.Pod{Phase: "Pending", Reason: "Unschedulable", Unscheduled: "0/3 nodes are available: pod has unbound immediate PersistentVolumeClaims."},
			nil, podStory{title: "The pod is not placed on any node", known: true, waiting: true,
				facts:  []string{"The pod waits for a volume: its PersistentVolumeClaim is not bound"},
				detail: "0/3 nodes are available: pod has unbound immediate PersistentVolumeClaims."}},
		{"unscheduled, words unknown", kube.Pod{Phase: "Pending", Reason: "Unschedulable", Unscheduled: "0/3 nodes are available: something new."},
			nil, podStory{title: "The pod is not placed on any node", waiting: true, detail: "0/3 nodes are available: something new."}},
		// Kubernetes' reason without Hearth's explanation stays unknown.
		{"config error", kube.Pod{Phase: "Pending", Reason: "CreateContainerConfigError", Containers: []kube.Container{{
			Name: "api", State: "waiting", Reason: "CreateContainerConfigError", Message: `secret "db" not found`}}},
			nil, podStory{title: "Container api: CreateContainerConfigError", detail: `secret "db" not found`}},
		{"creating, with a warning", kube.Pod{Phase: "Pending", Containers: []kube.Container{{Name: "api", State: "waiting", Reason: "ContainerCreating"}}},
			[]kube.Event{{Type: "Warning", Reason: "FailedMount", Message: `configmap "app" not found`}, {Type: "Normal", Reason: "Scheduled"}},
			podStory{title: "The containers are still being created", known: true, waiting: true, detail: `FailedMount: configmap "app" not found`}},
		{"not ready", kube.Pod{Phase: "Running", Containers: []kube.Container{{Name: "api", State: "running"}}}, nil,
			podStory{title: "Container api runs but is not ready", known: true, facts: []string{"Its readiness check does not pass"}}},
		{"evicted", kube.Pod{Phase: "Failed", Reason: "Evicted", Message: "The node was low on resource: memory."}, nil,
			podStory{title: "The pod ended: Evicted", detail: "The node was low on resource: memory."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := storyOf(tt.pod, tt.events, now)
			if !ok {
				t.Fatal("no story for a pod in trouble")
			}
			if got.title != tt.want.title || got.detail != tt.want.detail || got.known != tt.want.known || got.waiting != tt.want.waiting ||
				!slices.Equal(got.facts, tt.want.facts) || !slices.Equal(got.guesses, tt.want.guesses) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
	healthy := kube.Pod{Phase: "Running", Ready: 1, Containers: []kube.Container{{Name: "api", Ready: true, State: "running"}}}
	if _, ok := storyOf(healthy, nil, now); ok {
		t.Error("a healthy pod has a story")
	}
}

func TestRegistry(t *testing.T) {
	for image, want := range map[string]string{
		"ghcr.io/acme/web:1.4.2": "ghcr.io",
		"localhost:5001/web":     "localhost:5001",
		"postgres:17":            "docker.io",
		"bitnami/redis":          "docker.io",
	} {
		if got := registry(image); got != want {
			t.Errorf("registry(%q) = %q, want %q", image, got, want)
		}
	}
}

// openShop is the cluster page of kind-dev showing shopCluster.
func openShop(t *testing.T) (*App, *ui.Tester, *clusterWatches) {
	t.Helper()
	a := kubeApp()
	w := &clusterWatches{}
	a.WatchCluster = w.watch
	a.ListNamespaces = func(kube.Target, func([]string, error)) {}
	a.router.Push(kubeClusterPath("kind-dev"))
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Advance(time.Second)
	w.got[0](shopCluster(), nil)
	tt.Frame()
	tt.Advance(time.Second)
	return a, tt, w
}

// Pods group by owner: groups in trouble first and open, healthy ones
// closed; tabs and the search narrow the list.
func TestKubePods(t *testing.T) {
	a, tt, _ := openShop(t)
	hasTexts(t, tt, "checkout-api", "1 of 3 not ready", "web-frontend", "worker", "2 of 2 ready", "db-migrate-29381", "completed", "debug",
		"Container api keeps failing after it starts. Its last run ended with exit code 1 (Error) 3s after it started",
		"Image ghcr.io/acme/web:1.4.2 does not pull. Registry ghcr.io has no such image or tag",
		"Owners: 4 · pods: 8")
	for _, tab := range []string{"All", "Problems", "Restarted", "Completed"} {
		if err := tt.Click(tab); err != nil {
			t.Error(err)
		}
	}
	tt.Advance(time.Second)
	if a.cluster.podFilter != podsDone {
		t.Errorf("tab = %d, want the last clicked", a.cluster.podFilter)
	}
	a.cluster.podFilter = podsAll
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "kube-pods")
	texts := tt.Texts()
	if slices.Index(texts, "checkout-api") > slices.Index(texts, "worker") {
		t.Error("the group in trouble is not first")
	}
	// The healthy group is closed, the one in trouble open.
	if tt.HasText("worker-6f7d8-aaaaa") || !tt.HasText("checkout-api-7d9f8c5b-m8q2z") {
		t.Errorf("groups: %q", texts)
	}
	if err := tt.Click("worker"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "worker-6f7d8-aaaaa")
	if err := tt.Click("Expand all"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "db-migrate-29381-abcde")
	if err := tt.Click("Collapse all"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("worker-6f7d8-aaaaa") || tt.HasText("checkout-api-7d9f8c5b-m8q2z") {
		t.Error("Collapse all left a group open")
	}

	// Restarted pods show, in groups opened for them.
	a.cluster.podFilter, a.cluster.toggled = podsRestarted, nil
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "checkout-api-7d9f8c5b-x2k4p", "worker-6f7d8-bbbbb")
	if tt.HasText("worker-6f7d8-aaaaa") || tt.HasText("debug") {
		t.Errorf("restarted: %q", tt.Texts())
	}

	a.cluster.podFilter, a.cluster.toggled = podsAll, nil
	for query, want := range map[string][]string{
		"node:kind-worker2":    {"checkout-api-7d9f8c5b-m8q2z", "web-frontend-5c8d7f9b4-k2j9x", "worker-6f7d8-bbbbb"},
		"app=checkout-api":     {"checkout-api-7d9f8c5b-x2k4p"},
		"WEB":                  {"web-frontend-5c8d7f9b4-k2j9x"},
		"checkout node:worker": {"checkout-api-7d9f8c5b-x2k4p", "checkout-api-7d9f8c5b-p9w4d", "checkout-api-7d9f8c5b-m8q2z"},
	} {
		a.cluster.query = query
		tt.Frame()
		tt.Advance(time.Second)
		var got []string
		for _, s := range tt.Texts() {
			if strings.Count(s, "-") >= 2 && !strings.Contains(s, " ") {
				got = append(got, s)
			}
		}
		slices.Sort(got)
		got = slices.Compact(got) // a row's label and its text
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%q: pods %q, want %q", query, got, want)
		}
	}
}

// A pod's details say what is going on and read its events with the
// cluster; its owner opens the Deployment's, whose pods open theirs.
func TestKubePodDetail(t *testing.T) {
	a, tt, w := openShop(t)
	if err := tt.Click("checkout-api-7d9f8c5b-x2k4p"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	running := w.running()
	if len(running) != 1 || running[0].Events != (kube.Ref{Namespace: "default", UID: "p1"}) {
		t.Fatalf("watches = %+v, want the pod's events read", running)
	}
	// What was read stays while its events are read.
	hasTexts(t, tt, "Container api keeps failing after it starts", "14 restarts in 2h", "Kubernetes waits up to 5m before the next try",
		"CONTAINERS · 2", "ghcr.io/acme/checkout:2.8.1", "14 restarts · exit code 1", "envoy", "10.244.1.17", "Burstable",
		"app=checkout-api +1", "config · tmp", "Reading events…", "owner Deployment checkout-api ›")

	cl := shopCluster()
	cl.Events, cl.EventsOf = []kube.Event{
		{Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container api", Count: 52, Last: time.Now().Add(-time.Minute)},
		{Type: "Normal", Reason: "Pulled", Message: "Container image already present on machine", Count: 15, Last: time.Now().Add(-4 * time.Minute)},
	}, kube.Ref{Namespace: "default", UID: "p1"}
	w.got[len(w.got)-1](cl, nil)
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "BackOff", "Back-off restarting failed container api", "×52 · 1m")
	screenshot(t, tt, "kube-pod-detail")

	if err := tt.Click("owner Deployment checkout-api"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.cluster.kind != kindDeployments || a.cluster.deployment != "d1" {
		t.Fatalf("kind %d, deployment %q", a.cluster.kind, a.cluster.deployment)
	}
	if running := w.running(); len(running) != 1 || running[0].Events.UID != "d1" {
		t.Fatalf("watches = %+v, want the Deployment's events read", running)
	}
	hasTexts(t, tt, "revision 7", "2 of 3 ready", "1 of 3 pods not ready", "REVISIONS · 2", "ghcr.io/acme/checkout:2.8.0", "current", "RollingUpdate")
	screenshot(t, tt, "kube-deployment-detail")
	a.cluster.tab = 1 // Pods
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("checkout-api-7d9f8c5b-m8q2z"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.cluster.kind != kindPods || a.cluster.pod != "p2" {
		t.Fatalf("kind %d, pod %q", a.cluster.kind, a.cluster.pod)
	}

	// Closing the details reads no more events.
	if err := tt.Click("Close details"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if running := w.running(); len(running) != 1 || running[0].Events != (kube.Ref{}) {
		t.Fatalf("watches = %+v, want no events read", running)
	}
}

// Deployments: tabs by state, the tag of each.
func TestKubeDeployments(t *testing.T) {
	a, tt, _ := openShop(t)
	a.cluster.kind = kindDeployments
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "2 of 3 ready", "0 of 1 ready", "2 of 2 ready", "scaled to 0", "Rolling out", "Scaled to 0")
	a.cluster.deploymentFilter = deploymentsStopped
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("search-indexer") || tt.HasText("worker") {
		t.Errorf("stopped: %q", tt.Texts())
	}
}

// The cluster's data may come while its page still slides in beside the
// Kubernetes page, both built at once: their searches are not one
// control.
func TestKubeClusterSlidesIn(t *testing.T) {
	a := kubeApp()
	w := &clusterWatches{}
	a.WatchCluster = w.watch
	a.ListNamespaces = func(kube.Target, func([]string, error)) {}
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Advance(time.Second)
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	w.got[0](shopCluster(), nil)
	tt.Advance(time.Second)
	hasTexts(t, tt, "checkout-api-7d9f8c5b-x2k4p")
	a.router.Back()
	tt.Advance(time.Second)
}
