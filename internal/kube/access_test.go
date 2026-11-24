package kube

import (
	"errors"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

func TestReadAccessReview(t *testing.T) {
	t.Parallel()
	var fake run.Fake
	target := Target{Kubectl: "kubectl", Context: "dev", Namespaces: []string{"unrelated"}, AccessNamespace: "shop"}
	auth := []string{"kubectl", "--context=dev", "--namespace=shop", "auth", "--request-timeout=15s"}
	get := func(kind ResourceKind, namespace ...string) []string {
		return slices.Concat([]string{"kubectl", "--context=dev", "get", string(kind)}, namespace, []string{"--output=json", "--request-timeout=15s"})
	}
	output := "Resources  Non-Resource URLs  Resource Names  Verbs\n" +
		"pods/log   []                 []              [get]\n" +
		"users      []                 [Ann Example]   [impersonate]\n" +
		"*.*        []                 []              [*]\n" +
		"           [/api/*]           []              [get]\n" +
		"           [/api/*]           []              [get]\n"
	fake.Set(run.Result{Stdout: []byte(output), Stderr: []byte("Warning: the list may be incomplete: webhook")}, nil,
		slices.Concat(auth, []string{"can-i", "--list"})...)
	fake.Stdout(`{"status":{"userInfo":{"username":"system:serviceaccount:shop:web","groups":["system:authenticated"]}}}`,
		slices.Concat(auth, []string{"whoami", "--output=json"})...)
	fake.Stdout(`{"items":[{"metadata":{"name":"web","namespace":"shop","uid":"account"}}]}`, get(ServiceAccounts, "--namespace=shop")...)
	fake.Stdout(`{"items":[{"metadata":{"name":"logs","namespace":"shop","uid":"role"},"rules":[{"apiGroups":[""],"resources":["pods/log"],"verbs":["get"]}]}]}`,
		get(Roles, "--namespace=shop")...)
	fake.Set(run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden): cannot list clusterroles")}, nil, get(ClusterRoles)...)
	fake.Stdout(`{"items":[{"metadata":{"name":"web-logs","namespace":"shop","uid":"rb"},"roleRef":{"kind":"Role","name":"logs"},"subjects":[{"kind":"ServiceAccount","name":"web"}]}]}`,
		get(RoleBindings, "--namespace=shop")...)
	fake.Stdout(`{"items":[]}`, get(ClusterRoleBindings)...)

	cluster, err := Read(t.Context(), &fake, nil, target)
	if err != nil || cluster.Access == nil {
		t.Fatalf("Read = %+v, %v", cluster, err)
	}
	a := cluster.Access
	if a.Namespace != "shop" || a.RulesErr != nil || a.IdentityErr != nil || a.RulesWarning == "" {
		t.Fatalf("review = %+v", a)
	}
	want := []AccessRule{
		{Resource: "pods/log", Verbs: "get"},
		{Resource: "users", Names: "Ann Example", Verbs: "impersonate"},
		{Resource: "*.*", Verbs: "*"},
		{URL: "/api/*", Verbs: "get"},
	}
	if !slices.Equal(a.Rules, want) {
		t.Errorf("rules = %+v, want %+v", a.Rules, want)
	}
	if len(a.Resources) != 5 || len(a.Resources[0].Items) != 1 || len(a.Resources[1].Items) != 1 || len(a.Resources[3].Items) != 1 {
		t.Fatalf("resources = %+v", a.Resources)
	}
	if denial, ok := errors.AsType[*Error](a.Resources[2].Err); !ok || denial.Access.State != detect.KubeForbidden {
		t.Errorf("expected ClusterRole denial, got %v", a.Resources[2].Err)
	}
	role := a.Resources[1].Items[0].RBAC
	if len(role.Rules) != 1 || !slices.Equal(role.Rules[0].Resources, []string{"pods/log"}) {
		t.Errorf("role rules = %+v", role.Rules)
	}
	binding := a.Resources[3].Items[0]
	if binding.RBAC.RoleRef != (RBACRoleRef{Kind: "Role", Name: "logs"}) || len(binding.RBAC.Subjects) != 1 ||
		!a.Identity.Matches(binding.RBAC.Subjects[0], binding.Namespace) || a.Identity.Matches(binding.RBAC.Subjects[0], "other") {
		t.Errorf("identity = %+v, binding = %+v", a.Identity, binding.RBAC)
	}
}

func TestAccessIdentityMatches(t *testing.T) {
	t.Parallel()
	id := AccessIdentity{Username: "system:serviceaccount:shop:web", Groups: []string{"system:authenticated"}}
	for _, tc := range []struct {
		name    string
		subject RBACSubject
		want    bool
	}{
		{"user", RBACSubject{Kind: "User", Name: id.Username}, true},
		{"group", RBACSubject{Kind: "Group", Name: "system:authenticated"}, true},
		{"different kind", RBACSubject{Kind: "User", Name: "system:authenticated"}, false},
		{"namespaced account", RBACSubject{Kind: "ServiceAccount", Name: "web", Namespace: "shop"}, true},
		{"other account scope", RBACSubject{Kind: "ServiceAccount", Name: "web", Namespace: "other"}, false},
		{"missing cluster binding scope", RBACSubject{Kind: "ServiceAccount", Name: "web"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := id.Matches(tc.subject, ""); got != tc.want {
				t.Errorf("Matches(%+v) = %t, want %t", tc.subject, got, tc.want)
			}
		})
	}
}

func TestParseAccessRulesRequiresCompleteTable(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"", "unsupported", "Resources  Non-Resource URLs  Resource Names  Verbs\npods  []  []"} {
		if rules, err := parseAccessRules(input); rules != nil || err == nil {
			t.Errorf("parseAccessRules(%q) = %#v, %v", input, rules, err)
		}
	}
}

