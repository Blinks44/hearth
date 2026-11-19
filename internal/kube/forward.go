package kube

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ForwardPod identifies the pod and TCP port a forward is pinned to.
type ForwardPod struct {
	Name, UID string
	Port      int
}

// ResolveForwardPod reads the approved resource again. For a Service,
// it chooses a running pod matched by the selector, preferring Ready pods.
// The caller stays with the chosen pod for the life of the forward.
func ResolveForwardPod(ctx context.Context, r run.Runner, env sysenv.Env, t Target, ref Ref, resource string, port int) (ForwardPod, error) {
	ns := "--namespace=" + ref.Namespace
	if strings.HasPrefix(resource, "pod/") {
		var p pod
		if err := get(ctx, r, env, t, []string{resource, ns}, &p); err != nil {
			return ForwardPod{}, err
		}
		if p.Metadata.UID != ref.UID || !runningPod(p) {
			return ForwardPod{}, fmt.Errorf("%s was replaced or is no longer running", resource)
		}
		return ForwardPod{Name: p.Metadata.Name, UID: p.Metadata.UID, Port: port}, nil
	}

	var s service
	if err := get(ctx, r, env, t, []string{resource, ns}, &s); err != nil {
		return ForwardPod{}, err
	}
	if s.Metadata.UID != ref.UID || !s.hasSelector() {
		return ForwardPod{}, fmt.Errorf("%s was replaced or has no pod selector", resource)
	}
	var target string
	found := false
	for _, p := range s.Spec.Ports {
		if p.Port == port && p.Protocol == "TCP" {
			target, found = serviceTargetPort(p.TargetPort), true
			break
		}
	}
	if !found {
		return ForwardPod{}, fmt.Errorf("%s no longer declares TCP port %d", resource, port)
	}
	// A headless Service keeps its port, as kubectl port-forward does.
	if s.Spec.ClusterIP == "None" || target == "" || target == "0" {
		target = strconv.Itoa(port)
	}
	selector := make([]string, 0, len(s.Spec.Selector))
	for _, label := range slices.Sorted(maps.Keys(s.Spec.Selector)) {
		selector = append(selector, label+"="+s.Spec.Selector[label])
	}
	var list struct{ Items []pod }
	if err := get(ctx, r, env, t, []string{"pods", ns, "--selector=" + strings.Join(selector, ",")}, &list); err != nil {
		return ForwardPod{}, err
	}
	var chosen ForwardPod
	for _, p := range list.Items {
		if !runningPod(p) {
			continue
		}
		number := targetPort(p, target)
		if number == 0 {
			continue
		}
		candidate := ForwardPod{Name: p.Metadata.Name, UID: p.Metadata.UID, Port: number}
		if readyPod(p) {
			return candidate, nil
		}
		if chosen.Name == "" {
			chosen = candidate
		}
	}
	if chosen.Name == "" {
		return ForwardPod{}, fmt.Errorf("%s has no running pod for TCP port %d", resource, port)
	}
	return chosen, nil
}

func targetPort(p pod, target string) int {
	if number, err := strconv.Atoi(target); err == nil {
		return number
	}
	for _, containers := range [][]podContainer{p.Spec.Containers, p.Spec.InitContainers} {
		for _, ctr := range containers {
			for _, port := range ctr.Ports {
				if port.Name == target && (port.Protocol == "" || port.Protocol == "TCP") {
					return port.Number
				}
			}
		}
	}
	return 0
}

func runningPod(p pod) bool {
	return p.Status.Phase == "Running" && p.Metadata.DeletionTimestamp.IsZero()
}

func readyPod(p pod) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == "Ready" {
			return cond.Status == "True"
		}
	}
	return false
}

// ForwardPodEnded reports a confirmed end or replacement of the chosen pod.
// An unavailable API or an access error says nothing about its lifetime.
func ForwardPodEnded(ctx context.Context, r run.Runner, env sysenv.Env, t Target, ns string, dst ForwardPod) bool {
	var current pod
	if err := get(ctx, r, env, t, []string{"pod/" + dst.Name, "--namespace=" + ns}, &current); err != nil {
		kerr, ok := errors.AsType[*Error](err)
		return ok && strings.HasPrefix(kerr.Access.Detail, "Error from server (NotFound):")
	}
	return current.Metadata.UID != dst.UID || !current.Metadata.DeletionTimestamp.IsZero() ||
		current.Status.Phase == "Succeeded" || current.Status.Phase == "Failed"
}
