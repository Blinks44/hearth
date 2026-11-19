package kube

import (
	"errors"
	"testing"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

func TestReadBatch(t *testing.T) {
	target := target
	target.Batch = true
	var f run.Fake
	f.Stdout(`{"items":[
		{"metadata":{"name":"sync-old","namespace":"shop","creationTimestamp":"2026-11-17T00:00:00Z","ownerReferences":[{"kind":"CronJob","name":"sync","uid":"c1","controller":true}]},"status":{"conditions":[{"type":"Complete","status":"True"}],"succeeded":1}},
		{"metadata":{"name":"sync-new","namespace":"shop","creationTimestamp":"2026-11-18T00:00:00Z","ownerReferences":[{"kind":"CronJob","name":"sync","uid":"c1","controller":true}]},"status":{"conditions":[{"type":"Failed","status":"True","reason":"BackoffLimitExceeded","message":"Job has reached the specified backoff limit"}]}},
		{"metadata":{"name":"sync-other","namespace":"shop","creationTimestamp":"2026-11-19T00:00:00Z","ownerReferences":[{"kind":"CronJob","name":"sync","uid":"previous-c1","controller":true}]},"status":{"active":1,"failed":1}},
		{"metadata":{"name":"migrate","namespace":"shop"},"spec":{"completions":3},"status":{"succeeded":2,"conditions":[{"type":"Suspended","status":"True"}]}}
	]}`, overviewArgv("jobs.batch", "--namespace=shop")...)
	f.Stdout(`{"items":[{"metadata":{"name":"sync","namespace":"shop","uid":"c1"},"spec":{"schedule":"0 3 * * *","timeZone":"Etc/UTC","suspend":true}}]}`,
		overviewArgv("cronjobs.batch", "--namespace=shop")...)
	c, err := Read(t.Context(), &f, nil, target)
	if err != nil || c.Batch == nil || c.Batch.JobsErr != nil || c.Batch.CronJobsErr != nil {
		t.Fatalf("cluster = %+v, err = %v", c, err)
	}
	b := c.Batch
	if len(b.Jobs) != 4 || b.Jobs[0].State != JobRunning || b.Jobs[3].State != JobSuspended || b.Jobs[3].Want != 3 || b.Jobs[3].Completed != 2 {
		t.Fatalf("jobs = %+v", b.Jobs)
	}
	if len(b.CronJobs) != 1 {
		t.Fatalf("cronjobs = %+v", b.CronJobs)
	}
	j := b.CronJobs[0]
	if !j.Suspended || j.TimeZone != "Etc/UTC" || j.Schedule != "0 3 * * *" || len(j.Runs) != 2 || j.Runs[0].State != JobFailed || j.Runs[1].State != JobSucceeded {
		t.Errorf("cronjob = %+v", j)
	}
	if len(f.Calls()) != 2 {
		t.Errorf("calls = %+v, only batch resources should be queried", f.Calls())
	}
}

func TestReadBatchPartialAccess(t *testing.T) {
	target := target
	target.Batch = true
	var f run.Fake
	f.Set(run.Result{Stderr: []byte("Error from server (Forbidden): forbidden"), ExitCode: 1}, nil, overviewArgv("jobs.batch", "--namespace=shop")...)
	f.Stdout(`{"items":[{"metadata":{"name":"report","namespace":"shop","uid":"c1"},"spec":{"schedule":"@daily"}}]}`,
		overviewArgv("cronjobs.batch", "--namespace=shop")...)
	c, err := Read(t.Context(), &f, nil, target)
	if err != nil || c.Batch == nil || len(c.Batch.CronJobs) != 1 || c.Batch.CronJobsErr != nil {
		t.Fatalf("cluster = %+v, err = %v", c, err)
	}
	e, ok := errors.AsType[*Error](c.Batch.JobsErr)
	if !ok || e.Access.State != detect.KubeForbidden {
		t.Errorf("jobs err = %v", c.Batch.JobsErr)
	}
}
