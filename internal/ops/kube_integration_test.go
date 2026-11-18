//go:build integration

package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Only temporary kubeconfigs are touched; kubectl must not contact a cluster
// or run the deliberately invalid authentication hook while switching.
func TestKubeContextIntegration(t *testing.T) {
	env := sysenv.GUI()
	paths := sysenv.LookPathAll("kubectl", env)
	if len(paths) == 0 {
		t.Skip("kubectl not installed")
	}
	config := filepath.Join(t.TempDir(), "config")
	data := `apiVersion: v1
kind: Config
current-context: staging
contexts:
- name: staging
  context:
    cluster: stage
    user: auth
- name: production
  context:
    cluster: prod
    user: auth
clusters:
- name: stage
  cluster:
    server: https://127.0.0.1:1
- name: prod
  cluster:
    server: https://127.0.0.1:1
users:
- name: auth
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: hearth-auth-must-not-run
      interactiveMode: Never
`
	if err := os.WriteFile(config, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	// A last environment entry overrides any inherited KUBECONFIG.
	env = append(env, "KUBECONFIG="+config)
	p, err := KubeContextPlan(paths[0], "staging", "production", env, run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	var journal strings.Builder
	got := Execute(t.Context(), run.Exec{}, p, &journal, func(State) {})
	if got.Phase != Succeeded {
		t.Fatalf("result: %+v, journal: %s", got, journal.String())
	}
	updated, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "current-context: production") || !strings.Contains(string(updated), "hearth-auth-must-not-run") {
		t.Fatalf("unexpected config: %s", updated)
	}
}

// With KUBECONFIG naming two files, the namespace goes to the file that
// defines the context and the other file stays as it was; the check reads
// it back through kubectl without contacting a cluster.
func TestKubeNamespaceIntegration(t *testing.T) {
	env := sysenv.GUI()
	paths := sysenv.LookPathAll("kubectl", env)
	if len(paths) == 0 {
		t.Skip("kubectl not installed")
	}
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	firstData := `apiVersion: v1
kind: Config
current-context: staging
contexts:
- name: staging
  context:
    cluster: stage
    user: auth
clusters:
- name: stage
  cluster:
    server: https://127.0.0.1:1
users:
- name: auth
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: hearth-auth-must-not-run
      interactiveMode: Never
`
	secondData := `apiVersion: v1
kind: Config
contexts:
- name: production
  context:
    cluster: prod
    user: auth
    namespace: shop
clusters:
- name: prod
  cluster:
    server: https://127.0.0.1:1
users:
- name: auth
  user:
    token: keep-me
`
	for file, data := range map[string]string{first: firstData, second: secondData} {
		if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env = append(env, "KUBECONFIG="+first+string(os.PathListSeparator)+second)
	p, err := KubeNamespacePlan(paths[0], "production", "shop", "payments", second, env, run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	var journal strings.Builder
	got := Execute(t.Context(), run.Exec{}, p, &journal, func(State) {})
	if got.Phase != Succeeded {
		t.Fatalf("result: %+v, journal: %s", got, journal.String())
	}
	if data, _ := os.ReadFile(first); string(data) != firstData {
		t.Fatalf("first file changed:\n%s", data)
	}
	data, _ := os.ReadFile(second)
	if !strings.Contains(string(data), "namespace: payments") || !strings.Contains(string(data), "keep-me") {
		t.Fatalf("second file:\n%s", data)
	}
}
