package ops

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

func TestKubeDrainPlan(t *testing.T) {
	prev := drainPoll
	drainPoll = time.Millisecond
	t.Cleanup(func() { drainPoll = prev })
	ref := KubeResource{Kubectl: "kubectl", Context: "test", Kind: "Node", Name: "worker", UID: "node-uid"}
	node := func(cordoned bool) string {
		return fmt.Sprintf(`{"apiVersion":"v1","kind":"Node","metadata":{"name":"worker","uid":"node-uid","resourceVersion":"17"},"spec":{"unschedulable":%t},"status":{"conditions":[{"type":"Ready","status":"True"}]}}`, cordoned)
	}
	const pod = `{"metadata":{"namespace":"work","name":"web","uid":"pod-uid","ownerReferences":[{"kind":"ReplicaSet","name":"web","controller":true}]},"spec":{"nodeName":"worker"}}`
	const daemon = `{"metadata":{"namespace":"work","name":"agent","uid":"daemon-uid","ownerReferences":[{"kind":"DaemonSet","name":"agent","controller":true}]},"spec":{"nodeName":"worker"}}`
	for _, name := range []string{"success", "replaced", "emptyDir"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			executing, cordoned, evicted := false, false, false
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				result := func(s string) run.Result { return run.Result{Stdout: []byte(s)} }
				switch {
				case slices.Contains(cmd.Args, "config"):
					return result("test context"), nil
				case slices.Contains(cmd.Args, ref.path()):
					return result(node(cordoned)), nil
				case slices.Contains(cmd.Args, "--field-selector=spec.nodeName=worker"):
					items := daemon
					if !evicted {
						candidate := pod
						if executing && name == "replaced" {
							candidate = strings.Replace(candidate, "pod-uid", "replacement", 1)
						}
						if name == "emptyDir" {
							candidate = strings.Replace(candidate, `"nodeName":"worker"`, `"nodeName":"worker","volumes":[{"name":"data","emptyDir":{}}]`, 1)
						}
						items += "," + candidate
					}
					return result(`{"items":[` + items + `]}`), nil
				case slices.Contains(cmd.Args, "patch"):
					if slices.Contains(cmd.Args, "--dry-run=server") {
						return result(node(true)), nil
					}
					if !executing || name != "success" {
						t.Fatalf("unexpected cordon: %v", cmd.Args)
					}
					cordoned = true
					return result(node(true)), nil
				case slices.Contains(cmd.Args, "/api/v1/namespaces/work/pods/web/eviction"):
					data, err := io.ReadAll(cmd.Stdin)
					if err != nil || !strings.Contains(string(data), `"uid":"pod-uid"`) {
						t.Fatalf("missing eviction precondition: %q, %v", data, err)
					}
					if notDry := !strings.Contains(string(data), "dryRun"); notDry != executing {
						t.Fatalf("unexpected eviction: %q", data)
					}
					if !executing {
						return run.Result{ExitCode: 1, Stderr: []byte("Error from server (TooManyRequests): disruption budget")}, nil
					}
					if !cordoned || name != "success" {
						t.Fatalf("eviction before cordon or with stale plan: %v", cmd.Args)
					}
					evicted = true
					return result("{}"), nil
				}
				t.Fatalf("unexpected command: %v", cmd.Args)
				return run.Result{}, nil
			})
			p, err := PlanKubeDrain(t.Context(), r, nil, ref)
			if name == "emptyDir" {
				if err == nil || !strings.Contains(err.Error(), "work/web") || !strings.Contains(err.Error(), "emptyDir") {
					t.Fatalf("expected emptyDir to block drain: %v", err)
				}
				return
			}
			if err != nil || len(p.Notes) != 2 || !strings.Contains(p.Notes[0].Text, "DaemonSet") ||
				!strings.Contains(p.Notes[1].Text, "disruption budget") {
				t.Fatalf("unexpected plan: %+v, %v", p.Notes, err)
			}
			if p.Steps[0].Kube.DeletesResource() {
				t.Fatal("drain must not be classified as a resource deletion")
			}
			executing = true
			got := Execute(t.Context(), r, p, io.Discard, func(State) {})
			if name == "replaced" {
				if got.Phase != NotVerified || cordoned || evicted {
					t.Fatalf("stale plan wrote: %+v, cordoned=%v evicted=%v", got, cordoned, evicted)
				}
				return
			}
			if got.Phase != Succeeded || !cordoned || !evicted {
				t.Fatalf("unexpected drain result: %+v, cordoned=%v evicted=%v", got, cordoned, evicted)
			}
		})
	}
}
