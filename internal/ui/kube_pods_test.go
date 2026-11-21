package ui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
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
		// Between its tries the container shows the run that just ended.
		{"crash, between tries", kube.Pod{Phase: "Running", Reason: "Error", Created: created, Containers: []kube.Container{{
			Name: "api", Restarts: 27, State: "terminated", Reason: "Error",
			Ended: &kube.Run{Reason: "Error", ExitCode: 1, Started: now.Add(-time.Minute), Finished: now.Add(-time.Minute + 3*time.Second)},
		}}}, nil, podStory{title: "Container api keeps failing after it starts", known: true, facts: []string{
			"Its last run ended with exit code 1 (Error) 3s after it started", "27 restarts in 41m",
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

// openShop is the cluster page of kind-dev showing shopCluster, its
// groups of pods open.
func openShop(t *testing.T) (*App, *ui.Tester, *clusterWatches) {
	t.Helper()
	a, tt, w := openShopClosed(t)
	if err := tt.Click("Expand all"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	return a, tt, w
}

// expandGroups opens every group of the cluster's pods, as Expand all
// does.
func expandGroups(a *App, cl kube.Cluster) {
	a.cluster.toggled = map[string]bool{}
	for _, g := range podGroups(cl.Pods, tableSort{}) {
		a.cluster.toggled[g.key()] = true
	}
}

// openShopClosed is openShop with the groups as they first show: closed.
func openShopClosed(t *testing.T) (*App, *ui.Tester, *clusterWatches) {
	t.Helper()
	a := kubeApp()
	w := &clusterWatches{}
	a.WatchCluster = w.watch
	a.ListNamespaces = func(kube.Target, func([]string, error)) func() { return func() {} }
	a.StreamLogs = (&logStreams{}).stream
	a.router.Push(kubeClusterPath("kind-dev"))
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Advance(time.Second)
	w.got[0](shopCluster(), nil)
	tt.Frame()
	tt.Advance(time.Second)
	return a, tt, w
}

// Pods group by owner: groups in trouble first, every group closed with
// its totals; tabs and the search narrow the list.
func TestKubePods(t *testing.T) {
	a, tt, _ := openShopClosed(t)
	hasTexts(t, tt, "checkout-api", "1 of 3 not ready", "web-frontend", "worker", "2 of 2 ready", "db-migrate-29381", "completed", "debug",
		"2/3", "14", "Owners: 4 · pods: 8")
	if tt.HasText("checkout-api-7d9f8c5b-x2k4p") {
		t.Error("a group in trouble shows open")
	}
	if err := tt.Click("Deployment checkout-api"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "checkout-api-7d9f8c5b-x2k4p", "Details · checkout-api-7d9f8c5b-x2k4p")
	if err := tt.Click("Details · checkout-api-7d9f8c5b-x2k4p"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	hasTexts(t, tt, "Container api keeps failing after it starts",
		"Its last run ended with exit code 1 (Error) 3s after it started")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	a.cluster.toggled = nil
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
	if tt.HasText("worker-6f7d8-aaaaa") || tt.HasText("checkout-api-7d9f8c5b-m8q2z") {
		t.Errorf("groups: %q", texts)
	}
	if err := tt.Click("Deployment worker"); err != nil {
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
		"app=checkout-api +1", "config · tmp", "Reading events…", "Owner", "Open Deployment checkout-api")

	cl := shopCluster()
	now := tt.Now()
	cl.Events, cl.EventsOf = []kube.Event{
		{Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container api", Count: 52, Last: now.Add(-time.Minute)},
		{Type: "Normal", Reason: "Pulled", Message: "Container image already present on machine", Count: 15, Last: now.Add(-4 * time.Minute)},
	}, kube.Ref{Namespace: "default", UID: "p1"}
	w.got[len(w.got)-1](cl, nil)
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "BackOff", "Back-off restarting failed container api", "52 times · 1m")
	screenshot(t, tt, "kube-pod-detail")

	if err := tt.Click("Open Deployment checkout-api"); err != nil {
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

func TestWorkloadEventFailureStaysInDetails(t *testing.T) {
	for _, tc := range []struct {
		name, label string
		kind        int
	}{
		{"Pod", "checkout-api-7d9f8c5b-x2k4p", kindPods},
		{"Deployment", "checkout-api", kindDeployments},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, tt, watches := openShop(t)
			a.cluster.kind = tc.kind
			tt.Frame()
			tt.Advance(time.Second)
			watches.got[len(watches.got)-1](shopCluster(), nil)
			tt.Frame()
			tt.Advance(time.Second)
			if err := tt.Click(tc.label); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			running := watches.running()
			if len(running) != 1 || running[0].Events.UID == "" {
				t.Fatalf("missing request for %s events: %+v", tc.name, running)
			}
			data := shopCluster()
			data.EventsOf = running[0].Events
			data.EventsErr = errors.New("Forbidden to list workload events")
			watches.got[len(watches.got)-1](data, nil)
			tt.Frame()
			tt.Advance(time.Second)
			if err := tt.Click("Events"); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			hasTexts(t, tt, tc.label, "Forbidden to list workload events")
			if tt.HasText("No events. Kubernetes keeps them for about an hour.") || tt.HasText("Reading events…") {
				t.Fatalf("event denial was shown as an empty or pending list: %v", tt.Texts())
			}
		})
	}
}

// The cluster's data may come while its page still slides in beside the
// Kubernetes page, both built at once: their searches are not one
// control.
func TestKubeClusterSlidesIn(t *testing.T) {
	a := kubeApp()
	w := &clusterWatches{}
	a.WatchCluster = w.watch
	a.ListNamespaces = func(kube.Target, func([]string, error)) func() { return func() {} }
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Advance(time.Second)
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	w.got[0](shopCluster(), nil)
	expandGroups(a, shopCluster())
	tt.Advance(time.Second)
	hasTexts(t, tt, "checkout-api-7d9f8c5b-x2k4p")
	a.router.Back()
	tt.Advance(time.Second)
}

// Opening a workload deep in the list leaves its details at the top of
// the right pane. Both panes can scroll without moving the other one.
func TestKubeWorkloadPanes(t *testing.T) {
	for _, kind := range []string{"Pods", "Deployments"} {
		t.Run(kind, func(t *testing.T) {
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.router.Push(projectsPage.path)
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, 1000, 680)
			tt.Advance(time.Second)

			cl := kube.Cluster{Taken: time.Now()}
			for i := range 90 {
				pod := kube.Pod{UID: fmt.Sprintf("p%d", i), Name: fmt.Sprintf("pod-%03d", i), Namespace: "shop", Phase: "Running", Created: cl.Taken.Add(-3 * time.Hour)}
				deployment := kube.Deployment{UID: fmt.Sprintf("d%d", i), Name: fmt.Sprintf("deploy-%03d", i), Namespace: "shop", Created: cl.Taken.Add(-3 * time.Hour)}
				if i == 50 {
					for j := range 28 {
						pod.Containers = append(pod.Containers, kube.Container{Name: fmt.Sprintf("container-%02d", j), Ready: true, State: "running"})
						deployment.Revisions = append(deployment.Revisions, kube.Revision{Name: fmt.Sprintf("revision-%02d", j), Number: j + 1, Created: deployment.Created})
					}
					pod.Ready = len(pod.Containers)
				}
				cl.Pods = append(cl.Pods, pod)
				cl.Deployments = append(cl.Deployments, deployment)
			}
			watches.got[0](cl, nil)
			expandGroups(a, cl)
			tt.Frame()
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			selected, anchor, next := "pod-050", "pod-060", "pod-058"
			bodyAnchor := "CONTAINERS · 28"
			if kind == "Deployments" {
				click(kind)
				selected, anchor, next = "deploy-050", "deploy-060", "deploy-058"
				bodyAnchor = "Wanted"
			}
			find := func(text string) ui.Rect {
				t.Helper()
				r, ok := tt.Find(text)
				if !ok {
					t.Fatalf("missing %q", text)
				}
				return r
			}
			header := find("STATUS")
			tt.Scroll(400, 380, 0, find(selected).Y-420)
			if got := find("STATUS"); got != header {
				t.Fatalf("column header moved on scroll: %+v -> %+v", header, got)
			}
			click(selected)
			close := find("Close details")
			if close.Y < 125 || close.Y > 240 {
				t.Fatalf("details header opened outside the top of the pane: %+v", close)
			}
			if selectedRow := find(selected); selectedRow.X >= 575 || selectedRow.Y < 190 || selectedRow.Y > 650 {
				t.Errorf("selected row moved out of the viewport: %+v", selectedRow)
			}
			screenshot(t, tt, "kube-"+kind+"-independent-panes")
			left, body := find(anchor), find(bodyAnchor)
			tt.Scroll(790, 380, 0, 250)
			moved := find(bodyAnchor)
			if find(anchor).Y != left.Y || find("Close details") != close || (moved.Y >= body.Y && moved.H >= body.H) {
				t.Fatalf("details scroll: left %v -> %v, close %v -> %v, body %v -> %v", left, find(anchor), close, find("Close details"), body, moved)
			}
			body = find(bodyAnchor)
			tt.Scroll(400, 380, 0, 160)
			if find("Close details") != close || find(bodyAnchor) != body || find(anchor).Y >= left.Y {
				t.Fatal("scrolling the list moved the details or left the list still")
			}
			click(next)
			if got := find("Close details"); got.Y != close.Y {
				t.Fatalf("new details inherited the previous pane's scroll: %+v", got)
			}
			click("Back to Kubernetes")
			tt.Advance(time.Second)
			if a.router.Path() != kubePage || len(watches.running()) != 0 {
				t.Fatalf("back opened %q with active cluster watches %+v", a.router.Path(), watches.running())
			}
		})
	}
}

func TestKubeWorkloadColumns(t *testing.T) {
	for _, width := range []int{1000, 1440} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, width, 680)
			tt.Advance(time.Second)
			watches.got[0](shopCluster(), nil)
			expandGroups(a, shopCluster())
			tt.Frame()
			find := func(label string) ui.Rect {
				t.Helper()
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing %q", label)
				}
				return r
			}
			status := find("STATUS")
			for _, value := range []string{"1 of 3 not ready", "CrashLoopBackOff", "Running"} {
				if got := find(value); got.X != status.X {
					t.Errorf("%q starts at %g, status column starts at %g", value, got.X, status.X)
				}
			}
			for _, pair := range [][2]string{{"READY", "1/2"}, {"RESTARTS", "14"}} {
				if head, value := find(pair[0]), find(pair[1]); head.X != value.X {
					t.Errorf("%s and %s start at %g and %g", pair[0], pair[1], head.X, value.X)
				}
			}
			if err := tt.Click("Deployments"); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if got := find("2 of 3 ready"); got.X != status.X {
				t.Errorf("Deployment status starts at %g, want %g", got.X, status.X)
			}
			screenshot(t, tt, "kube-deployments-table-"+strconv.Itoa(width))
		})
	}
}

