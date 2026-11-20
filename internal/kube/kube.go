// Package kube reads a cluster through kubectl and Helm for the cluster view.
// It runs
// only while the view is open and never from the scan; it reads and
// changes nothing else. kubectl brings the kubeconfig's sign-in, as for
// the access check.
package kube

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
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
	// Helm reads releases instead of workloads when its executable is set.
	Helm, HelmVersion string
	// Namespaces are those to read; none reads them all.
	Namespaces []string
	// Overview includes nodes, their requests and recent warnings.
	Overview bool
	// Metrics reads CPU and memory use alongside the selected view.
	Metrics bool
	// Batch includes Jobs and CronJobs.
	Batch bool
	// Services reads the service list instead of workloads.
	Services bool
	// Secrets reads Secret metadata and key names.
	Secrets bool
	// Resource reads one of the additional namespaced resource kinds.
	Resource ResourceKind
	// Policies reads autoscalers and namespace policies instead of workloads.
	Policies bool
	// DeploymentAutoscalers reads HPAs for the selected Deployment.
	DeploymentAutoscalers bool
	// CustomKind is a CRD selected by its full name.
	CustomKind string
	// Events is the object whose events to read too; zero reads none.
	Events Ref
	// Tail is the container whose previous run's last line to read too;
	// zero reads none.
	Tail Tail
}

// Tail names a container of a pod and its run: the previous one, or the
// current one that ended.
type Tail struct {
	Namespace, Pod, Container string
	Previous                  bool
}

// Ref names an object by its namespace and UID.
type Ref struct {
	Namespace, UID string
}

// Owner is the workload a pod belongs to: a Deployment through its
// ReplicaSet, or the controller that made it.
type Owner struct {
	Kind, Name string
}

// Pod is a pod as the view lists and shows it.
type Pod struct {
	Namespace, Name, UID string
	// Phase is Kubernetes' Pending, Running, Succeeded, Failed or
	// Unknown.
	Phase string
	// Reason is why a container waits or ended, as CrashLoopBackOff, or
	// the pod's own, as Evicted; empty when nothing is wrong. Message is
	// the pod's own message, as an eviction's.
	Reason, Message string
	// Unscheduled is the scheduler's message while no node takes the pod.
	Unscheduled                string
	Ready                      int
	Restarts                   int
	Containers, InitContainers []Container
	TCPPorts                   []int
	Node                       string
	IP, QoS                    string
	Account                    string
	References                 []Relationship
	controllerUID              string
	Labels                     map[string]string
	// Volumes are the volumes the pod declares, without the API token
	// every pod gets.
	Volumes          []string
	Claims           []string
	Created          time.Time
	Requests, Limits Resources
	Usage            *Resources
	// Owner is zero for a pod nothing owns.
	Owner Owner
}

// Container is a pod's container and its status.
type Container struct {
	Name, Image      string
	Ready            bool
	Restarts         int
	Requests, Limits Resources
	Usage            *Resources
	// State is running, waiting or terminated; empty before the
	// container has a status.
	State string
	// Reason and Message say why it waits or ended.
	Reason, Message string
	// Started is when its current run started, while State is running.
	Started time.Time
	// Ended is how its current run ended, when State is terminated; Last
	// how its previous one did. Either is nil when there is none.
	Ended, Last *Run
}

// Run is how a container's run ended.
type Run struct {
	Reason, Message   string
	ExitCode          int
	Started, Finished time.Time
}

// Deployment is a Deployment as the view lists and shows it.
type Deployment struct {
	Namespace, Name, UID string
	// Replicas is how many pods it wants; Pods how many it has, old ones
	// of a rollout too.
	Replicas, Pods            int
	Ready, Updated, Available int
	Paused                    bool
	Strategy                  string
	Images                    []string
	// Revision is the current one; Revisions are those its ReplicaSets
	// keep, newest first.
	Revision  int
	Revisions []Revision
	// Stalled is Kubernetes' message when the rollout made no progress
	// in time.
	Stalled        string
	Created        time.Time
	Autoscalers    []Resource
	AutoscalersErr error
	podLabels      map[string]string
}

