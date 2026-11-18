package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
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
	broken, missing := filepath.Join(home, "broken"), filepath.Join(home, "missing")
	writeFile(t, broken, "contexts: [\n")
	sep := string(os.PathListSeparator)
	env := sysenv.Env{"KUBECONFIG=" + first + sep + second + sep + sep + first + sep + broken + sep + missing}
	kc := readKubeConfig(env, home)
	if kc.current == nil || kc.current.Name != "staging" || kc.current.File != first {
		t.Fatalf("current: %+v", kc.current)
	}
	// By file in merge order; a later context of a taken name is shadowed.
	want := []inventory.KubeContext{
		{Name: "production", Cluster: "prod-first", Namespace: "digital", File: first},
		{Name: "staging", Cluster: "stage", File: first},
		{Name: "local", Cluster: "local-cluster", File: second},
		{Name: "production", Cluster: "prod-second", File: second, Shadowed: true},
	}
	if !slices.Equal(kc.contexts, want) {
		t.Fatalf("contexts:\n got %+v\nwant %+v", kc.contexts, want)
	}
	files := []inventory.KubeFile{{Path: first}, {Path: second}, {Path: broken, Invalid: true}, {Path: missing, Missing: true}}
	if !slices.Equal(kc.files, files) {
		t.Fatalf("files: %+v", kc.files)
	}
	data, err := json.Marshal(inventory.CliState{KubeContexts: kc.contexts, KubeFiles: kc.files})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "DO-NOT-RETAIN") {
		t.Fatal("credentials retained")
	}
	// Re-read after a terminal changes selection; no cached context survives.
	writeFile(t, first, "current-context: local\n")
	if current := kubeContext(env, home); current == nil || current.Name != "local" {
		t.Fatalf("stale current: %+v", current)
	}
	// A blank KUBECONFIG entry must not replace the default config.
	env = sysenv.Env{"KUBECONFIG=" + string(os.PathListSeparator)}
	if got := KubeConfigFiles(env, home); len(got) != 1 || got[0] != filepath.Join(home, ".kube", "config") {
		t.Fatalf("default files: %v", got)
	}
}
