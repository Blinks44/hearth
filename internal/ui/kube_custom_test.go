package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeCustomResourceBrowser(t *testing.T) {
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			if size[0] == 1000 {
				i18n.SetLocaleFunc(func() string { return "ru" })
				t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			}
			def := kube.CustomDefinition{Group: "hearth.example", Scope: "Namespaced", Versions: []kube.CustomVersion{{
				Name: "v1", Served: true, Storage: true, Columns: []kube.CustomColumn{
					{Name: "Ready", Type: "string"}, {Name: "Issuer", Type: "string"}, {Name: "Revision", Type: "integer", Priority: 1},
				},
			}}}
			def.Names.Kind, def.Names.Plural, def.Names.ShortNames = "Certificate", "certificates", []string{"hcert"}
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.ListCustomDefinitions = func(target kube.Target, done func(*kube.CustomDefinitions)) func() {
				if target.Context != "kind-dev" {
					t.Fatalf("catalog target = %+v", target)
				}
				done(&kube.CustomDefinitions{Items: []kube.CustomDefinition{def}})
				return func() {}
			}
			a.ReadResourceYAML = func(target kube.Target, item kube.Resource, done func(string, error)) func() {
				if target.Context != "kind-dev" || item.UID != "cert1" || item.Namespace != "default" {
					t.Fatalf("YAML target = %+v, %+v", target, item)
				}
				done("kind: Certificate", nil)
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
			show := func(cl kube.Cluster, err error) {
				t.Helper()
				watches.got[len(watches.got)-1](cl, err)
				tt.Frame()
				tt.Advance(time.Second)
			}
			click(locales.KubeKindsAll())
			tt.Type("hcert")
			tt.Advance(time.Second)
			hasTexts(t, tt, "HEARTH.EXAMPLE", "certificates")
			screenshot(t, tt, fmt.Sprintf("custom-picker-%d", size[0]))
			tt.Key(0, ui.KeyEnter)
			tt.Advance(time.Second)
			if running := watches.running(); len(running) != 1 || running[0].CustomKind != def.Name() {
				t.Fatalf("selected watch = %+v", running)
			}
			const longIssuer = "acme-production-issuer-with-a-name-that-does-not-fit-in-the-table"
			cert := kube.Resource{Kind: "certificates.v1.hearth.example", Namespace: "default", Name: "tls-shop", UID: "cert1",
				Custom: &kube.CustomObject{Definition: &def, Version: "v1", Cells: []string{"False", longIssuer, "42"},
					Conditions: []kube.CustomCondition{{Type: "Ready", Status: "False", Reason: "Failed", Message: "ACME challenge failed."}},
				},
			}
			cl := kube.Cluster{Taken: time.Now(), Resources: &kube.ResourceList{Kind: cert.Kind, Custom: &def, Items: []kube.Resource{cert}}}
			show(cl, nil)
			readyHeader, headerOK := tt.Find("READY")
			readyCell, cellOK := tt.Find("False")
			if !headerOK || !cellOK || readyHeader.X != readyCell.X {
				t.Fatalf("misaligned column: %+v / %+v", readyHeader, readyCell)
			}
			click(cert.Name)
			hasTexts(t, tt, "Failed · ACME challenge failed.", longIssuer)
			screenshot(t, tt, fmt.Sprintf("custom-details-%d", size[0]))
			if running := watches.running(); len(running) != 1 || running[0].Events.UID != cert.UID {
				t.Fatalf("event watch = %+v", running)
			}
			click(locales.KubeResourceYaml())
			hasTexts(t, tt, "kind: Certificate")
			show(kube.Cluster{}, &kube.Error{Access: detect.KubeAccess{State: detect.KubeForbidden}})
			hasTexts(t, tt, locales.KubeCustomForbiddenTitle())
			if tt.HasText("kind: Certificate") || tt.HasText(cert.Name) {
				t.Fatal("custom resource remained visible after permission denial")
			}
		})
	}
}

