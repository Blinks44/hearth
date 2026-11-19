package kube

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Service is a Service and the pods matched by its selector. A Service
// without a selector has no pod counts.
type Service struct {
	Namespace, Name, UID  string
	Type, ClusterIP       string
	External, HasSelector bool
	Ports                 []ServicePort
	Pods, Ready           int
	PodsKnown             bool
}

// ServicePort is a Service's port and the number or name it maps to in a pod.
type ServicePort struct {
	Port             int
	Protocol, Target string
}

func (p ServicePort) String() string {
	port := strconv.Itoa(p.Port)
	if p.Target != "" && p.Target != port {
		port += " → " + p.Target
	}
	return port + "/" + p.Protocol
}

type service struct {
	meta
	Spec struct {
		Type        string            `json:"type"`
		ClusterIP   string            `json:"clusterIP"`
		ExternalIPs []string          `json:"externalIPs"`
		Selector    map[string]string `json:"selector"`
		Ports       []struct {
			Port       int             `json:"port"`
			Protocol   string          `json:"protocol"`
			TargetPort json.RawMessage `json:"targetPort"`
		} `json:"ports"`
	} `json:"spec"`
}

func (s service) hasSelector() bool { return s.Spec.Type != "ExternalName" && len(s.Spec.Selector) > 0 }

func readServices(ctx context.Context, r run.Runner, env sysenv.Env, t Target, scopes [][]string) ([]Service, error) {
	services := []Service{}
	for _, scope := range scopes {
		var list struct{ Items []service }
		if err := get(ctx, r, env, t, append([]string{"services"}, scope...), &list); err != nil {
			return nil, err
		}
		var pods struct {
			Items []struct {
				meta
				Status struct {
					Conditions []struct{ Type, Status string }
				}
			}
		}
		var podsKnown bool
		if slices.ContainsFunc(list.Items, func(s service) bool { return s.hasSelector() }) {
			podsKnown = get(ctx, r, env, t, append([]string{"pods"}, scope...), &pods) == nil
		}
		for _, it := range list.Items {
			s := Service{
				Namespace: it.Metadata.Namespace, Name: it.Metadata.Name, UID: it.Metadata.UID,
				Type: it.Spec.Type, ClusterIP: it.Spec.ClusterIP,
				External:    it.Spec.Type != "ClusterIP" || len(it.Spec.ExternalIPs) > 0,
				HasSelector: it.hasSelector(),
				PodsKnown:   it.hasSelector() && podsKnown,
			}
			for _, port := range it.Spec.Ports {
				s.Ports = append(s.Ports, ServicePort{Port: port.Port, Protocol: port.Protocol, Target: serviceTargetPort(port.TargetPort)})
			}
			if s.PodsKnown {
				for _, p := range pods.Items {
					if p.Metadata.Namespace != s.Namespace || !selectorMatches(it.Spec.Selector, p.Metadata.Labels) {
						continue
					}
					s.Pods++
					for _, cond := range p.Status.Conditions {
						if cond.Type == "Ready" && cond.Status == "True" {
							s.Ready++
							break
						}
					}
				}
			}
			services = append(services, s)
		}
	}
	return services, nil
}

func selectorMatches(selector, labels map[string]string) bool {
	for key, want := range selector {
		if got, ok := labels[key]; !ok || got != want {
			return false
		}
	}
	return true
}

// serviceTargetPort returns a port number or named port from Kubernetes' int-or-string field.
func serviceTargetPort(raw json.RawMessage) string {
	target := string(raw)
	if name, err := strconv.Unquote(target); err == nil {
		return name
	}
	if target == "null" {
		return ""
	}
	return target
}
