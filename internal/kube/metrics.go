package kube

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
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

func readMetrics(ctx context.Context, r run.Runner, env sysenv.Env, t Target, path string) ([]metricSample, error) {
	var list struct {
		Items []metricSample `json:"items"`
	}
	// kubectl top reads these same endpoints. JSON preserves nanocores,
	// per-container values and the sample timestamp.
	for _, version := range []string{"v1", "v1beta1"} {
		path := "/apis/metrics.k8s.io/" + version + "/" + path
		err := kubectlJSON(ctx, r, env, t, []string{"--raw", path}, &list)
		if err == nil {
			return list.Items, nil
		}
		var rejected *Error
		if !errors.As(err, &rejected) || !strings.Contains(rejected.Access.Detail, "Error from server (NotFound)") {
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

func addPodMetrics(ctx context.Context, r run.Runner, env sysenv.Env, t Target, pods []Pod) error {
	type podKey struct{ namespace, name string }
	byName := make(map[podKey]*Pod, len(pods))
	for i := range pods {
		p := &pods[i]
		byName[podKey{p.Namespace, p.Name}] = p
	}
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
			return err
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", path, err)
			}
			continue
		}
		for _, sample := range samples {
			p := byName[podKey{sample.Metadata.Namespace, sample.Metadata.Name}]
			if p != nil && !sample.Timestamp.Before(p.Created) {
				p.applyMetrics(sample)
			}
		}
	}
	return firstErr
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
