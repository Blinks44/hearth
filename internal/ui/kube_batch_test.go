package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/kube"
)

func TestKubeBatch(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	a.WatchCluster = w.watch
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	w.got[0](shopCluster(), nil)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyJ)
	hasTexts(t, tt, "Resource kind: pods, jobs…", "deploy", "cj")
	screenshot(t, tt, "kube-kind-picker")
	tt.Type("cj")
	tt.Key(0, ui.KeyEnter)
	if got := w.running(); len(got) != 1 || !got[0].Batch || got[0].Overview || !w.stopped[0] {
		t.Fatalf("watches = %+v", got)
	}
	now := time.Now()
	failed := kube.Job{Namespace: "default", Name: "cleanup-sessions-failed", UID: "j1", State: kube.JobFailed,
		Message: "Job has reached the specified backoff limit", Want: 1, Created: now.Add(-4*time.Minute - 30*time.Second)}
	succeeded := kube.Job{Namespace: "default", Name: "cleanup-sessions-success", UID: "j2", State: kube.JobSucceeded,
		Completed: 1, Want: 1, Created: now.Add(-19*time.Minute - 30*time.Second)}
	batch := &kube.Batch{Jobs: []kube.Job{failed, succeeded}, CronJobs: []kube.CronJob{
		{Name: "cleanup-sessions", Namespace: "default", UID: "c1", Schedule: "*/15 * * * *", TimeZone: "Etc/UTC", Runs: []kube.Job{failed, succeeded}},
		{Name: "report-nightly", Namespace: "default", UID: "c2", Schedule: "0 3 * * *", Suspended: true},
	}}
	w.got[1](kube.Cluster{Taken: now, Batch: batch}, nil)
	tt.Frame()
	hasTexts(t, tt, "cleanup-sessions", "Every 15 min · Etc/UTC", "Failed · 4m",
		"report-nightly", "Daily at 03:00", "No Jobs in history", "Suspended")
	screenshot(t, tt, "kube-cronjobs")
	if err := tt.Click("Jobs"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "cleanup-sessions-failed", "cleanup-sessions-success", "Details · cleanup-sessions-failed", "1/1")
	if err := tt.Click("Details · cleanup-sessions-failed"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	hasTexts(t, tt, "Job has reached the specified backoff limit")
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	screenshot(t, tt, "kube-jobs")
	if len(w.targets) != 2 {
		t.Fatal("switching between batch views restarted the same read")
	}
	batchWithError := *batch
	batchWithError.Jobs, batchWithError.JobsErr = nil, &kube.Error{Access: detect.KubeAccess{State: detect.KubeForbidden}}
	w.got[1](kube.Cluster{Taken: now, Batch: &batchWithError}, nil)
	tt.Frame()
	hasTexts(t, tt, "Could not read Jobs: no access.")
	if err := tt.Click("CronJobs"); err != nil {
		t.Fatal(err)
	}
	hasTexts(t, tt, "report-nightly", "no access")
}

func TestCronCaption(t *testing.T) {
	tests := map[string]string{
		"* * * * *":               "Every minute",
		"*/1 * * * *":             "Every minute",
		"*/15 * * * *":            "Every 15 min",
		"*/9 * * * *":             "Every 9 min of the hour",
		"*/13 * * * *":            "Every 13 min of the hour",
		"0 * * * *":               "Every hour",
		"1 * * * *":               "Every hour at :01",
		"30 * * * *":              "Every hour at :30",
		"0 */6 * * *":             "Every 6 h from midnight",
		"30 */4 * * *":            "Every 4 h at :30",
		"0 3 * * *":               "Daily at 03:00",
		"30 6,14,22 * * *":        "Daily at 06:30, 14:30, 22:30",
		"0 20,21,22,23,0,1 * * *": "Daily at 20:00, 21:00, 22:00, 23:00, 00:00, 01:00",
		"0 9 * * 1-5":             "Weekdays at 09:00",
		"0 9 * * MON-FRI":         "Weekdays at 09:00",
		"0 9 * * MON":             "Mondays at 09:00",
		"0 9 * * 7":               "Sundays at 09:00",
		"15 0 1 * *":              "Day 1 of each month at 00:15",
		"15 0 20 * *":             "Day 20 of each month at 00:15",
		"30 23 23 12 *":           "Dec 23 at 23:30",
		"45 0 24 12 *":            "Dec 24 at 00:45",
		"@daily":                  "Daily at 00:00",
		"30 2 1 * MON":            "",
	}
	for schedule, want := range tests {
		if got := cronCaption(schedule); got != want {
			t.Errorf("cronCaption(%q) = %q, want %q", schedule, got, want)
		}
	}
}

