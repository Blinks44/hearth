package kube

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ErrMetricsUnavailable means neither version of the resource Metrics API
// is registered. It is distinct from access errors and missing samples.
var ErrMetricsUnavailable = errors.New("metrics API not available")

type metricContainer struct {
	Name  string     `json:"name"`
	Usage quantities `json:"usage"`
}

type metricSample struct {
	meta
	Timestamp  time.Time         `json:"timestamp"`
	Usage      quantities        `json:"usage"`
	Containers []metricContainer `json:"containers"`
}

// metricsVersion remembers, for a watch's reads, the version of the
// Metrics API the cluster served: most serve only v1beta1, and asking for
// v1 first would cost every read a call of kubectl and its sign-in.
type metricsVersion struct {
	mu      sync.Mutex
	version string
}

func (m *metricsVersion) get() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version
}

func (m *metricsVersion) set(v string) {
	if m != nil {
		m.mu.Lock()
		m.version = v
		m.mu.Unlock()
	}
}

func readMetrics(ctx context.Context, r run.Runner, env sysenv.Env, t Target, path string) ([]metricSample, error) {
	var list struct {
		Items []metricSample `json:"items"`
	}
	versions := []string{"v1", "v1beta1"}
	if served := t.metrics.get(); served != "" {
		versions = []string{served, "v1", "v1beta1"}
	}
	// kubectl top reads these same endpoints. JSON preserves nanocores,
	// per-container values and the sample timestamp.
	for i, version := range versions {
		if i > 0 && version == versions[0] {
			continue
		}
		path := "/apis/metrics.k8s.io/" + version + "/" + path
		err := kubectlJSON(ctx, r, env, t, []string{"--raw", path}, &list)
		if err == nil {
			t.metrics.set(version)
			return list.Items, nil
		}
		var rejected *Error
		if !errors.As(err, &rejected) || (!strings.Contains(rejected.Access.Detail, "Error from server (NotFound)") && !strings.Contains(rejected.Access.Detail, "Error from server (Not Found)")) {
			return nil, err
		}
	}
	return nil, ErrMetricsUnavailable
}

func (q quantities) usage() *Resources {
	if !q.CPU.set || !q.Memory.set {
		return nil
	}
	value := q.resources()
	return &value
}

// podSamples reads the pods' metrics in the target's namespaces, all of
// them for none.
func podSamples(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]metricSample, error) {
	var all []metricSample
	var firstErr error
	namespaces := t.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""}
	}
	for _, ns := range namespaces {
		path := "pods"
		if ns != "" {
			path = "namespaces/" + url.PathEscape(ns) + "/pods"
		}
		samples, err := readMetrics(ctx, r, env, t, path)
		if errors.Is(err, ErrMetricsUnavailable) || ctx.Err() != nil {
			return nil, err
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", path, err)
			}
			continue
		}
		all = append(all, samples...)
	}
	return all, firstErr
}

// applyPodMetrics gives each pod its sample.
func applyPodMetrics(pods []Pod, samples []metricSample) {
	type podKey struct{ namespace, name string }
	byName := make(map[podKey]*Pod, len(pods))
	for i := range pods {
		p := &pods[i]
		byName[podKey{p.Namespace, p.Name}] = p
	}
	for _, sample := range samples {
		p := byName[podKey{sample.Metadata.Namespace, sample.Metadata.Name}]
		if p != nil && !sample.Timestamp.Before(p.Created) {
			p.applyMetrics(sample)
		}
	}
}

func (p *Pod) applyMetrics(sample metricSample) {
	usageByName := make(map[string]*Resources, len(sample.Containers))
	var total Resources
	complete := len(sample.Containers) > 0
	for _, container := range sample.Containers {
		value := container.Usage.usage()
		usageByName[container.Name] = value
		if value == nil {
			complete = false
			continue
		}
		total = total.add(*value)
	}
	for _, containers := range [][]Container{p.Containers, p.InitContainers} {
		for i := range containers {
			ct := &containers[i]
			if !sample.Timestamp.Before(ct.Started) {
				ct.Usage = usageByName[ct.Name]
			}
			if ct.Usage == nil && ct.State == "running" {
				complete = false
			}
		}
	}
	if complete {
		p.Usage = &total
	}
}

func nodeMetrics(ctx context.Context, r run.Runner, env sysenv.Env, t Target) (map[string]*Resources, error) {
	samples, err := readMetrics(ctx, r, env, t, "nodes")
	if err != nil {
		return nil, err
	}
	values := make(map[string]*Resources, len(samples))
	for _, sample := range samples {
		values[sample.Metadata.Name] = sample.Usage.usage()
	}
	return values, nil
}
