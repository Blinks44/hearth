package ui

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
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
			{Name: "staging", Cluster: "eks-staging", Namespace: "shop", File: config, Auth: "aws"},
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
	hasTexts(t, tt, "Kubernetes", "Infrastructure · kubeconfig contexts", "Contexts from 3 kubeconfig files, kubectl, Helm and kind",
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
	a.kubeTab = 3
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
	// kind-dev's menu: kubectl is on staging.
	if err := tt.Click(locales.MoreActions()); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Equal(menu, []string{"Use in kubectl…", "Change namespace…", "-", "Copy context name"}) {
		t.Fatalf("menu %q", menu)
	}
	if err := tt.ChooseMenuItem("Use in kubectl…"); err != nil {
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

// A check runs only on its button, says it runs, and then says what it
// found: the chip, and why in kubectl's or the program's words.
func TestKubeAccessCheck(t *testing.T) {
	a := kubeApp()
	var asked []inventory.KubeContext
	var answer []func(detect.KubeAccess)
	a.CheckKube = func(kubectl string, k inventory.KubeContext, done func(detect.KubeAccess)) {
		if kubectl != "/opt/homebrew/bin/kubectl" {
			t.Errorf("kubectl %q", kubectl)
		}
		asked = append(asked, k)
		answer = append(answer, done)
	}
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	// The sign-in program shows before any check runs it.
	hasTexts(t, tt, "cluster eks-staging · namespace shop · sign-in through aws")
	if len(asked) != 0 || tt.HasText("checking…") {
		t.Fatal("checked without the button")
	}
	// kind-dev's menu checks it.
	if err := tt.Click(locales.MoreActions()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Check access"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "checking…")
	answer[0](detect.KubeAccess{State: detect.KubeReachable, Version: "v1.31.2", Took: 140 * time.Millisecond})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "access")

	for _, tc := range []struct {
		res   detect.KubeAccess
		texts []string
	}{
		{detect.KubeAccess{State: detect.KubeNoAuthProgram, Detail: "aws"}, []string{"no aws", "kubectl runs aws to sign in, and the terminal's PATH has none"}},
		{detect.KubeAccess{State: detect.KubeNeedsLogin, Detail: "error: You must be logged in to the server (Unauthorized)"}, []string{"sign-in needed", "error: You must be logged in to the server (Unauthorized)"}},
		{detect.KubeAccess{State: detect.KubeAuthFailed, Detail: "getting credentials: exec: executable aws failed with exit code 1"}, []string{"no token"}},
		{detect.KubeAccess{State: detect.KubeForbidden, Detail: "kubectl auth can-i list pods -n shop: no"}, []string{"no access to pods", "kubectl auth can-i list pods -n shop: no"}},
		{detect.KubeAccess{State: detect.KubeUnreachable, Detail: "dial tcp: connection refused"}, []string{"not answering"}},
		{detect.KubeAccess{Detail: "x509: certificate signed by unknown authority"}, []string{"check failed", "x509: certificate signed by unknown authority"}},
	} {
		a.checkKube("/opt/homebrew/bin/kubectl", inventory.KubeContext{Name: "staging", Auth: "aws"})
		answer[len(answer)-1](tc.res)
		tt.Frame()
		tt.Advance(time.Second)
		hasTexts(t, tt, tc.texts...)
	}
	screenshot(t, tt, "kube-access")
	// The shadowed staging shows nothing of the other's check.
	n := 0
	for _, s := range tt.Texts() {
		if s == "x509: certificate signed by unknown authority" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the check's detail shows %d times", n)
	}
	// A shadowed context is not checked: kubectl never uses it.
	if n := len(tt.Texts()); n == 0 {
		t.Fatal("no texts")
	}
	// Every context kubectl can use is checked at once, the shadowed one
	// is not.
	if err := tt.Click("Check all"); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1+6+3 {
		t.Errorf("checked %d times, want the 3 contexts kubectl uses", len(asked)-7)
	}
}

// A tool not found installs from the catalog through mise, checked by the
// CLI detector.
func TestKubeInstall(t *testing.T) {
	a := kubeApp()
	env := sysenv.Env{"HOME=" + t.TempDir(), "SHELL=/bin/zsh", "PATH="}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(env, nil))
	}
	a.DetectTool = func(_ context.Context, tool string) (inventory.ToolState, error) {
		if tool != "cli" {
			t.Errorf("detects %q", tool)
		}
		return inventory.ToolState{Tool: "cli", Cli: &inventory.CliState{Tools: []inventory.Installation{{Tool: "kind", Version: "0.29.0", OnPath: 1}}}}, nil
	}
	a.router.Push(kubePage)
	a.kubeTab = 1
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	hasTexts(t, tt, "kind", "not found · kind", locales.AgentInstall())
	screenshot(t, tt, "kube-tools")
	if err := tt.Click(locales.AgentInstall()); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Install kind" || a.op.plan.Verify == nil {
		t.Fatalf("plan %+v, error %q", a.op, a.planErr)
	}
	last := a.op.plan.Steps[len(a.op.plan.Steps)-1]
	if !slices.Contains(last.Argv, "kind@latest") {
		t.Errorf("steps %+v", a.op.plan.Steps)
	}
	if found, err := a.op.plan.Verify(t.Context()); err != nil || found == "" {
		t.Errorf("verify %q, %v", found, err)
	}
}

