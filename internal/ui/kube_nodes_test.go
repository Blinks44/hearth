package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeNodeDetails(t *testing.T) {
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
			a.cluster.kind = kindNodes
			tt := ui.NewTester(a.View, size[0], size[1])
			tt.Advance(time.Second)
			if running := watches.running(); len(running) != 1 || running[0].Resource != kube.Nodes {
				t.Fatalf("expected node watch: %+v", running)
			}
			node := kube.Resource{Kind: kube.Nodes, Name: "kind-worker-with-a-long-name", UID: "worker", Node: &kube.Node{
				Version: "v1.37.0", Allocatable: kube.Resources{CPU: 4, Memory: 8 << 30}, Addresses: []string{"172.18.0.4"}, PodCapacity: 110,
				Conditions: []kube.NodeCondition{{Type: "Ready", Status: "Unknown", Message: "Kubelet stopped posting node status."},
					{Type: "MemoryPressure", Status: "True"}},
				Taints: []kube.Taint{{Key: "node.kubernetes.io/unreachable", Effect: "NoExecute"}},
			}}
			list := &kube.ResourceList{Kind: kube.Nodes, Items: []kube.Resource{node}}
			show := func() {
				watches.got[len(watches.got)-1](kube.Cluster{Taken: time.Now(), Resources: list,
					EventsOf: kube.Ref{UID: a.cluster.other.uid}}, nil)
				tt.Frame()

				tt.Advance(time.Second)
			}
			show()
			if err := tt.Click(node.Name); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			show()
			hasTexts(t, tt, locales.KubeNodeNotReady(), locales.KubeNodeCordon(), locales.KubeNodeDrain(), "v1.37.0")
			cordon, foundCordon := tt.Find(locales.KubeNodeCordon())
			drain, foundDrain := tt.Find(locales.KubeNodeDrain())
			if !foundCordon || !foundDrain || cordon.H != drain.H || cordon.H != podActionHeight {
				t.Fatalf("inconsistent action sizes: cordon=%+v drain=%+v", cordon, drain)
			}
			screenshot(t, tt, fmt.Sprintf("nodes-%d", size[0]))
			list.NodePods = []kube.NodePod{{Name: "demo", Namespace: "work", Phase: "Running"}}
			a.cluster.other.tab = 1
			show()
			hasTexts(t, tt, "demo", "work", "Running")
		})
	}
}
