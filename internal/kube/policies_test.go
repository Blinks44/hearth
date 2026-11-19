package kube

import (
	"errors"
	"os"
	"testing"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

func TestReadPolicies(t *testing.T) {
	t.Parallel()
	var fake run.Fake
	target := Target{Kubectl: "kubectl", Context: "dev", Namespaces: []string{"shop"}, Policies: true, Events: Ref{Namespace: "shop", UID: "hpa"}}
	args := func(kind ResourceKind) []string {
		return []string{"kubectl", "--context=dev", "get", string(kind), "--namespace=shop", "--output=json", "--request-timeout=15s"}
	}
	fake.Stdout(policyFixture(t, "hpa"), args(Autoscalers)...)
	fake.Stdout(policyFixture(t, "budget"), args(DisruptionBudgets)...)
	fake.Stdout(policyFixture(t, "quota"), args(ResourceQuotas)...)
	fake.Set(run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden): cannot list limitranges")}, nil, args(LimitRanges)...)
	fake.Stdout(`{"items":[]}`, "kubectl", "--context=dev", "get", "events", "--namespace=shop", "--field-selector=involvedObject.uid=hpa", "--output=json", "--request-timeout=15s")
	cluster, err := Read(t.Context(), &fake, nil, target)
	if err != nil || len(cluster.Policies) != 4 || cluster.EventsOf != target.Events {
		t.Fatalf("Read = %+v, %v", cluster, err)
	}
	h := cluster.Policies[0].Items[0].Autoscaler
	if h.Min != 1 || h.Max != 3 || h.Current == nil || *h.Current != 0 || !h.Target.TargetsDeployment("checkout") {
		t.Fatalf("autoscaler = %+v", h)
	}
	want := []AutoscalerMetric{
		{Name: "cpu", Source: "Resource", Current: "85%", Target: "70%"},
		{Name: "memory · web", Source: "ContainerResource", Target: "256Mi"},
		{Name: "queue_depth", Source: "External", Current: "7", Target: "10"},
		{Name: "queue_depth", Source: "External", Current: "20", Target: "25"},
	}
	if len(h.Metrics) != len(want) {
		t.Fatalf("metrics = %+v", h.Metrics)
	}
	for i, metric := range h.Metrics {
		if metric != want[i] {
			t.Errorf("metric %d = %+v, want %+v", i, metric, want[i])
		}
	}
	budgets := cluster.Policies[1].Items
	if b := budgets[0].Budget; b.CurrentStatus || string(b.MaxUnavailable) != `"25%"` {
		t.Fatalf("stale budget = %+v", b)
	}
	if b := budgets[1].Budget; !b.CurrentStatus || b.DisruptionsAllowed != 0 || string(b.MinAvailable) != "1" {
		t.Fatalf("current budget = %+v", b)
	}
	if q := cluster.Policies[2].Items[0].Quota; q.Hard["requests.cpu"] != "4" || q.Used["requests.cpu"] != "750m" {
		t.Fatalf("quota = %+v", q)
	}
	denial, ok := errors.AsType[*Error](cluster.Policies[3].Err)
	if !ok || denial.Access.State != detect.KubeForbidden {
		t.Fatalf("LimitRange denial = %v", cluster.Policies[3].Err)
	}
}

func TestDeploymentAutoscalerLookup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		denied bool
	}{
		{name: "target and namespace match only"},
		{name: "denial preserves deployment", denied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var fake run.Fake
			target := Target{Kubectl: "kubectl", Context: "dev", DeploymentAutoscalers: true, Events: Ref{Namespace: "shop", UID: "deploy"}}
			fake.Stdout(`{"items":[{"kind":"Deployment","metadata":{"name":"checkout","namespace":"shop","uid":"deploy"}}]}`,
				"kubectl", "--context=dev", "get", "pods,replicasets,deployments", "--all-namespaces", "--output=json", "--request-timeout=15s")
			fake.Stdout(`{"items":[]}`, "kubectl", "--context=dev", "get", "events", "--namespace=shop", "--field-selector=involvedObject.uid=deploy", "--output=json", "--request-timeout=15s")
			args := []string{"kubectl", "--context=dev", "get", string(Autoscalers), "--namespace=shop", "--output=json", "--request-timeout=15s"}
			if tc.denied {
				fake.Set(run.Result{ExitCode: 1, Stderr: []byte("Error from server (Forbidden): cannot list horizontalpodautoscalers")}, nil, args...)
			} else {
				fake.Stdout(policyFixture(t, "hpa"), args...)
			}
			cluster, err := Read(t.Context(), &fake, nil, target)
			if err != nil || len(cluster.Deployments) != 1 {
				t.Fatalf("Read = %+v, %v", cluster, err)
			}
			deployment := cluster.Deployments[0]
			if tc.denied {
				if len(deployment.Autoscalers) != 0 || deployment.AutoscalersErr == nil {
					t.Fatalf("deployment = %+v", deployment)
				}
			} else if len(deployment.Autoscalers) != 1 || deployment.Autoscalers[0].UID != "hpa" {
				t.Fatalf("deployment = %+v", deployment)
			}
		})
	}
}

func policyFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/policies/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
