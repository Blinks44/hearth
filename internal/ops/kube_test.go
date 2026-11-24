package ops

import (
	"os"
	"path/filepath"
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
	p, err := KindDeletePlan("kind", "dev", "kind-dev", nil, nil, f)
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
	if p, _ := KindDeletePlan("kind", "dev", "staging", nil, nil, f); len(p.Changes) != 2 || len(p.Notes) != 1 {
		t.Fatalf("changes: %q, notes: %+v", p.Changes, p.Notes)
	}
	// A linked project is named in a note; the plan changes nothing more.
	p, _ = KindDeletePlan("kind", "dev", "staging", []string{"shop", "blog"}, nil, f)
	if len(p.Notes) != 2 || !strings.Contains(p.Notes[1].Title, "shop, blog") || len(p.Changes) != 2 {
		t.Fatalf("linked: changes %q, notes %+v", p.Changes, p.Notes)
	}
}

func TestParseKubeConfigYAML(t *testing.T) {
	summary, err := ParseKubeConfigYAML(KubeConfigTemplate())
	if err != nil || len(summary.Contexts) != 0 || summary.HasSecrets {
		t.Fatalf("template: %+v, err=%v", summary, err)
	}

	valid := `apiVersion: v1
kind: Config
current-context: staging
contexts:
  - name: staging
    context: {cluster: eks, user: dev}
  - name: prod
    context: {cluster: gke, user: ops}
`
	summary, err = ParseKubeConfigYAML(valid)
	if err != nil || !slices.Equal(summary.Contexts, []string{"staging", "prod"}) || summary.CurrentContext != "staging" || summary.HasSecrets {
		t.Fatalf("valid: %+v, err=%v", summary, err)
	}

	for _, bad := range []string{
		"",
		"   \n",
		"- just-a-list",
		"apiVersion: apps/v1\nkind: Deployment\n",
		"apiVersion: [broken",
		strings.Repeat("a", maxKubeConfigBytes+1),
	} {
		if _, err := ParseKubeConfigYAML(bad); err == nil {
			t.Errorf("accepted invalid kubeconfig %q", bad[:min(len(bad), 40)])
		}
	}
}

func TestKubeConfigCreatePlan(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/.kube/config"
	if _, err := KubeConfigCreatePlan("relative/config", "relative/config", KubeConfigTemplate()); err == nil {
		t.Fatal("accepted relative kubeconfig path")
	}

	secretYAML := `apiVersion: v1
kind: Config
current-context: staging
clusters:
  - name: eks
    cluster:
      server: https://eks.example.com
contexts:
  - name: staging
    context:
      cluster: eks
      user: dev
users:
  - name: dev
    user:
      token: super-secret-token-123
      password: my-db-password
      client-key-data: LS0tLS1CRUdJTi1QUklWQVRF
      auth-provider:
        name: oidc
        config:
          id-token: oidc-secret-id-token
          refresh-token: oidc-secret-refresh
      exec:
        command: aws
        env:
          - name: AWS_SECRET_ACCESS_KEY
            value: aws-secret-env-value
`
	p, err := KubeConfigCreatePlan(cfgPath, "~/.kube/config", secretYAML)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Edit == nil || !p.Steps[0].Edit.New || p.Steps[0].Edit.Mode != 0o600 {
		t.Fatalf("unexpected edit step: %+v", p.Steps)
	}
	if len(p.Notes) != 1 {
		t.Fatalf("expected secret masking note, got %+v", p.Notes)
	}
	// Preview diff must never expose any secret values.
	secrets := []string{
		"super-secret-token-123",
		"my-db-password",
		"LS0tLS1CRUdJTi1QUklWQVRF",
		"oidc-secret-id-token",
		"oidc-secret-refresh",
		"aws-secret-env-value",
	}
	for _, line := range p.Steps[0].Edit.PreviewDiff() {
		for _, s := range secrets {
			if strings.Contains(line.Text, s) {
				t.Fatalf("preview diff leaked secret %q in line %q", s, line.Text)
			}
		}
	}

	var journal strings.Builder
	got := Execute(t.Context(), &run.Fake{}, p, &journal, func(State) {})
	if got.Phase != Succeeded {
		t.Fatalf("execute phase=%d err=%q", got.Phase, got.Err)
	}
	for _, s := range secrets {
		if strings.Contains(journal.String(), s) {
			t.Fatalf("journal leaked secret %q", s)
		}
	}
	// Written file preserves the real credentials and uses 0600 permissions.
	checkFile(t, cfgPath, secretYAML, 0o600)

	// Refuses overwriting an existing kubeconfig.
	if _, err := KubeConfigCreatePlan(cfgPath, "~/.kube/config", KubeConfigTemplate()); err == nil {
		t.Fatal("overwrote existing kubeconfig")
	}
}

