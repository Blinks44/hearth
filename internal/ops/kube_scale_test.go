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

func scaleTestObject(uid, version string, desired, actual, ready, generation, observed int) string {
	return fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"demo","namespace":"work",
		"uid":%q,"resourceVersion":%q,"generation":%d},"spec":{"replicas":%d},
		"status":{"replicas":%d,"readyReplicas":%d,"observedGeneration":%d}}`,
		uid, version, generation, desired, actual, ready, observed)
}

func TestPlanKubeScale(t *testing.T) {
	t.Parallel()
	const preconditions = "scale deployment.apps/demo --namespace=work --current-replicas=3 --replicas=4 --resource-version=17"
	ref := KubeResource{Kubectl: "kubectl", Context: "staging", Kind: "Deployment", Namespace: "work", Name: "demo", UID: "uid"}
	before := scaleTestObject("uid", "17", 3, 3, 3, 1, 1)
	for _, tc := range []struct {
		name, beforeApply, afterApply, message string
		noHPAAccess, cancelAfterWrite, timeout bool
		wantWrites                             int
		want                                   Phase
	}{
		{name: "ready", beforeApply: before, afterApply: scaleTestObject("uid", "18", 4, 4, 4, 2, 2), wantWrites: 1, want: Succeeded},
		{name: "timeout after server applied", beforeApply: before, afterApply: scaleTestObject("uid", "18", 4, 4, 4, 2, 2), timeout: true, wantWrites: 1, want: Succeeded},
		{name: "requested but not all ready", beforeApply: before, afterApply: scaleTestObject("uid", "18", 4, 4, 3, 2, 2),
			cancelAfterWrite: true, wantWrites: 1, want: NotVerified, message: locales.KubeScalePending(4, 3, 4)},
		{name: "status not yet observed", beforeApply: before, afterApply: scaleTestObject("uid", "18", 4, 4, 4, 2, 1),
			cancelAfterWrite: true, wantWrites: 1, want: NotVerified, message: locales.KubeScalePending(4, 4, 4)},
		{name: "rollout has excess pods", beforeApply: before, afterApply: scaleTestObject("uid", "18", 4, 5, 4, 2, 2),
			cancelAfterWrite: true, wantWrites: 1, want: NotVerified, message: locales.KubeScalePending(4, 4, 5)},
		{name: "replica count overwritten", beforeApply: before, afterApply: scaleTestObject("uid", "19", 5, 5, 5, 3, 3),
			wantWrites: 1, want: NotVerified, message: locales.KubeScaleChanged(4, 5)},
		{name: "changed version", beforeApply: scaleTestObject("uid", "18", 3, 3, 3, 1, 1), want: NotVerified},
		{name: "replaced deployment", beforeApply: scaleTestObject("other", "18", 3, 3, 3, 1, 1), want: NotVerified},
		{name: "autoscaler read forbidden", noHPAAccess: true, beforeApply: before,
			afterApply: scaleTestObject("uid", "18", 4, 4, 4, 2, 2), wantWrites: 1, want: Succeeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			executing := false
			writes, previews := 0, 0
			r := resourceRunner(func(ctx context.Context, cmd run.Cmd) (run.Result, error) {
				ok := func(s string) (run.Result, error) { return run.Result{Stdout: []byte(s)}, nil }
				if !slices.Contains(cmd.Args, "--context=staging") {
					t.Fatalf("wrong context: %v", cmd.Args)
				}
				switch {
				case slices.Contains(cmd.Args, "config"):
					return ok("workspace")
				case slices.Contains(cmd.Args, "--raw"):
					if !slices.Contains(cmd.Args, ref.path()) {
						t.Fatalf("wrong path: %v", cmd.Args)
					}
					if writes > 0 {
						if ctx.Err() != nil {
							t.Fatalf("check after write was canceled: %v", ctx.Err())
						}
						return ok(tc.afterApply)
					}
					if executing {
						return ok(tc.beforeApply)
					}
					return ok(before)
				case slices.Contains(cmd.Args, "horizontalpodautoscalers.autoscaling"):
					if tc.noHPAAccess {
						return run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden)")}, nil
					}
					return ok(`{"items":[
						{"metadata":{"name":"unrelated"},"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"other"}}},
						{"metadata":{"name":"demo-hpa"},"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"Deployment","name":"demo"}}}
					]}`)
				case slices.Contains(cmd.Args, "scale"):
					argv := cmd.Args[slices.Index(cmd.Args, "scale"):]
					want := preconditions
					if !executing {
						want += " --dry-run=server"
						previews++
					}
					if strings.Join(argv, " ") != want {
						t.Fatalf("scale argv = %v, want %s", argv, want)
					}
					if executing {
						writes++
						if tc.cancelAfterWrite {
							cancel()
						}
						if tc.timeout {
							return run.Result{}, context.DeadlineExceeded
						}
					}
					return ok("")
				}
				t.Fatalf("unexpected command %v", cmd.Args)
				return run.Result{}, nil
			})
			p, err := PlanKubeScale(ctx, r, nil, ref, 3, 4)
			if err != nil {
				t.Fatal(err)
			}
			if previews != 1 || writes != 0 || p.Steps[0].Kube.DeletesResource() {
				t.Fatalf("preview = %d, writes = %d, plan = %+v", previews, writes, p)
			}
			hpaNote := locales.KubeScaleAutoscalerManages("demo-hpa")
			if tc.noHPAAccess {
				hpaNote = locales.KubeScaleAutoscalerUnknown()
			}
			if !slices.ContainsFunc(p.Notes, func(n Note) bool { return n.Text == hpaNote }) {
				t.Fatalf("missing HPA note: %+v", p.Notes)
			}
			executing = true
			got := Execute(ctx, r, p, io.Discard, func(State) {})
			if got.Phase != tc.want || writes != tc.wantWrites || (tc.message != "" && got.Err != tc.message) {
				t.Fatalf("scale result = %+v, writes = %d", got, writes)
			}
		})
	}
}

func TestPlanKubeScaleStatefulSet(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "staging", Kind: "StatefulSet", Namespace: "work", Name: "db", UID: "sts-uid"}
	stsObject := func(version string, desired, actual, ready, gen, obs int) string {
		return fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"db","namespace":"work",
			"uid":"sts-uid","resourceVersion":%q,"generation":%d},"spec":{"replicas":%d},
			"status":{"replicas":%d,"readyReplicas":%d,"observedGeneration":%d}}`,
			version, gen, desired, actual, ready, obs)
	}
	const wantScale = "scale statefulset.apps/db --namespace=work --current-replicas=2 --replicas=0 --resource-version=9"
	executing := false
	writes, previews := 0, 0
	r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
		ok := func(s string) (run.Result, error) { return run.Result{Stdout: []byte(s)}, nil }
		switch {
		case slices.Contains(cmd.Args, "config"):
			return ok("workspace")
		case slices.Contains(cmd.Args, "--raw"):
			if !slices.Contains(cmd.Args, ref.path()) {
				t.Fatalf("wrong path: %v", cmd.Args)
			}
			if writes > 0 {
				return ok(stsObject("10", 0, 0, 0, 2, 2))
			}
			return ok(stsObject("9", 2, 2, 2, 1, 1))
		case slices.Contains(cmd.Args, "horizontalpodautoscalers.autoscaling"):
			return ok(`{"items":[
				{"metadata":{"name":"db-hpa"},"spec":{"scaleTargetRef":{"apiVersion":"apps/v1","kind":"StatefulSet","name":"db"}}}
			]}`)
		case slices.Contains(cmd.Args, "scale"):
			argv := cmd.Args[slices.Index(cmd.Args, "scale"):]
			want := wantScale
			if !executing {
				want += " --dry-run=server"
				previews++
			} else {
				writes++
			}
			if strings.Join(argv, " ") != want {
				t.Fatalf("scale argv = %v, want %s", argv, want)
			}
			return ok("")
		}
		t.Fatalf("unexpected command: %v", cmd.Args)
		return run.Result{}, nil
	})
	p, err := PlanKubeScale(t.Context(), r, nil, ref, 2, 0)
	if err != nil {
		t.Fatalf("PlanKubeScale StatefulSet: %v", err)
	}
	if previews != 1 || writes != 0 {
		t.Fatalf("previews=%d writes=%d", previews, writes)
	}
	wantStops := locales.KubeScaleWorkloadStops("StatefulSet")
	wantHPA := locales.KubeScaleWorkloadAutoscalerManages("db-hpa", "StatefulSet")
	if !slices.ContainsFunc(p.Notes, func(n Note) bool { return n.Text == wantStops }) {
		t.Fatalf("missing stop note %q in %+v", wantStops, p.Notes)
	}
	if !slices.ContainsFunc(p.Notes, func(n Note) bool { return n.Text == wantHPA }) {
		t.Fatalf("missing HPA note %q in %+v", wantHPA, p.Notes)
	}
	executing = true
	got := Execute(t.Context(), r, p, io.Discard, func(State) {})
	if got.Phase != Succeeded || writes != 1 {
		t.Fatalf("StatefulSet scale result = %+v, writes = %d", got, writes)
	}
}
