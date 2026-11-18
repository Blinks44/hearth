package kube

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

var target = Target{Kubectl: "kubectl", Context: "kind-dev", Namespaces: []string{"shop"}}

func getArgv(scope ...string) []string {
	argv := []string{"kubectl", "--context=kind-dev", "get", "pods,replicasets,deployments"}
	argv = append(argv, scope...)
	return append(argv, "--output=json", "--request-timeout=15s")
}

func TestRead(t *testing.T) {
	data, err := os.ReadFile("testdata/shop.json")
	if err != nil {
		t.Fatal(err)
	}
	var f run.Fake
	f.Stdout(string(data), getArgv("--namespace=shop")...)
	c, err := Read(context.Background(), &f, nil, target)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		want Pod
	}{
		{"checkout-api-7d9f8c5b-x2k4p", Pod{Namespace: "shop", Name: "checkout-api-7d9f8c5b-x2k4p", UID: "p1", Phase: "Running", Reason: "CrashLoopBackOff",
			Ready: 1, Containers: 2, Restarts: 14, Node: "kind-worker", Created: created, Owner: Owner{"Deployment", "checkout-api"}}},
		{"checkout-api-7d9f8c5b-m8q2z", Pod{Namespace: "shop", Name: "checkout-api-7d9f8c5b-m8q2z", UID: "p2", Phase: "Running",
			Ready: 1, Containers: 1, Node: "kind-worker2", Created: created, Owner: Owner{"Deployment", "checkout-api"}}},
		{"postgres-0", Pod{Namespace: "shop", Name: "postgres-0", UID: "p3", Phase: "Pending", Reason: "Unschedulable",
			Containers: 1, Created: time.Date(2026, 11, 18, 9, 57, 0, 0, time.UTC), Owner: Owner{"StatefulSet", "postgres"}}},
		// Its ReplicaSet was not in the answer: the pod keeps it as owner.
		{"web-frontend-5c8d7f9b4-k2j9x", Pod{Namespace: "shop", Name: "web-frontend-5c8d7f9b4-k2j9x", UID: "p4", Phase: "Pending", Reason: "ImagePullBackOff",
			Containers: 1, Node: "kind-worker2", Created: time.Date(2026, 11, 18, 9, 54, 0, 0, time.UTC), Owner: Owner{"ReplicaSet", "web-frontend-5c8d7f9b4"}}},
		// A container that completed is no problem.
		{"db-migrate-29381-abcde", Pod{Namespace: "shop", Name: "db-migrate-29381-abcde", UID: "p5", Phase: "Succeeded",
			Containers: 1, Created: time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC), Owner: Owner{"Job", "db-migrate-29381"}}},
	}
	if len(c.Pods) != len(tests) {
		t.Fatalf("pods = %d, want %d", len(c.Pods), len(tests))
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := slices.IndexFunc(c.Pods, func(p Pod) bool { return p.Name == tt.name })
			if i < 0 {
				t.Fatal("missing")
			}
			if c.Pods[i] != tt.want {
				t.Errorf("got  %+v\nwant %+v", c.Pods[i], tt.want)
			}
		})
	}
	want := []Deployment{
		{Namespace: "shop", Name: "checkout-api", UID: "d1", Replicas: 3, Ready: 2, Updated: 3, Created: time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)},
		{Namespace: "shop", Name: "search-indexer", UID: "d2", Created: time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)},
	}
	if !slices.Equal(c.Deployments, want) {
		t.Errorf("deployments = %+v, want %+v", c.Deployments, want)
	}
	if c.Taken.IsZero() {
		t.Error("Taken is zero")
	}
}

func TestReadScopes(t *testing.T) {
	empty := `{"items": []}`
	tests := []struct {
		name       string
		namespaces []string
		want       [][]string
	}{
		{"all", nil, [][]string{getArgv("--all-namespaces")}},
		{"two", []string{"shop", "payments"}, [][]string{getArgv("--namespace=shop"), getArgv("--namespace=payments")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f run.Fake
			for _, argv := range tt.want {
				f.Stdout(empty, argv...)
			}
			tg := target
			tg.Namespaces = tt.namespaces
			if _, err := Read(context.Background(), &f, nil, tg); err != nil {
				t.Fatal(err)
			}
			var got [][]string
			for _, call := range f.Calls() {
				got = append(got, call.Args)
			}
			if !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("calls = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadErrors(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   detect.KubeAccessState
	}{
		{"forbidden", `Error from server (Forbidden): pods is forbidden: User "dev" cannot list resource "pods" in API group "" in the namespace "shop"`, detect.KubeForbidden},
		{"login", "error: You must be logged in to the server (Unauthorized)", detect.KubeNeedsLogin},
		{"unreachable", "Unable to connect to the server: dial tcp 127.0.0.1:52341: connect: connection refused", detect.KubeUnreachable},
		{"unknown", "error: something new", detect.KubeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f run.Fake
			f.Set(run.Result{ExitCode: 1, Stderr: []byte(tt.stderr)}, nil, getArgv("--namespace=shop")...)
			_, err := Read(context.Background(), &f, nil, target)
			kerr, ok := errors.AsType[*Error](err)
			if !ok {
				t.Fatalf("err = %v, want *Error", err)
			}
			if kerr.Access.State != tt.want {
				t.Errorf("state = %v, want %v", kerr.Access.State, tt.want)
			}
			if tt.want == detect.KubeUnknown && kerr.Access.Detail != tt.stderr {
				t.Errorf("detail = %q, want kubectl's line", kerr.Access.Detail)
			}
		})
	}
}

func TestNamespaces(t *testing.T) {
	var f run.Fake
	f.Stdout(`{"items": [{"metadata": {"name": "shop"}}, {"metadata": {"name": "default"}}]}`,
		"kubectl", "--context=kind-dev", "get", "namespaces", "--output=json", "--request-timeout=15s")
	got, err := Namespaces(context.Background(), &f, nil, target)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"default", "shop"}; !slices.Equal(got, want) {
		t.Errorf("namespaces = %q, want %q", got, want)
	}
}

func TestWatchStops(t *testing.T) {
	var f run.Fake
	f.Stdout(`{"items": []}`, getArgv("--namespace=shop")...)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	reads := 0
	go func() {
		defer close(done)
		Watch(ctx, &f, nil, target, func(Cluster, error) {
			reads++
			cancel() // the view closed after the first read
		})
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Watch kept going after its context ended")
	}
	if reads != 1 {
		t.Errorf("reads = %d, want 1", reads)
	}
}