func TestKubeKindPickerFiltersByAccess(t *testing.T) {
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			if size[0] == 1000 {
				i18n.SetLocaleFunc(func() string { return "ru" })
				t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			}
			allowedCRD := kube.CustomDefinition{Group: "hearth.example", Scope: "Namespaced", Versions: []kube.CustomVersion{{Name: "v1", Served: true, Storage: true}}}
			allowedCRD.Names.Kind, allowedCRD.Names.Plural, allowedCRD.Names.ShortNames = "Certificate", "certificates", []string{"hcert"}
			deniedCRD := kube.CustomDefinition{Group: "aquasecurity.github.io", Scope: "Cluster", Versions: []kube.CustomVersion{{Name: "v1alpha1", Served: true, Storage: true}}}
			deniedCRD.Names.Kind, deniedCRD.Names.Plural = "ClusterComplianceReport", "clustercompliancereports"

			access := kube.NewListAccess([]kube.RBACRule{
				{APIGroups: []string{""}, Resources: []string{"pods", "services"}, Verbs: []string{"list"}},
				{APIGroups: []string{"hearth.example"}, Resources: []string{"certificates"}, Verbs: []string{"list"}},
			})
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.ListCustomDefinitions = func(target kube.Target, done func(*kube.CustomDefinitions)) func() {
				done(&kube.CustomDefinitions{Items: []kube.CustomDefinition{allowedCRD, deniedCRD}, Access: access})
				return func() {}
			}
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, size[0], size[1])
			tt.Advance(time.Second)

			if err := tt.Click(locales.KubeKindsAll()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			hasTexts(t, tt, "Pods", "Services", "certificates", locales.KubeKindShowHidden())
			if tt.HasText("clustercompliancereports") || tt.HasText("Nodes") {
				t.Fatal("inaccessible resources should be hidden by default")
			}

			// Toggle to show all reveals denied kinds and CRDs.
			if err := tt.Click(locales.KubeKindShowHidden()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			hasTexts(t, tt, "Nodes", "clustercompliancereports", locales.KubeKindHideNoAccess())

			// Toggle back hides them again.
			if err := tt.Click(locales.KubeKindHideNoAccess()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if tt.HasText("clustercompliancereports") || tt.HasText("Nodes") {
				t.Fatal("inaccessible resources remained visible after hiding again")
			}
		})
	}
}

func TestKubeKindPickerFiltersEmptyCRDs(t *testing.T) {
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			if size[0] == 1000 {
				i18n.SetLocaleFunc(func() string { return "ru" })
				t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
			}
			nonEmptyCRD := kube.CustomDefinition{Group: "hearth.example", Scope: "Namespaced", Versions: []kube.CustomVersion{{Name: "v1", Served: true, Storage: true}}}
			nonEmptyCRD.Names.Kind, nonEmptyCRD.Names.Plural = "Certificate", "certificates"
			emptyCRD := kube.CustomDefinition{Group: "flink.apache.org", Scope: "Namespaced", Versions: []kube.CustomVersion{{Name: "v1beta1", Served: true, Storage: true}}}
			emptyCRD.Names.Kind, emptyCRD.Names.Plural = "FlinkBlueGreenDeployment", "flinkbluegreendeployments"
			unknownCRD := kube.CustomDefinition{Group: "custom.io", Scope: "Namespaced", Versions: []kube.CustomVersion{{Name: "v1", Served: true, Storage: true}}}
			unknownCRD.Names.Kind, unknownCRD.Names.Plural = "Widget", "widgets"

			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.ListCustomDefinitions = func(target kube.Target, done func(*kube.CustomDefinitions)) func() {
				done(&kube.CustomDefinitions{Items: []kube.CustomDefinition{nonEmptyCRD, emptyCRD, unknownCRD}})
				return func() {}
			}
			probeCalls := 0
			a.ProbeCustomPresence = func(target kube.Target, defs []kube.CustomDefinition, done func(map[string]bool)) func() {
				probeCalls++
				done(map[string]bool{
					nonEmptyCRD.Name(): true,
					emptyCRD.Name():    false,
					// unknownCRD omitted intentionally -> fail open
				})
				return func() {}
			}
			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, size[0], size[1])
			tt.Advance(time.Second)

			// Open picker: Non-empty only is ON by default.
			if err := tt.Click(locales.KubeKindsAll()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if probeCalls != 1 {
				t.Fatalf("probeCalls = %d, want 1", probeCalls)
			}
			hasTexts(t, tt, locales.KubeKindNonEmptyOnly(), locales.KubeKindHiddenEmpty(1), "certificates", "widgets")
			if tt.HasText("flinkbluegreendeployments") {
				t.Fatal("empty CRD should be hidden while Non-empty only is enabled")
			}

			// Toggle OFF reveals empty CRDs.
			if err := tt.Click(locales.KubeKindNonEmptyOnly()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			hasTexts(t, tt, "flinkbluegreendeployments", "certificates", "widgets")

			// Toggle back ON hides empty CRDs again using cached presence without re-probing.
			if err := tt.Click(locales.KubeKindNonEmptyOnly()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if probeCalls != 1 {
				t.Fatalf("probeCalls after toggling back ON = %d, want cached 1", probeCalls)
			}
			if tt.HasText("flinkbluegreendeployments") {
				t.Fatal("empty CRD remained visible after enabling Non-empty only")
			}
			// Typing an explicit query reveals matching empty CRDs even while Non-empty only is ON.
			a.cluster.kindQuery = "flink"
			tt.Frame()
			hasTexts(t, tt, "flinkbluegreendeployments")
		})
	}
}

