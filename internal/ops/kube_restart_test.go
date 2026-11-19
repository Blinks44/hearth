package ops

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

func TestPlanKubeRestart(t *testing.T) {
	t.Parallel()
	ref := KubeResource{Kubectl: "kubectl", Context: "staging", Kind: "Deployment", Namespace: "work", Name: "demo", UID: "uid"}
	object := func(uid, version, stamp, image string, paused bool) string {
		return fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"demo","namespace":"work",
			"uid":%q,"resourceVersion":%q},"spec":{"replicas":1,"paused":%t,"template":{"metadata":{"annotations":
			{"kubectl.kubernetes.io/restartedAt":%q}},"spec":{"containers":[{"name":"demo","image":%q}]}}}}`, uid, version, paused, stamp, image)
	}
	for _, tc := range []struct {
		name, currentUID, currentVersion, rolloutError, message string
		paused, timeout, superseded                             bool
		wantWrites, wantChecks                                  int
		wantPhase                                               Phase
	}{
		{name: "completed", wantWrites: 1, wantChecks: 1, wantPhase: Succeeded},
		{name: "write timed out after acceptance", timeout: true, wantWrites: 1, wantChecks: 1, wantPhase: Succeeded},
		{name: "progress deadline", rolloutError: "exceeded its progress deadline", message: "exceeded its progress deadline",
			wantWrites: 1, wantChecks: 1, wantPhase: NotVerified},
		{name: "another rollout followed", superseded: true, message: locales.KubeResourceChanged(),
			wantWrites: 1, wantChecks: 1, wantPhase: NotVerified},
		{name: "changed version", currentVersion: "18", message: locales.KubeResourceChanged(), wantPhase: NotVerified},
		{name: "recreated Deployment", currentUID: "replacement", message: locales.KubeResourceChanged(), wantPhase: NotVerified},
		{name: "paused Deployment", paused: true, message: locales.KubeRestartPaused()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stamp, approvedPatch string
			writes, previews, checks := 0, 0, 0
			executing := false
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				ok := func(s string) (run.Result, error) { return run.Result{Stdout: []byte(s)}, nil }
				if !slices.Contains(cmd.Args, "--context=staging") {
					t.Fatalf("wrong context: %v", cmd.Args)
				}
				switch {
				case slices.Contains(cmd.Args, "config"):
					return ok("workspace")
				case slices.Contains(cmd.Args, "--raw"):
					if !slices.Contains(cmd.Args, ref.path()) {
						t.Fatalf("wrong resource: %v", cmd.Args)
					}
					if writes > 0 {
						image := "v1"
						if checks > 0 && tc.superseded {
							image = "v2"
						}
						return ok(object("uid", "18", stamp, image, false))
					}
					if executing {
						return ok(object(cmp.Or(tc.currentUID, "uid"), cmp.Or(tc.currentVersion, "17"), "old", "v1", false))
					}
					return ok(object("uid", "17", "old", "v1", tc.paused))
				case slices.Contains(cmd.Args, "patch"):
					if !slices.Contains(cmd.Args, "deployment.apps/demo") || !slices.Contains(cmd.Args, "--namespace=work") {
						t.Fatalf("wrong patch target: %v", cmd.Args)
					}
					flag := slices.IndexFunc(cmd.Args, func(arg string) bool { return strings.HasPrefix(arg, "--patch=") })
					if flag < 0 {
						t.Fatalf("missing patch: %v", cmd.Args)
					}
					patch := strings.TrimPrefix(cmd.Args[flag], "--patch=")
					var body resourceJSON
					if err := json.Unmarshal([]byte(patch), &body); err != nil || body.metadata()["uid"] != "uid" || body.version() != "17" {
						t.Fatalf("patch with missing preconditions: %s", patch)
					}
					if slices.Contains(cmd.Args, "--dry-run=server") && !executing {
						previews++
						approvedPatch = patch
						stamp, _ = restartTime(body).(string)
						if stamp == "" || stamp == "old" {
							t.Fatalf("missing restart annotation: %s", patch)
						}
						return ok(object("uid", "17", stamp, "v1", false))
					}
					if !executing || patch != approvedPatch {
						t.Fatalf("unexpected live patch: %v", cmd.Args)
					}
					writes++
					if tc.timeout {
						return run.Result{}, context.DeadlineExceeded
					}
					return ok("")
				case slices.Contains(cmd.Args, "status"):
					checks++
					if tc.rolloutError != "" {
						return run.Result{ExitCode: 1, Stderr: []byte(tc.rolloutError)}, nil
					}
					return ok("")
				}
				t.Fatalf("unexpected command: %v", cmd.Args)
				return run.Result{}, nil
			})
			plan, err := PlanKubeRestart(t.Context(), r, nil, ref)
			if tc.paused {
				if err == nil || err.Error() != tc.message || previews != 0 || writes != 0 {
					t.Fatalf("plan for a paused Deployment: %v, previews = %d, writes = %d", err, previews, writes)
				}
				return
			}
			if err != nil || previews != 1 || writes != 0 || len(plan.Notes) != 1 || plan.Notes[0].Text != locales.KubeRestartSingle() {
				t.Fatalf("plan = %+v, error = %v, previews = %d, writes = %d", plan, err, previews, writes)
			}
			executing = true
			got := Execute(t.Context(), r, plan, io.Discard, func(State) {})
			if got.Phase != tc.wantPhase || !strings.Contains(got.Err, tc.message) || writes != tc.wantWrites || checks != tc.wantChecks {
				t.Fatalf("result = %+v, writes = %d, checks = %d", got, writes, checks)
			}
		})
	}
}
