package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ResourceKind is a kind available in the read-only resource view.
type ResourceKind string

const (
	Nodes                  ResourceKind = "nodes"
	NamespaceKind          ResourceKind = "namespaces"
	StatefulSets           ResourceKind = "statefulsets.apps"
	DaemonSets             ResourceKind = "daemonsets.apps"
	ReplicaSets            ResourceKind = "replicasets.apps"
	Ingresses              ResourceKind = "ingresses.networking.k8s.io"
	Endpoints              ResourceKind = "endpoints"
	NetworkPolicies        ResourceKind = "networkpolicies.networking.k8s.io"
	ConfigMaps             ResourceKind = "configmaps"
	ServiceAccounts        ResourceKind = "serviceaccounts"
	PersistentVolumeClaims ResourceKind = "persistentvolumeclaims"
	PersistentVolumes      ResourceKind = "persistentvolumes"
	StorageClasses         ResourceKind = "storageclasses.storage.k8s.io"
)

func (k ResourceKind) supported() bool {
	switch k {
	case Roles, ClusterRoles, RoleBindings, ClusterRoleBindings, Nodes, NamespaceKind, ServiceAccounts, StatefulSets, DaemonSets, ReplicaSets, Ingresses, Endpoints, NetworkPolicies, ConfigMaps, PersistentVolumeClaims, PersistentVolumes, StorageClasses, Autoscalers, DisruptionBudgets, ResourceQuotas, LimitRanges:
		return true
	}
	return false
}

func (k ResourceKind) namespaced() bool {
	return k != Nodes && k != NamespaceKind && k != PersistentVolumes && k != StorageClasses && k != ClusterRoles && k != ClusterRoleBindings
}

// ResourceList is a successful list read, with an independent event read result.
type ResourceList struct {
	Kind                ResourceKind
	Custom              *CustomDefinition
	Items               []Resource
	Err                 error
	EventsErr           error
	Claims              *ClaimDetails
	NodePods            []NodePod
	NodePodsErr         error
	NamespaceMembers    []NamespaceMember
	NamespaceMembersErr error
}

func readResourceCluster(ctx context.Context, r run.Runner, env sysenv.Env, t Target) (Cluster, error) {
	items, err := ReadResources(ctx, r, env, t, t.Resource)
	if err != nil {
		return Cluster{}, err
	}
	c := Cluster{Taken: time.Now(), Resources: &ResourceList{Kind: t.Resource, Items: items}}
	if t.Resource == Nodes && t.Metrics && len(items) > 0 {
		values, err := nodeMetrics(ctx, r, env, t)
		c.MetricsErr = err
		for _, item := range items {
			item.Node.Usage = values[item.Name]
		}
	}
	if t.Resource == PersistentVolumeClaims && len(items) > 0 {
		c.Resources.Claims = readClaimDetails(ctx, r, env, t)
	}
	if t.Events.UID != "" && t.Resource == Nodes {
		for i := range items {
			if items[i].UID == t.Events.UID {
				c.Resources.NodePods, c.Resources.NodePodsErr = readNodePods(ctx, r, env, t, items[i].Node)
				break
			}
		}
	}
	if t.Events.UID != "" && t.Resource == NamespaceKind {
		for _, item := range items {
			if item.UID == t.Events.UID {
				c.Resources.NamespaceMembers, c.Resources.NamespaceMembersErr = ReadNamespaceInventory(ctx, r, env, t, "", item.Name)
				break
			}
		}
	}
	c.readListEvents(ctx, r, env, t, c.Resources)
	return c, nil
}

func (c *Cluster) readListEvents(ctx context.Context, r run.Runner, env sysenv.Env, t Target, list *ResourceList) {
	if t.Events.UID != "" {
		scope := "--all-namespaces"
		if t.Events.Namespace != "" {
			scope = "--namespace=" + t.Events.Namespace
		}
		c.Events, list.EventsErr = readEvents(ctx, r, env, t, []string{"events", scope,
			"--field-selector=involvedObject.uid=" + t.Events.UID})
		c.EventsOf = t.Events
	}
}

