package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/kube"
)

func TestKubeOverview(t *testing.T) {
	a := kubeApp()
	var w clusterWatches
	a.WatchCluster = w.watch
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if err := tt.Click("Cluster overview"); err != nil {
		t.Fatal(err)
	}
	if got := w.running(); len(got) != 1 || !got[0].Overview || got[0].Events != (kube.Ref{}) || !w.stopped[0] {
		t.Fatalf("watches = %+v", got)
	}
	cl := shopCluster()
	cl.Overview = &kube.Overview{
		Nodes: []kube.Node{
			{Name: "kind-control-plane", Version: "v1.37.0", Ready: true, Allocatable: kube.Resources{CPU: 4, Memory: 8 << 30}, Requested: kube.Resources{CPU: 1, Memory: 2 << 30}},
			{Name: "kind-worker", Version: "v1.37.0", Allocatable: kube.Resources{CPU: 4, Memory: 8 << 30}, Requested: kube.Resources{CPU: 2, Memory: 6 << 30}},
		},
		Warnings: []kube.Event{{Type: "Warning", Reason: "BackOff", Object: kube.Owner{Kind: "Pod", Name: cl.Pods[1].Name}, Namespace: "default", Count: 52}},
	}
	w.got[1](cl, nil)
	tt.Frame()
	hasTexts(t, tt, "1 of 2 ready", "CPU: requested", "38%", "50%", "Needs attention", "BackOff", "52 times", "Node · kind-worker")
	screenshot(t, tt, "kube-overview")
	if err := tt.Click("Open Pod · " + cl.Pods[1].Name); err != nil {
		t.Fatal(err)
	}
	if got := w.running(); len(got) != 1 || got[0].Overview || got[0].Events.UID != cl.Pods[1].UID || !w.stopped[1] {
		t.Fatalf("watches = %+v", got)
	}
	if err := tt.Click("Cluster overview"); err != nil {
		t.Fatal(err)
	}
	// A cancelled watch may already have queued its last result.
	w.got[1](kube.Cluster{}, nil)
	tt.Frame()
	hasTexts(t, tt, "Needs attention")
	cl.Overview = &kube.Overview{NodesErr: &kube.Error{Access: detect.KubeAccess{State: detect.KubeForbidden}}}
	w.got[3](cl, nil)
	tt.Frame()
	hasTexts(t, tt, "no access", "Needs attention", "No warnings in the last hour.")
	if tt.HasText("38%") || tt.HasText("kind-control-plane") {
		t.Fatal("the old nodes and requests are still shown after an access error")
	}
	a.router.Push(kubePage)
	tt.Frame()
	if got := w.running(); len(got) != 0 {
		t.Fatalf("still reading after leaving the cluster: %+v", got)
	}
}
