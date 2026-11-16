package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestKubeContextsMerge(t *testing.T) {
	home := t.TempDir()
	first, second := filepath.Join(home, "first"), filepath.Join(home, "second")
	writeFile(t, first, `current-context: staging
contexts:
- name: production
  context:
    cluster: prod-first
    namespace: digital
- name: staging
  context:
    cluster: stage
users:
- name: private
  user:
    token: DO-NOT-RETAIN
`)
	writeFile(t, second, `current-context: production
contexts:
- name: production
  context:
    cluster: prod-second
- name: local
  context:
    cluster: local-cluster
`)
	env := sysenv.Env{"KUBECONFIG=" + first + string(os.PathListSeparator) + second}
	contexts, current := kubeContexts(env, home)
	if current == nil || current.Name != "staging" || current.File != first {
		t.Fatalf("current: %+v", current)
	}
	if len(contexts) != 3 || contexts[0].Name != "local" || contexts[1].Cluster != "prod-first" || contexts[1].Namespace != "digital" {
		t.Fatalf("contexts: %+v", contexts)
	}
	data, err := json.Marshal(contexts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "DO-NOT-RETAIN") {
		t.Fatal("credentials retained")
	}
	// Re-read after a terminal changes selection; no cached context survives.
	writeFile(t, first, "current-context: local\n")
	_, current = kubeContexts(env, home)
	if current == nil || current.Name != "local" {
		t.Fatalf("stale current: %+v", current)
	}
	// A blank KUBECONFIG entry must not replace the default config.
	env = sysenv.Env{"KUBECONFIG=" + string(os.PathListSeparator)}
	if got := KubeConfigFiles(env, home); len(got) != 1 || got[0] != filepath.Join(home, ".kube", "config") {
		t.Fatalf("default files: %v", got)
	}
}
