//go:build integration

package detect

import (
	"os"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// HEARTH_KUBE_LIVE_CONFIG names a kubeconfig, made in a VM, whose contexts
// are named for what a check must find in them: a live cluster (kind)
// reached with full rights, with none, with a wrong token, an address
// nothing answers at, a sign-in program that is not there and one that
// says its session expired. Only that file is read; nothing in the
// cluster changes.
func TestCheckKubeAccessLive(t *testing.T) {
	config := os.Getenv("HEARTH_KUBE_LIVE_CONFIG")
	if config == "" {
		t.Skip("HEARTH_KUBE_LIVE_CONFIG not set")
	}
	env := append(sysenv.GUI(), "KUBECONFIG="+config)
	paths := sysenv.LookPathAll("kubectl", env)
	if len(paths) == 0 {
		t.Fatal("kubectl not found")
	}
	want := map[string]KubeAccessState{
		"ok": KubeReachable, "forbidden": KubeForbidden, "unauthorized": KubeNeedsLogin,
		"unreachable": KubeUnreachable, "noprogram": KubeNoAuthProgram, "expired": KubeNeedsLogin, "failing": KubeAuthFailed,
	}
	kc := readKubeConfig(env, "")
	for _, k := range kc.contexts {
		state, ok := want[k.Name]
		if !ok {
			continue
		}
		delete(want, k.Name)
		t.Run(k.Name, func(t *testing.T) {
			got := CheckKubeAccess(t.Context(), run.Exec{}, paths[0], k, env)
			t.Logf("%s: state %d, version %q, detail %q, %v", k.Name, got.State, got.Version, got.Detail, got.Took)
			if got.State != state {
				t.Errorf("state %d, want %d", got.State, state)
			}
		})
	}
	if len(want) > 0 {
		t.Errorf("contexts missing from %s: %v", config, want)
	}
}
