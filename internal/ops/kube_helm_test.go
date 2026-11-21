package ops

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeHelmPlan(t *testing.T) {
	const before = `[{"revision":1,"status":"superseded","chart":"demo-1.0.0"},{"revision":2,"status":"failed","chart":"demo-2.0.0"}]`
	const restored = `[{"revision":3,"status":"deployed","chart":"demo-1.0.0","description":"Rollback to 1"}]`
	tgt := kube.HelmTarget{Context: "dev", Helm: "helm", Namespace: "shop", Name: "demo.web"}
	current := kube.HelmRevision{Revision: 2, Chart: "demo-2.0.0", Status: "failed"}
	for _, tc := range []struct {
		name                         string
		to                           int
		stale, switched, interrupted bool
		want                         Phase
	}{
		{name: "rollback", to: 1, want: Succeeded},
		{name: "uninstall", want: Succeeded},
		{name: "history changed", to: 1, stale: true, want: NotVerified},
		{name: "context changed", switched: true, want: NotVerified},
		{name: "read after interruption", to: 1, interrupted: true, want: NotVerified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reads, writes, configs int
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				if cmd.Args[0] == "kubectl" {
					configs++
					config := "original context"
					if tc.switched && configs > 1 {
						config = "different context"
					}
					return run.Result{Stdout: []byte(config)}, nil
				}
				if !slices.Equal(cmd.Args[:3], tgt.Args()) || !strings.HasPrefix(cmd.Args[3], "--kubeconfig=") ||
					!slices.Equal(cmd.Env, []string{"HELM_DRIVER=secret"}) {
					t.Fatalf("wrong Helm scope: %v  %v", cmd.Args, cmd.Env)
				}
				switch cmd.Args[4] {
				case "history":
					reads++
					switch {
					case tc.stale && reads == 2:
						return run.Result{Stdout: []byte(restored)}, nil
					case reads <= 2:
						return run.Result{Stdout: []byte(before)}, nil
					case tc.to == 0:
						return run.Result{ExitCode: 1, Stderr: []byte("Error: release: not found")}, nil
					default:
						return run.Result{Stdout: []byte(restored)}, nil
					}
				case "rollback", "uninstall":
					writes++
					if tc.to == 0 && (cmd.Args[4] != "uninstall" || !slices.Contains(cmd.Args, "--cascade=foreground")) {
						t.Fatalf("uninstall args: %v", cmd.Args)
					}
					if tc.to > 0 && (cmd.Args[4] != "rollback" || !slices.Equal(cmd.Args[len(cmd.Args)-2:], []string{tgt.Name, "1"})) {
						t.Fatalf("rollback args: %v", cmd.Args)
					}
					if tc.interrupted {
						return run.Result{Stderr: []byte("chart-private-data")}, context.DeadlineExceeded
					}
					return run.Result{Stdout: []byte("chart-private-data")}, nil
				default:
					t.Fatalf("unexpected command: %v", cmd.Args)
					return run.Result{}, nil
				}
			})
			env := sysenv.Env{"HELM_DRIVER=secret", "HELM_KUBETOKEN=discard"}
			p, err := PlanKubeHelm(t.Context(), r, env, "kubectl", tgt, current, tc.to)
			if err != nil || writes != 0 {
				t.Fatalf("plan error: %v, writes: %d", err, writes)
			}
			if got := p.Steps[0].Kube.DeletesResource(); got != (tc.to == 0) {
				t.Fatalf("DeletesResource = %v, want %v", got, tc.to == 0)
			}
			var journal bytes.Buffer
			state := Execute(t.Context(), r, p, &journal, func(State) {})
			wantWrites := 1
			if tc.stale || tc.switched {
				wantWrites = 0
			}
			if state.Phase != tc.want || writes != wantWrites {
				t.Fatalf("execution: %+v, writes: %d", state, writes)
			}
			if tc.interrupted && (reads != 3 || state.Err != locales.KubeHelmCheckRevision(3, "deployed")) {
				t.Fatalf("read after write: %d, result %+v", reads, state)
			}
			if strings.Contains(journal.String(), "chart-private-data") {
				t.Fatal("chart content leaked into the journal")
			}
		})
	}
}
