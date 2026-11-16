package ops

import (
	"slices"
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
