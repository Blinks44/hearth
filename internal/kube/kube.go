// Package kube reads a cluster through kubectl for the cluster view: its
// namespaces, pods and workloads. It runs only while the view is open and
// never from the scan; it reads and changes nothing else. kubectl brings
// the kubeconfig's sign-in, as for the access check.
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Every is how often the view reads the cluster again.
const Every = 5 * time.Second

// timeout bounds one read: a sign-in program may take a while.
const timeout = 30 * time.Second

// Target is what the view reads: kubectl, the context and its namespaces.
type Target struct {
	Kubectl, Context string
	// Namespaces are those to read; none reads them all.
	Namespaces []string
}

// Owner is the workload a pod belongs to: a Deployment through its
// ReplicaSet, or the controller that made it.
type Owner struct {
	Kind, Name string
}

// Pod is a pod as the view lists it.
type Pod struct {
	Namespace, Name, UID string
	// Phase is Kubernetes' Pending, Running, Succeeded, Failed or
	// Unknown.
	Phase string
	// Reason is why a container waits or ended, as CrashLoopBackOff, or
	// the pod's own, as Evicted; empty when nothing is wrong.
	Reason            string
	Ready, Containers int
	Restarts          int
	Node              string
	Created           time.Time
	// Owner is zero for a pod nothing owns.
	Owner Owner
}

// Deployment is a Deployment as the view lists it.
type Deployment struct {
	Namespace, Name, UID     string
	Replicas, Ready, Updated int
	Created                  time.Time
}

// Cluster is what one read found.
type Cluster struct {
	Pods        []Pod
	Deployments []Deployment
	Taken       time.Time
}

// Error is a read kubectl refused or that failed: Access says why, by the
// access check's rules, and Detail of an unknown reason is kubectl's line.
type Error struct {
	Access detect.KubeAccess
}

func (e *Error) Error() string {
	if e.Access.Detail != "" {
		return "kubectl: " + e.Access.Detail
	}
	return "kubectl failed"
}

// get runs kubectl get with the target's context and decodes its JSON.
func get(ctx context.Context, r run.Runner, env sysenv.Env, t Target, args []string, v any) error {
	argv := append([]string{t.Kubectl, "--context=" + t.Context, "get"}, args...)
	argv = append(argv, "--output=json", "--request-timeout=15s")
	out, err := r.Run(ctx, run.Cmd{Args: argv, Env: env, Timeout: timeout, MaxOutput: 64 << 20})
	if err != nil {
		return &Error{Access: detect.KubeAccess{Detail: err.Error()}}
	}
	if out.ExitCode != 0 {
		return &Error{Access: detect.KubeFailure(string(out.Stderr))}
	}
	if out.Truncated {
		return fmt.Errorf("kubectl get %s: the answer is too large", strings.Join(args, " "))
	}
	if err := json.Unmarshal(out.Stdout, v); err != nil {
		return fmt.Errorf("kubectl get %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Namespaces lists the context's namespaces, sorted.
func Namespaces(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]string, error) {
	var list struct {
		Items []meta `json:"items"`
	}
	if err := get(ctx, r, env, t, []string{"namespaces"}, &list); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Items))
	for _, it := range list.Items {
		names = append(names, it.Metadata.Name)
	}
	slices.Sort(names)
	return names, nil
}

// Read reads the target's pods and Deployments, in one kubectl get for
// each namespace, or one for all of them.
func Read(ctx context.Context, r run.Runner, env sysenv.Env, t Target) (Cluster, error) {
	scopes := make([][]string, 0, max(len(t.Namespaces), 1))
	for _, ns := range t.Namespaces {
		scopes = append(scopes, []string{"--namespace=" + ns})
	}
	if len(scopes) == 0 {
		scopes = append(scopes, []string{"--all-namespaces"})
	}
	var c Cluster
	for _, scope := range scopes {
		var list struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := get(ctx, r, env, t, append([]string{"pods,replicasets,deployments"}, scope...), &list); err != nil {
			return Cluster{}, err
		}
		if err := c.add(list.Items); err != nil {
			return Cluster{}, err
		}
	}
	c.Taken = time.Now()
	return c, nil
}

// Watch reads the target now and every Every after the last read ended,
// giving each result to got, until ctx ends.
func Watch(ctx context.Context, r run.Runner, env sysenv.Env, t Target, got func(Cluster, error)) {
	for {
		c, err := Read(ctx, r, env, t)
		if ctx.Err() != nil {
			return
		}
		got(c, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(Every):
		}
	}
}

