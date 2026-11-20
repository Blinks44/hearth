package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeStorageNavigation(t *testing.T) {
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			if size[0] == 1000 {
				i18n.SetLocaleFunc(func() string { return "ru" })
				t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			}
			openClaim, diagnosis := "Open PVC", "Waiting for a volume. StorageClass fast-ssd does not exist in this cluster."
			if size[0] == 1000 {
				openClaim, diagnosis = "Открыть PVC", "PVC ждёт том. StorageClass fast-ssd нет в кластере."
			}
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
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
			now := time.Now()
			show := func(cluster kube.Cluster) {
				t.Helper()
				cluster.Taken = now
				watches.got[len(watches.got)-1](cluster, nil)
				tt.Frame()
				tt.Advance(time.Second)
			}
			pod := kube.Pod{Namespace: "default", Name: "postgres-0", UID: "pod", Phase: "Pending", Created: now,
				References: []kube.Relationship{{Kind: "PersistentVolumeClaim", Namespace: "default", Name: "data-postgres-0"}}}
			show(kube.Cluster{Pods: []kube.Pod{pod}})
			click(pod.Name)
			click(openClaim)
			if running := watches.running(); len(running) != 1 || running[0].Resource != kube.PersistentVolumeClaims {
				t.Fatalf("expected PVC watch, got %+v", running)
			}
			claim := kube.Resource{Kind: kube.PersistentVolumeClaims, Name: "data-postgres-0", Namespace: pod.Namespace, UID: "pvc", Created: now,
				Storage: kube.Storage{Phase: "Pending", Class: "fast-ssd", Requested: "10Gi", AccessModes: []string{"ReadWriteOnce"}}}
			class := kube.Resource{Kind: kube.StorageClasses, Name: "standard", UID: "class", Created: now,
				Storage: kube.Storage{Default: true, Provisioner: "rancher.io/local-path"}}
			claims := kube.ClaimDetails{Classes: []kube.Resource{class}, Consumers: map[kube.ClaimKey][]kube.ClaimConsumer{
				{Namespace: pod.Namespace, Name: claim.Name}: {{Namespace: pod.Namespace, UID: pod.UID, Name: pod.Name, Phase: pod.Phase}},
			}}
			show(kube.Cluster{Resources: &kube.ResourceList{Kind: kube.PersistentVolumeClaims, Items: []kube.Resource{claim}, Claims: &claims}})
			if a.cluster.other.uid != claim.UID {
				t.Fatalf("selected claim = %q", a.cluster.other.uid)
			}
			hasTexts(t, tt, diagnosis, pod.Name, "standard", "ReadWriteOnce")
			// The header's sort button spans its column.
			if header, ok := tt.Find(locales.KubeSortBy(locales.KubeClusterStatus())); !ok || header.W < 68 {
				t.Fatalf("status header at %d px: %+v", size[0], header)
			}
			screenshot(t, tt, fmt.Sprintf("storage-%d", size[0]))
		})
	}
}
