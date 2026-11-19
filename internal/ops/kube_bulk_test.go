package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

func TestPlanKubeBulk(t *testing.T) {
	t.Parallel()
	refs := []KubeResource{
		{Kubectl: "kubectl", Context: "test", Kind: "Pod", Namespace: "one", Name: "web", UID: "uid-one"},
		{Kubectl: "kubectl", Context: "test", Kind: "Pod", Namespace: "two", Name: "web", UID: "uid-two"},
	}
	for _, tc := range []struct {
		name, owner           string
		stale, remap, restart bool
		want                  []StepStatus
	}{
		{name: "both deleted", want: []StepStatus{Done, Done}},
		{name: "replacement skipped and next target deleted", stale: true, want: []StepStatus{StepFailed, Done}},
		{name: "remapped context", remap: true, want: []StepStatus{Skipped, Skipped}},
		{name: "replace managed pods", owner: "ReplicaSet", restart: true, want: []StepStatus{Done, Done}},
		{name: "standalone pod cannot be recreated", restart: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			configReads, previews := 0, 0
			executing := false
			written := map[string]bool{}
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				if slices.Contains(cmd.Args, "config") {
					configReads++
					if executing && tc.remap {
						return run.Result{Stdout: []byte("other cluster")}, nil
					}
					return run.Result{Stdout: []byte("test cluster")}, nil
				}
				for _, ref := range refs {
					if !slices.Contains(cmd.Args, ref.path()) {
						continue
					}
					if slices.Contains(cmd.Args, "get") {
						uid := ref.UID
						if written[uid] {
							return run.Result{ExitCode: 1, Stderr: []byte("Error from server (NotFound): pod gone")}, nil
						}
						if executing && tc.stale && ref == refs[0] {
							uid = "replacement"
						}
						return run.Result{Stdout: fmt.Appendf(nil, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":%q,"namespace":%q,
							"uid":%q,"resourceVersion":"17","ownerReferences":[{"controller":true,"kind":%q}]}}`, ref.Name, ref.Namespace, uid, tc.owner)}, nil
					}
					var opts struct {
						DryRun        []string
						Preconditions struct{ UID, ResourceVersion string }
					}
					if err := json.NewDecoder(cmd.Stdin).Decode(&opts); err != nil ||
						opts.Preconditions.UID != ref.UID || opts.Preconditions.ResourceVersion != "17" {
						t.Fatalf("missing UID or resourceVersion in delete: %+v, %v", opts, err)
					}
					if !executing && slices.Equal(opts.DryRun, []string{"All"}) {
						previews++
						return run.Result{}, nil
					}
					if !executing || len(opts.DryRun) != 0 || tc.stale && ref == refs[0] {
						t.Fatalf("unexpected write: %v", cmd.Args)
					}
					written[ref.UID] = true
					return run.Result{}, nil
				}
				t.Fatalf("unexpected command: %v", cmd.Args)
				return run.Result{}, nil
			})
			p, err := PlanKubeBulk(t.Context(), r, nil, refs, tc.restart)
			if tc.want == nil {
				if err == nil || previews != 0 {
					t.Fatalf("unmanaged pod: %v, dry runs = %d", err, previews)
				}
				return
			}
			if err != nil || previews != 2 || len(p.Steps) != 2 || p.Steps[0].Kube.UID() != refs[0].UID {
				t.Fatalf("unexpected plan: %+v, dry runs = %d, err = %v", p, previews, err)
			}
			executing = true
			result := Execute(t.Context(), r, p, io.Discard, func(State) {})
			if !slices.Equal(result.Steps, tc.want) || configReads != 2 {
				t.Fatalf("unexpected results: %+v, config reads = %d", result, configReads)
			}
			for i, status := range tc.want {
				if status == Done && (!written[refs[i].UID] || result.StepDetails[i] != locales.KubeResourceDeleted()) {
					t.Fatalf("unverified target %s: %+v", refs[i].UID, result)
				}
			}
			if tc.stale && (result.Phase != NotVerified || result.StepDetails[0] != locales.KubeResourceChanged()) {
				t.Fatalf("stale target: %+v", result)
			}
		})
	}
}