func TestKubeBatchDetailAndRelationships(t *testing.T) {
	a, tt, w := openShop(t)
	a.cluster.kind = kindCronJobs
	tt.Advance(time.Second)
	now := time.Now()
	backoff := 2
	job := kube.Job{
		Namespace:    "default",
		Name:         "cleanup-sessions-run1",
		UID:          "j1",
		CronJob:      "cleanup-sessions",
		CronJobUID:   "c1",
		State:        kube.JobSucceeded,
		Completed:    1,
		Want:         1,
		Parallelism:  1,
		BackoffLimit: &backoff,
		Images:       []string{"ghcr.io/acme/cleanup:1.0"},
		Created:      now.Add(-5 * time.Minute),
		Started:      now.Add(-5 * time.Minute),
		CompletedAt:  now.Add(-4*time.Minute - 30*time.Second),
	}
	cj := kube.CronJob{
		Namespace:         "default",
		Name:              "cleanup-sessions",
		UID:               "c1",
		Schedule:          "30 */4 * * *",
		TimeZone:          "Etc/UTC",
		ConcurrencyPolicy: "Forbid",
		Images:            []string{"ghcr.io/acme/cleanup:1.0"},
		Created:           now.Add(-24 * time.Hour),
		LastSchedule:      now.Add(-5 * time.Minute),
		Runs:              []kube.Job{job},
	}
	batch := &kube.Batch{Jobs: []kube.Job{job}, CronJobs: []kube.CronJob{cj}}
	w.got[len(w.got)-1](kube.Cluster{Taken: now, Batch: batch}, nil)
	tt.Frame()

	// Click CronJob to open detail pane and verify facts + relationships.
	if err := tt.Click("cleanup-sessions"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := w.running(); len(got) != 1 || got[0].Events != (kube.Ref{Namespace: "default", UID: "c1"}) {
		t.Fatalf("cronjob detail watch = %+v", got)
	}
	w.got[len(w.got)-1](kube.Cluster{
		Taken: now,
		Batch: batch,
		Relationships: &kube.Relationships{
			Object: kube.Ref{Namespace: "default", UID: "c1"},
			Links: []kube.Relationship{
				{Kind: "Job", Namespace: "default", Name: "cleanup-sessions-run1", UID: "j1"},
				{Kind: "Secret", Namespace: "default", Name: "cleanup-secret", Detail: "worker"},
			},
		},
	}, nil)
	tt.Frame()
	hasTexts(t, tt, "30 */4 * * * · Every 4 h at :30", "Forbid", "Job cleanup-sessions-run1", "Secret cleanup-secret")

	// Open the owned Job from the CronJob's Jobs tab.
	if err := tt.Click("Jobs"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("cleanup-sessions-run1"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.cluster.kind != kindJobs || a.cluster.job != "j1" {
		t.Fatalf("after clicking owned job: kind=%d, job=%q", a.cluster.kind, a.cluster.job)
	}
	w.got[len(w.got)-1](kube.Cluster{
		Taken: now,
		Batch: batch,
		Relationships: &kube.Relationships{
			Object: kube.Ref{Namespace: "default", UID: "j1"},
			Links: []kube.Relationship{
				{Kind: "CronJob", Namespace: "default", Name: "cleanup-sessions", UID: "c1", Inbound: true},
				{Kind: "Pod", Namespace: "default", Name: "cleanup-sessions-run1-pod", UID: "p1", Detail: "Succeeded"},
				{Kind: "ConfigMap", Namespace: "default", Name: "cleanup-cfg", Detail: "worker"},
			},
		},
	}, nil)
	tt.Frame()
	hasTexts(t, tt, "CronJob", "cleanup-sessions", "Pod cleanup-sessions-run1-pod", "ConfigMap cleanup-cfg", "30s")

	// Clicking the CronJob owner chip jumps back to the CronJob detail pane.
	if err := tt.Click("Open CronJob cleanup-sessions"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.cluster.kind != kindCronJobs || a.cluster.cronJob != "c1" {
		t.Fatalf("after clicking CronJob owner chip: kind=%d, cronJob=%q", a.cluster.kind, a.cluster.cronJob)
	}
}

func TestKubeFullRowClickAndServiceDetails(t *testing.T) {
	a, tt, w := openShop(t)
	now := time.Now()

	// 1. Clicking the right side of a Job row (outside the Name column) opens details.
	a.cluster.kind = kindJobs
	tt.Advance(time.Second)
	job := kube.Job{Namespace: "default", Name: "db-migrate", UID: "j-row", State: kube.JobSucceeded, Completed: 1, Want: 1, Created: now.Add(-5 * time.Minute)}
	w.got[len(w.got)-1](kube.Cluster{Taken: now, Batch: &kube.Batch{Jobs: []kube.Job{job}}}, nil)
	tt.Frame()
	if err := tt.Click("1/1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cluster.job != "j-row" {
		t.Fatalf("clicking Job completed column did not select Job: %q", a.cluster.job)
	}

	// 2. Clicking the schedule column of a CronJob row opens details.
	a.cluster.kind = kindCronJobs
	tt.Advance(time.Second)
	cj := kube.CronJob{Namespace: "default", Name: "nightly-sync", UID: "c-row", Schedule: "0 3 * * *"}
	w.got[len(w.got)-1](kube.Cluster{Taken: now, Batch: &kube.Batch{CronJobs: []kube.CronJob{cj}}}, nil)
	tt.Frame()
	if err := tt.Click("0 3 * * *"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cluster.cronJob != "c-row" {
		t.Fatalf("clicking CronJob schedule column did not select CronJob: %q", a.cluster.cronJob)
	}

	// 3. Clicking the ClusterIP column of a Service row opens enriched Service details + Events tab.
	a.cluster.kind = kindServices
	tt.Advance(time.Second)
	svc := kube.Service{
		Namespace: "default", Name: "shop-api", UID: "svc-row", Created: now.Add(-2 * time.Hour),
		Type: "LoadBalancer", ClusterIP: "10.96.42.10", LoadBalancer: []string{"198.51.100.7"},
		SessionAffinity: "ClientIP", ExternalTrafficPolicy: "Local",
		Selector: map[string]string{"app": "shop-api"}, Labels: map[string]string{"tier": "api"},
		HasSelector: true, PodsKnown: true, Pods: 2, Ready: 2,
		Ports: []kube.ServicePort{{Name: "https", Port: 443, NodePort: 30443, Protocol: "TCP", Target: "8443"}},
	}
	w.got[len(w.got)-1](kube.Cluster{Taken: now, Services: []kube.Service{svc}}, nil)
	tt.Frame()
	if err := tt.Click("10.96.42.10"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.cluster.service != "svc-row" {
		t.Fatalf("clicking Service Cluster IP column did not select Service: %q", a.cluster.service)
	}
	w.got[len(w.got)-1](kube.Cluster{
		Taken:    now,
		Services: []kube.Service{svc},
		EventsOf: kube.Ref{Namespace: "default", UID: "svc-row"},
		Events:   []kube.Event{{Type: "Normal", Reason: "UpdatedLoadBalancer", Message: "Updated load balancer"}},
	}, nil)
	tt.Frame()
	hasTexts(t, tt,
		"shop-api.default.svc.cluster.local",
		"198.51.100.7",
		"https: 443 → 8443/TCP (NodePort 30443)",
		"ClientIP",
		"external: Local",
		"app=shop-api",
		"tier=api",
	)
	if err := tt.Click("Events"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	hasTexts(t, tt, "UpdatedLoadBalancer", "Updated load balancer")
}