func TestKubeContextDeletePlan(t *testing.T) {
	f := &run.Fake{}
	env := sysenv.Env{"KUBECONFIG=/tmp/a"}

	// Non-active context: single delete-context step.
	p, err := KubeContextDeletePlan("kubectl", "dev", "staging", "~/.kube/config", nil, env, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, []string{"kubectl", "config", "delete-context", "--", "dev"}) {
		t.Fatalf("steps: %+v", p.Steps)
	}
	f.Stdout("staging\ndev\n", "kubectl", "config", "get-contexts", "-o", "name")
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("verified when context still listed")
	}
	f.Stdout("staging\n", "kubectl", "config", "get-contexts", "-o", "name")
	if _, err := p.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Active context + linked project: adds unset current-context and both notes.
	p, err = KubeContextDeletePlan("kubectl", "staging", "staging", "~/.kube/config", []string{"shop"}, env, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 2 || !slices.Equal(p.Steps[1].Argv, []string{"kubectl", "config", "unset", "current-context"}) || len(p.Notes) != 2 {
		t.Fatalf("active delete plan: steps=%+v notes=%+v", p.Steps, p.Notes)
	}
	f.Stdout("dev\n", "kubectl", "config", "get-contexts", "-o", "name")
	f.Stdout("staging\n", "kubectl", "config", "current-context")
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("verified when current-context still pointed at deleted context")
	}
	f.Set(run.Result{ExitCode: 1}, nil, "kubectl", "config", "current-context")
	if _, err := p.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}

	f.Set(run.Result{ExitCode: 1, Stderr: []byte("error: invalid configuration")}, nil, "kubectl", "config", "get-contexts", "-o", "name")
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("verified when get-contexts exited non-zero")
	}
	f.Set(run.Result{ExitCode: 0, Truncated: true}, nil, "kubectl", "config", "get-contexts", "-o", "name")
	if _, err := p.Verify(t.Context()); err == nil {
		t.Fatal("verified when get-contexts output was truncated")
	}

	for _, bad := range []string{"", "bad\nname", "bad\x00name"} {
		if _, err := KubeContextDeletePlan("kubectl", bad, "", "~/.kube/config", nil, env, f); err == nil {
			t.Errorf("accepted invalid context name %q", bad)
		}
	}
}

func TestKubeConfigCreatePlanRedactsYAMLAliasSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alias-kubeconfig")
	raw := `apiVersion: v1
kind: Config
x-secret: &tok super-alias-secret-987
clusters:
- name: dev
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: dev
  context:
    cluster: dev
    user: dev
users:
- name: dev
  user:
    token: *tok
`
	plan, err := KubeConfigCreatePlan(path, "~/.kube/alias-kubeconfig", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Notes) == 0 {
		t.Fatal("expected masked-secrets note for YAML alias secret")
	}
	for _, line := range plan.Steps[0].Edit.preview {
		if strings.Contains(line.Text, "super-alias-secret-987") {
			t.Fatalf("alias secret leaked in preview: %q", line.Text)
		}
	}
}