func TestKubePodEventsLayout(t *testing.T) {
	for _, width := range []int{1000, 1280} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.StreamLogs = (&logStreams{}).stream
			a.CheckForwardPort = func(int, func(int, error)) {}
			a.RunForward = func(ops.Forward, func(kube.ForwardPod), func(error)) func() { return func() {} }
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, width, 680)
			tt.Advance(time.Second)
			now := time.Now()
			pod := kube.Pod{UID: "test", Name: "event-pod", Namespace: "default", Phase: "Running", Created: now.Add(-time.Hour)}
			for i := range 14 {
				pod.Containers = append(pod.Containers, kube.Container{Name: fmt.Sprintf("app-%02d", i), Ready: true, State: "running"})
			}
			pod.Containers[0].Reason, pod.Containers[0].State = "CreateContainerConfigError", "waiting"
			pod.Containers[0].Ready, pod.Containers[0].Restarts = false, 14
			pod.Ready = len(pod.Containers) - 1
			cl := kube.Cluster{Taken: now, Pods: []kube.Pod{pod}}
			watches.got[0](cl, nil)
			tt.Frame()
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			find := func(label string) ui.Rect {
				t.Helper()
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing %q", label)
				}
				return r
			}
			click(pod.Name)
			logs, forward := find("Logs"), find("Forward port…")
			if logs.Y != forward.Y || logs.H != forward.H {
				t.Fatalf("action sizes differ: logs %+v, forward %+v", logs, forward)
			}

			const first = "Readiness probe failed: HTTP probe failed with statuscode: 502"
			const second = "Container created"
			const third = "Container image registry.test/example/postgresql:18.6-standard-trixie@sha256:28c7e72a490d6d1d8a9139fc02eedaaa was found locally"
			if first, second := find("14 restarts"), find("0 restarts"); first.X != second.X {
				t.Fatalf("container restart columns differ: %+v %+v", first, second)
			}
			if first, second := find("CreateContainerConfigError"), find("running"); first.X != second.X {
				t.Fatalf("container state columns differ: %+v %+v", first, second)
			}
			cl.EventsOf = kube.Ref{Namespace: pod.Namespace, UID: pod.UID}
			cl.Events = []kube.Event{
				{Type: "Warning", Reason: "Unhealthy", Message: first, Count: 52, Last: now.Add(-time.Minute)},
				{Type: "Normal", Reason: "Created", Message: second, Count: 1, Last: now.Add(-2 * time.Minute)},
				{Type: "Normal", Reason: "Pulled", Message: third, Count: 143, Last: now.Add(-3 * time.Minute)},
				{Type: "Warning", Reason: "FailedCreatePodSandBox", Message: "The network plugin could not set up the pod sandbox", Count: 3, Last: now.Add(-4 * time.Minute)},
			}
			watches.got[len(watches.got)-1](cl, nil)
			tt.Frame()
			summary := find(first)
			link := find("All events")
			x := link.X + link.W/2 - 60
			tt.Scroll(x, 440, 0, link.Y-404)
			click("All events")

			if find("Logs") != logs || find("Forward port…") != forward {
				t.Fatal("the header moved when switching to events")
			}
			f, s, l := find(first), find(second), find(third)
			if f.Y >= 405 || f.X != summary.X || f.X != s.X || f.X != l.X || l.H <= s.H {
				t.Fatalf("events did not open at the top of an aligned list: %+v %+v %+v", f, s, l)
			}
			if a, b := find("52 times · 1m"), find("2m"); a.X+a.W != b.X+b.W {
				t.Fatalf("event ages do not share the right edge: %+v %+v", a, b)
			}
			screenshot(t, tt, "kube-pod-events-"+strconv.Itoa(width))
		})
	}
}