// Resource holds metadata and the facts shown for its kind. ConfigMap list
// entries keep key names, and the selected object's YAML provides its values.
type Resource struct {
	Kind                        ResourceKind
	Namespace, Name, UID        string
	Created                     time.Time
	Deleting                    bool
	Owner                       Owner
	Desired, Ready, Updated     int
	Service, Strategy, PodOrder string
	Images                      []string
	Selector                    map[string]string
	podLabels                   map[string]string
	IngressClass                string
	Addresses, TLSSecrets       []string
	Routes                      []IngressRoute
	Ports                       []ServicePort
	PolicyTypes                 []string
	IngressRules, EgressRules   int
	Keys                        []string
	Immutable                   bool
	Storage                     Storage
	Node                        *Node
	NamespaceInfo               *NamespaceInfo
	Custom                      *CustomObject
	Autoscaler                  *Autoscaler
	Budget                      *DisruptionBudget
	Quota                       *ResourceQuota
	Limits                      []LimitRangeItem
	RBAC                        *RBACDetails
}

// IngressRoute identifies the host and path routed to a backend Service.
type IngressRoute struct {
	Host, Path, Service, Port string
	TLS                       bool
}

// URL is the route's browser address when it names a concrete host.
func (r IngressRoute) URL() string {
	const hostChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-."
	host := r.Host
	if ip, err := netip.ParseAddr(host); err == nil && ip.Zone() == "" && ip.Is6() {
		host = "[" + ip.String() + "]"
	} else if host == "" || strings.ContainsFunc(host, func(c rune) bool { return !strings.ContainsRune(hostChars, c) }) {
		return ""
	}
	path := r.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	scheme := "http"
	if r.TLS {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: host, Path: path}).String()
}

type ingressBackend struct {
	Service struct {
		Name string
		Port struct {
			Name   string
			Number json.Number
		}
	}
}

type resourceObject struct {
	meta
	RBACDetails
	storageClassFields
	Spec struct {
		Replicas                                           *int
		ServiceName, PodManagementPolicy, IngressClassName string
		UpdateStrategy                                     struct{ Type string }
		Selector, PodSelector                              struct{ MatchLabels map[string]string }
		Template                                           template
		DefaultBackend                                     *ingressBackend
		Rules                                              []struct {
			Host string
			HTTP struct {
				Paths []struct {
					Path    string
					Backend ingressBackend
				}
			}
		}
		TLS []struct {
			Hosts      []string
			SecretName string
		}
		PolicyTypes     []string
		Ingress, Egress []json.RawMessage
		volumeSpec
	}
	Status struct {
		Phase                                                                                       string
		Capacity                                                                                    map[string]string
		ReadyReplicas, UpdatedReplicas, DesiredNumberScheduled, NumberReady, UpdatedNumberScheduled int
		LoadBalancer                                                                                struct {
			Ingress []struct{ IP, Hostname string }
		}
	}
	Subsets []struct {
		Addresses, NotReadyAddresses []struct{ IP string }
		Ports                        []struct {
			Port     int
			Protocol string
		}
	}
	Data, BinaryData map[string]json.RawMessage
	Immutable        bool
}

