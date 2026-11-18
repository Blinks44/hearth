package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// kubeProjectApp is kindApp with projects: shop with manifests, linked to
// kind-dev; api linked to a context gone; pay with manifests and no link;
// notes with neither. Linking keeps the links as main does.
func kubeProjectApp() *App {
	a := kindApp()
	a.Projects = []projects.Project{
		{Dir: home + "/work/shop", Kube: []string{"k8s/", "Chart.yaml"}},
		{Dir: home + "/work/api"},
		{Dir: home + "/work/pay", Kube: []string{"skaffold.yaml"}},
		{Dir: home + "/work/notes"},
	}
	a.KubeLinks = map[string]store.KubeLink{
		home + "/work/shop": {Context: "kind-dev", Namespace: "shop"},
		home + "/work/api":  {Context: "kind-gone", Namespace: "api"},
	}
	a.LinkKube = func(dir string, link *store.KubeLink) {
		if link == nil {
			delete(a.KubeLinks, dir)
			return
		}
		a.KubeLinks[dir] = *link
	}
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(nil, nil))
	}
	return a
}

// The project's card says where its link stands without asking the
// cluster: the context, the kind cluster, kubectl's own context, the
// manifests, which are not applied.
func TestProjectKubeCard(t *testing.T) {
	a := kubeProjectApp()
	a.router.Push(projectPath(home + "/work/shop"))
	tt := ui.NewTester(a.View, 1280, 1000)
	tt.Advance(time.Second)
	screenshot(t, tt, "project-kube")
	hasTexts(t, tt, "Kubernetes", "Hearth never applies manifests", "Change link…",
		"kind-dev · namespace shop", "the project's context · ~/.kube/config · the link is kept in Hearth",
		"kind cluster dev", "3 nodes · Kubernetes v1.31.2 · in Docker (OrbStack)", "running",
		"kubectl in the terminal uses staging", "another context",
		"Manifests: k8s/ · Chart.yaml", "found in the project folder · Hearth does not apply them")
	if err := tt.Click("Use kind-dev in kubectl…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	hasTexts(t, tt, "Use Kubernetes context kind-dev", "/opt/homebrew/bin/kubectl config use-context -- kind-dev")
	if a.op == nil || a.op.started {
		t.Fatalf("unexpected operation: %+v", a.op)
	}
	a.op = nil

	// kubectl on the project's context: nothing to choose.
	a.Snapshot.Tools[0].Cli.Contexts[0].Name = "kind-dev"
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("another context") {
		t.Errorf("kubectl is on the project's context: %q", tt.Texts())
	}

	// The cluster was deleted: its context and nodes are gone.
	cli := a.Snapshot.Tools[0].Cli
	cli.KubeContexts = cli.KubeContexts[1:]
	a.Snapshot.Tools[1].Docker.KindClusters = a.Snapshot.Tools[1].Docker.KindClusters[1:]
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "project-kube-gone")
	hasTexts(t, tt, "context not found", "the context is no longer in kubeconfig · the link stays until you change it",
		"Choose another…", "kind cluster dev is not in Docker", "Unlink")
	if err := tt.Click("Create kind cluster…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !a.kind.open || a.kind.name != "dev" || a.kind.project != "shop" {
		t.Fatalf("kind dialog: %+v", a.kind)
	}
	hasTexts(t, tt, "after creating, project shop is linked to kind-dev")
	a.kind.open = false
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Unlink"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if _, ok := a.KubeLinks[home+"/work/shop"]; ok {
		t.Fatal("still linked")
	}
	hasTexts(t, tt, "Not linked to a context", "Link…", "Manifests: k8s/ · Chart.yaml")

	// A project with neither manifests nor a link has no card.
	a.router.Push(projectPath(home + "/work/notes"))
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Hearth never applies manifests") {
		t.Errorf("card without manifests or a link: %q", tt.Texts())
	}
}

