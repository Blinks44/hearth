package kube

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Relationship identifies a resource named by an object reference, selected
// by labels or reported as a Service backend. It carries no Secret values.
type Relationship struct {
	Kind, Namespace, Name, UID string
	Inbound                    bool
	Detail                     string
}

type Relationships struct {
	Object Ref
	Links  []Relationship
	Errors []error
}

func (d *Relationships) add(link Relationship) {
	if link.Name == "" {
		return
	}
	i := slices.IndexFunc(d.Links, func(old Relationship) bool {
		return old.Kind == link.Kind && old.Namespace == link.Namespace && old.Name == link.Name
	})
	if i < 0 {
		d.Links = append(d.Links, link)
	} else if d.Links[i].UID == "" && link.UID != "" {
		d.Links[i].UID = link.UID
	}
}

type objectName struct{ Name string }

// keyRef names a key of a ConfigMap or Secret.
type keyRef struct{ Name, Key string }

func (k *keyRef) object() *objectName {
	if k == nil {
		return nil
	}
	return &objectName{k.Name}
}

type podVolume struct {
	Name                  string
	PersistentVolumeClaim struct{ ClaimName string }
	ConfigMap             *objectName
	Secret                *struct{ SecretName string }
	Projected             *struct {
		Sources []struct{ ConfigMap, Secret *objectName }
	}
	Ephemeral *struct{ VolumeClaimTemplate *struct{} }
}

func (v podVolume) apiAccess() bool {
	return v.Projected != nil && strings.HasPrefix(v.Name, "kube-api-access-")
}

func (p pod) references() []Relationship {
	return podSpecReferences(p.Metadata.Namespace, p.Metadata.Name, p.Spec)
}

func podSpecReferences(namespace, podName string, spec podSpec) []Relationship {
	d := Relationships{}
	add := func(kind, name, detail string) {
		d.add(Relationship{Kind: kind, Namespace: namespace, Name: name, Detail: detail})
	}
	addNames := func(detail string, cm, secret *objectName) {
		if cm != nil {
			add("ConfigMap", cm.Name, detail)
		}
		if secret != nil {
			add("Secret", secret.Name, detail)
		}
	}
	for _, volume := range spec.Volumes {
		if volume.apiAccess() {
			continue
		}
		if volume.Projected != nil {
			for _, src := range volume.Projected.Sources {
				addNames(volume.Name, src.ConfigMap, src.Secret)
			}
		}
		add("PersistentVolumeClaim", volume.claimName(podName), volume.Name)
		addNames(volume.Name, volume.ConfigMap, nil)
		if volume.Secret != nil {
			add("Secret", volume.Secret.SecretName, volume.Name)
		}
	}
	for _, container := range slices.Concat(spec.Containers, spec.InitContainers, spec.EphemeralContainers) {
		for _, env := range container.Env {
			addNames(env.Name, env.ValueFrom.ConfigMapKeyRef.object(), env.ValueFrom.SecretKeyRef.object())
		}
		for _, from := range container.EnvFrom {
			addNames(container.Name, from.ConfigMapRef, from.SecretRef)
		}
	}
	for _, secret := range spec.ImagePullSecrets {
		add("Secret", secret.Name, "")
	}
	add("ServiceAccount", spec.ServiceAccountName, "")
	return d.Links
}

// labelSelector follows policy/v1 semantics. A nil selector matches no pods.
type labelSelector struct {
	MatchLabels      map[string]string
	MatchExpressions []struct {
		Key, Operator string
		Values        []string
	}
}

