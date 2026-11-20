package ui

import (
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/kube"
)

func TestClusterWorkloadCountsOnlyForReadWorkloads(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name  string
		data  kube.Cluster
		known bool
	}{
		{"empty workload list", kube.Cluster{Taken: now}, true},
		{"overview with batch", kube.Cluster{Taken: now, Overview: &kube.Overview{}, Batch: &kube.Batch{}}, true},
		{"services", kube.Cluster{Taken: now, Services: []kube.Service{}}, false},
		{"batch without workloads", kube.Cluster{Taken: now, Batch: &kube.Batch{}}, false},
		{"helm", kube.Cluster{Taken: now, HelmReleases: []kube.HelmRelease{}}, false},
		{"secrets", kube.Cluster{Taken: now, Secrets: []kube.Secret{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, kind := range []int{kindPods, kindDeployments} {
				if _, _, known := clusterKindCounts(&tc.data, kind); known != tc.known {
					t.Errorf("kind %d: count known = %t, want %t", kind, known, tc.known)
				}
			}
		})
	}
}