// ReadResources reads a single resource kind within its API scope.
func ReadResources(ctx context.Context, r run.Runner, env sysenv.Env, t Target, kind ResourceKind) ([]Resource, error) {
	if !kind.supported() {
		return nil, fmt.Errorf("unsupported Kubernetes resource kind %q", kind)
	}
	if kind == NamespaceKind {
		return readNamespaceResources(ctx, r, env, t)
	}
	if kind == Nodes {
		objects, err := readNodeObjects(ctx, r, env, t)
		if err != nil {
			return nil, err
		}
		items := make([]Resource, 0, len(objects))
		for _, o := range objects {
			m := o.Metadata
			n := o.node()
			items = append(items, Resource{Kind: Nodes, Name: m.Name, UID: m.UID,
				Created: m.CreationTimestamp, Deleting: !m.DeletionTimestamp.IsZero(), Node: &n})
		}
		return items, nil
	}
	switch kind {
	case Autoscalers, DisruptionBudgets, ResourceQuotas, LimitRanges:
		return readPolicyResources(ctx, r, env, t, kind)
	}
	scopes := [][]string{nil}
	if kind.namespaced() {
		scopes = namespaceScopes(t.Namespaces)
	}
	resources := []Resource{}
	for _, scope := range scopes {
		var list struct{ Items []resourceObject }
		if err := get(ctx, r, env, t, append([]string{string(kind)}, scope...), &list); err != nil {
			return nil, err
		}
		for _, obj := range list.Items {
			resources = append(resources, obj.resource(kind))
		}
	}
	return resources, nil
}

func (o resourceObject) resource(kind ResourceKind) Resource {
	m := o.Metadata
	r := Resource{Kind: kind, Namespace: m.Namespace, Name: m.Name, UID: m.UID, Created: m.CreationTimestamp,
		Deleting: !m.DeletionTimestamp.IsZero(), Owner: o.controller()}
	switch kind {
	case Roles, ClusterRoles, RoleBindings, ClusterRoleBindings:
		r.RBAC = &o.RBACDetails
	case StatefulSets, DaemonSets, ReplicaSets:
		desired := 1
		if o.Spec.Replicas != nil {
			desired = *o.Spec.Replicas
		}
		r.Desired, r.Ready, r.Updated = desired, o.Status.ReadyReplicas, o.Status.UpdatedReplicas
		if kind == DaemonSets {
			r.Desired, r.Ready, r.Updated = o.Status.DesiredNumberScheduled, o.Status.NumberReady, o.Status.UpdatedNumberScheduled
		}
		r.Service, r.Strategy, r.PodOrder = o.Spec.ServiceName, o.Spec.UpdateStrategy.Type, o.Spec.PodManagementPolicy
		r.Selector = o.Spec.Selector.MatchLabels
		r.podLabels = o.Spec.Template.Metadata.Labels
		for _, c := range o.Spec.Template.Spec.Containers {
			r.Images = append(r.Images, c.Image)
		}
	case Ingresses:
		o.ingress(&r)
	case Endpoints:
		for _, subset := range o.Subsets {
			r.Ready += len(subset.Addresses)
			r.Desired += len(subset.Addresses) + len(subset.NotReadyAddresses)
			for _, addr := range subset.Addresses {
				r.Addresses = append(r.Addresses, addr.IP)
			}
			for _, port := range subset.Ports {
				r.Ports = append(r.Ports, ServicePort{Port: port.Port, Protocol: port.Protocol})
			}
		}
	case NetworkPolicies:
		r.Selector, r.PolicyTypes = o.Spec.PodSelector.MatchLabels, o.Spec.PolicyTypes
		r.IngressRules, r.EgressRules = len(o.Spec.Ingress), len(o.Spec.Egress)
	case ConfigMaps:
		r.Keys = slices.Sorted(maps.Keys(o.Data))
		for key := range o.BinaryData {
			r.Keys = append(r.Keys, key)
		}
		slices.Sort(r.Keys)
		r.Immutable = o.Immutable
	case PersistentVolumeClaims, PersistentVolumes, StorageClasses:
		r.Storage = o.storage()
	}
	return r
}