func (s *labelSelector) matches(labels map[string]string) bool {
	if s == nil || !selectorMatches(s.MatchLabels, labels) {
		return false
	}
	for _, req := range s.MatchExpressions {
		value, exists := labels[req.Key]
		contains := slices.Contains(req.Values, value)
		switch req.Operator {
		case "In":
			if !exists || !contains {
				return false
			}
		case "NotIn":
			if exists && contains {
				return false
			}
		case "Exists":
			if !exists {
				return false
			}
		case "DoesNotExist":
			if exists {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (d *Relationships) readBudgets(ctx context.Context, r run.Runner, env sysenv.Env, t Target, labels []map[string]string) {
	t.Namespaces = []string{d.Object.Namespace}
	budgets, err := readPolicyResources(ctx, r, env, t, DisruptionBudgets)
	if err != nil {
		d.Errors = append(d.Errors, err)
	}
	for _, item := range budgets {
		if item.Namespace == d.Object.Namespace && slices.ContainsFunc(labels, item.Budget.Selector.matches) {
			d.add(Relationship{Kind: "PodDisruptionBudget", Namespace: item.Namespace, Name: item.Name, UID: item.UID, Inbound: true})
		}
	}
}

func (c *Cluster) readRelationships(ctx context.Context, r run.Runner, env sysenv.Env, t Target) {
	if t.Events.UID == "" {
		return
	}
	d := &Relationships{Object: t.Events}
	ns := d.Object.Namespace
	switch {
	case t.Batch && c.Batch != nil:
		for _, job := range c.Batch.Jobs {
			if job.UID != d.Object.UID || job.Namespace != ns {
				continue
			}
			if job.CronJob != "" {
				d.add(Relationship{Kind: "CronJob", Namespace: ns, Name: job.CronJob, UID: job.CronJobUID, Inbound: true})
			}
			var pods struct{ Items []pod }
			if err := get(ctx, r, env, t, []string{"pods", "--namespace=" + ns}, &pods); err != nil {
				d.Errors = append(d.Errors, err)
			}
			var refs []Relationship
			for _, p := range pods.Items {
				m := p.Metadata
				if m.Namespace != ns || p.controllerUID() != job.UID {
					continue
				}
				d.add(Relationship{Kind: "Pod", Namespace: m.Namespace, Name: m.Name, UID: m.UID, Detail: p.Status.Phase})
				refs = append(refs, p.references()...)
			}
			for _, ref := range refs {
				d.add(ref)
			}
			for _, ref := range job.templateRefs {
				d.add(ref)
			}
			c.Relationships = d
			return
		}
		for _, cj := range c.Batch.CronJobs {
			if cj.UID != d.Object.UID || cj.Namespace != ns {
				continue
			}
			for _, run := range cj.Runs {
				d.add(Relationship{Kind: "Job", Namespace: run.Namespace, Name: run.Name, UID: run.UID})
			}
			for _, ref := range cj.templateRefs {
				d.add(ref)
			}
			c.Relationships = d
			return
		}
	case t.Resource == Ingresses || t.Resource == StatefulSets || t.Resource == DaemonSets || t.Resource == ReplicaSets || t.Resource == Endpoints:
		for _, obj := range c.Resources.Items {
			if obj.UID != d.Object.UID || obj.Namespace != ns {
				continue
			}
			switch obj.Kind {
			case Ingresses:
				for _, route := range obj.Routes {
					d.add(Relationship{Kind: "Service", Namespace: ns, Name: route.Service})
				}
				for _, secret := range obj.TLSSecrets {
					d.add(Relationship{Kind: "Secret", Namespace: ns, Name: secret, Detail: "TLS"})
				}
			case Endpoints:
				d.add(Relationship{Kind: "Service", Namespace: ns, Name: obj.Name, Inbound: true})
				for _, pod := range obj.endpointPods {
					d.add(pod)
				}
			default:
				d.readWorkloadController(ctx, r, env, t, obj)
			}
			c.Relationships = d
			return
		}
	case t.Services:
		for _, service := range c.Services {
			if service.UID == d.Object.UID && service.Namespace == ns {
				for _, pod := range service.relatedPods {
					d.add(pod)
				}
				if service.podsErr != nil {
					d.Errors = append(d.Errors, service.podsErr)
				}
				if service.Type != "ExternalName" {
					d.readEndpoints(ctx, r, env, t, service.Name)
				}
				c.Relationships = d
				return
			}
		}
	case t.DeploymentAutoscalers:
		for _, deployment := range c.Deployments {
			if deployment.UID != d.Object.UID || deployment.Namespace != ns {
				continue
			}
			for _, hpa := range deployment.Autoscalers {
				d.add(Relationship{Kind: "HorizontalPodAutoscaler", Namespace: ns, Name: hpa.Name, UID: hpa.UID, Inbound: true})
			}
			if deployment.AutoscalersErr != nil {
				d.Errors = append(d.Errors, deployment.AutoscalersErr)
			}
			labels := []map[string]string{deployment.podLabels}
			for _, pod := range c.Pods {
				if pod.Namespace == ns && slices.ContainsFunc(deployment.Revisions, func(rev Revision) bool { return rev.UID != "" && rev.UID == pod.controllerUID }) {
					labels = append(labels, pod.Labels)
				}
			}
			d.readBudgets(ctx, r, env, t, labels)
			c.Relationships = d
			return
		}
	}
}

func (d *Relationships) readWorkloadController(ctx context.Context, r run.Runner, env sysenv.Env, t Target, obj Resource) {
	if obj.Owner.Kind != "" {
		d.add(Relationship{Kind: obj.Owner.Kind, Namespace: obj.Namespace, Name: obj.Owner.Name, Inbound: true})
	}
	if obj.Service != "" {
		d.add(Relationship{Kind: "Service", Namespace: obj.Namespace, Name: obj.Service, Inbound: true})
	}
	var pods struct{ Items []pod }
	if err := get(ctx, r, env, t, []string{"pods", "--namespace=" + obj.Namespace}, &pods); err != nil {
		d.Errors = append(d.Errors, err)
	}
	labels := []map[string]string{obj.podLabels}
	var refs []Relationship
	for _, pod := range pods.Items {
		m := pod.Metadata
		if m.Namespace != obj.Namespace || pod.controllerUID() != obj.UID {
			continue
		}
		d.add(Relationship{Kind: "Pod", Namespace: m.Namespace, Name: m.Name, UID: m.UID, Detail: pod.Status.Phase})
		labels = append(labels, m.Labels)
		refs = append(refs, pod.references()...)
	}
	for _, ref := range refs {
		d.add(ref)
	}
	d.readBudgets(ctx, r, env, t, labels)
}

type endpointAddress struct {
	TargetRef struct{ Kind, Namespace, Name, UID string }
}

func (d *Relationships) readEndpoints(ctx context.Context, r run.Runner, env sysenv.Env, t Target, name string) {
	var list struct {
		Items []struct {
			meta
			Subsets []struct{ Addresses, NotReadyAddresses []endpointAddress }
		}
	}
	ns := d.Object.Namespace
	args := []string{"endpoints", "--namespace=" + ns, "--field-selector=metadata.name=" + name}
	if err := get(ctx, r, env, t, args, &list); err != nil {
		d.Errors = append(d.Errors, err)
		return
	}
	for _, obj := range list.Items {
		if obj.Metadata.Namespace != ns || obj.Metadata.Name != name {
			continue
		}
		for _, subset := range obj.Subsets {
			for _, addr := range slices.Concat(subset.Addresses, subset.NotReadyAddresses) {
				ref := addr.TargetRef
				if ref.Kind == "Pod" && cmp.Or(ref.Namespace, ns) == ns {
					d.add(Relationship{Kind: "Pod", Namespace: ns, Name: ref.Name, UID: ref.UID})
				}
			}
		}
	}
}
