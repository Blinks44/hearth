package kube

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

func metricArgv(version, resource string) []string {
	return []string{"kubectl", "--context=kind-dev", "get", "--raw", "/apis/metrics.k8s.io/" + version + "/" + resource, "--request-timeout=15s"}
}

func TestPodMetricsScopesAndSamples(t *testing.T) {
	data, err := os.ReadFile("testdata/metrics/pods.json")
	if err != nil {
		t.Fatal(err)
	}
	var fake run.Fake
	fake.Stdout(string(data), metricArgv("v1", "pods")...)
	tgt := target
	tgt.Namespaces = nil
	created := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	pods := []Pod{
		{Namespace: "shop", Name: "checkout", Created: created, Containers: []Container{{Name: "api", State: "running"}, {Name: "worker", State: "running"}}},
		{Namespace: "tools", Name: "checkout", Created: created, Containers: []Container{{Name: "api", State: "running", Started: created.Add(2 * time.Minute)}}},
		{Namespace: "shop", Name: "recreated", Created: created},
		{Namespace: "shop", Name: "pending", Created: created},
	}
	if err := addPodMetrics(t.Context(), &fake, nil, tgt, pods); err != nil {
		t.Fatal(err)
	}
	if p := pods[0]; p.Usage == nil || p.Usage.CPU < .179 || p.Usage.CPU > .181 || p.Usage.Memory != 96<<20 ||
		p.Containers[0].Usage == nil || p.Containers[1].Usage == nil || *p.Containers[1].Usage != (Resources{}) {
		t.Fatalf("metrics missing or incorrect: %+v, containers %+v", p.Usage, p.Containers)
	}
	for _, p := range pods[1:] {
		if p.Usage != nil {
			t.Errorf("%s/%s has a stale or missing sample: %+v", p.Namespace, p.Name, p.Usage)
		}
	}
}

func TestReadWithMetricsFailure(t *testing.T) {
	pods, err := os.ReadFile("testdata/shop.json")
	if err != nil {
		t.Fatal(err)
	}
	const missing = "Error from server (NotFound): the server could not find the requested resource"
	const denied = `Error from server (Forbidden): pods.metrics.k8s.io is forbidden`
	for _, test := range []struct {
		name, response string
		versionProbes  int
		check          func(error) bool
	}{
		{"not installed", missing, 2, func(err error) bool { return errors.Is(err, ErrMetricsUnavailable) }},
		{"forbidden", denied, 1, func(err error) bool {
			e, ok := errors.AsType[*Error](err)
			return ok && e.Access.State == detect.KubeForbidden
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var fake run.Fake
			fake.Stdout(string(pods), getArgv("--namespace=shop")...)
			for _, version := range []string{"v1", "v1beta1"} {
				fake.Set(run.Result{ExitCode: 1, Stderr: []byte(test.response)}, nil,
					metricArgv(version, "namespaces/shop/pods")...)
			}
			tgt := target
			tgt.Metrics = true
			c, err := Read(t.Context(), &fake, nil, tgt)
			if err != nil || len(c.Pods) == 0 || c.Pods[0].Usage != nil || !test.check(c.MetricsErr) {
				t.Fatalf("Read: %d pods, metrics error %v, read error %v", len(c.Pods), c.MetricsErr, err)
			}
			if len(fake.Calls()) != test.versionProbes+1 {
				t.Fatalf("unexpected retries: %v", fake.Calls())
			}
		})
	}
}

func TestNodeMetricsBeta(t *testing.T) {
	var fake run.Fake
	fake.Set(run.Result{ExitCode: 1, Stderr: []byte("Error from server (NotFound): the server could not find the requested resource")}, nil, metricArgv("v1", "nodes")...)
	fake.Stdout(`{"items":[{"metadata":{"name":"worker"},"usage":{"cpu":"250m","memory":"1Gi"}}]}`, metricArgv("v1beta1", "nodes")...)
	nodes, err := nodeMetrics(t.Context(), &fake, nil, target)
	if got := nodes["worker"]; got == nil || *got != (Resources{CPU: .25, Memory: 1 << 30}) || err != nil {
		t.Fatalf("node metrics = %+v, %v", got, err)
	}
}
