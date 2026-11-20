package kube

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

func TestReadOverview(t *testing.T) {
	target := target
	target.Overview = true
	var f run.Fake
	f.Stdout(`{"items":[]}`, getArgv("--namespace=shop")...)
	f.Stdout(`{"items":[
		{"metadata":{"name":"worker"},"status":{"allocatable":{"cpu":"2","memory":"4Gi"},"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.37.0"}}},
		{"metadata":{"name":"control-plane"},"status":{"allocatable":{"cpu":"4000m","memory":"8388608Ki"},"conditions":[{"type":"Ready","status":"False"}]}}
	]}`, overviewArgv("nodes")...)
	f.Stdout(`{"items":[
		{"kind":"Pod","metadata":{"namespace":"kube-system"},"spec":{"nodeName":"worker","containers":[{"resources":{"requests":{"cpu":"500m","memory":"256Mi"}}}],"initContainers":[
			{"restartPolicy":"Always","resources":{"requests":{"cpu":"200m","memory":"128Mi"}}},
			{"resources":{"requests":{"cpu":"1","memory":"64Mi"}}}
		],"overhead":{"cpu":"100m","memory":"16Mi"}},"status":{"phase":"Running"}},
		{"kind":"Pod","spec":{"nodeName":"worker","resources":{"requests":{"cpu":"2.5e-1","memory":"1Gi"}},"containers":[{"resources":{"requests":{"cpu":"125m"}}}]},"status":{"phase":"Pending"}},
		{"kind":"Pod","spec":{"containers":[{"resources":{"requests":{"cpu":"1"}}}]},"status":{"phase":"Pending"}},
		{"kind":"Pod","spec":{"nodeName":"worker","containers":[{"resources":{"requests":{"cpu":"1"}}}]},"status":{"phase":"Succeeded"}},
		{"kind":"Pod","spec":{"nodeName":"worker","containers":[{"resources":{"requests":{"cpu":"1"}}}]},"status":{"phase":"Failed"}}
	]}`, overviewArgv("pods", "--all-namespaces")...)
	now := time.Now().UTC()
	f.Stdout(fmt.Sprintf(`{"items":[
		{"type":"Warning","reason":"Old","lastTimestamp":%q},
		{"type":"Normal","reason":"Scheduled","lastTimestamp":%q},
		{"type":"Warning","reason":"BackOff","involvedObject":{"kind":"Pod","namespace":"shop","name":"web"},"count":2,"lastTimestamp":%q},
		{"type":"Warning","reason":"FailedScheduling","lastTimestamp":%q}
	]}`, now.Add(-2*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), now.Add(-time.Minute).Format(time.RFC3339)),
		overviewArgv("events", "--field-selector=type=Warning", "--namespace=shop")...)
	c, err := Read(t.Context(), &f, nil, target)
	if err != nil {
		t.Fatal(err)
	}
	o := c.Overview
	if o == nil || len(o.Nodes) != 2 || o.NodesErr != nil || o.RequestsErr != nil || o.WarningsErr != nil {
		t.Fatalf("overview = %+v", o)
	}
	if n := o.Nodes[0]; n.Name != "control-plane" || n.Ready || n.Allocatable != (Resources{CPU: 4, Memory: 8 << 30}) {
		t.Errorf("control plane = %+v", n)
	}
	if n := o.Nodes[1]; !n.Ready || n.Version != "v1.37.0" || n.Requested != (Resources{CPU: 1.55, Memory: 1424 << 20}) {
		t.Errorf("worker = %+v", n)
	}
	if len(o.Warnings) != 2 || o.Warnings[0].Object.Name != "web" || o.Warnings[0].Namespace != "shop" || o.Warnings[0].Count != 2 || o.Warnings[1].Reason != "FailedScheduling" {
		t.Errorf("warnings = %+v", o.Warnings)
	}
}

func TestReadOverviewForbidden(t *testing.T) {
	target := target
	target.Overview = true
	var f run.Fake
	f.Stdout(`{"items":[{"kind":"Pod","metadata":{"name":"web"}}]}`, getArgv("--namespace=shop")...)
	f.Set(run.Result{Stderr: []byte("Error from server (Forbidden): forbidden"), ExitCode: 1}, nil, overviewArgv("nodes")...)
	f.Stdout(`{"items":[]}`, overviewArgv("events", "--field-selector=type=Warning", "--namespace=shop")...)
	c, err := Read(t.Context(), &f, nil, target)
	if err != nil || len(c.Pods) != 1 || c.Overview == nil || c.Overview.WarningsErr != nil {
		t.Fatalf("cluster = %+v, error = %v", c, err)
	}
	e, ok := errors.AsType[*Error](c.Overview.NodesErr)
	// The pods of every namespace are read beside the nodes, not after them.
	if !ok || e.Access.State != detect.KubeForbidden || len(f.Calls()) != 4 {
		t.Fatalf("node error = %v, calls = %+v", c.Overview.NodesErr, f.Calls())
	}
}

func overviewArgv(args ...string) []string {
	argv := append([]string{"kubectl", "--context=kind-dev", "get"}, args...)
	return append(argv, "--output=json", "--request-timeout=15s")
}