func TestListAccessCanList(t *testing.T) {
	t.Parallel()
	var nilAccess *ListAccess
	if !nilAccess.CanList("constraints.gatekeeper.sh", "k8spspreadonlyrootfilesystem") {
		t.Fatal("nil ListAccess must fail open")
	}
	access := NewListAccess([]RBACRule{
		{APIGroups: []string{""}, Resources: []string{"pods", "services"}, Verbs: []string{"get", "list"}},
		{APIGroups: []string{""}, Resources: []string{"secrets"}, ResourceNames: []string{"only-one"}, Verbs: []string{"list"}},
		{APIGroups: []string{""}, Resources: []string{"pods/log"}, Verbs: []string{"list"}},
		{APIGroups: []string{"apps"}, Resources: []string{"*"}, Verbs: []string{"*"}},
		{APIGroups: []string{"hearth.example"}, Resources: []string{"certificates"}, Verbs: []string{"list"}},
		{APIGroups: []string{"hearth.example"}, Resources: []string{"issuers"}, Verbs: []string{"get"}},
	})
	for _, tc := range []struct {
		group, resource string
		want            bool
	}{
		{"", "pods", true},
		{"", "services", true},
		{"", "nodes", false},
		{"", "secrets", false},
		{"apps", "deployments", true},
		{"apps", "daemonsets", true},
		{"hearth.example", "certificates", true},
		{"hearth.example", "issuers", false},
		{"constraints.gatekeeper.sh", "k8spspreadonlyrootfilesystem", false},
	} {
		if got := access.CanList(tc.group, tc.resource); got != tc.want {
			t.Errorf("CanList(%q, %q) = %t, want %t", tc.group, tc.resource, got, tc.want)
		}
	}
}

func TestReadListAccess(t *testing.T) {
	t.Parallel()
	var fake run.Fake
	args := []string{"kubectl", "--context=dev", "--namespace=digital", "auth", "--request-timeout=15s", "can-i", "--list"}
	output := "Resources                      Non-Resource URLs  Resource Names  Verbs\n" +
		"pods                           []                 []              [get list watch]\n" +
		"pods/log                       []                 []              [get]\n" +
		"deployments.apps               []                 []              [list]\n" +
		"secrets                        []                 [one-secret]    [get list]\n" +
		"certificates.hearth.example    []                 []              [list]\n"
	fake.Set(run.Result{Stdout: []byte(output)}, nil, args...)
	target := Target{Kubectl: "kubectl", Context: "dev", Namespaces: []string{"digital"}}
	access := readListAccess(t.Context(), &fake, nil, target)
	if access == nil {
		t.Fatal("expected non-nil ListAccess")
	}
	if !access.CanList("", "pods") || !access.CanList("apps", "deployments") || !access.CanList("hearth.example", "certificates") {
		t.Fatalf("expected listed kinds to be allowed")
	}
	if access.CanList("", "nodes") || access.CanList("", "secrets") || access.CanList("aquasecurity.github.io", "clustercompliancereports") {
		t.Fatalf("expected unlisted or resourceName-scoped kinds to be denied")
	}

	// Incomplete warning must fail open.
	fake.Set(run.Result{Stdout: []byte(output), Stderr: []byte("Warning: the list may be incomplete: webhook")}, nil, args...)
	if got := readListAccess(t.Context(), &fake, nil, target); got != nil {
		t.Fatalf("incomplete rules must fail open (nil), got %+v", got)
	}

	// All namespaces mode with zero list rules in fallback "default" must fail open.
	defaultArgs := []string{"kubectl", "--context=dev", "--namespace=default", "auth", "--request-timeout=15s", "can-i", "--list"}
	emptyOut := "Resources  Non-Resource URLs  Resource Names  Verbs\n" +
		"selfsubjectrulesreviews.authorization.k8s.io  []  []  [create]\n"
	fake.Set(run.Result{Stdout: []byte(emptyOut)}, nil, defaultArgs...)
	if got := readListAccess(t.Context(), &fake, nil, Target{Kubectl: "kubectl", Context: "dev"}); got != nil {
		t.Fatalf("all-namespaces fallback with zero list rules must fail open (nil), got %+v", got)
	}
}
