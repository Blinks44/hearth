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
