package kube

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type NamespaceInfo struct {
	Phase      string
	Labels     map[string]string
	DeletingAt time.Time
	Finalizers []string
	Conditions []NodeCondition
	Pods       *int
	Quota      float64
}

type namespaceObject struct {
	meta
	Spec   struct{ Finalizers []string }
	Status struct {
		Phase      string
		Conditions []NodeCondition
	}
}

func readNamespaceObjects(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]namespaceObject, error) {
	var list struct{ Items []namespaceObject }
	err := get(ctx, r, env, t, []string{"namespaces"}, &list)
	return list.Items, err
}

func readNamespaceResources(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]Resource, error) {
	objects, err := readNamespaceObjects(ctx, r, env, t)
	if err != nil {
		return nil, err
	}
	var pods struct{ Items []meta }
	podCounts := map[string]int{}
	podErr := get(ctx, r, env, t, []string{"pods", "--all-namespaces"}, &pods)
	for _, pod := range pods.Items {
		podCounts[pod.Metadata.Namespace]++
	}
	quotas, _ := readPolicyResources(ctx, r, env, Target{Kubectl: t.Kubectl, Context: t.Context}, ResourceQuotas)
	usage := map[string]float64{}
	for _, resource := range quotas {
		for key, hard := range resource.Quota.Hard {
			raw, ok := resource.Quota.Used[key]
			if !ok {
				continue
			}
			var used, limit quantity
			if used.UnmarshalJSON([]byte(raw)) == nil && limit.UnmarshalJSON([]byte(hard)) == nil && limit.value > 0 {
				usage[resource.Namespace] = max(usage[resource.Namespace], used.value/limit.value)
			}
		}
	}
	items := make([]Resource, 0, len(objects))
	for _, obj := range objects {
		m := obj.Metadata
		ns := &NamespaceInfo{Phase: obj.Status.Phase, Labels: m.Labels, DeletingAt: m.DeletionTimestamp,
			Finalizers: slices.Concat(obj.Spec.Finalizers, m.Finalizers), Conditions: obj.Status.Conditions, Quota: usage[m.Name]}
		if podErr == nil {
			count := podCounts[m.Name]
			ns.Pods = &count
		}
		items = append(items, Resource{Kind: NamespaceKind, Name: m.Name, UID: m.UID,
			Created: m.CreationTimestamp, Deleting: !ns.DeletingAt.IsZero() || ns.Phase == "Terminating", NamespaceInfo: ns})
	}
	return items, nil
}

// NamespaceMember contains only metadata, including for Secrets.
type NamespaceMember struct {
	Kind, Name, UID, Finalizers string
}

var discoverableResource = regexp.MustCompile(`^[a-z][a-z0-9.-]*$`)

// ReadNamespaceInventory lists each deletable namespaced kind. Any
// discovery or list error means the inventory cannot authorize deletion.
func ReadNamespaceInventory(ctx context.Context, r run.Runner, env sysenv.Env, t Target, config, namespace string) ([]NamespaceMember, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	prefix := []string{t.Kubectl, "--context=" + t.Context, "--request-timeout=15s"}
	if config != "" {
		prefix = append(prefix, "--kubeconfig="+config)
	}
	list := func(args ...string) ([]byte, error) {
		out, err := r.Run(ctx, run.Cmd{Args: slices.Concat(prefix, args), Env: env, Timeout: timeout, MaxOutput: 4 << 20})
		if err != nil {
			return nil, fmt.Errorf("kubectl %s: %w", args[0], err)
		}
		if out.ExitCode != 0 {
			return nil, &Error{command: "kubectl " + args[0], Access: detect.KubeFailure(string(out.Stderr))}
		}
		if out.Truncated {
			return nil, fmt.Errorf("kubectl %s: the answer is too large", args[0])
		}
		return out.Stdout, nil
	}
	// Use the namespace controller's scope. Derived data such as PodMetrics
	// has no stored object or UID and is not deleted by the controller.
	data, err := list("api-resources", "--cached=false", "--namespaced=true", "--verbs=delete", "-o", "name")
	if err != nil {
		return nil, err
	}
	kinds := strings.Fields(string(data))
	if len(kinds) == 0 || slices.ContainsFunc(kinds, func(kind string) bool { return !discoverableResource.MatchString(kind) }) {
		return nil, fmt.Errorf("invalid Kubernetes resource discovery")
	}
	const template = `go-template={{range .items}}{{printf "%s\t%s\t%s\t" .kind .metadata.name .metadata.uid}}{{range .metadata.finalizers}}{{printf "%s," .}}{{end}}{{"\n"}}{{end}}`
	data, err = list("get", strings.Join(kinds, ","), "--namespace="+namespace, "-o", template)
	if err != nil {
		return nil, err
	}
	members := make([]NamespaceMember, 0)
	seen := make(map[string]bool)
	for line := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid Kubernetes inventory")
		}
		if seen[parts[2]] {
			continue
		}
		seen[parts[2]] = true
		finalizers := strings.Split(strings.TrimSuffix(parts[3], ","), ",")
		slices.Sort(finalizers)
		members = append(members, NamespaceMember{parts[0], parts[1], parts[2], strings.Join(finalizers, ", ")})
	}
	slices.SortFunc(members, func(a, b NamespaceMember) int {
		return cmp.Or(strings.Compare(a.Kind, b.Kind), strings.Compare(a.Name, b.Name), strings.Compare(a.UID, b.UID))
	})
	return members, nil
}