// Revision is a Deployment's revision, as its ReplicaSet keeps it.
type Revision struct {
	Number      int
	Name, UID   string
	Images      []string
	Pods, Ready int
	Created     time.Time
}

// Event is something Kubernetes reported about an object.
type Event struct {
	// Type is Normal or Warning.
	Type, Reason, Message string
	Count                 int
	Last                  time.Time
	Object                Owner
	Namespace             string
}

// Cluster is what one read found.
type Cluster struct {
	Pods          []Pod
	Deployments   []Deployment
	Overview      *Overview
	Batch         *Batch
	Services      []Service
	HelmReleases  []HelmRelease
	Secrets       []Secret
	Resources     *ResourceList
	Policies      []ResourceList
	Relationships *Relationships
	MetricsErr    error
	// Events are those of the target's Events object, newest first.
	Events   []Event
	EventsOf Ref
	// LastLine is the last line of the run of the target's Tail
	// container, empty when it has none.
	LastLine   string
	LastLineOf Tail
	Taken      time.Time
}

// Error is a read kubectl refused or that failed: Access says why, by the
// access check's rules, and Detail of an unknown reason is kubectl's line.
type Error struct {
	Access  detect.KubeAccess
	command string
}

func (e *Error) Error() string {
	if e.Access.Detail != "" {
		return cmp.Or(e.command, "kubectl") + ": " + e.Access.Detail
	}
	return cmp.Or(e.command, "kubectl") + " failed"
}

// get runs kubectl get with the target's context and decodes its JSON.
func get(ctx context.Context, r run.Runner, env sysenv.Env, t Target, args []string, v any) error {
	return kubectlJSON(ctx, r, env, t, slices.Concat(args, []string{"--output=json"}), v)
}

func kubectlJSON(ctx context.Context, r run.Runner, env sysenv.Env, t Target, args []string, v any) error {
	argv := append([]string{t.Kubectl, "--context=" + t.Context, "get"}, args...)
	argv = append(argv, "--request-timeout=15s")
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
	objects, err := readNamespaceObjects(ctx, r, env, t)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(objects))
	for _, it := range objects {
		names = append(names, it.Metadata.Name)
	}
	slices.Sort(names)
	return names, nil
}

func namespaceScopes(namespaces []string) [][]string {
	scopes := make([][]string, 0, max(len(namespaces), 1))
	for _, ns := range namespaces {
		scopes = append(scopes, []string{"--namespace=" + ns})
	}
	if len(scopes) == 0 {
		scopes = append(scopes, []string{"--all-namespaces"})
	}
	return scopes
}

// Read reads the target's pods and Deployments, in one kubectl get for
// each namespace, or one for all of them, and the optional overview,
// object events and container log tail.
func Read(ctx context.Context, r run.Runner, env sysenv.Env, t Target) (Cluster, error) {
	c, err := readCluster(ctx, r, env, t)
	if err == nil {
		c.readRelationships(ctx, r, env, t)
	}
	return c, err
}

