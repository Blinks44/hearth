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

func TestReadBatchRelationships(t *testing.T) {
	jobsJSON := `{"items":[
		{"metadata":{"name":"sync-1","namespace":"shop","uid":"j1","creationTimestamp":"2026-11-18T00:00:00Z","ownerReferences":[{"kind":"CronJob","name":"sync","uid":"c1","controller":true}]},"spec":{"completions":2,"parallelism":2,"backoffLimit":0,"template":{"spec":{"serviceAccountName":"batch-sa","containers":[{"name":"worker","image":"ghcr.io/acme/worker:1.2","envFrom":[{"configMapRef":{"name":"worker-cfg"}}]}]}}},"status":{"succeeded":1,"failed":1,"startTime":"2026-11-18T00:00:05Z","completionTime":"2026-11-18T00:01:05Z","conditions":[{"type":"Complete","status":"True"}]}}
	]}`
	cronsJSON := `{"items":[
		{"metadata":{"name":"sync","namespace":"shop","uid":"c1","creationTimestamp":"2026-11-10T00:00:00Z"},"spec":{"schedule":"30 */4 * * *","concurrencyPolicy":"Forbid","jobTemplate":{"spec":{"template":{"spec":{"serviceAccountName":"batch-sa","containers":[{"name":"worker","image":"ghcr.io/acme/worker:1.2","envFrom":[{"secretRef":{"name":"worker-secret"}}]}]}}}}},"status":{"lastScheduleTime":"2026-11-18T00:00:00Z"}}
	]}`
	podsJSON := `{"items":[
		{"metadata":{"name":"sync-1-abc","namespace":"shop","uid":"p1","ownerReferences":[{"kind":"Job","name":"sync-1","uid":"j1","controller":true}]},"spec":{"serviceAccountName":"batch-sa","volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"worker-pvc"}}]},"status":{"phase":"Succeeded"}},
		{"metadata":{"name":"other-pod","namespace":"shop","uid":"p2"},"status":{"phase":"Running"}}
	]}`
	eventsJSON := `{"items":[{"type":"Normal","reason":"Completed","message":"Job completed","count":1}]}`

	// Selected Job: reads events, pods, inbound CronJob, outbound Pod, PVC, ConfigMap, ServiceAccount.
	jobTarget := target
	jobTarget.Batch = true
	jobTarget.Events = Ref{Namespace: "shop", UID: "j1"}
	var f run.Fake
	f.Stdout(jobsJSON, overviewArgv("jobs.batch", "--namespace=shop")...)
	f.Stdout(cronsJSON, overviewArgv("cronjobs.batch", "--namespace=shop")...)
	f.Stdout(eventsJSON, overviewArgv("events", "--namespace=shop", "--field-selector=involvedObject.uid=j1")...)
	f.Stdout(podsJSON, overviewArgv("pods", "--namespace=shop")...)
	c, err := Read(t.Context(), &f, nil, jobTarget)
	if err != nil || c.Batch == nil || c.Relationships == nil {
		t.Fatalf("job read = %+v, err = %v", c, err)
	}
	if len(c.Events) != 1 || c.Events[0].Reason != "Completed" {
		t.Fatalf("job events = %+v", c.Events)
	}
	j := c.Batch.Jobs[0]
	if j.Parallelism != 2 || j.BackoffLimit == nil || *j.BackoffLimit != 0 || j.Failed != 1 || len(j.Images) != 1 || j.Images[0] != "ghcr.io/acme/worker:1.2" {
		t.Fatalf("job metadata = %+v", j)
	}
	wantJobLinks := map[string]bool{
		"CronJob/sync/true":                true,
		"Pod/sync-1-abc/false":             true,
		"PersistentVolumeClaim/worker-pvc": true,
		"ConfigMap/worker-cfg":             true,
		"ServiceAccount/batch-sa":          true,
	}
	for _, l := range c.Relationships.Links {
		delete(wantJobLinks, l.Kind+"/"+l.Name)
		if l.Inbound {
			delete(wantJobLinks, l.Kind+"/"+l.Name+"/true")
		} else {
			delete(wantJobLinks, l.Kind+"/"+l.Name+"/false")
		}
	}
	if len(wantJobLinks) != 0 {
		t.Fatalf("missing job links %v in %+v", wantJobLinks, c.Relationships.Links)
	}

	// Selected CronJob: reads events, outbound Job runs and template references without querying pods.
	cronTarget := target
	cronTarget.Batch = true
	cronTarget.Events = Ref{Namespace: "shop", UID: "c1"}
	var f2 run.Fake
	f2.Stdout(jobsJSON, overviewArgv("jobs.batch", "--namespace=shop")...)
	f2.Stdout(cronsJSON, overviewArgv("cronjobs.batch", "--namespace=shop")...)
	f2.Stdout(eventsJSON, overviewArgv("events", "--namespace=shop", "--field-selector=involvedObject.uid=c1")...)
	c2, err := Read(t.Context(), &f2, nil, cronTarget)
	if err != nil || c2.Relationships == nil {
		t.Fatalf("cronjob read = %+v, err = %v", c2, err)
	}
	if cj := c2.Batch.CronJobs[0]; cj.ConcurrencyPolicy != "Forbid" || cj.LastSchedule.IsZero() {
		t.Fatalf("cronjob metadata = %+v", cj)
	}
	wantCronLinks := map[string]bool{
		"Job/sync-1":              true,
		"Secret/worker-secret":    true,
		"ServiceAccount/batch-sa": true,
	}
	for _, l := range c2.Relationships.Links {
		delete(wantCronLinks, l.Kind+"/"+l.Name)
	}
	if len(wantCronLinks) != 0 {
		t.Fatalf("missing cronjob links %v in %+v", wantCronLinks, c2.Relationships.Links)
	}
}
