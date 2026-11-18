package ops

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestKubeContextPlan(t *testing.T) {
	f := &run.Fake{}
	env := sysenv.Env{"KUBECONFIG=/tmp/a:/tmp/b"}
	p, err := KubeContextPlan("kubectl", "staging", "production", env, f)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Steps[0].Argv, []string{"kubectl", "config", "use-context", "--", "production"}) || !slices.Equal(p.Env, env) {
		t.Fatalf("plan: %+v", p)
	}
	f.Stdout("production\n", "kubectl", "config", "current-context")
	if _, err := p.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.Stdout("staging\n", "kubectl", "config", "current-context")
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("wrong context passed verification")
	}
	for _, name := range []string{"", "invalid\nname", "bad\x00name"} {
		if _, err := KubeContextPlan("kubectl", "staging", name, env, f); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	// Names are arguments, never command options or shell fragments.
	p, err = KubeContextPlan("kubectl", "staging", "--production; echo bad", env, f)
	if err != nil || p.Steps[0].Argv[3] != "--" {
		t.Fatalf("argument isolation: %+v, %v", p, err)
	}
}

func TestKubeNamespacePlan(t *testing.T) {
	f := &run.Fake{}
	env := sysenv.Env{"KUBECONFIG=/tmp/a:/tmp/b"}
	p, err := KubeNamespacePlan("kubectl", "staging", "", "payments", "/tmp/a", env, f)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Steps[0].Argv, []string{"kubectl", "config", "set-context", "--namespace=payments", "--", "staging"}) || !slices.Equal(p.Env, env) {
		t.Fatalf("plan: %+v", p)
	}
	if len(p.Changes) != 1 || p.Changes[0] != "Namespace of staging: default → payments in /tmp/a" {
		t.Fatalf("changes: %q", p.Changes)
	}
	view := []string{"kubectl", "config", "view", "--minify", "--context=staging", "-o", "jsonpath={.contexts[0].context.namespace}"}
	f.Stdout("payments", view...)
	if _, err := p.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.Stdout("shop", view...)
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("wrong namespace passed verification")
	}
	for _, ns := range []string{"", "Payments", "-pay", "pay-", "pay_ments", "a.b", strings.Repeat("a", 64), "--namespace=x"} {
		if _, err := KubeNamespacePlan("kubectl", "staging", "", ns, "/tmp/a", env, f); err == nil {
			t.Errorf("accepted namespace %q", ns)
		}
	}
	if _, err := KubeNamespacePlan("kubectl", "bad\nname", "", "payments", "/tmp/a", env, f); err == nil {
		t.Error("accepted context with a newline")
	}
}

func TestKindCreatePlan(t *testing.T) {
	f := &run.Fake{}
	env := sysenv.Env{"PATH=/bin"}
	list := []string{"kind", "get", "clusters"}
	p, err := KindCreatePlan("kind", "kubectl", "dev", 0, "staging", env, f)
	if err != nil {
		t.Fatal(err)
	}
	create := p.Steps[0]
	if !slices.Equal(create.Argv, []string{"kind", "create", "cluster", "--name", "dev", "--wait", "3m"}) || !create.UndoAlways ||
		!slices.Equal(create.Undo.Argv, []string{"kind", "delete", "cluster", "--name", "dev"}) {
		t.Fatalf("create step: %+v", create)
	}
	// Nodes kind was still making as it failed are deleted once they come.
	nodes := []string{"kind", "get", "nodes", "--name", "dev"}
	f.Stdout("dev-worker\n", nodes...)
	if !create.Undo.Again(t.Context()) {
		t.Fatal("left a node")
	}
	f.Stdout("", nodes...)
	if create.Undo.Again(t.Context()) {
		t.Fatal("undid again with no nodes")
	}
	// kind makes its context the current one: the plan puts staging back.
	if len(p.Steps) != 2 || !slices.Equal(p.Steps[1].Argv, []string{"kubectl", "config", "use-context", "--", "staging"}) {
		t.Fatalf("steps: %+v", p.Steps)
	}
	// A cluster of that name is the user's: the plan's undo would delete it.
	f.Stdout("dev\nci\n", list...)
	if err := p.Before(t.Context()); err == nil {
		t.Fatal("created over an existing cluster")
	}
	f.Stdout("ci\n", list...)
	if err := p.Before(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("verified a cluster kind does not list")
	}
	f.Stdout("ci\ndev\n", list...)
	f.Stdout("kind-dev\n", "kubectl", "config", "current-context")
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("verified with kubectl's context left on the new cluster")
	}
	f.Stdout("staging\n", "kubectl", "config", "current-context")
	if _, err := p.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Workers come from a config file written only once the plan runs.
	p, err = KindCreatePlan("kind", "", "multi", 2, "", env, f)
	if err != nil {
		t.Fatal(err)
	}
	argv := p.Steps[0].Argv
	config := argv[len(argv)-1]
	if argv[len(argv)-2] != "--config" || len(p.Steps) != 1 {
		t.Fatalf("plan: %+v", p.Steps)
	}
	t.Cleanup(func() { _ = os.Remove(config) })
	if _, err := os.Stat(config); err == nil {
		t.Fatal("config written before the plan ran")
	}
	if err := p.Before(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config)
	if err != nil || strings.Count(string(data), "role: worker") != 2 || strings.Count(string(data), "role: control-plane") != 1 {
		t.Fatalf("config %q: %v", data, err)
	}

	for _, name := range []string{"", "Dev", "-dev", "dev_1", "a;b", strings.Repeat("a", 33)} {
		if _, err := KindCreatePlan("kind", "kubectl", name, 0, "", env, f); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}

func TestKindDeletePlan(t *testing.T) {
	f := &run.Fake{}
	list := []string{"kind", "get", "clusters"}
	p, err := KindDeletePlan("kind", "dev", "kind-dev", nil, f)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Steps[0].Argv, []string{"kind", "delete", "cluster", "--name", "dev"}) || len(p.Notes) != 2 || len(p.Changes) != 3 {
		t.Fatalf("plan: %+v", p)
	}
	f.Stdout("dev\n", list...)
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("verified a cluster kind still lists")
	}
	f.Stdout("", list...)
	if _, err := p.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if p, _ := KindDeletePlan("kind", "dev", "staging", nil, f); len(p.Changes) != 2 || len(p.Notes) != 1 {
		t.Fatalf("changes: %q, notes: %+v", p.Changes, p.Notes)
	}
}