func readCluster(ctx context.Context, r run.Runner, env sysenv.Env, t Target) (Cluster, error) {
	if t.Policies {
		return readPolicies(ctx, r, env, t), nil
	}
	if t.CustomKind != "" {
		return readCustomCluster(ctx, r, env, t)
	}
	if t.Resource != "" {
		return readResourceCluster(ctx, r, env, t)
	}
	scopes := namespaceScopes(t.Namespaces)
	var c Cluster
	if t.Helm != "" {
		releases, err := readHelm(ctx, r, env, t, scopes)
		return Cluster{HelmReleases: releases, Taken: time.Now()}, err
	}
	if t.Secrets {
		secrets, err := readSecrets(ctx, r, env, t, scopes)
		return Cluster{Secrets: secrets, Taken: time.Now()}, err
	}
	if t.Services {
		services, err := readServices(ctx, r, env, t, scopes)
		return Cluster{Services: services, Taken: time.Now()}, err
	}
	if t.Batch {
		c.Batch = readBatch(ctx, r, env, t, scopes)
		if !t.Overview {
			// Batch views can be used with rights only to Jobs and CronJobs.
			c.Taken = time.Now()
			return c, nil
		}
	}
	var allPods []json.RawMessage
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
		if t.Overview && len(t.Namespaces) == 0 {
			allPods = list.Items
		}
	}
	if t.Overview {
		c.Overview = readOverview(ctx, r, env, t, scopes, allPods)
		if t.Metrics && len(c.Overview.Nodes) > 0 {
			values, err := nodeMetrics(ctx, r, env, t)
			c.MetricsErr = err
			for i := range c.Overview.Nodes {
				n := &c.Overview.Nodes[i]
				n.Usage = values[n.Name]
			}
		}
	} else if t.Metrics && len(c.Pods) > 0 {
		c.MetricsErr = addPodMetrics(ctx, r, env, t, c.Pods)
	}
	if t.Events.UID != "" {
		events, err := readEvents(ctx, r, env, t, []string{"events", "--namespace=" + t.Events.Namespace, "--field-selector=involvedObject.uid=" + t.Events.UID})
		if err != nil {
			return Cluster{}, err
		}
		c.Events, c.EventsOf = events, t.Events
	}
	if t.DeploymentAutoscalers && t.Events.UID != "" {
		c.readDeploymentAutoscalers(ctx, r, env, t)
	}
	if t.Tail.Container != "" {
		c.LastLine, c.LastLineOf = lastLine(ctx, r, env, t), t.Tail
	}
	c.Taken = time.Now()
	return c, nil
}