func TestKubeBlock4Regressions(t *testing.T) {
	// 1. Cluster admin with zero CRDs must not show "hidden (no access)".
	a := kubeApp()
	var watches clusterWatches
	a.WatchCluster = watches.watch
	a.ListCustomDefinitions = func(target kube.Target, done func(*kube.CustomDefinitions)) func() {
		done(&kube.CustomDefinitions{
			Items: nil,
			Access: kube.NewListAccess([]kube.RBACRule{
				{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
			}),
		})
		return func() {}
	}
	a.openCluster("kind-dev")
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	if err := tt.Click(locales.KubeKindsAll()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText(locales.KubeKindHiddenNoAccess(1)) {
		t.Fatal("zero CRDs should not report 1 hidden (no access)")
	}

	// 2. Failing init container gets crash story pointing at the init container.
	initPod := kube.Pod{
		Namespace: "shop", Name: "migrate-1", UID: "pod-init", Phase: "Pending", Reason: "Init:CrashLoopBackOff",
		InitContainers: []kube.Container{{Name: "db-migrate", State: "waiting", Reason: "CrashLoopBackOff", Restarts: 4}},
		Containers:     []kube.Container{{Name: "api", State: "waiting", Reason: "PodInitializing"}},
	}
	story, ok := storyOf(initPod, nil, time.Now())
	if !ok || story.crashed != "db-migrate" {
		t.Fatalf("storyOf init crash = (%+v, %t), want crashed=db-migrate", story, ok)
	}

	// 3. Closing kubeconfig create dialog clears pasted secrets from memory.
	a2 := kubeApp()
	a2.router.Push(kubePage)
	tt2 := ui.NewTester(a2.View, 1280, 820)
	tt2.Advance(time.Second)
	a2.openKubeConfigCreate("/tmp/kubeconfig")
	a2.kubeConfig.mode = 1
	a2.kubeConfig.pasted = "apiVersion: v1\nkind: Config\nusers:\n- name: u\n  user:\n    token: super-secret-token\n"
	tt2.Frame()
	tt2.Advance(time.Second)
	if err := tt2.Click(locales.Cancel()); err != nil {
		t.Fatalf("click Cancel (path=%q open=%t): %v", a2.router.Path(), a2.kubeConfig.open, err)
	}
	tt2.Frame()
	if a2.kubeConfig.pasted != "" || a2.kubeConfig.cachedText != "" {
		t.Fatalf("kubeconfig secrets remained in memory after close: pasted=%q cached=%q", a2.kubeConfig.pasted, a2.kubeConfig.cachedText)
	}
}
