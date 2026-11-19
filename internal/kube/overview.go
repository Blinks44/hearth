package kube

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Overview is the cluster-wide node summary and the selected namespaces'
// warnings. Each section can fail independently of the workload list.
type Overview struct {
	Nodes                              []Node
	Warnings                           []Event
	NodesErr, RequestsErr, WarningsErr error
}

// Node is a node's readiness and its allocatable and requested resources.
type Node struct {
	Name, Version string
	Ready         bool
	Allocatable   Resources
	Requested     Resources
}

// Resources holds CPU in cores and memory in bytes.
type Resources struct {
	CPU, Memory float64
}

func (r Resources) add(s Resources) Resources {
	return Resources{CPU: r.CPU + s.CPU, Memory: r.Memory + s.Memory}
}

func (r Resources) max(s Resources) Resources {
	return Resources{CPU: max(r.CPU, s.CPU), Memory: max(r.Memory, s.Memory)}
}

func readOverview(ctx context.Context, r run.Runner, env sysenv.Env, t Target, scopes [][]string, allPods []json.RawMessage) *Overview {
	o := &Overview{}
	o.Nodes, o.NodesErr = readNodes(ctx, r, env, t)
	if o.NodesErr == nil && len(o.Nodes) > 0 {
		if len(t.Namespaces) > 0 {
			var pods struct {
				Items []json.RawMessage `json:"items"`
			}
			o.RequestsErr = get(ctx, r, env, t, []string{"pods", "--all-namespaces"}, &pods)
			allPods = pods.Items
		}
		if o.RequestsErr == nil {
			o.RequestsErr = addNodeRequests(o.Nodes, allPods)
		}
	}
	for _, scope := range scopes {
		args := append([]string{"events", "--field-selector=type=Warning"}, scope...)
		events, err := readEvents(ctx, r, env, t, args)
		if err != nil {
			o.Warnings, o.WarningsErr = nil, err
			break
		}
		o.Warnings = append(o.Warnings, events...)
	}
	cutoff := time.Now().Add(-time.Hour)
	o.Warnings = slices.DeleteFunc(o.Warnings, func(e Event) bool { return e.Type != "Warning" || e.Last.Before(cutoff) })
	slices.SortStableFunc(o.Warnings, func(x, y Event) int { return y.Last.Compare(x.Last) })
	return o
}

func readNodes(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]Node, error) {
	var list struct {
		Items []struct {
			meta
			Status struct {
				Allocatable quantities `json:"allocatable"`
				Conditions  []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
				NodeInfo struct {
					KubeletVersion string `json:"kubeletVersion"`
				} `json:"nodeInfo"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := get(ctx, r, env, t, []string{"nodes"}, &list); err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(list.Items))
	for _, it := range list.Items {
		n := Node{Name: it.Metadata.Name, Version: it.Status.NodeInfo.KubeletVersion, Allocatable: it.Status.Allocatable.resources()}
		for _, cond := range it.Status.Conditions {
			if cond.Type == "Ready" {
				n.Ready = cond.Status == "True"
				break
			}
		}
		nodes = append(nodes, n)
	}
	slices.SortFunc(nodes, func(x, y Node) int { return cmp.Compare(x.Name, y.Name) })
	return nodes, nil
}

type quantities struct {
	CPU    quantity `json:"cpu"`
	Memory quantity `json:"memory"`
}

func (q quantities) resources() Resources { return Resources{CPU: q.CPU.value, Memory: q.Memory.value} }

// quantity decodes Kubernetes' decimal SI, binary SI and exponent forms.
// The view needs ratios and rounded display values, in base units.
type quantity struct {
	value float64
	set   bool
}

var quantityUnits = map[string]float64{
	"": 1, "n": 1e-9, "u": 1e-6, "m": 1e-3, "k": 1e3, "M": 1e6, "G": 1e9, "T": 1e12, "P": 1e15, "E": 1e18,
	"Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30, "Ti": 1 << 40, "Pi": 1 << 50, "Ei": 1 << 60,
}

func (q *quantity) UnmarshalJSON(data []byte) error {
	value := string(data)
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("resource quantity: %w", err)
		}
	}
	number := strings.TrimRight(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	scale, ok := quantityUnits[value[len(number):]]
	x, err := strconv.ParseFloat(number, 64)
	if !ok || err != nil || x < 0 || math.IsInf(x*scale, 0) || math.IsNaN(x) {
		return fmt.Errorf("invalid resource quantity %q", value)
	}
	*q = quantity{value: x * scale, set: true}
	return nil
}

type resourceSpec struct {
	Requests quantities `json:"requests"`
}

type podResources struct {
	meta
	Spec struct {
		NodeName   string       `json:"nodeName"`
		Resources  resourceSpec `json:"resources"`
		Overhead   quantities   `json:"overhead"`
		Containers []struct {
			Resources resourceSpec `json:"resources"`
		} `json:"containers"`
		InitContainers []struct {
			Resources     resourceSpec `json:"resources"`
			RestartPolicy string       `json:"restartPolicy"`
		} `json:"initContainers"`
	} `json:"spec"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

// requests is the effective request declared in the pod spec: the greater
// of the initialization peak and the steady state, with pod-level requests
// taking precedence and pod overhead added afterwards.
func (p podResources) requests() Resources {
	var peak, steady Resources
	for _, init := range p.Spec.InitContainers {
		concurrent := steady.add(init.Resources.Requests.resources())
		peak = peak.max(concurrent)
		if init.RestartPolicy == "Always" {
			steady = concurrent
		}
	}
	for _, c := range p.Spec.Containers {
		steady = steady.add(c.Resources.Requests.resources())
	}
	requested := steady.max(peak)
	if q := p.Spec.Resources.Requests; q.CPU.set {
		requested.CPU = q.CPU.value
	}
	if q := p.Spec.Resources.Requests; q.Memory.set {
		requested.Memory = q.Memory.value
	}
	return requested.add(p.Spec.Overhead.resources())
}

func addNodeRequests(nodes []Node, items []json.RawMessage) error {
	indices := make(map[string]int, len(nodes))
	for i, n := range nodes {
		indices[n.Name] = i
	}
	for _, raw := range items {
		var m meta
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("kubectl get pods: %w", err)
		}
		if m.Kind != "Pod" {
			continue
		}
		var p podResources
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("kubectl get pod %s: %w", m.Metadata.Name, err)
		}
		if i, ok := indices[p.Spec.NodeName]; ok && p.Status.Phase != "Succeeded" && p.Status.Phase != "Failed" {
			nodes[i].Requested = nodes[i].Requested.add(p.requests())
		}
	}
	return nil
}
