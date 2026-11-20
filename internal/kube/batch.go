package kube

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Batch holds the requested scopes' Jobs and CronJobs. Access to either
// resource type can fail independently.
type Batch struct {
	Jobs                 []Job
	CronJobs             []CronJob
	JobsErr, CronJobsErr error
}

// JobState is a Job's reported state.
type JobState int

const (
	JobPending JobState = iota
	JobRunning
	JobSucceeded
	JobFailed
	JobSuspended
)

// Job is a run, standalone or created by a CronJob.
type Job struct {
	Namespace, Name, UID string
	CronJob, CronJobUID  string
	State                JobState
	Reason, Message      string
	Completed, Want      int
	Created, Started     time.Time
}

// CronJob is a schedule and the recent Jobs it still owns, newest first.
type CronJob struct {
	Namespace, Name, UID string
	Schedule, TimeZone   string
	Suspended            bool
	Active               int
	Runs                 []Job
}

type batchJob struct {
	meta
	Spec struct {
		Completions *int `json:"completions"`
	} `json:"spec"`
	Status struct {
		Active     int       `json:"active"`
		Succeeded  int       `json:"succeeded"`
		StartTime  time.Time `json:"startTime"`
		Conditions []struct {
			Type, Status, Reason, Message string
		} `json:"conditions"`
	} `json:"status"`
}

func (j batchJob) value() Job {
	m := j.Metadata
	job := Job{Namespace: m.Namespace, Name: m.Name, UID: m.UID, Created: m.CreationTimestamp, Started: j.Status.StartTime,
		Completed: j.Status.Succeeded, Want: 1}
	if j.Spec.Completions != nil {
		job.Want = *j.Spec.Completions
	}
	for _, o := range m.OwnerReferences {
		if o.Controller && o.Kind == "CronJob" {
			job.CronJob, job.CronJobUID = o.Name, o.UID
			break
		}
	}
	if j.Status.Active > 0 {
		job.State = JobRunning
	}
	for _, c := range j.Status.Conditions {
		if c.Status != "True" {
			continue
		}
		switch c.Type {
		case "Failed":
			job.State, job.Reason, job.Message = JobFailed, c.Reason, c.Message
			return job
		case "Complete":
			job.State = JobSucceeded
			return job
		case "Suspended":
			job.State = JobSuspended
		}
	}
	return job
}

type cronJob struct {
	meta
	Spec struct {
		Schedule string `json:"schedule"`
		TimeZone string `json:"timeZone"`
		Suspend  bool   `json:"suspend"`
	} `json:"spec"`
	Status struct {
		Active []struct{} `json:"active"`
	} `json:"status"`
}

func readBatch(ctx context.Context, r run.Runner, env sysenv.Env, t Target, scopes [][]string) *Batch {
	b := &Batch{}
	// Jobs and CronJobs are read at once, each on its own: the rights to
	// one may lack the other.
	var wg sync.WaitGroup
	wg.Go(func() {
		for _, scope := range scopes {
			var list struct {
				Items []batchJob `json:"items"`
			}
			if b.JobsErr = get(ctx, r, env, t, append([]string{"jobs.batch"}, scope...), &list); b.JobsErr != nil {
				return
			}
			for _, j := range list.Items {
				b.Jobs = append(b.Jobs, j.value())
			}
		}
	})
	wg.Go(func() {
		for _, scope := range scopes {
			var list struct {
				Items []cronJob `json:"items"`
			}
			if b.CronJobsErr = get(ctx, r, env, t, append([]string{"cronjobs.batch"}, scope...), &list); b.CronJobsErr != nil {
				return
			}
			for _, j := range list.Items {
				m := j.Metadata
				b.CronJobs = append(b.CronJobs, CronJob{Namespace: m.Namespace, Name: m.Name, UID: m.UID, Schedule: j.Spec.Schedule,
					TimeZone: j.Spec.TimeZone, Suspended: j.Spec.Suspend, Active: len(j.Status.Active)})
			}
		}
	})
	wg.Wait()
	if b.JobsErr != nil {
		b.Jobs = nil
	}
	if b.CronJobsErr != nil {
		b.CronJobs = nil
	}
	slices.SortFunc(b.Jobs, func(a, b Job) int {
		if order := b.Created.Compare(a.Created); order != 0 {
			return order
		}
		return cmp.Compare(a.Namespace+"/"+a.Name, b.Namespace+"/"+b.Name)
	})
	runs := make(map[string][]Job)
	for _, j := range b.Jobs {
		if j.CronJobUID != "" && len(runs[j.CronJobUID]) < 10 {
			runs[j.CronJobUID] = append(runs[j.CronJobUID], j)
		}
	}
	for i, j := range b.CronJobs {
		b.CronJobs[i].Runs = runs[j.UID]
	}
	slices.SortFunc(b.CronJobs, func(a, b CronJob) int { return cmp.Compare(a.Namespace+"/"+a.Name, b.Namespace+"/"+b.Name) })
	return b
}
