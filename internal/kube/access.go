package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const (
	Roles               ResourceKind = "roles.rbac.authorization.k8s.io"
	ClusterRoles        ResourceKind = "clusterroles.rbac.authorization.k8s.io"
	RoleBindings        ResourceKind = "rolebindings.rbac.authorization.k8s.io"
	ClusterRoleBindings ResourceKind = "clusterrolebindings.rbac.authorization.k8s.io"
)

// AccessReview keeps independent results: listing a binding and reviewing
// one's own permissions require different permissions.
type AccessReview struct {
	Namespace             string
	Identity              AccessIdentity
	IdentityErr, RulesErr error
	Rules                 []AccessRule
	RulesWarning          string
	Resources             []ResourceList
}

type AccessIdentity struct {
	Username string
	Groups   []string
}

type AccessRule struct {
	Resource, URL, Names, Verbs string
}

type RBACRule struct {
	APIGroups, Resources, ResourceNames, Verbs, NonResourceURLs []string
}

type RBACSubject struct {
	Kind, Name, Namespace string
}

type RBACRoleRef struct {
	Kind, Name string
}

type RBACDetails struct {
	Rules    []RBACRule
	Subjects []RBACSubject
	RoleRef  RBACRoleRef
}

// Matches reports whether the API identity appears among the binding's subjects.
func (id AccessIdentity) Matches(subject RBACSubject, bindingNamespace string) bool {
	if id.Username == "" {
		return false
	}
	switch subject.Kind {
	case "User":
		return id.Username == subject.Name
	case "Group":
		return slices.Contains(id.Groups, subject.Name)
	case "ServiceAccount":
		ns := subject.Namespace
		if ns == "" {
			ns = bindingNamespace
		}
		return ns != "" && id.Username == "system:serviceaccount:"+ns+":"+subject.Name
	}
	return false
}

func readAccess(ctx context.Context, r run.Runner, env sysenv.Env, t Target) Cluster {
	a := &AccessReview{Namespace: t.AccessNamespace}
	t.Namespaces = []string{a.Namespace}
	out, err := accessCommand(ctx, r, env, t, "can-i", "--list")
	a.RulesErr = err
	if err == nil {
		a.Rules, a.RulesErr = parseAccessRules(string(out.Stdout))
		a.RulesWarning = strings.TrimSpace(string(out.Stderr))
	}
	out, a.IdentityErr = accessCommand(ctx, r, env, t, "whoami", "--output=json")
	if a.IdentityErr == nil {
		var review struct {
			Status struct{ UserInfo AccessIdentity }
		}
		if err := json.Unmarshal(out.Stdout, &review); err != nil {
			a.IdentityErr = fmt.Errorf("kubectl auth whoami: %w", err)
		} else if review.Status.UserInfo.Username == "" {
			a.IdentityErr = fmt.Errorf("kubectl auth whoami: no username in the response")
		} else {
			a.Identity = review.Status.UserInfo
		}
	}

	for _, kind := range []ResourceKind{ServiceAccounts, Roles, ClusterRoles, RoleBindings, ClusterRoleBindings} {
		items, err := ReadResources(ctx, r, env, t, kind)
		a.Resources = append(a.Resources, ResourceList{Kind: kind, Items: items, Err: err})
	}
	return Cluster{Access: a, Taken: time.Now()}
}

func accessCommand(ctx context.Context, r run.Runner, env sysenv.Env, t Target, args ...string) (run.Result, error) {
	command := "kubectl auth " + args[0]
	if t.Kubectl == "" {
		return run.Result{}, &Error{Access: detect.KubeAccess{Detail: "kubectl is required for auth checks"}, command: command}
	}
	argv := []string{t.Kubectl, "--context=" + t.Context, "--namespace=" + t.AccessNamespace, "auth", "--request-timeout=15s"}
	out, err := r.Run(ctx, run.Cmd{Args: append(argv, args...), Env: env, Timeout: timeout, MaxOutput: 4 << 20})
	if err != nil {
		return out, fmt.Errorf("%s: %w", command, err)
	}
	if out.ExitCode != 0 {
		return out, &Error{Access: detect.KubeFailure(string(out.Stderr)), command: command}
	}
	if out.Truncated {
		return out, fmt.Errorf("%s: the answer is too large", command)
	}
	return out, nil
}

// kubectl prints three bracketed lists after the resource column. Preserve
// these as printed: resource names can contain spaces (for example for users).
var accessRuleLine = regexp.MustCompile(`^\s*(.*?)\s*\[(.*?)\]\s+\[(.*)\]\s+\[(.*?)\]\s*$`)

func parseAccessRules(output string) ([]AccessRule, error) {
	header, rows, _ := strings.Cut(strings.TrimSpace(output), "\n")
	if strings.Join(strings.Fields(header), " ") != "Resources Non-Resource URLs Resource Names Verbs" {
		return nil, fmt.Errorf("kubectl auth can-i: unexpected table header")
	}
	var rules []AccessRule
	seen := make(map[AccessRule]bool)
	for line := range strings.Lines(rows) {
		m := accessRuleLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("kubectl auth can-i: invalid rule row")
		}
		rule := AccessRule{Resource: m[1], URL: m[2], Names: m[3], Verbs: m[4]}
		if !seen[rule] {
			seen[rule] = true
			rules = append(rules, rule)
		}
	}
	return rules, nil
}
