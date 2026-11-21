package kube

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const (
	Autoscalers       ResourceKind = "horizontalpodautoscalers.v2.autoscaling"
	DisruptionBudgets ResourceKind = "poddisruptionbudgets.v1.policy"
	ResourceQuotas    ResourceKind = "resourcequotas"
	LimitRanges       ResourceKind = "limitranges"
)

// ScaleTarget is the reference an HPA uses within its own namespace.
type ScaleTarget struct {
	APIVersion, Kind, Name string
}

func (t ScaleTarget) Targets(kind, name string) bool {
	group, _, _ := strings.Cut(t.APIVersion, "/")
	if t.Kind != kind || t.Name != name {
		return false
	}
	switch kind {
	case "Deployment":
		return group == "apps" || group == "extensions"
	case "StatefulSet":
		return group == "apps"
	}
	return false
}

func (t ScaleTarget) TargetsDeployment(name string) bool {
	return t.Targets("Deployment", name)
}

type Autoscaler struct {
	Target           ScaleTarget
	Min, Max         int
	Current, Desired *int
	DownscaleWindow  *int
	LastScale        time.Time
	Conditions       []CustomCondition
	Metrics          []AutoscalerMetric
}

type AutoscalerMetric struct {
	Name, Source, Current, Target string
}

type DisruptionBudget struct {
	Selector                                                         *labelSelector
	MinAvailable, MaxUnavailable                                     json.RawMessage
	CurrentHealthy, DesiredHealthy, ExpectedPods, DisruptionsAllowed int
	CurrentStatus                                                    bool
	Conditions                                                       []CustomCondition
}

type ResourceQuota struct {
	Hard, Used map[string]string
}

type LimitRangeItem struct {
	Type                                                    string
	Min, Max, Default, DefaultRequest, MaxLimitRequestRatio map[string]string
}

type policyObject struct {
	meta
	Spec struct {
		ScaleTargetRef ScaleTarget
		Selector       *labelSelector
		MinReplicas    *int
		MaxReplicas    int
		Metrics        []metricObject
		Behavior       struct {
			ScaleDown struct{ StabilizationWindowSeconds *int }
		}
		MinAvailable, MaxUnavailable json.RawMessage
		Limits                       []LimitRangeItem
	}
	Status struct {
		CurrentReplicas, DesiredReplicas             *int
		LastScaleTime                                time.Time
		CurrentMetrics                               []metricObject
		Conditions                                   []CustomCondition
		ObservedGeneration                           int64
		CurrentHealthy, DesiredHealthy, ExpectedPods int
		DisruptionsAllowed                           *int
		Hard, Used                                   map[string]string
	}
}

type metricObject struct {
	Type                                                string
	Resource, ContainerResource, External, Object, Pods metricSource
}

type metricSource struct {
	Name, Container string
	Metric          struct {
		Name     string
		Selector map[string]any
	}
	DescribedObject ScaleTarget
	Target, Current metricValue
}

type metricValue struct {
	Type, Value, AverageValue string
	AverageUtilization        *int
}

func (v metricValue) format(valueType string) string {
	switch valueType {
	case "Utilization":
		if v.AverageUtilization != nil {
			return strconv.Itoa(*v.AverageUtilization) + "%"
		}
	case "AverageValue":
		return v.AverageValue
	case "Value":
		return v.Value
	}
	return ""
}

func (m metricObject) source() metricSource {
	switch m.Type {
	case "Resource":
		return m.Resource
	case "ContainerResource":
		return m.ContainerResource
	case "External":
		return m.External
	case "Object":
		return m.Object
	case "Pods":
		return m.Pods
	}
	return metricSource{}
}

func autoscalerMetrics(specs, current []metricObject) []AutoscalerMetric {
	var metrics []AutoscalerMetric
	for _, spec := range specs {
		src := spec.source()
		name := src.Name
		if src.Container != "" {
			name += " · " + src.Container
		}
		if src.Metric.Name != "" {
			name = src.Metric.Name
			if src.DescribedObject.Name != "" {
				name += " · " + src.DescribedObject.Kind + "/" + src.DescribedObject.Name
			}
		}
		m := AutoscalerMetric{Name: name, Source: spec.Type, Target: src.Target.format(src.Target.Type)}
		for _, status := range current {
			s := status.source()
			if status.Type == spec.Type && s.Name == src.Name && s.Container == src.Container &&
				s.DescribedObject == src.DescribedObject && reflect.DeepEqual(s.Metric, src.Metric) {
				m.Current = s.Current.format(src.Target.Type)
				break
			}
		}
		metrics = append(metrics, m)
	}
	return metrics
}

