package kube

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Service is a Service and the pods matched by its selector. A Service
// without a selector has no pod counts.
type Service struct {
	Namespace, Name, UID  string
	Created               time.Time
	Type, ClusterIP       string
	ExternalName          string
	ExternalIPs           []string
	LoadBalancer          []string
	SessionAffinity       string
	ExternalTrafficPolicy string
	InternalTrafficPolicy string
	Selector              map[string]string
	Labels                map[string]string
	External, HasSelector bool
	Ports                 []ServicePort
	Pods, Ready           int
	PodsKnown             bool
	relatedPods           []Relationship
	podsErr               error
}

// ServicePort is a Service's port and the number or name it maps to in a pod.
type ServicePort struct {
	Name             string
	Port, NodePort   int
	Protocol, Target string
}

func (p ServicePort) String() string {
	port := strconv.Itoa(p.Port)
	if p.Target != "" && p.Target != port {
		port += " → " + p.Target
	}
	return port + "/" + p.Protocol
}

// DetailString includes the port name and NodePort when present.
func (p ServicePort) DetailString() string {
	text := p.String()
	if p.Name != "" {
		text = p.Name + ": " + text
	}
	if p.NodePort > 0 {
		text += " (NodePort " + strconv.Itoa(p.NodePort) + ")"
	}
	return text
}

type service struct {
	meta
	Spec struct {
		Type                  string            `json:"type"`
		ClusterIP             string            `json:"clusterIP"`
		ExternalName          string            `json:"externalName"`
		ExternalIPs           []string          `json:"externalIPs"`
		SessionAffinity       string            `json:"sessionAffinity"`
		ExternalTrafficPolicy string            `json:"externalTrafficPolicy"`
		InternalTrafficPolicy string            `json:"internalTrafficPolicy"`
		Selector              map[string]string `json:"selector"`
		Ports                 []struct {
			Name       string          `json:"name"`
			Port       int             `json:"port"`
			NodePort   int             `json:"nodePort"`
			Protocol   string          `json:"protocol"`
			TargetPort json.RawMessage `json:"targetPort"`
		} `json:"ports"`
	} `json:"spec"`
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	} `json:"status"`
}

func (s service) hasSelector() bool { return s.Spec.Type != "ExternalName" && len(s.Spec.Selector) > 0 }

func readServices(ctx context.Context, r run.Runner, env sysenv.Env, t Target, scopes [][]string) ([]Service, error) {
	services := []Service{}
	for _, scope := range scopes {
		var list struct{ Items []service }
		var pods struct {
			Items []struct {
				meta
				Status struct {
					Conditions []struct{ Type, Status string }
				}
			}
		}
		// The pods are read beside the Services, for those with a selector.
		var err, podsErr error
		var wg sync.WaitGroup
		wg.Go(func() { err = get(ctx, r, env, t, append([]string{"services"}, scope...), &list) })
		wg.Go(func() { podsErr = get(ctx, r, env, t, append([]string{"pods"}, scope...), &pods) })
		wg.Wait()
		if err != nil {
			return nil, err
		}
		podsKnown := podsErr == nil
		for _, it := range list.Items {
			var lb []string
			for _, ing := range it.Status.LoadBalancer.Ingress {
				lb = append(lb, ing.IP, ing.Hostname)
			}
			lb = slices.DeleteFunc(lb, func(addr string) bool { return addr == "" })
			s := Service{
				Namespace: it.Metadata.Namespace, Name: it.Metadata.Name, UID: it.Metadata.UID,
				Created:               it.Metadata.CreationTimestamp,
				Type:                  it.Spec.Type,
				ClusterIP:             it.Spec.ClusterIP,
				ExternalName:          it.Spec.ExternalName,
				ExternalIPs:           it.Spec.ExternalIPs,
				LoadBalancer:          lb,
				SessionAffinity:       it.Spec.SessionAffinity,
				ExternalTrafficPolicy: it.Spec.ExternalTrafficPolicy,
				InternalTrafficPolicy: it.Spec.InternalTrafficPolicy,
				Selector:              it.Spec.Selector,
				Labels:                it.Metadata.Labels,
				External:              it.Spec.Type != "ClusterIP" || len(it.Spec.ExternalIPs) > 0,
				HasSelector:           it.hasSelector(),
				PodsKnown:             it.hasSelector() && podsKnown,
			}
			for _, port := range it.Spec.Ports {
				s.Ports = append(s.Ports, ServicePort{
					Name:     port.Name,
					Port:     port.Port,
					NodePort: port.NodePort,
					Protocol: port.Protocol,
					Target:   serviceTargetPort(port.TargetPort),
				})
			}
			selected := s.UID == t.Events.UID && s.Namespace == t.Events.Namespace && t.Events.UID != ""
			if selected && s.HasSelector {
				s.podsErr = podsErr
			}
			if s.PodsKnown {
				for _, p := range pods.Items {
					if p.Metadata.Namespace != s.Namespace || !selectorMatches(it.Spec.Selector, p.Metadata.Labels) {
						continue
					}
					s.Pods++
					if selected {
						s.relatedPods = append(s.relatedPods, Relationship{Kind: "Pod", Namespace: s.Namespace, Name: p.Metadata.Name, UID: p.Metadata.UID})
					}
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
