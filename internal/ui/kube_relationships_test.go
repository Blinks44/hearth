package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func TestResourceRelationshipRow(t *testing.T) {
	i18n.SetLocaleFunc(func() string { return "ru" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	const detail = "POSTGRES_PASSWORD"
	name := "postgres-auth-" + strings.Repeat("x", 120)
	a := kubeApp()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(360).Children(func() {
			a.resourceRelationships(c, []kube.Relationship{
				{Kind: "Secret", Namespace: "shop", Name: name, Detail: detail},
			}, nil)
		})
	}, 1000, 680)
	target, hasTarget := tt.Find("Secret " + name)
	action, hasAction := tt.Find(locales.KubeClusterOpen())
	desc, hasDesc := tt.Find(detail)
	if !hasTarget || !hasAction || !hasDesc || target.W <= 0 || action.W < 68 || desc.X+desc.W > action.X ||
		target.Y >= action.Y+action.H || desc.Y >= action.Y+action.H {
		t.Fatalf("relationship at minimum width: name=%+v, detail=%+v, action=%+v", target, desc, action)
	}
	if err := tt.Click(locales.KubeClusterOpen()); err != nil {
		t.Fatal(err)
	}
	if link := a.cluster.resourceLink; link.kind != kindSecrets || link.namespace != "shop" || link.name != name {
		t.Fatalf("wrong link: %+v", link)
	}
}

func TestFollowResourceLink(t *testing.T) {
	for _, tc := range []struct {
		name string
		link resourceLink
		data kube.Cluster
	}{
		{"service namespace", resourceLink{kind: kindServices, namespace: "shop", name: "app"},
			kube.Cluster{Services: []kube.Service{
				{Namespace: "tools", Name: "app", UID: "other"},
				{Namespace: "shop", Name: "app", UID: "expected"},
			}}},
		{"pod UID and namespace", resourceLink{kind: kindPods, namespace: "shop", name: "app", uid: "expected"},
			kube.Cluster{Pods: []kube.Pod{
				{Namespace: "shop", Name: "app", UID: "recreated"},
				{Namespace: "shop", Name: "app", UID: "expected"},
			}}},
		{"secret namespace", resourceLink{kind: kindSecrets, namespace: "shop", name: "app"},
			kube.Cluster{Secrets: []kube.Secret{
				{Namespace: "tools", Name: "app", UID: "other"},
				{Namespace: "shop", Name: "app", UID: "expected"},
			}}},
		{"service account namespace", resourceLink{kind: kindServiceAccounts, namespace: "shop", name: "app"},
			kube.Cluster{Resources: &kube.ResourceList{Kind: kube.ServiceAccounts, Items: []kube.Resource{
				{Namespace: "tools", Name: "app", UID: "other"},
				{Namespace: "shop", Name: "app", UID: "expected"},
			}}}},
		{"pod replacement", resourceLink{kind: kindPods, namespace: "shop", name: "app", uid: "original"},
			kube.Cluster{Pods: []kube.Pod{{Namespace: "shop", Name: "app", UID: "recreated"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := kubeClusterState{kind: tc.link.kind, resourceLink: tc.link}
			s.followResourceLink(tc.data)
			if tc.name == "pod replacement" {
				if s.pod != "" || !s.resourceLink.missing {
					t.Fatalf("replaced target opened: %+v", s.resourceLink)
				}
				return
			}
			if s.revealWorkload != "expected" || s.resourceLink.name != "" {
				t.Fatalf("linked UID = %q, pending link = %+v", s.revealWorkload, s.resourceLink)
			}
		})
	}
}