func (o resourceObject) ingress(r *Resource) {
	r.IngressClass = o.Spec.IngressClassName
	if r.IngressClass == "" {
		r.IngressClass = o.Metadata.Annotations["kubernetes.io/ingress.class"]
	}
	for _, addr := range o.Status.LoadBalancer.Ingress {
		r.Addresses = append(r.Addresses, addr.IP, addr.Hostname)
	}
	r.Addresses = slices.DeleteFunc(r.Addresses, func(address string) bool { return address == "" })
	tlsHosts := map[string]bool{}
	for _, tls := range o.Spec.TLS {
		if tls.SecretName != "" {
			r.TLSSecrets = append(r.TLSSecrets, tls.SecretName)
		}
		for _, h := range tls.Hosts {
			tlsHosts[h] = true
		}
	}
	add := func(host, path string, b ingressBackend) {
		port := b.Service.Port.Name
		if port == "" {
			port = b.Service.Port.Number.String()
		}
		r.Routes = append(r.Routes, IngressRoute{Host: host, Path: path, Service: b.Service.Name, Port: port, TLS: tlsHosts[host]})
	}
	for _, rule := range o.Spec.Rules {
		for _, path := range rule.HTTP.Paths {
			add(rule.Host, path.Path, path.Backend)
		}
	}
	if o.Spec.DefaultBackend != nil {
		add("", "/", *o.Spec.DefaultBackend)
	}
}

// ReadResourceYAML reads the selected object, checking its UID in the same
// response so a replacement with the same name cannot take its place.
func ReadResourceYAML(ctx context.Context, r run.Runner, env sysenv.Env, t Target, resource Resource) (string, error) {
	supported, namespaced := resource.Kind.supported(), resource.Kind.namespaced()
	if custom := resource.Custom; custom != nil && custom.Definition != nil {
		def := custom.Definition
		supported = def.valid() && custom.Version == def.Version().Name && resource.Kind == def.resourceKind(custom.Version)
		namespaced = def.Namespaced()
	}
	if !supported || resource.Name == "" || namespaced != (resource.Namespace != "") || resource.UID == "" {
		return "", fmt.Errorf("invalid Kubernetes resource target")
	}
	if t.API != nil && t.Kubectl == "" {
		apiArgs := []string{string(resource.Kind) + "/" + resource.Name}
		if namespaced {
			apiArgs = append(apiArgs, "--namespace="+resource.Namespace)
		}
		var raw json.RawMessage
		err := t.API.get(ctx, env, t, apiArgs, &raw)
		if err != nil {
			if errors.Is(err, errUseKubectl) {
				return "", &Error{Access: detect.KubeAccess{Detail: "kubectl is required to read this resource"}}
			}
			return "", err
		}
		var obj map[string]any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&obj); err != nil {
			return "", fmt.Errorf("decode Kubernetes resource: %w", err)
		}
		customNumbers(obj)
		metadata, _ := obj["metadata"].(map[string]any)
		uid, _ := metadata["uid"].(string)
		if uid != resource.UID {
			return "", fmt.Errorf("resource %q was replaced", resource.Name)
		}
		out, err := yaml.Marshal(obj)
		if err != nil {
			return "", fmt.Errorf("encode Kubernetes YAML: %w", err)
		}
		return string(out), nil
	}
	if t.Kubectl == "" {
		return "", &Error{Access: detect.KubeAccess{Detail: "kubectl is required to read this resource"}}
	}
	args := []string{t.Kubectl, "--context=" + t.Context, "get", string(resource.Kind)}
	if namespaced {
		args = append(args, "--namespace="+resource.Namespace)
	}
	args = append(args, "--output=yaml", "--request-timeout=15s", "--", resource.Name)
	out, err := r.Run(ctx, run.Cmd{Args: args, Env: env, Timeout: timeout, MaxOutput: 2 << 20})
	if err != nil {
		return "", fmt.Errorf("read Kubernetes resource: %w", err)
	}
	if out.ExitCode != 0 {
		return "", &Error{Access: detect.KubeFailure(string(out.Stderr))}
	}
	if out.Truncated {
		return "", fmt.Errorf("kubectl get %s: the answer is too large", resource.Kind)
	}
	var object struct{ Metadata struct{ UID string } }
	if err := yaml.Unmarshal(out.Stdout, &object); err != nil {
		return "", fmt.Errorf("decode Kubernetes YAML: %w", err)
	}
	if object.Metadata.UID != resource.UID {
		return "", fmt.Errorf("resource %q was replaced", resource.Name)
	}
	return string(out.Stdout), nil
}
