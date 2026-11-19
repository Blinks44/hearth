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
		Message: "Job has reached the specified backoff limit", Want: 1, Created: now.Add(-4 * time.Minute)}
	succeeded := kube.Job{Namespace: "default", Name: "cleanup-sessions-success", UID: "j2", State: kube.JobSucceeded,
		Completed: 1, Want: 1, Created: now.Add(-19 * time.Minute)}
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
	hasTexts(t, tt, "cleanup-sessions-failed", "cleanup-sessions-success", "Job has reached the specified backoff limit", "1/1")
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
		"*/15 * * * *": "Every 15 min",
		"0 * * * *":    "Every hour",
		"0 */6 * * *":  "Every 6 h from midnight",
		"0 3 * * *":    "Daily at 03:00",
		"0 9 * * MON":  "Mondays at 09:00",
		"@daily":       "Daily at 00:00",
		"30 2 1 * MON": "",
		"*/13 * * * *": "",
	}
	for schedule, want := range tests {
		if got := cronCaption(schedule); got != want {
			t.Errorf("cronCaption(%q) = %q, want %q", schedule, got, want)
		}
	}
}
