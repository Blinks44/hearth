package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func TestKubePolicyBrowser(t *testing.T) {
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			if size[0] == 1000 {
				i18n.SetLocaleFunc(func() string { return "ru" })
				t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			}
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.ReadResourceYAML = func(_ kube.Target, item kube.Resource, done func(string, error)) func() {
				done("kind: "+policyKindName(item.Kind), nil)
				return func() {}
			}
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
			show := func(cl kube.Cluster) {
				t.Helper()
				cl.Taken = time.Now()
				watches.got[len(watches.got)-1](cl, nil)
				tt.Frame()
				tt.Advance(time.Second)
			}
			click(locales.KubeKindsAll())
			tt.Type("hpa")
			tt.Key(0, ui.KeyEnter)
			tt.Advance(time.Second)
			if running := watches.running(); len(running) != 1 || !running[0].Policies {
				t.Fatalf("selected watch = %+v", running)
			}
			hpa := kube.Resource{Kind: kube.Autoscalers, Namespace: "default", Name: "scaler-checkout", UID: "hpa",
				Autoscaler: &kube.Autoscaler{
					Target: kube.ScaleTarget{APIVersion: "apps/v1", Kind: "Deployment", Name: "checkout"},
					Min:    1, Max: 3, Current: new(3), Desired: new(3),
					Conditions: []kube.CustomCondition{{Type: "ScalingLimited", Status: "True", Reason: "TooManyReplicas", Message: "The desired replica count is more than the maximum replica count"}},
					Metrics:    []kube.AutoscalerMetric{{Name: "cpu", Source: "Resource", Current: "85%", Target: "70%"}},
				}}
			quota := kube.Resource{Kind: kube.ResourceQuotas, Namespace: "default", UID: "quota", Name: "compute",
				Quota: &kube.ResourceQuota{Hard: map[string]string{"cpu": "8"}, Used: map[string]string{"cpu": "750m"}}}
			cl := kube.Cluster{Policies: []kube.ResourceList{
				{Kind: kube.Autoscalers, Items: []kube.Resource{hpa}},
				{Kind: kube.DisruptionBudgets, Err: &kube.Error{Access: detect.KubeAccess{State: detect.KubeForbidden}}},
				{Kind: kube.ResourceQuotas, Items: []kube.Resource{quota}},
				{Kind: kube.LimitRanges},
			}}
			show(cl)
			click(hpa.Name)
			header, headerOK := tt.Find(strings.ToUpper(locales.KubeReplicas()))
			cell, cellOK := tt.Find(locales.KubeAutoscalerAtMax())
			if !headerOK || !cellOK || cell.X < header.X || cell.X > header.X+12 {
				t.Fatalf("replicas column is misaligned: header %+v, cell %+v", header, cell)
			}
			hasTexts(t, tt, locales.KubeAutoscalerAtMax(), "85% / 70%", "Deployment checkout")
			screenshot(t, tt, fmt.Sprintf("policies-%d", size[0]))
			if running := watches.running(); len(running) != 1 || running[0].Events.UID != hpa.UID {
				t.Fatalf("event watch = %+v", running)
			}
			click(locales.KubeAutoscalerMetrics())
			hasTexts(t, tt, strings.ToUpper(locales.KubeAutoscalerCurrentOfTarget()), "cpu · Resource", "85% / 70%")
			click(locales.KubeResourceYaml())
			hasTexts(t, tt, "kind: HorizontalPodAutoscaler")
			click("PDB")
			hasTexts(t, tt, locales.KubeClusterForbiddenTitle())
			if tt.HasText("kind: HorizontalPodAutoscaler") {
				t.Fatal("HPA YAML remained visible after switching to PDB")
			}
			click(locales.KubePolicyQuotas())
			click(quota.Name)
			hasTexts(t, tt, "750m / 8")
		})
	}
}

func TestKubeAutoscalerStatus(t *testing.T) {
	for _, tc := range []struct {
		name       string
		conditions []kube.CustomCondition
		metrics    []kube.AutoscalerMetric
		want       string
	}{
		{name: "limit reached", conditions: []kube.CustomCondition{{Type: "ScalingLimited", Status: "True", Reason: "TooManyReplicas"}}, want: locales.KubeAutoscalerAtMax()},
		{name: "at max with no excess demand", conditions: []kube.CustomCondition{{Type: "ScalingLimited", Status: "False"}}},
		{name: "metrics unavailable", conditions: []kube.CustomCondition{{Type: "ScalingActive", Status: "False", Reason: "FailedGetResourceMetric"}}, metrics: []kube.AutoscalerMetric{{Target: "70%"}}, want: locales.KubeAutoscalerNoMetrics()},
		{name: "target disabled", conditions: []kube.CustomCondition{{Type: "ScalingActive", Status: "False", Reason: "ScalingDisabled"}}, metrics: []kube.AutoscalerMetric{{Target: "70%"}}, want: locales.KubeAutoscalerInactive()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &kube.Autoscaler{Min: 1, Max: 3, Current: new(3), Conditions: tc.conditions, Metrics: tc.metrics}
			if label, _ := autoscalerProblem(h); label != tc.want {
				t.Errorf("status = %q, want %q", label, tc.want)
			}
		})
	}
}
