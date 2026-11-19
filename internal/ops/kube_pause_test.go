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

func TestPlanKubePause(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "test", Kind: "Deployment", Namespace: "work", Name: "demo", UID: "uid"}
	object := func(version string, paused bool, replicas int) string {
		pause := ""
		if paused {
			pause = `,"paused":true`
		}
		return fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"demo","namespace":"work","uid":"uid","resourceVersion":%q},"spec":{"replicas":%d%s}}`, version, replicas, pause)
	}
	for _, tc := range []struct {
		name                          string
		resume, stale, changed, drift bool
	}{
		{name: "pause"},
		{name: "resume", resume: true},
		{name: "stale preview", stale: true},
		{name: "state already changed", changed: true},
		{name: "dry run changes other fields", drift: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			previews, writes := 0, 0
			executing := false
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				ok := func(s string) (run.Result, error) { return run.Result{Stdout: []byte(s)}, nil }
				switch {
				case slices.Contains(cmd.Args, "config"):
					return ok("test context")
				case slices.Contains(cmd.Args, ref.path()):
					if writes > 0 {
						return ok(object("18", !tc.resume, 2))
					}
					if executing && tc.stale {
						return ok(object("18", tc.resume, 2))
					}
					return ok(object("17", tc.resume != tc.changed, 1))
				case slices.Contains(cmd.Args, "patch"):
					patch := fmt.Sprintf(`--patch={"metadata":{"resourceVersion":"17","uid":"uid"},"spec":{"paused":%t}}`, !tc.resume)
					for _, arg := range []string{"deployment.apps/demo", "--namespace=work", patch} {
						if !slices.Contains(cmd.Args, arg) {
							t.Fatalf("missing %s in %v", arg, cmd.Args)
						}
					}
					if slices.Contains(cmd.Args, "--dry-run=server") && !executing {
						previews++
						if tc.drift {
							return ok(object("17", !tc.resume, 2))
						}
						return ok(object("17", !tc.resume, 1))
					}
					if !executing {
						t.Fatal("write before confirmation")
					}
					writes++
					return ok("")
				}
				t.Fatalf("unexpected command: %v", cmd.Args)
				return run.Result{}, nil
			})
			p, err := PlanKubePause(t.Context(), r, nil, ref, !tc.resume)
			switch {
			case tc.changed:
				if err == nil || err.Error() != locales.KubeResourceChanged() || previews != 0 {
					t.Fatalf("unexpected preview for changed state: %v (%d previews)", err, previews)
				}
				return
			case tc.drift:
				if err == nil || err.Error() != locales.KubeResourceUnverified() || previews != 1 {
					t.Fatalf("unexpected preview for extra changes: %v (%d previews)", err, previews)
				}
				return
			case err != nil || previews != 1:
				t.Fatalf("plan error: %v (%d previews)", err, previews)
			}
			executing = true
			got := Execute(t.Context(), r, p, io.Discard, func(State) {})
			if tc.stale {
				if got.Phase != NotVerified || writes != 0 || !strings.Contains(got.Err, locales.KubeResourceChanged()) {
					t.Fatalf("stale plan: %+v, writes = %d", got, writes)
				}
				return
			}
			if got.Phase != Succeeded || writes != 1 {
				t.Fatalf("result = %+v, writes = %d", got, writes)
			}
		})
	}
}
