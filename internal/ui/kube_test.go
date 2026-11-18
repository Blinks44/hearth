package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// kubeApp has two kubeconfig files from KUBECONFIG, the second with a
// context of a name the first has, and kubectl on staging.
func kubeApp() *App {
	a := NewApp()
	a.Home = home
	config, prod := home+"/.kube/config", home+"/.kube/work/prod.yaml"
	brew := func(name, version string) inventory.Installation {
		return inventory.Installation{Tool: name, Version: version, Path: "/opt/homebrew/bin/" + name, OnPath: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/" + name + "/" + version}}
	}
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{Tool: "cli", Cli: &inventory.CliState{
		Tools:    []inventory.Installation{brew("kubectl", "1.31.4"), brew("helm", "3.16.3")},
		Contexts: []inventory.CliContext{{Tool: "kubectl", Name: "staging", File: config}},
		KubeContexts: []inventory.KubeContext{
			{Name: "kind-dev", Cluster: "kind-dev", File: config},
			{Name: "staging", Cluster: "eks-staging", Namespace: "shop", File: config},
			{Name: "production", Cluster: "gke-prod", Namespace: "prod", File: prod},
			{Name: "staging", Cluster: "other", File: prod, Shadowed: true},
		},
		KubeFiles:     []inventory.KubeFile{{Path: config}, {Path: prod}, {Path: home + "/.kube/gone", Missing: true}},
		KubeconfigSet: true,
	}}}}
	return a
}

// Contexts by file in merge order, kubectl's current one and a shadowed
// one marked, the files and the tools in their tabs.
func TestKubePage(t *testing.T) {
	a := kubeApp()
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "kube")
	hasTexts(t, tt, "Kubernetes", "Tools · kubeconfig contexts", "Contexts from 3 kubeconfig files, kubectl and Helm",
		"~/.KUBE/CONFIG", "~/.KUBE/WORK/PROD.YAML", "kind-dev", "cluster kind-dev · namespace default",
		"cluster eks-staging · namespace shop", "in kubectl", "shadowed", "kubectl uses the context of this name from a file above",
		"kubectl in the terminal", "staging · shop", "current-context from ~/.kube/config", "KUBECONFIG: 3 files",
		"kubectl 1.31.4 · helm 3.16.3", "How Hearth works with clusters")
	// Three contexts kubectl can use; the shadowed one is not among them.
	if !slices.Contains(tt.Texts(), "3") {
		t.Errorf("count: %q", tt.Texts())
	}
	a.kubeFilter = "prod"
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("production") || tt.HasText("kind-dev · namespace default") || tt.HasText("cluster eks-staging · namespace shop") {
		t.Errorf("filter: %q", tt.Texts())
	}
	a.kubeFilter, a.kubeTab = "", 1
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "kubectl", "brew · context staging", "helm", "3.16.3")
	a.kubeTab = 2
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "~/.kube/config", "2 contexts · sets current-context staging", "~/.kube/work/prod.yaml", "~/.kube/gone", "no such file")
	screenshot(t, tt, "kube-files")

	// kubectl on a context of the second file, chosen in the first.
	a.Snapshot.Tools[0].Cli.Contexts[0].Name = "production"
	a.kubeTab = 0
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "production · prod", "current-context from ~/.kube/config")
	if tt.HasText("current-context from ~/.kube/work/prod.yaml") {
		t.Error("current-context shown from the file that defines the context")
	}
}

// Using a context and changing its namespace are plans of kubectl's own
// config commands, with the terminal's KUBECONFIG.
func TestKubePlans(t *testing.T) {
	a := kubeApp()
	env := sysenv.Env{"KUBECONFIG=/tmp/a:/tmp/b"}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(env, nil))
	}
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 1000)
	tt.Advance(time.Second)
	if err := tt.Click("Use in kubectl…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Use Kubernetes context kind-dev", "/opt/homebrew/bin/kubectl config use-context -- kind-dev")
	if a.op == nil || a.op.started || a.op.plan.Env[0] != env[0] {
		t.Fatalf("unexpected operation: %+v", a.op)
	}

	a.op = nil
	a.kubeNS = kubeNamespaceDialog{open: true, context: "staging", from: "shop", file: home + "/.kube/config", value: "shop"}
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Namespace of staging")
	a.kubeNS.value = "Bad_Name"
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Use lowercase letters, digits and hyphens, up to 63 characters, starting and ending with a letter or digit.")
	a.kubeNS.value = "payments"
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "kube-namespace")
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Set namespace payments for staging", "/opt/homebrew/bin/kubectl config set-context --namespace=payments -- staging")
	if a.op == nil || a.op.started || a.kubeNS.open {
		t.Fatalf("unexpected operation: %+v", a.op)
	}
}

// Contexts are found from the search and open the page filtered to them;
// kubectl's copies open it too.
func TestKubeSearch(t *testing.T) {
	a := kubeApp()
	var production, kubectl *searchResult
	for _, r := range a.searchIndex() {
		switch {
		case r.id == "kube:production":
			production = &r
		case r.id == "version:cli:/opt/homebrew/bin/kubectl":
			kubectl = &r
		case r.id == "kube:staging" && r.detail != "eks-staging · ~/.kube/config":
			t.Errorf("shadowed context in the search: %+v", r)
		}
	}
	if production == nil || kubectl == nil {
		t.Fatalf("search: production %v, kubectl %v", production, kubectl)
	}
	kubectl.open()
	if a.router.Path() != kubePage {
		t.Errorf("kubectl opens %s", a.router.Path())
	}
	production.open()
	if a.router.Path() != kubePage || a.kubeFilter != "production" || a.kubeTab != 0 {
		t.Errorf("context opens %s, filter %q", a.router.Path(), a.kubeFilter)
	}
}
