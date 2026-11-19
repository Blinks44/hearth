package ui

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
)

func TestKubeResourceBrowser(t *testing.T) {
	for _, width := range []int{1000, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			var receive func(string, error)
			readCount, canceled := 0, 0
			a.ReadResourceYAML = func(target kube.Target, resource kube.Resource, done func(string, error)) func() {
				if target.Context != "kind-dev" || resource.UID != "cm1" || resource.Namespace != "default" {
					t.Fatalf("unexpected target: %+v, %+v", target, resource)
				}
				readCount++
				receive = done
				return func() { canceled++ }
			}
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, width, 680)
			tt.Advance(time.Second)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Advance(time.Second)
			}
			pick := func(query string, kind kube.ResourceKind) {
				t.Helper()
				click("All resources")
				tt.Type(query)
				tt.Key(0, ui.KeyEnter)
				tt.Advance(time.Second)
				if got := watches.running(); len(got) != 1 || got[0].Resource != kind {
					t.Fatalf("watches = %+v, want %s", got, kind)
				}
			}
			show := func(cl kube.Cluster) {
				t.Helper()
				watches.got[len(watches.got)-1](cl, nil)
				tt.Frame()
				tt.Advance(time.Second)
			}
			pick("ds", kube.DaemonSets)
			pick("ep", kube.Endpoints)
			pick("cm", kube.ConfigMaps)
			config := kube.Resource{Kind: kube.ConfigMaps, Name: "checkout-config", Namespace: "default", UID: "cm1",
				Keys: []string{"APP_ENV", "DB_HOST"}, Created: time.Now(), Immutable: true}
			cl := kube.Cluster{Taken: time.Now(), Resources: &kube.ResourceList{Kind: kube.ConfigMaps, Items: []kube.Resource{config}}}
			show(cl)
			click("checkout-config")
			hasTexts(t, tt, "2 keys", "DB_HOST", "Immutable")
			if readCount != 0 {
				t.Fatal("the overview read YAML")
			}
			want := kube.Ref{Namespace: "default", UID: "cm1"}
			if running := watches.running(); len(running) != 1 || running[0].Events != want {
				t.Fatalf("watches = %+v, want events for %+v", running, want)
			}
			if header, ok := tt.Find("STATUS"); !ok || header.X+header.W > float32(width)/2+125 {
				t.Fatalf("table exceeds its pane: %+v", header)
			}
			screenshot(t, tt, fmt.Sprintf("configmap-%d", width))

			cl.EventsOf = want
			cl.Resources.EventsErr = errors.New("not allowed to read events")
			show(cl)
			click("Events")
			hasTexts(t, tt, "not allowed to read events")
			click("YAML")
			if readCount != 1 {
				t.Fatalf("YAML reads = %d", readCount)
			}
			const manifest = "kind: ConfigMap\ndata:\n  APP_ENV: test\n"
			receive(manifest, nil)
			tt.Frame()
			click("Copy YAML")
			if tt.Clipboard() != manifest {
				t.Fatalf("clipboard = %q", tt.Clipboard())
			}
			click("Events")
			click("YAML")
			if readCount != 2 || canceled != 1 {
				t.Fatalf("YAML reads = %d, cancels = %d", readCount, canceled)
			}
			late := receive
			pick("ing", kube.Ingresses)
			late(manifest, nil)
			show(kube.Cluster{Taken: time.Now(), Resources: &kube.ResourceList{Kind: kube.Ingresses, Items: []kube.Resource{{
				Kind: kube.Ingresses, Name: "shop-ingress", Namespace: "default", UID: "ing1", IngressClass: "nginx",
				Addresses: []string{"192.0.2.20"}, Routes: []kube.IngressRoute{{Host: "shop.test", Path: "/api", Port: "80", Service: "backend", TLS: true}},
			}}}})
			if canceled != 2 || tt.HasText(manifest) || tt.HasText("checkout-config") {
				t.Fatal("old detail survived the kind switch")
			}
			click("shop-ingress")
			hasTexts(t, tt, "nginx", "shop.test/api → backend:80")
			screenshot(t, tt, fmt.Sprintf("ingress-%d", width))
			click("Open")
			if got := tt.OpenedURLs(); !slices.Equal(got, []string{"https://shop.test/api"}) {
				t.Fatalf("opened URLs = %v", got)
			}
		})
	}
}

func TestKubeResourceBrowserRussianLabels(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a := kubeApp()
	var watches clusterWatches
	a.WatchCluster = watches.watch
	a.cluster.kind = kindStatefulSets
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1000, 680)
	tt.Advance(time.Second)
	watches.got[0](kube.Cluster{Taken: time.Now(), Resources: &kube.ResourceList{Kind: kube.StatefulSets, Items: []kube.Resource{{
		Kind: kube.StatefulSets, Namespace: "default", Name: "checkout-store", UID: "sts1", Created: time.Now(),
		Desired: 2, Ready: 1, Service: "checkout-db", Strategy: "RollingUpdate", PodOrder: "OrderedReady",
	}}}}, nil)
	tt.Frame()
	tt.Advance(time.Second)
	if err := tt.Click("checkout-store"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	age, ok := tt.Find("ВОЗРАСТ")
	if !ok || age.W < 58 {
		t.Fatalf("age column is too narrow: %+v", age)
	}
	hasTexts(t, tt, "Стратегия", "RollingUpdate", "OrderedReady")
	screenshot(t, tt, "statefulset-1000-ru")
}