// readEvents reads core/v1 events, newest first.
func readEvents(ctx context.Context, r run.Runner, env sysenv.Env, t Target, args []string) ([]Event, error) {
	var list struct {
		Items []struct {
			Type           string    `json:"type"`
			Reason         string    `json:"reason"`
			Message        string    `json:"message"`
			Count          int       `json:"count"`
			LastTimestamp  time.Time `json:"lastTimestamp"`
			EventTime      time.Time `json:"eventTime"`
			FirstTimestamp time.Time `json:"firstTimestamp"`
			Object         struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"involvedObject"`
			Series *struct {
				Count            int       `json:"count"`
				LastObservedTime time.Time `json:"lastObservedTime"`
			} `json:"series"`
		} `json:"items"`
	}
	if err := get(ctx, r, env, t, args, &list); err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(list.Items))
	for _, it := range list.Items {
		e := Event{Type: it.Type, Reason: it.Reason, Message: it.Message, Count: max(it.Count, 1),
			Object: Owner{Kind: it.Object.Kind, Name: it.Object.Name}, Namespace: it.Object.Namespace}
		// Newer clusters report through eventTime and series, older ones
		// through lastTimestamp and count.
		for _, at := range []time.Time{it.LastTimestamp, it.EventTime, it.FirstTimestamp} {
			if !at.IsZero() {
				e.Last = at
				break
			}
		}
		if s := it.Series; s != nil {
			e.Count = max(s.Count, e.Count)
			if !s.LastObservedTime.IsZero() {
				e.Last = s.LastObservedTime
			}
		}
		events = append(events, e)
	}
	slices.SortStableFunc(events, func(x, y Event) int { return y.Last.Compare(x.Last) })
	return events, nil
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

// backOff finds the wait of kubelet's "back-off 5m0s restarting failed
// container" message.
var backOff = regexp.MustCompile(`back-off (\S+) restarting`)

// BackOff is how long kubelet waits before starting a crashed container
// again, by its waiting message; zero when the message does not say.
func BackOff(message string) time.Duration {
	m := backOff.FindStringSubmatch(message)
	if m == nil {
		return 0
	}
	d, err := time.ParseDuration(m[1])
	if err != nil {
		return 0
	}
	return d
}

// Pull is why an image could not be pulled, by the words of the
// container's waiting message.
type Pull int

const (
	PullUnknown Pull = iota
	// PullUnreachable: the node could not reach the registry.
	PullUnreachable
	// PullDenied: the registry refused, as for a private image without
	// credentials; Docker Hub says so of an image that does not exist too.
	PullDenied
	// PullNotFound: the registry has no such image or tag.
	PullNotFound
)

// PullCause says why an image could not be pulled by kubelet's message,
// PullUnknown when its words are none Hearth knows.
func PullCause(message string) Pull {
	m := strings.ToLower(message)
	has := func(words ...string) bool {
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(m, w) })
	}
	switch {
	case has("no such host", "i/o timeout", "connection refused", "network is unreachable", "tls handshake timeout"):
		return PullUnreachable
	case has("pull access denied", "unauthorized", "authentication required", "403 forbidden", "denied"):
		return PullDenied
	case has("manifest unknown", "not found"):
		return PullNotFound
	}
	return PullUnknown
}

// Unschedulable is a reason the scheduler gives for placing a pod on no
// node.
type Unschedulable int

const (
	// NoResources: the nodes lack the CPU or memory the pod requests.
	NoResources Unschedulable = iota
	// NoVolume: a PersistentVolumeClaim of the pod is not bound.
	NoVolume
	// NoMatchingNode: no node matches the pod's selector or affinity.
	NoMatchingNode
	// Tainted: the nodes have taints the pod does not tolerate.
	Tainted
)

// ScheduleCauses are the reasons the scheduler's message names, in the
// order above; none when its words are none Hearth knows.
func ScheduleCauses(message string) []Unschedulable {
	m := strings.ToLower(message)
	var causes []Unschedulable
	for _, c := range []struct {
		cause Unschedulable
		words []string
	}{
		{NoResources, []string{"insufficient cpu", "insufficient memory"}},
		{NoVolume, []string{"persistentvolumeclaim"}},
		{NoMatchingNode, []string{"didn't match pod's node affinity/selector", "didn't match node selector"}},
		{Tainted, []string{"untolerated taint"}},
	} {
		if slices.ContainsFunc(c.words, func(w string) bool { return strings.Contains(m, w) }) {
			causes = append(causes, c.cause)
		}
	}
	return causes
}

// meta is the metadata every object has.
type meta struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name              string            `json:"name"`
		Namespace         string            `json:"namespace"`
		Generation        int64             `json:"generation"`
		UID               string            `json:"uid"`
		CreationTimestamp time.Time         `json:"creationTimestamp"`
		DeletionTimestamp time.Time         `json:"deletionTimestamp"`
		Labels            map[string]string `json:"labels"`
		Finalizers        []string          `json:"finalizers"`
		Annotations       map[string]string `json:"annotations"`
		OwnerReferences   []struct {
			Kind       string `json:"kind"`
			Name       string `json:"name"`
			UID        string `json:"uid"`
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

func (m meta) controllerUID() string {
	for _, owner := range m.Metadata.OwnerReferences {
		if owner.Controller {
			return owner.UID
		}
	}
	return ""
}

// revision is the Deployment revision a ReplicaSet or Deployment carries.
func (m meta) revision() int {
	n, _ := strconv.Atoi(m.Metadata.Annotations["deployment.kubernetes.io/revision"])
	return n
}

// template is a pod template's containers, as Deployments and ReplicaSets
// have.
type template struct {
	Metadata struct{ Labels map[string]string }
	Spec     struct {
		Containers []struct {
			Image string `json:"image"`
		} `json:"containers"`
	} `json:"spec"`
}

func (t template) images() []string {
	var images []string
	for _, c := range t.Spec.Containers {
		images = append(images, c.Image)
	}
	return images
}

type state struct {
	Running *struct {
		StartedAt time.Time `json:"startedAt"`
	} `json:"running"`
	Waiting *struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	} `json:"waiting"`
	Terminated *terminated `json:"terminated"`
}

type terminated struct {
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	ExitCode   int       `json:"exitCode"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

func (t *terminated) run() *Run {
	if t == nil {
		return nil
	}
	return &Run{Reason: t.Reason, Message: t.Message, ExitCode: t.ExitCode, Started: t.StartedAt, Finished: t.FinishedAt}
}

type containerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int    `json:"restartCount"`
	State        state  `json:"state"`
	LastState    state  `json:"lastState"`
}

type podContainer struct {
	Name          string       `json:"name"`
	Image         string       `json:"image"`
	Resources     resourceSpec `json:"resources"`
	RestartPolicy string       `json:"restartPolicy"`
	Env           []struct {
		Name      string
		ValueFrom struct{ ConfigMapKeyRef, SecretKeyRef *objectName }
	}
	EnvFrom []struct{ ConfigMapRef, SecretRef *objectName }
	Ports   []struct {
		Name     string `json:"name"`
		Number   int    `json:"containerPort"`
		Protocol string `json:"protocol"`
	} `json:"ports"`
}

type pod struct {
	meta
	Spec struct {
		NodeName           string `json:"nodeName"`
		ServiceAccountName string `json:"serviceAccountName"`
		podResourceSpec
		Volumes             []podVolume `json:"volumes"`
		EphemeralContainers []podContainer
		ImagePullSecrets    []objectName
	} `json:"spec"`
	Status struct {
		Phase                 string            `json:"phase"`
		Reason                string            `json:"reason"`
		Message               string            `json:"message"`
		PodIP                 string            `json:"podIP"`
		QOSClass              string            `json:"qosClass"`
		ContainerStatuses     []containerStatus `json:"containerStatuses"`
		InitContainerStatuses []containerStatus `json:"initContainerStatuses"`
		Conditions            []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

type replicaSet struct {
	meta
	Spec struct {
		Template template `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas      int `json:"replicas"`
		ReadyReplicas int `json:"readyReplicas"`
	} `json:"status"`
}

type deployment struct {
	meta
	Spec struct {
		Replicas *int `json:"replicas"`
		Paused   bool `json:"paused"`
		Strategy struct {
			Type string `json:"type"`
		} `json:"strategy"`
		Template template `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas          int `json:"replicas"`
		ReadyReplicas     int `json:"readyReplicas"`
		UpdatedReplicas   int `json:"updatedReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
		Conditions        []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

// add decodes a list's items into c, pods owned by a ReplicaSet going to
// the ReplicaSet's own owner, as kubectl's users think of them.
func (c *Cluster) add(items []json.RawMessage) error {
	sets := map[string]Owner{}           // ReplicaSet by namespace/name
	revisions := map[string][]Revision{} // by namespace/Deployment
	var pods []pod
	first := len(c.Deployments)
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
			var rs replicaSet
			if err := json.Unmarshal(raw, &rs); err != nil {
				return fmt.Errorf("kubectl get replicaset %s: %w", m.Metadata.Name, err)
			}
			owner := m.controller()
			sets[m.Metadata.Namespace+"/"+m.Metadata.Name] = owner
			if owner.Kind == "Deployment" {
				key := m.Metadata.Namespace + "/" + owner.Name
				revisions[key] = append(revisions[key], Revision{
					Number: m.revision(), Name: m.Metadata.Name, UID: m.Metadata.UID, Images: rs.Spec.Template.images(),
					Pods: rs.Status.Replicas, Ready: rs.Status.ReadyReplicas, Created: m.Metadata.CreationTimestamp,
				})
			}
		case "Deployment":
			var d deployment
			if err := json.Unmarshal(raw, &d); err != nil {
				return fmt.Errorf("kubectl get deployment %s: %w", m.Metadata.Name, err)
			}
			replicas := 1 // Kubernetes' default
			if d.Spec.Replicas != nil {
				replicas = *d.Spec.Replicas
			}
			out := Deployment{
				Namespace: m.Metadata.Namespace, Name: m.Metadata.Name, UID: m.Metadata.UID,
				Replicas: replicas, Pods: d.Status.Replicas, Ready: d.Status.ReadyReplicas,
				Updated: d.Status.UpdatedReplicas, Available: d.Status.AvailableReplicas,
				Paused: d.Spec.Paused, Strategy: d.Spec.Strategy.Type, Images: d.Spec.Template.images(),
				Revision: m.revision(), Created: m.Metadata.CreationTimestamp, podLabels: d.Spec.Template.Metadata.Labels,
			}
			for _, cond := range d.Status.Conditions {
				if cond.Type == "Progressing" && cond.Status == "False" {
					out.Stalled = cond.Message
				}
			}
			c.Deployments = append(c.Deployments, out)
		}
	}
	for i := first; i < len(c.Deployments); i++ {
		d := &c.Deployments[i]
		d.Revisions = revisions[d.Namespace+"/"+d.Name]
		slices.SortFunc(d.Revisions, func(x, y Revision) int { return cmp.Compare(y.Number, x.Number) })
	}
	for _, p := range pods {
		owner := p.controller()
		if owner.Kind == "ReplicaSet" {
			if up, ok := sets[p.Metadata.Namespace+"/"+owner.Name]; ok && up.Kind != "" {
				owner = up
			}
		}
		c.Pods = append(c.Pods, p.pod(owner))
	}
	return nil
}

// pod is what the view shows of p.
func (p pod) pod(owner Owner) Pod {
	out := Pod{
		Namespace: p.Metadata.Namespace, Name: p.Metadata.Name, UID: p.Metadata.UID,
		Phase: p.Status.Phase, Reason: p.Status.Reason, Message: p.Status.Message,
		Node: p.Spec.NodeName, IP: p.Status.PodIP, QoS: p.Status.QOSClass, Account: p.Spec.ServiceAccountName,
		Labels: maps.Clone(p.Metadata.Labels), Created: p.Metadata.CreationTimestamp, Owner: owner,
		Requests: p.Spec.requests(), Limits: p.Spec.limits(),
	}
	for _, v := range p.Spec.Volumes {
		// Every pod gets a projected kube-api-access volume: no news.
		if v.apiAccess() {
			continue
		}
		out.Volumes = append(out.Volumes, v.Name)
	}
	out.Claims = p.claims()
	out.References, out.controllerUID = p.references(), p.controllerUID()
	out.Containers = containerViews(p.Spec.Containers, p.Status.ContainerStatuses)
	out.InitContainers = containerViews(p.Spec.InitContainers, p.Status.InitContainerStatuses)
	for _, spec := range p.Spec.Containers {
		for _, port := range spec.Ports {
			if (port.Protocol == "TCP" || port.Protocol == "") && !slices.Contains(out.TCPPorts, port.Number) {
				out.TCPPorts = append(out.TCPPorts, port.Number)
			}
		}
	}
	for _, ct := range out.Containers {
		if ct.Ready {
			out.Ready++
		}
		out.Restarts += ct.Restarts
		if out.Reason != "" {
			continue
		}
		switch {
		case ct.State == "waiting" && ct.Reason != "" && ct.Reason != "ContainerCreating":
			out.Reason = ct.Reason
		case ct.State == "terminated" && ct.Reason != "" && ct.Reason != "Completed":
			out.Reason = ct.Reason
		}
	}
	for _, cond := range p.Status.Conditions {
		if cond.Type == "PodScheduled" && cond.Status == "False" {
			out.Unscheduled = cond.Message
			if out.Reason == "" && out.Phase == "Pending" && cond.Reason != "" {
				out.Reason = cond.Reason
			}
		}
	}
	return out
}

func containerViews(specs []podContainer, states []containerStatus) []Container {
	statuses := make(map[string]containerStatus, len(states))
	for _, status := range states {
		statuses[status.Name] = status
	}
	var containers []Container
	for _, spec := range specs {
		ct := Container{Name: spec.Name, Image: spec.Image,
			Requests: spec.Resources.Requests.resources(), Limits: spec.Resources.Limits.resources()}
		if s, ok := statuses[spec.Name]; ok {
			ct.Ready, ct.Restarts = s.Ready, s.RestartCount
			ct.Last = s.LastState.Terminated.run()
			switch st := s.State; {
			case st.Waiting != nil:
				ct.State, ct.Reason, ct.Message = "waiting", st.Waiting.Reason, st.Waiting.Message
			case st.Terminated != nil:
				ct.State, ct.Reason, ct.Message = "terminated", st.Terminated.Reason, st.Terminated.Message
				ct.Ended = st.Terminated.run()
			case st.Running != nil:
				ct.State, ct.Started = "running", st.Running.StartedAt
			}
		}
		containers = append(containers, ct)
	}
	return containers
}
