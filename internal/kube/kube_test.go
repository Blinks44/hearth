package kube

import (
	"context"
	"errors"
	"os"
	"reflect"
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
	at := func(hour, min, sec int) time.Time { return time.Date(2026, 11, 18, hour, min, sec, 0, time.UTC) }
	created := at(8, 0, 0)
	tests := []struct {
		name string
		want Pod
	}{
		{"checkout-api-7d9f8c5b-x2k4p", Pod{Namespace: "shop", Name: "checkout-api-7d9f8c5b-x2k4p", UID: "p1", Phase: "Running", Reason: "CrashLoopBackOff",
			Ready: 1, Restarts: 14, Node: "kind-worker", IP: "10.244.1.17", QoS: "Burstable", Account: "checkout",
			References: []Relationship{
				{Kind: "ConfigMap", Namespace: "shop", Name: "checkout", Detail: "config"},
				{Kind: "Secret", Namespace: "shop", Name: "checkout-db", Detail: "DB_PASSWORD"},
				{Kind: "ServiceAccount", Namespace: "shop", Name: "checkout"},
			},
			Labels: map[string]string{"app": "checkout-api", "pod-template-hash": "7d9f8c5b"}, Volumes: []string{"config"},
			Created: created, Owner: Owner{"Deployment", "checkout-api"},
			// In the spec's order, whatever the statuses' order.
			Containers: []Container{
				{Name: "api", Image: "ghcr.io/acme/checkout:2.8.1", Restarts: 14, State: "waiting", Reason: "CrashLoopBackOff",
					Message: "back-off 5m0s restarting failed container=api pod=checkout-api-7d9f8c5b-x2k4p_shop(p1)",
					Last:    &Run{Reason: "Error", ExitCode: 1, Started: at(9, 58, 0), Finished: at(9, 58, 3)},
					// A Secret's key is named, never read.
					Env: []EnvVar{{Name: "LOG_LEVEL", Value: "debug"}, {Name: "DB_PASSWORD", From: "Secret", Ref: "checkout-db", Key: "password"},
						{Name: "POD_IP", From: "Field", Ref: "status.podIP"}},
					EnvFrom: []EnvSource{{Kind: "ConfigMap", Name: "checkout", Prefix: "APP_"}}},
				{Name: "envoy", Image: "envoyproxy/envoy:v1.31.2", Ready: true, State: "running", Started: at(8, 0, 5)},
			}}},
		{"checkout-api-7d9f8c5b-m8q2z", Pod{Namespace: "shop", Name: "checkout-api-7d9f8c5b-m8q2z", UID: "p2", Phase: "Running",
			Ready: 1, Node: "kind-worker2", Created: created, Owner: Owner{"Deployment", "checkout-api"},
			Containers: []Container{{Name: "api", Image: "ghcr.io/acme/checkout:2.8.1", Ready: true, State: "running"}}}},
		{"postgres-0", Pod{Namespace: "shop", Name: "postgres-0", UID: "p3", Phase: "Pending", Reason: "Unschedulable",
			Unscheduled: "0/3 nodes are available: pod has unbound immediate PersistentVolumeClaims. preemption: 0/3 nodes are available: 3 Preemption is not helpful for scheduling.",
			Volumes:     []string{"data"}, Claims: []string{"data-postgres-0"}, Created: at(9, 57, 0), Owner: Owner{"StatefulSet", "postgres"},
			References: []Relationship{{Kind: "PersistentVolumeClaim", Namespace: "shop", Name: "data-postgres-0", Detail: "data"}},
			Containers: []Container{{Name: "postgres", Image: "postgres:17"}}}},
		// Its ReplicaSet was not in the answer: the pod keeps it as owner.
		{"web-frontend-5c8d7f9b4-k2j9x", Pod{Namespace: "shop", Name: "web-frontend-5c8d7f9b4-k2j9x", UID: "p4", Phase: "Pending", Reason: "ImagePullBackOff",
			Node: "kind-worker2", Created: at(9, 54, 0), Owner: Owner{"ReplicaSet", "web-frontend-5c8d7f9b4"},
			Containers: []Container{{Name: "web", Image: "ghcr.io/acme/web:1.4.2", State: "waiting", Reason: "ImagePullBackOff",
				Message: `Back-off pulling image "ghcr.io/acme/web:1.4.2": ErrImagePull: ghcr.io/acme/web:1.4.2: not found: manifest unknown`}}}},
		// A container that completed is no problem.
		{"db-migrate-29381-abcde", Pod{Namespace: "shop", Name: "db-migrate-29381-abcde", UID: "p5", Phase: "Succeeded",
			Created: at(9, 0, 0), Owner: Owner{"Job", "db-migrate-29381"},
			Containers: []Container{{Name: "migrate", Image: "ghcr.io/acme/migrate:3", State: "terminated", Reason: "Completed",
				Ended: &Run{Reason: "Completed", Started: at(9, 0, 2), Finished: at(9, 0, 40)}}}}},
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
			if !reflect.DeepEqual(c.Pods[i], tt.want) {
				t.Errorf("got  %+v\nwant %+v", c.Pods[i], tt.want)
			}
		})
	}
	nov6 := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	want := []Deployment{
		{Namespace: "shop", Name: "checkout-api", UID: "d1", Replicas: 3, Pods: 3, Ready: 2, Updated: 3, Available: 2,
			Strategy: "RollingUpdate", Images: []string{"ghcr.io/acme/checkout:2.8.1", "envoyproxy/envoy:v1.31.2"}, Revision: 7,
			// Newest first, whatever the answer's order.
			Revisions: []Revision{
				{Number: 7, Name: "checkout-api-7d9f8c5b", UID: "r1", Images: []string{"ghcr.io/acme/checkout:2.8.1", "envoyproxy/envoy:v1.31.2"}, Pods: 3, Ready: 2, Created: created},
				{Number: 6, Name: "checkout-api-6b4f7d9c8", UID: "r0", Images: []string{"ghcr.io/acme/checkout:2.8.0", "envoyproxy/envoy:v1.31.2"}, Created: time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)},
			},
			Stalled: `ReplicaSet "checkout-api-7d9f8c5b" has timed out progressing.`, Created: nov6},
		{Namespace: "shop", Name: "search-indexer", UID: "d2", Paused: true, Created: nov6},
	}
	if !reflect.DeepEqual(c.Deployments, want) {
		t.Errorf("deployments = %+v\nwant %+v", c.Deployments, want)
	}
	if c.Taken.IsZero() {
		t.Error("Taken is zero")
	}
	if len(f.Calls()) != 1 {
		t.Errorf("calls = %d, want 1: no events were asked for", len(f.Calls()))
	}
}