func (o policyObject) resource(kind ResourceKind) Resource {
	m := o.Metadata
	r := Resource{Kind: kind, Namespace: m.Namespace, Name: m.Name, UID: m.UID,
		Created: m.CreationTimestamp, Deleting: !m.DeletionTimestamp.IsZero()}
	switch kind {
	case Autoscalers:
		minReplicas := 1
		if o.Spec.MinReplicas != nil {
			minReplicas = *o.Spec.MinReplicas
		}
		r.Autoscaler = &Autoscaler{Target: o.Spec.ScaleTargetRef, Min: minReplicas, Max: o.Spec.MaxReplicas,
			Current: o.Status.CurrentReplicas, Desired: o.Status.DesiredReplicas, LastScale: o.Status.LastScaleTime,
			DownscaleWindow: o.Spec.Behavior.ScaleDown.StabilizationWindowSeconds,
			Conditions:      o.Status.Conditions, Metrics: autoscalerMetrics(o.Spec.Metrics, o.Status.CurrentMetrics)}
	case DisruptionBudgets:
		r.Budget = &DisruptionBudget{Selector: o.Spec.Selector, MinAvailable: o.Spec.MinAvailable, MaxUnavailable: o.Spec.MaxUnavailable,
			CurrentHealthy: o.Status.CurrentHealthy, DesiredHealthy: o.Status.DesiredHealthy, ExpectedPods: o.Status.ExpectedPods,
			Conditions: o.Status.Conditions}
		if o.Status.DisruptionsAllowed != nil && o.Status.ObservedGeneration == m.Generation {
			r.Budget.CurrentStatus, r.Budget.DisruptionsAllowed = true, *o.Status.DisruptionsAllowed
		}
	case ResourceQuotas:
		r.Quota = &ResourceQuota{Hard: o.Status.Hard, Used: o.Status.Used}
	case LimitRanges:
		r.Limits = o.Spec.Limits
	}
	return r
}

func readPolicies(ctx context.Context, r run.Runner, env sysenv.Env, t Target) Cluster {
	var list [4]ResourceList
	var wg sync.WaitGroup
	for i, kind := range []ResourceKind{Autoscalers, DisruptionBudgets, ResourceQuotas, LimitRanges} {
		wg.Go(func() {
			items, err := readPolicyResources(ctx, r, env, t, kind)
			list[i] = ResourceList{Kind: kind, Items: items, Err: err}
		})
	}
	wg.Wait()
	c := Cluster{Taken: time.Now(), Policies: list[:]}
	for i := range list {
		if slices.ContainsFunc(list[i].Items, func(item Resource) bool { return item.UID == t.Events.UID }) && t.Events.UID != "" {
			c.readListEvents(ctx, r, env, t, &list[i])
			break
		}
	}
	return c
}

func readPolicyResources(ctx context.Context, r run.Runner, env sysenv.Env, t Target, kind ResourceKind) ([]Resource, error) {
	items := []Resource{}
	for _, scope := range namespaceScopes(t.Namespaces) {
		var list struct{ Items []policyObject }
		if err := get(ctx, r, env, t, append([]string{string(kind)}, scope...), &list); err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			items = append(items, item.resource(kind))
		}
	}
	return items, nil
}

func (c *Cluster) readDeploymentAutoscalers(ctx context.Context, r run.Runner, env sysenv.Env, t Target) {
	for i := range c.Deployments {
		d := &c.Deployments[i]
		if d.UID != t.Events.UID || d.Namespace != t.Events.Namespace {
			continue
		}
		t.Namespaces = []string{d.Namespace}
		items, err := readPolicyResources(ctx, r, env, t, Autoscalers)
		d.AutoscalersErr = err
		for _, item := range items {
			if item.Namespace == d.Namespace && item.Autoscaler.Target.TargetsDeployment(d.Name) {
				d.Autoscalers = append(d.Autoscalers, item)
			}
		}
		return
	}
}
