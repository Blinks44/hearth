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

func TestKubeMetricsLayout(t *testing.T) {
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			if size[0] == 1000 {
				i18n.SetLocaleFunc(func() string { return "ru" })
				t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			}
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, size[0], size[1])
			tt.Advance(time.Second)
			if running := watches.running(); len(running) != 1 || !running[0].Metrics {
				t.Fatalf("expected metrics in pod watch: %+v", running)
			}

			cl := shopCluster()
			p := &cl.Pods[1]
			p.Usage = &kube.Resources{CPU: .46, Memory: 97 << 20}
			p.Requests = kube.Resources{CPU: .1, Memory: 25 << 20}
			p.Limits = kube.Resources{CPU: .5, Memory: 100 << 20}
			p.Containers[0].Usage = p.Usage
			p.Containers[0].Requests, p.Containers[0].Limits = p.Requests, p.Limits
			watches.got[len(watches.got)-1](cl, nil)
			expandGroups(a, cl)
			tt.Frame()
			tt.Advance(time.Second)
			cpu := metricsCPU(.46) + " · 92%"
			memory := metricsMemory(97<<20) + " · 97%"
			hasTexts(t, tt, cpu, memory)
			cpuHead, cpuOK := tt.Find("CPU")
			memoryHead, memoryOK := tt.Find("ПАМЯТЬ")
			if size[0] == 1280 {
				memoryHead, memoryOK = tt.Find("MEMORY")
			}
			cpuCell, _ := tt.Find(cpu)
			memoryCell, _ := tt.Find(memory)
			if !cpuOK || !memoryOK || cpuCell.X != cpuHead.X || memoryCell.X != memoryHead.X || cpuCell.X+cpuCell.W > memoryCell.X {
				t.Fatalf("misaligned metrics: CPU %+v / %+v, memory %+v / %+v", cpuHead, cpuCell, memoryHead, memoryCell)
			}
			screenshot(t, tt, fmt.Sprintf("pod-metrics-%d", size[0]))

			if err := tt.Click(p.Name); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			hasTexts(t, tt, metricsCPU(.46), locales.KubeMetricsOfLimit("92%"), metricsMemory(97<<20), locales.KubeMetricsOfLimit("97%"),
				locales.KubeMetricsRequestIs(metricsCPU(.1))+" · "+locales.KubeMetricsLimitIs(metricsCPU(.5)))
			screenshot(t, tt, fmt.Sprintf("pod-metrics-detail-%d", size[0]))

			a.cluster.kind = kindOverview
			tt.Frame()
			tt.Advance(time.Second)
			cl.Overview = &kube.Overview{Nodes: []kube.Node{{Name: "kind-worker", Ready: true, Allocatable: kube.Resources{CPU: 8, Memory: 8 << 30}}}}
			cl.MetricsErr = kube.ErrMetricsUnavailable
			watches.got[len(watches.got)-1](cl, nil)
			tt.Frame()
			tt.Advance(time.Second)
			hasTexts(t, tt, locales.KubeMetricsUnavailable(), "kind-worker")
			screenshot(t, tt, fmt.Sprintf("overview-no-metrics-%d", size[0]))
		})
	}
}

func TestKubeMetricsNoticeDismissAndReset(t *testing.T) {
	a := kubeApp()
	var watches clusterWatches
	a.WatchCluster = watches.watch
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)

	cl := shopCluster()
	cl.MetricsErr = kube.ErrMetricsUnavailable
	watches.got[len(watches.got)-1](cl, nil)
	tt.Frame()
	if !tt.HasText(locales.KubeMetricsUnavailable()) {
		t.Fatalf("expected metrics notice: %q", tt.Texts())
	}

	// Dismiss hides the banner and stays hidden across subsequent reads with the same error.
	if err := tt.Click(locales.OpClose()); err != nil {
		t.Fatalf("dismiss metrics notice: %v", err)
	}
	tt.Frame()
	if tt.HasText(locales.KubeMetricsUnavailable()) {
		t.Fatalf("metrics notice stayed visible after dismiss: %q", tt.Texts())
	}
	watches.got[len(watches.got)-1](cl, nil)
	tt.Frame()
	if tt.HasText(locales.KubeMetricsUnavailable()) {
		t.Fatalf("metrics notice reappeared on same error: %q", tt.Texts())
	}

	// Recovery followed by a later failure shows the banner again.
	cl.MetricsErr = nil
	watches.got[len(watches.got)-1](cl, nil)
	tt.Frame()
	cl.MetricsErr = kube.ErrMetricsUnavailable
	watches.got[len(watches.got)-1](cl, nil)
	tt.Frame()
	if !tt.HasText(locales.KubeMetricsUnavailable()) {
		t.Fatalf("metrics notice did not reappear after recovery: %q", tt.Texts())
	}
}
