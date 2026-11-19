package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
)

func TestKubeForward(t *testing.T) {
	a := kubeApp()
	var watches clusterWatches
	a.WatchCluster = watches.watch
	a.CheckForwardPort = func(port int, done func(int, error)) {
		if port == 8080 {
			done(18080, ops.ErrForwardPortBusy)
			return
		}
		done(0, nil)
	}
	var ready func(kube.ForwardPod)
	var done func(error)
	var stopped bool
	wantRemote := 80
	a.RunForward = func(f ops.Forward, onReady func(kube.ForwardPod), onDone func(error)) func() {
		want := ops.Forward{Kubectl: "/opt/homebrew/bin/kubectl", Context: "kind-dev", Namespace: "shop", Kind: "service", Name: "checkout-api", UID: "svc-1", Local: 18080, Remote: wantRemote}
		if f != want {
			t.Fatalf("forward = %+v, want %+v", f, want)
		}
		ready, done = onReady, onDone
		return func() { stopped = true }
	}
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("Services")
	targets := watches.running()
	if len(targets) != 1 || !targets[0].Services {
		t.Fatalf("read targets = %+v", targets)
	}
	watches.got[len(watches.got)-1](kube.Cluster{Taken: time.Now(), Services: []kube.Service{
		{Namespace: "shop", Name: "checkout-api", UID: "svc-1", Type: "ClusterIP", ClusterIP: "10.96.40.12", HasSelector: true,
			PodsKnown: true, Pods: 3, Ready: 2, Ports: []kube.ServicePort{{Port: 80, Protocol: "TCP", Target: "8080"}}},
		{Namespace: "shop", Name: "gateway", UID: "svc-2", Type: "ExternalName", External: true},
	}}, nil)
	tt.Frame()
	hasTexts(t, tt, "checkout-api", "2 of 3 ready", "gateway", "80 → 8080/TCP")
	click("Forward port…")
	hasTexts(t, tt, "Local port 8080 is already in use.", "Use free port 18080.", "127.0.0.1:8080")
	screenshot(t, tt, "kube-forward-conflict")
	if ready != nil {
		t.Fatal("opening the form started forwarding")
	}
	click("Use free port 18080.")
	hasTexts(t, tt, "127.0.0.1:18080")
	click("Forward")
	hasTexts(t, tt, "Port forwards", "Connecting")
	ready(kube.ForwardPod{Name: "checkout-api-123", UID: "pod-1", Port: 8080})
	tt.Frame()
	hasTexts(t, tt, "127.0.0.1:18080", "Active")
	click("Ports ›")
	tt.Advance(time.Second)
	hasTexts(t, tt, "Port forwards", "Active", "kind-dev ›")
	if stopped {
		t.Fatal("navigating away stopped the forward")
	}
	click("Stop")
	hasTexts(t, tt, "Stopping")
	if !stopped {
		t.Fatal("stop did not cancel the forward")
	}
	done(nil)
	tt.Frame()
	if len(a.forwards) != 0 {
		t.Fatal("the stopped forward is still listed")
	}

	click("Kubernetes")
	a.openCluster("kind-dev")
	tt.Advance(time.Second)
	watches.got[len(watches.got)-1](kube.Cluster{Taken: time.Now(), Services: []kube.Service{
		{Namespace: "shop", Name: "checkout-api", UID: "svc-1", HasSelector: true,
			Ports: []kube.ServicePort{{Port: 80, Protocol: "TCP", Target: "8080"}, {Port: 443, Protocol: "TCP", Target: "8443"}}},
	}}, nil)
	tt.Frame()
	click("Forward port…")
	click("Use free port 18080.")
	click("80 → 8080/TCP")
	click("443 → 8443/TCP")
	wantRemote = 443
	click("Forward")
	ready(kube.ForwardPod{Name: "checkout-api-123", UID: "pod-1", Port: 8080})
	done(errors.New("kubectl: lost connection to pod"))
	tt.Frame()
	hasTexts(t, tt, "Disconnected", "kubectl: lost connection to pod")
	click("Dismiss")
	if len(a.forwards) != 0 {
		t.Fatal("the dismissed forward is still listed")
	}
}