// meta is the metadata every object has.
type meta struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		UID               string    `json:"uid"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
		OwnerReferences   []struct {
			Kind       string `json:"kind"`
			Name       string `json:"name"`
			Controller bool   `json:"controller"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
}

// controller is the object's controlling owner, zero for none.
func (m meta) controller() Owner {
	for _, o := range m.Metadata.OwnerReferences {
		if o.Controller {
			return Owner{Kind: o.Kind, Name: o.Name}
		}
	}
	return Owner{}
}

type containerStatus struct {
	Ready        bool `json:"ready"`
	RestartCount int  `json:"restartCount"`
	State        struct {
		Waiting *struct {
			Reason string `json:"reason"`
		} `json:"waiting"`
		Terminated *struct {
			Reason string `json:"reason"`
		} `json:"terminated"`
	} `json:"state"`
}

type pod struct {
	meta
	Spec struct {
		NodeName   string     `json:"nodeName"`
		Containers []struct{} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase             string            `json:"phase"`
		Reason            string            `json:"reason"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
		Conditions        []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"conditions"`
	} `json:"status"`
}

type deployment struct {
	meta
	Spec struct {
		Replicas *int `json:"replicas"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas   int `json:"readyReplicas"`
		UpdatedReplicas int `json:"updatedReplicas"`
	} `json:"status"`
}

// add decodes a list's items into c, pods owned by a ReplicaSet going to
// the ReplicaSet's own owner, as kubectl's users think of them.
func (c *Cluster) add(items []json.RawMessage) error {
	sets := map[string]Owner{} // ReplicaSet by namespace/name
	var pods []pod
	for _, raw := range items {
		var m meta
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("kubectl get: %w", err)
		}
		switch m.Kind {
		case "Pod":
			var p pod
			if err := json.Unmarshal(raw, &p); err != nil {
				return fmt.Errorf("kubectl get pod %s: %w", m.Metadata.Name, err)
			}
			pods = append(pods, p)
		case "ReplicaSet":
			sets[m.Metadata.Namespace+"/"+m.Metadata.Name] = m.controller()
		case "Deployment":
			var d deployment
			if err := json.Unmarshal(raw, &d); err != nil {
				return fmt.Errorf("kubectl get deployment %s: %w", m.Metadata.Name, err)
			}
			replicas := 1 // Kubernetes' default
			if d.Spec.Replicas != nil {
				replicas = *d.Spec.Replicas
			}
			c.Deployments = append(c.Deployments, Deployment{
				Namespace: m.Metadata.Namespace, Name: m.Metadata.Name, UID: m.Metadata.UID,
				Replicas: replicas, Ready: d.Status.ReadyReplicas, Updated: d.Status.UpdatedReplicas,
				Created: m.Metadata.CreationTimestamp,
			})
		}
	}
	for _, p := range pods {
		owner := p.controller()
		if owner.Kind == "ReplicaSet" {
			if up, ok := sets[p.Metadata.Namespace+"/"+owner.Name]; ok && up.Kind != "" {
				owner = up
			}
		}
		out := Pod{
			Namespace: p.Metadata.Namespace, Name: p.Metadata.Name, UID: p.Metadata.UID,
			Phase: p.Status.Phase, Reason: p.Status.Reason, Containers: len(p.Spec.Containers),
			Node: p.Spec.NodeName, Created: p.Metadata.CreationTimestamp, Owner: owner,
		}
		for _, s := range p.Status.ContainerStatuses {
			if s.Ready {
				out.Ready++
			}
			out.Restarts += s.RestartCount
			if out.Reason != "" {
				continue
			}
			if w := s.State.Waiting; w != nil && w.Reason != "" && w.Reason != "ContainerCreating" {
				out.Reason = w.Reason
			} else if t := s.State.Terminated; t != nil && t.Reason != "" && t.Reason != "Completed" {
				out.Reason = t.Reason
			}
		}
		if out.Reason == "" && out.Phase == "Pending" {
			for _, cond := range p.Status.Conditions {
				if cond.Type == "PodScheduled" && cond.Status == "False" && cond.Reason != "" {
					out.Reason = cond.Reason
				}
			}
		}
		c.Pods = append(c.Pods, out)
	}
	return nil
}