// kindApp is kubeApp with kind and OrbStack's engine running a cluster.
func kindApp() *App {
	a := kubeApp()
	cli := a.Snapshot.Tools[0].Cli
	cli.Tools = append(cli.Tools, inventory.Installation{Tool: "kind", Version: "0.25.0", Path: "/opt/homebrew/bin/kind", OnPath: 1,
		Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: "/opt/homebrew/Cellar/kind/0.25.0"}})
	a.Snapshot.Tools = append(a.Snapshot.Tools, inventory.ToolState{Tool: "docker", Docker: &inventory.DockerState{
		Selected: "orbstack",
		Contexts: []inventory.DockerContext{{Name: "orbstack", Endpoint: "unix:///orbstack.sock", Engine: "OrbStack", Local: true, Status: "available"}},
		KindClusters: []inventory.KindCluster{
			{Name: "dev", Nodes: 3, Running: 3, Version: "v1.31.2"},
			{Name: "old", Nodes: 1},
		},
	}})
	return a
}

// Local clusters come from the engine's containers; creating and deleting
// one go through a plan.
func TestKindClusters(t *testing.T) {
	a := kindApp()
	env := sysenv.Env{"PATH=/opt/homebrew/bin"}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(env, nil))
	}
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 1000)
	tt.Advance(time.Second)
	screenshot(t, tt, "kind-clusters")
	hasTexts(t, tt, "Local clusters", "dev", "in Docker (OrbStack)", "kind · v1.31.2 · 3 nodes · running", "kind · 1 node · stopped")
	if err := tt.Click("Delete…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Delete kind cluster dev", "/opt/homebrew/bin/kind delete cluster --name dev", "Everything in the cluster is lost")
	a.op = nil
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Create local cluster…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	// dev is taken: the dialog offers the next name.
	hasTexts(t, tt, "Create local cluster", "kind 0.25.0 · brew", "context kind-dev-2", "kind 0.25.0 default node image", "Docker · OrbStack running",
		"WHAT WILL HAPPEN", "kind create cluster --name dev-2 --wait 3m", "kubectl's current context stays staging")
	a.kind.name = "dev"
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "A kind cluster named dev already exists.")
	a.kind.name, a.kind.nodes = "payments", 1
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "kind-create")
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Create kind cluster payments", "/opt/homebrew/bin/kubectl config use-context -- staging")
	if a.op == nil || a.op.started || a.kind.open || len(a.op.plan.Steps) != 2 || !slices.Contains(a.op.plan.Steps[0].Argv, "--config") {
		t.Fatalf("unexpected operation: %+v", a.op)
	}

	// The engine is down: nothing is listed and nothing can be created.
	a.op = nil
	a.Snapshot.Tools[1].Docker.Contexts[0].Status = "unavailable"
	a.openKindDialog()
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Docker engine is not running", "kind clusters are listed once it runs")
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op != nil {
		t.Fatal("proposed a cluster without an engine")
	}
}
