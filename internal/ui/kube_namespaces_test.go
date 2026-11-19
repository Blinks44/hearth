package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeNamespaces(t *testing.T) {
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			if size[0] == 1000 {
				i18n.SetLocaleFunc(func() string { return "ru" })
				t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			}
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, size[0], size[1])
			tt.Advance(time.Second)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Advance(time.Second)
			}
			click(locales.KubeKindsAll())
			tt.Type("ns")
			tt.Key(0, ui.KeyEnter)
			tt.Advance(time.Second)
			if running := watches.running(); len(running) != 1 || running[0].Resource != kube.NamespaceKind {
				t.Fatalf("expected namespace watch: %+v", running)
			}
			ns := kube.Resource{Kind: kube.NamespaceKind, Name: "feature-long-namespace-name", UID: "terminating", Deleting: true,
				NamespaceInfo: &kube.NamespaceInfo{Phase: "Terminating", Labels: map[string]string{"team": "checkout"},
					Conditions: []kube.NodeCondition{{Type: "NamespaceFinalizersRemaining", Status: "True", Message: "1 object remains."}}}}
			list := &kube.ResourceList{Kind: kube.NamespaceKind, Items: []kube.Resource{ns},
				NamespaceMembers: []kube.NamespaceMember{{Kind: "ConfigMap", Name: "keep-me", UID: "cm", Finalizers: "hearth.example/test"}}}
			show := func(uid string) {
				watches.got[len(watches.got)-1](kube.Cluster{Taken: time.Now(), Resources: list, EventsOf: kube.Ref{UID: uid}}, nil)
				tt.Frame()
				tt.Advance(time.Second)
			}
			show("")
			click(ns.Name)
			show(ns.UID)
			hasTexts(t, tt, locales.KubeNamespaceNew(), strings.ToUpper(locales.KubeNamespaceDeletionBlockers()), "NamespaceFinalizersRemaining", "keep-me · hearth.example/test")
			header, headerOK := tt.Find(strings.ToUpper(locales.KubeClusterStatus()))
			status, statusOK := tt.Find(locales.KubeResourceTerminating())
			if !headerOK || !statusOK || status.X < header.X || status.X > header.X+12 {
				t.Fatalf("status column misaligned: header=%+v status=%+v", header, status)
			}
			if tt.HasText(locales.KubeResourceDelete()) {
				t.Fatal("a terminating namespace still offers deletion")
			}
			if running := watches.running(); len(running) != 1 || running[0].Events.UID != ns.UID {
				t.Fatalf("expected namespace details watch: %+v", running)
			}
			screenshot(t, tt, fmt.Sprintf("namespaces-%d", size[0]))
			click(locales.KubeNamespaceNew())
			hasTexts(t, tt, locales.KubeNamespaceNew(), "kind-dev")
		})
	}
}
