package ops

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

func TestPlanKubeRollback(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "test", Kind: "Deployment", Namespace: "work", Name: "demo", UID: "deployment"}
	const current = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"demo","namespace":"work","uid":"deployment","resourceVersion":"10",
		"annotations":{"deployment.kubernetes.io/revision":"3","release":"current"}},"spec":{"replicas":2,
		"template":{"metadata":{"labels":{"app":"demo"}},"spec":{"containers":[{"name":"app","image":"demo:3","env":[{"name":"NEW","value":"1"}]}]}}}}`
	const previous = `{"apiVersion":"apps/v1","kind":"ReplicaSet","metadata":{"name":"demo-old","namespace":"work","uid":"old","resourceVersion":"7",
		"annotations":{"deployment.kubernetes.io/revision":"2","deployment.kubernetes.io/desired-replicas":"0","release":"previous"},
		"ownerReferences":[{"kind":"Deployment","name":"demo","uid":"deployment","controller":true}]},"spec":{"replicas":0,
		"template":{"metadata":{"labels":{"app":"demo","pod-template-hash":"123"}},"spec":{"containers":[{"name":"app","image":"demo:2"}]}}}}`
	for _, tc := range []struct {
		name, current, previous, planError string
		stale, drift, stalled              bool
	}{
		{name: "restores template and verifies rollout"},
		{name: "paused", current: strings.Replace(current, `"replicas":2`, `"replicas":2,"paused":true`, 1), planError: locales.KubeRollbackPaused()},
		{name: "wrong owner UID", previous: strings.Replace(previous, `"uid":"deployment"`, `"uid":"replacement"`, 1), planError: locales.KubeResourceChanged()},
		{name: "renumbered revision", previous: strings.Replace(previous, `"deployment.kubernetes.io/revision":"2"`, `"deployment.kubernetes.io/revision":"4"`, 1), planError: locales.KubeResourceChanged()},
		{name: "admission changes replicas", drift: true, planError: locales.KubeResourceUnverified()},
		{name: "revision replaced after preview", stale: true},
		{name: "rollout stalled", stalled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			deploy, rs := current, previous
			if tc.current != "" {
				deploy = tc.current
			}
			if tc.previous != "" {
				rs = tc.previous
			}
			executing, watched := false, false
			writes := 0
			var preview, applied string
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				ok := func(s string) (run.Result, error) { return run.Result{Stdout: []byte(s)}, nil }
				switch {
				case slices.Contains(cmd.Args, "config"):
					return ok("test config")
				case slices.Contains(cmd.Args, ref.path()):
					if writes == 0 {
						return ok(deploy)
					}
					return ok(applied)
				case slices.Contains(cmd.Args, "/apis/apps/v1/namespaces/work/replicasets/demo-old"):
					if executing && tc.stale {
						return ok(strings.Replace(rs, `"uid":"old"`, `"uid":"new"`, 1))
					}
					return ok(rs)
				case slices.Contains(cmd.Args, "replace"):
					raw, err := io.ReadAll(cmd.Stdin)
					if err != nil {
						t.Fatal(err)
					}
					manifest := string(raw)
					for _, want := range []string{`"resourceVersion":"10"`, `"uid":"deployment"`, `"image":"demo:2"`, `"replicas":2`,
						`"release":"previous"`, `"deployment.kubernetes.io/revision":"3"`} {
						if !strings.Contains(manifest, want) {
							t.Fatalf("missing %s in %s", want, manifest)
						}
					}
					for _, absent := range []string{"pod-template-hash", "demo:3", `"env"`, "desired-replicas"} {
						if strings.Contains(manifest, absent) {
							t.Fatalf("unexpected %s in %s", absent, manifest)
						}
					}
					if slices.Contains(cmd.Args, ref.path()+"?fieldValidation=Strict&dryRun=All") {
						preview = manifest
						if tc.drift {
							return ok(strings.Replace(manifest, `"replicas":2`, `"replicas":5`, 1))
						}
						return ok(manifest)
					}
					if !executing || manifest != preview {
						t.Fatal("write was not reviewed and confirmed")
					}
					writes++
					applied = strings.Replace(manifest, `"resourceVersion":"10"`, `"resourceVersion":"11"`, 1)
					applied = strings.Replace(applied, `"deployment.kubernetes.io/revision":"3"`, `"deployment.kubernetes.io/revision":"4"`, 1)
					return ok(applied)
				case slices.Contains(cmd.Args, "status"):
					watched = true
					if tc.stalled {
						return run.Result{}, errors.New("progress deadline exceeded")
					}
					return ok("rolled out")
				}
				t.Fatalf("unexpected kubectl arguments: %v", cmd.Args)
				return run.Result{}, nil
			})
			plan, err := PlanKubeRollback(t.Context(), r, nil, ref, "demo-old", "old", 2)
			if tc.planError != "" {
				if err == nil || err.Error() != tc.planError || writes != 0 {
					t.Fatalf("error = %v, writes = %d", err, writes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			diff := plan.Steps[0].Kube.Diff
			for _, want := range []struct {
				text  string
				added bool
			}{{"demo:3", false}, {"demo:2", true}, {"NEW", false}} {
				if !slices.ContainsFunc(diff, func(line DiffLine) bool {
					return line.Added == want.added && strings.Contains(line.Text, want.text)
				}) {
					t.Fatalf("missing %+v in diff: %+v", want, diff)
				}
			}
			executing = true
			got := Execute(t.Context(), r, plan, io.Discard, func(State) {})
			switch {
			case tc.stale:
				if got.Phase != NotVerified || writes != 0 || watched {
					t.Fatalf("stale revision: %+v, writes = %d, watched = %t", got, writes, watched)
				}
			case tc.stalled:
				if got.Phase != NotVerified || writes != 1 || !strings.Contains(got.Err, "progress deadline exceeded") {
					t.Fatalf("stalled rollout: %+v, writes = %d", got, writes)
				}
			default:
				if got.Phase != Succeeded || writes != 1 || !watched {
					t.Fatalf("result = %+v, writes = %d, watched = %t", got, writes, watched)
				}
			}
		})
	}
}
