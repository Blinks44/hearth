package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func linkTo(r kube.Relationship) resourceLink {
	link := resourceLink{namespace: r.Namespace, name: r.Name, uid: r.UID, direction: "→"}
	if r.Inbound {
		link.direction = "←"
	}
	switch r.Kind {
	case "Service":
		link.kind = kindServices
	case "Pod":
		link.kind = kindPods
	case "PersistentVolumeClaim":
		link.kind = kindPVCs
	case "ConfigMap":
		link.kind = kindConfigMaps
	case "Secret":
		link.kind = kindSecrets
	case "ServiceAccount":
		link.kind = kindServiceAccounts
	case "HorizontalPodAutoscaler":
		link.kind = kindPolicies
	case "PodDisruptionBudget":
		link.kind, link.policyTab = kindPolicies, 1
	}
	return link
}

func (a *App) resourceRelationships(c *ui.Context, links []kube.Relationship, errs []error) {
	if len(links) == 0 && len(errs) == 0 {
		return
	}
	detailSection(c, locales.KubeRelationships())
	for _, err := range errs {
		storageReadNote(c, a.accessWords(err))
	}
	if len(links) == 0 {
		return
	}
	ui.Column(c).Margin(0, 16, 8, 16).Radius(8).Clip().BorderWidth(1).
		BorderColor(Colors(c.Theme()).Divider).Children(func() {
		for _, rel := range links {
			kind := rel.Kind
			switch rel.Kind {
			case "HorizontalPodAutoscaler":
				kind = "HPA"
			case "PodDisruptionBudget":
				kind = "PDB"
			case "PersistentVolumeClaim":
				kind = "PVC"
			}
			button := locales.KubeClusterOpen()
			if rel.Kind == "PersistentVolumeClaim" {
				button = locales.KubeStorageOpenClaim()
			}
			a.resourceLinkRow(c.Key("related-"+rel.Kind+"/"+rel.Namespace+"/"+rel.Name),
				kind+" "+rel.Name, rel.Detail, button, linkTo(rel))
		}
	})
}

func relationshipsFor(cl *kube.Cluster, namespace, uid string) *kube.Relationships {
	if d := cl.Relationships; d != nil && d.Object == (kube.Ref{Namespace: namespace, UID: uid}) {
		return d
	}
	return &kube.Relationships{}
}
