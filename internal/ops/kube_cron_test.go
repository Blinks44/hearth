package ops

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

func TestPlanKubeCronSuspend(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "test", Kind: "CronJob", Namespace: "work", Name: "demo", UID: "cron"}
	for _, suspend := range []bool{true, false} {
		t.Run(fmt.Sprintf("suspend=%t", suspend), func(t *testing.T) {
			t.Parallel()
			object := func(version string, value bool) run.Result {
				return run.Result{Stdout: fmt.Appendf(nil, `{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"name":"demo","namespace":"work","uid":"cron","resourceVersion":%q},"spec":{"suspend":%t}}`, version, value)}
			}
			writes, previews := 0, 0
			confirmed := false
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				switch {
				case slices.Contains(cmd.Args, "config"):
					return run.Result{Stdout: []byte("test context")}, nil
				case slices.Contains(cmd.Args, ref.path()):
					if writes > 0 {
						return object("2", suspend), nil
					}
					return object("1", !suspend), nil
				case slices.Contains(cmd.Args, "patch"):
					patch := fmt.Sprintf(`--patch={"metadata":{"resourceVersion":"1","uid":"cron"},"spec":{"suspend":%t}}`, suspend)
					if !slices.Contains(cmd.Args, "cronjob.batch/demo") || !slices.Contains(cmd.Args, patch) {
						t.Fatalf("unexpected patch: %v", cmd.Args)
					}
					if slices.Contains(cmd.Args, "--dry-run=server") {
						previews++
						return object("1", suspend), nil
					}
					if !confirmed {
						t.Fatal("write before confirmation")
					}
					writes++
					return object("2", suspend), nil
				}
				t.Fatalf("unexpected command: %v", cmd.Args)
				return run.Result{}, nil
			})
			p, err := PlanKubeCronSuspend(t.Context(), r, nil, ref, suspend)
			if err != nil || previews != 1 || len(p.Steps[0].Kube.Diff) != 2 {
				t.Fatalf("plan = %+v, previews = %d, err = %v", p, previews, err)
			}
			confirmed = true
			if result := Execute(t.Context(), r, p, io.Discard, func(State) {}); result.Phase != Succeeded || writes != 1 {
				t.Fatalf("result = %+v, writes = %d", result, writes)
			}
		})
	}
}

func TestPlanKubeCronRun(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "test", Kind: "CronJob", Namespace: "work", Name: "demo", UID: "cron"}
	for _, tc := range []struct {
		name                         string
		stale, replaced, interrupted bool
	}{
		{name: "manual job starts while schedule is suspended"},
		{name: "stale template", stale: true},
		{name: "job replaced before verification", replaced: true},
		{name: "creation response lost", interrupted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var generated, previewInput string
			writes := 0
			confirmed := false
			job := func(uid, status string) run.Result {
				return run.Result{Stdout: fmt.Appendf(nil, `{"apiVersion":"batch/v1","kind":"Job","metadata":{"name":%q,"namespace":"work","uid":%q,"resourceVersion":"2","ownerReferences":[{"apiVersion":"batch/v1","kind":"CronJob","name":"demo","uid":"cron","controller":true}]},"spec":{}%s}`, generated, uid, status)}
			}
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				switch {
				case slices.Contains(cmd.Args, "config"):
					return run.Result{Stdout: []byte("test context")}, nil
				case slices.Contains(cmd.Args, ref.path()):
					version := "1"
					if confirmed && tc.stale {
						version = "2"
					}
					return run.Result{Stdout: fmt.Appendf(nil, `{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"name":"demo","namespace":"work","uid":"cron","resourceVersion":%q},"spec":{"suspend":true}}`, version)}, nil
				case slices.Contains(cmd.Args, "--dry-run=client"):
					generated = cmd.Args[slices.Index(cmd.Args, "job")+1]
					if !strings.HasPrefix(generated, "demo-manual-") || !slices.Contains(cmd.Args, "--from=cronjob/demo") {
						t.Fatalf("unexpected generator: %v", cmd.Args)
					}
					return job("", ""), nil
				case slices.Contains(cmd.Args, "create") && slices.Contains(cmd.Args, "-f"):
					input, err := io.ReadAll(cmd.Stdin)
					if err != nil {
						t.Fatal(err)
					}
					if slices.Contains(cmd.Args, "--dry-run=server") {
						previewInput = string(input)
						return job("preview", ""), nil
					}
					if !confirmed || previewInput == "" || string(input) != previewInput {
						t.Fatalf("unapproved manifest: %s", input)
					}
					writes++
					if tc.interrupted {
						return run.Result{}, context.Canceled
					}
					return job("job", ""), nil
				case slices.Contains(cmd.Args, "/apis/batch/v1/namespaces/work/jobs/"+generated):
					uid := "job"
					if tc.replaced {
						uid = "another-job"
					}
					return job(uid, `,"status":{"startTime":"2026-11-19T12:00:00Z"}`), nil
				}
				t.Fatalf("unexpected command: %v", cmd.Args)
				return run.Result{}, nil
			})
			p, err := PlanKubeCronRun(t.Context(), r, nil, ref)
			if err != nil || p.Steps[0].Kube.ConfirmName() != "" {
				t.Fatalf("plan = %+v, err = %v", p, err)
			}
			confirmed = true
			got := Execute(t.Context(), r, p, io.Discard, func(State) {})
			switch {
			case tc.stale:
				if writes != 0 || got.Phase != NotVerified || !strings.Contains(got.Err, locales.KubeResourceChanged()) {
					t.Fatalf("stale template = %+v, writes = %d", got, writes)
				}
			case tc.replaced || tc.interrupted:
				if writes != 1 || got.Phase != NotVerified || !strings.Contains(got.Err, generated) {
					t.Fatalf("unverified Job = %+v, writes = %d", got, writes)
				}
			default:
				if writes != 1 || got.Phase != Succeeded {
					t.Fatalf("started Job = %+v, writes = %d", got, writes)
				}
			}
		})
	}
}
