package ops

import (
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