func TestReadEvents(t *testing.T) {
	data, err := os.ReadFile("testdata/events.json")
	if err != nil {
		t.Fatal(err)
	}
	var f run.Fake
	f.Stdout(`{"items": []}`, getArgv("--namespace=shop")...)
	f.Stdout(string(data), "kubectl", "--context=kind-dev", "get", "events", "--namespace=shop",
		"--field-selector=involvedObject.uid=p1", "--output=json", "--request-timeout=15s")
	tg := target
	tg.Events = Ref{Namespace: "shop", UID: "p1"}
	c, err := Read(context.Background(), &f, nil, tg)
	if err != nil {
		t.Fatal(err)
	}
	at := func(hour, min int) time.Time { return time.Date(2026, 11, 18, hour, min, 0, 0, time.UTC) }
	want := []Event{
		{Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container api in pod checkout-api-7d9f8c5b-x2k4p_shop(p1)", Count: 52, Last: at(9, 59)},
		// A newer cluster's event counts in its series.
		{Type: "Normal", Reason: "Pulled", Message: `Container image "ghcr.io/acme/checkout:2.8.1" already present on machine`, Count: 15, Last: at(9, 56)},
		{Type: "Normal", Reason: "Scheduled", Message: "Successfully assigned shop/checkout-api-7d9f8c5b-x2k4p to kind-worker", Count: 1, Last: at(8, 0)},
	}
	if !reflect.DeepEqual(c.Events, want) {
		t.Errorf("events = %+v\nwant %+v", c.Events, want)
	}
	if c.EventsOf != tg.Events {
		t.Errorf("EventsOf = %+v, want %+v", c.EventsOf, tg.Events)
	}
}

func TestBackOff(t *testing.T) {
	tests := map[string]time.Duration{
		"back-off 5m0s restarting failed container=api pod=x_shop(p1)": 5 * time.Minute,
		"back-off 40s restarting failed container=api":                 40 * time.Second,
		"Back-off pulling image":                                       0,
	}
	for msg, want := range tests {
		if got := BackOff(msg); got != want {
			t.Errorf("BackOff(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestPullCause(t *testing.T) {
	tests := []struct {
		message string
		want    Pull
	}{
		// What kind in the VM said of an image from a domain that does
		// not exist.
		{`Back-off pulling image "example.invalid/nope:1": ErrImagePull: failed to pull and unpack image "example.invalid/nope:1": failed to resolve reference "example.invalid/nope:1": failed to do request: Head "https://example.invalid/v2/nope/manifests/1": dial tcp: lookup example.invalid on 172.18.0.1:53: no such host`, PullUnreachable},
		{`Back-off pulling image "ghcr.io/acme/web:1.4.2": ErrImagePull: ghcr.io/acme/web:1.4.2: not found: manifest unknown`, PullNotFound},
		{`failed to pull and unpack image "docker.io/library/nope:1": failed to resolve reference: pull access denied, repository does not exist or may require authorization`, PullDenied},
		{`failed to authorize: failed to fetch anonymous token: unexpected status: 401 Unauthorized`, PullDenied},
		{`Back-off pulling image "web:1"`, PullUnknown},
	}
	for _, tt := range tests {
		if got := PullCause(tt.message); got != tt.want {
			t.Errorf("PullCause(%q) = %v, want %v", tt.message, got, tt.want)
		}
	}
}

func TestScheduleCauses(t *testing.T) {
	tests := []struct {
		message string
		want    []Unschedulable
	}{
		{"0/3 nodes are available: pod has unbound immediate PersistentVolumeClaims.", []Unschedulable{NoVolume}},
		{"0/3 nodes are available: 1 node(s) had untolerated taint {node-role.kubernetes.io/control-plane: }, 2 Insufficient cpu.", []Unschedulable{NoResources, Tainted}},
		{"0/1 nodes are available: 1 node(s) didn't match Pod's node affinity/selector.", []Unschedulable{NoMatchingNode}},
		{"0/1 nodes are available: something new.", nil},
	}
	for _, tt := range tests {
		if got := ScheduleCauses(tt.message); !slices.Equal(got, tt.want) {
			t.Errorf("ScheduleCauses(%q) = %v, want %v", tt.message, got, tt.want)
		}
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
			// The namespaces are read at once, in no order.
			key := func(x, y []string) int { return slices.Compare(x, y) }
			slices.SortFunc(got, key)
			want := slices.Clone(tt.want)
			slices.SortFunc(want, key)
			if !slices.EqualFunc(got, want, slices.Equal) {
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
