package kube

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type nodeObject struct {
	meta
	Spec struct {
		Unschedulable bool
		Taints        []Taint
	}
	Status struct {
		Allocatable struct {
			quantities
			Pods quantity
		}
		Conditions []NodeCondition
		Addresses  []struct{ Type, Address string }
		NodeInfo   struct {
			KubeletVersion, OperatingSystem, Architecture, ContainerRuntimeVersion string
		}
	}
}

// NodeCondition is the status and explanation reported by Kubernetes.
type NodeCondition struct {
	Type, Status, Reason, Message string
}

type Taint struct {
	Key, Value, Effect string
}

func (t Taint) String() string {
	name := t.Key
	if t.Value != "" {
		name += "=" + t.Value
	}
	return name + ":" + t.Effect
}

func (o nodeObject) node() Node {
	info := o.Status.NodeInfo
	n := Node{
		Name: o.Metadata.Name, Version: info.KubeletVersion,
		OS: strings.Trim(info.OperatingSystem+"/"+info.Architecture, "/"), Runtime: info.ContainerRuntimeVersion,
		Allocatable: o.Status.Allocatable.resources(), PodCapacity: int(o.Status.Allocatable.Pods.value),
		Conditions: o.Status.Conditions, Taints: o.Spec.Taints, Cordoned: o.Spec.Unschedulable,
		Labels: o.Metadata.Labels,
	}
	for _, addr := range o.Status.Addresses {
		if addr.Type == "InternalIP" {
			n.Addresses = append(n.Addresses, addr.Address)
		}
	}
	for label := range n.Labels {
		if role, ok := strings.CutPrefix(label, "node-role.kubernetes.io/"); ok && role != "" {
			n.Roles = append(n.Roles, role)
		}
	}
	slices.Sort(n.Roles)
	for _, condition := range n.Conditions {
		if condition.Type == "Ready" {
			n.Ready = condition.Status == "True"
			break
		}
	}
	return n
}

func (n Node) Pressured() bool {
	return slices.ContainsFunc(n.Conditions, func(c NodeCondition) bool {
		return c.Status == "True" && (c.Type == "MemoryPressure" || c.Type == "DiskPressure" || c.Type == "PIDPressure")
	})
}

func readNodeObjects(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]nodeObject, error) {
	var list struct{ Items []nodeObject }
	if err := get(ctx, r, env, t, []string{"nodes"}, &list); err != nil {
		return nil, err
	}
	slices.SortFunc(list.Items, func(a, b nodeObject) int { return cmp.Compare(a.Metadata.Name, b.Metadata.Name) })
	return list.Items, nil
}

type NodePod struct {
	Namespace, Name, Phase string
}

func readNodePods(ctx context.Context, r run.Runner, env sysenv.Env, t Target, n *Node) ([]NodePod, error) {
	var list struct{ Items []podResources }
	if err := get(ctx, r, env, t, []string{"pods", "--all-namespaces", "--field-selector=spec.nodeName=" + n.Name}, &list); err != nil {
		return nil, err
	}
	pods := make([]NodePod, 0, len(list.Items))
	for _, p := range list.Items {
		if p.Spec.NodeName != n.Name {
			continue
		}
		phase, requested := p.Status.Phase, p.requests()
		if phase != "Succeeded" && phase != "Failed" {
			n.Requested = n.Requested.add(requested)
		}
		pods = append(pods, NodePod{p.Metadata.Namespace, p.Metadata.Name, phase})
	}
	slices.SortFunc(pods, func(a, b NodePod) int {
		if order := cmp.Compare(a.Namespace, b.Namespace); order != 0 {
			return order
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return pods, nil
}
