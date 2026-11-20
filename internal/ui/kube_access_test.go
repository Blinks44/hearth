package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
)

func TestKubeAccessReviewNavigation(t *testing.T) {
	for _, width := range []int{1000, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			a.openCluster("kind-dev")
			a.selectClusterKind(kindAccess, nil)
			a.setNamespaces("kind-dev", []string{"shop", "tools"})
			tt := ui.NewTester(a.View, width, 680)
			tt.Advance(time.Second)
			running := watches.running()
			if len(running) != 1 || running[0].AccessNamespace != "shop" {
				t.Fatalf("access target = %+v", running)
			}

			rule := kube.AccessRule{Resource: "configmaps", Names: "app-settings", Verbs: "get"}
			role := kube.Resource{Kind: kube.Roles, Namespace: "shop", Name: strings.Repeat("config-edit-", 12), UID: "r",
				RBAC: &kube.RBACDetails{Rules: []kube.RBACRule{{Resources: []string{"configmaps"}, Verbs: []string{"get"}}}}}
			binding := kube.Resource{Kind: kube.RoleBindings, Namespace: "shop", Name: "devs", UID: "b", RBAC: &kube.RBACDetails{
				RoleRef: kube.RBACRoleRef{Kind: "Role", Name: role.Name}, Subjects: []kube.RBACSubject{{Kind: "Group", Name: "developers"}},
			}}
			review := &kube.AccessReview{Namespace: "shop", Rules: []kube.AccessRule{rule, {Resource: "pods/log", Verbs: "get"}},
				Identity: kube.AccessIdentity{Username: "oidc:dev", Groups: []string{"developers"}},
				Resources: []kube.ResourceList{{Kind: kube.Roles, Items: []kube.Resource{role}},
					{Kind: kube.RoleBindings, Items: []kube.Resource{binding}}},
			}
			got := watches.got[len(watches.got)-1]
			snapshot := kube.Cluster{Access: review, Taken: time.Now()}
			got(snapshot, nil)
			tt.Frame()
			tt.Advance(time.Second)
			hasTexts(t, tt, "oidc:dev", "app-settings", "RoleBinding devs")
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Advance(time.Second)
			}
			click("pods/log")
			hasTexts(t, tt, "pods/log", "kubectl auth can-i --list")
			click("Open")
			if v := a.cluster.access; v.tab != accessBindingsTab || v.selected != accessObjectKey(binding) {
				t.Fatalf("binding selection = %+v", v)
			}
			click("Open")
			if v := a.cluster.access; v.tab != accessRolesTab || v.selected != accessObjectKey(role) {
				t.Fatalf("role selection = %+v", v)
			}
			hasTexts(t, tt, role.Name, "RULE 1")
			click("What can I do?")
			hasTexts(t, tt, "oidc:dev", "app-settings")
			a.setNamespaces("kind-dev", []string{"tools"})
			tt.Frame()
			tt.Advance(time.Second)
			got(snapshot, nil)
			tt.Frame()
			tt.Advance(time.Second)
			if a.cluster.data != nil || watches.running()[0].AccessNamespace != "tools" {
				t.Fatalf("stale access data after namespace change: %+v", a.cluster.data)
			}
		})
	}
}