// Linking keeps the context and namespace in Hearth alone: nothing is
// proposed to change kubectl or the cluster.
func TestKubeLinkDialog(t *testing.T) {
	a := kubeProjectApp()
	dir := home + "/work/pay"
	a.router.Push(projectPath(dir))
	tt := ui.NewTester(a.View, 1280, 1000)
	tt.Advance(time.Second)
	if err := tt.Click("Link…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	// kubectl's current context first, with its namespace.
	hasTexts(t, tt, "Link pay to Kubernetes", "cluster eks-staging · ~/.kube/config", "may not exist yet",
		"the link is kept in Hearth's settings; the project's files do not change", "Hearth does not apply the project's manifests")
	if a.kubeLink.context != "staging" || tt.HasText("Unlink") {
		t.Fatalf("dialog: %+v", a.kubeLink)
	}
	a.kubeLink.context, a.kubeLink.namespace = "kind-dev", "Bad_Name"
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "kubectl's current context stays staging; kind-dev can be chosen separately",
		"Use lowercase letters, digits and hyphens, up to 63 characters, starting and ending with a letter or digit.")
	if err := tt.Click("Link"); err == nil {
		tt.Advance(time.Second)
	}
	if _, ok := a.KubeLinks[dir]; ok {
		t.Fatal("linked with a bad namespace")
	}
	a.kubeLink.namespace = "web"
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "kube-link")
	if err := tt.Click("Link"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := a.KubeLinks[dir]; got != (store.KubeLink{Context: "kind-dev", Namespace: "web"}) || a.kubeLink.open || a.op != nil {
		t.Fatalf("link %+v, dialog %+v, op %+v", got, a.kubeLink, a.op)
	}

	// An empty namespace is the context's own.
	a.openKubeLink(dir)
	a.kubeLink.context, a.kubeLink.namespace = "staging", ""
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("Link"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if got := a.KubeLinks[dir]; got != (store.KubeLink{Context: "staging", Namespace: "shop"}) {
		t.Fatalf("link %+v", got)
	}
}

// The Kubernetes page lists the linked projects and where they stand, and
// the ones with manifests to link; a context names its projects.
func TestKubeProjectsTab(t *testing.T) {
	a := kubeProjectApp()
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 1000)
	tt.Advance(time.Second)
	hasTexts(t, tt, "cluster kind-dev · namespace default · project shop")
	a.kubeTab = 2
	tt.Frame()
	tt.Advance(time.Second)
	screenshot(t, tt, "kube-projects")
	hasTexts(t, tt, "LINKED TO A CONTEXT", "shop", "kind-dev · namespace shop · ~/work/shop", "running",
		"api", "kind-gone · namespace api · ~/work/api", "context not found",
		"HAVE MANIFESTS, NOT LINKED", "pay", "skaffold.yaml · ~/work/pay", "not linked", "Link…")
	if tt.HasText("notes") {
		t.Errorf("a project with neither manifests nor a link: %q", tt.Texts())
	}
	if err := tt.Click("Link…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !a.kubeLink.open || a.kubeLink.dir != home+"/work/pay" {
		t.Fatalf("dialog: %+v", a.kubeLink)
	}
}

// A cluster created for a project is linked to it once the plan
// succeeds, not before.
func TestKindCreateLinksProject(t *testing.T) {
	a := kubeProjectApp()
	var update func(ops.State)
	a.RunPlan = func(_ ops.Plan, u func(ops.State)) func() {
		update = u
		return func() {}
	}
	a.router.Push(kubePage)
	tt := ui.NewTester(a.View, 1280, 1000)
	tt.Advance(time.Second)
	a.openKindDialog()
	a.kind.name, a.kind.project = "payments", "pay"
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Link to project", "manifests are not applied", "after creating, project pay is linked to kind-payments")
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.after == nil {
		t.Fatalf("operation: %+v", a.op)
	}
	a.startOperation()
	update(ops.State{Phase: ops.Running, Steps: make([]ops.StepStatus, len(a.op.plan.Steps))})
	if _, ok := a.KubeLinks[home+"/work/pay"]; ok {
		t.Fatal("linked before the cluster was created")
	}
	update(ops.State{Phase: ops.Succeeded, Steps: make([]ops.StepStatus, len(a.op.plan.Steps))})
	if got := a.KubeLinks[home+"/work/pay"]; got != (store.KubeLink{Context: "kind-payments", Namespace: "default"}) {
		t.Fatalf("link %+v", got)
	}
}
