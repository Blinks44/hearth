package ui

import (
	"slices"

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
	case "Deployment":
		link.kind = kindDeployments
	case "StatefulSet":
		link.kind = kindStatefulSets
	case "DaemonSet":
		link.kind = kindDaemonSets
	case "ReplicaSet":
		link.kind = kindReplicaSets
	}
	return link
}

// ownerChip names the workload a pod belongs to, as a button that opens
// it: its kind, faint, then its name.
func (a *App) ownerChip(c *ui.Context, namespace string, owner kube.Owner) {
	t := c.Theme()
	pal := Colors(t)
	open := func() {
		if d := slices.IndexFunc(a.cluster.data.Deployments, func(d kube.Deployment) bool {
			return owner.Kind == "Deployment" && d.Namespace == namespace && d.Name == owner.Name
		}); d >= 0 {
			s := &a.cluster
			s.kind, s.deployment, s.tab, s.query, s.deploymentFilter = kindDeployments, s.data.Deployments[d].UID, 0, "", deploymentsAll
			s.revealWorkload = s.deployment
			return
		}
		if owner.Kind == "Job" {
			// Jobs have no details: their list shows the one.
			a.openResourceLink(resourceLink{kind: kindJobs})
			a.cluster.query = owner.Name
			return
		}
		a.openResourceLink(linkTo(kube.Relationship{Kind: owner.Kind, Namespace: namespace, Name: owner.Name}))
	}
	known := owner.Kind == "Job" || linkTo(kube.Relationship{Kind: owner.Kind}).kind != kindPods
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.KubeOwnerLabel()).FontSize(12).TextColor(t.TextMuted).Shrink(0)
		b := ui.ButtonBase(c.Key("pod-owner")).Height(25).Padding(0, 10, 0, 9).Radius(13).Gap(6).AlignItems(ui.Center).MinWidth(0).
			BorderWidth(1).BorderColor(pal.ControlBorder).Label(locales.KubeOpenOwnerKind(owner.Kind, owner.Name)).Disabled(!known)
		if known {
			b.Tooltip(locales.KubeOpenOwnerKind(owner.Kind, owner.Name)).OnClick(open)
			if b.Hovered() {
				b.Background(pal.Muted)
			}
		}
		b.Children(func() {
			ui.Icon(c, Icon(ownerIcon(owner.Kind))).Size(13, 13).Shrink(0).TextColor(t.TextMuted)
			ui.Text(c, owner.Kind).FontSize(11.5).TextColor(t.TextMuted).Shrink(0)
			clipped(c, owner.Name).FontSize(12).FontWeight(500).MinWidth(0)
			if known {
				ui.Icon(c, Icon("chevron-right")).Size(12, 12).Shrink(0).TextColor(pal.TextFaint)
			}
		})
	})
}

// ownerIcon is the icon of a workload's kind, as its chip in the cluster's
// header shows it.
func ownerIcon(kind string) string {
	switch kind {
	case "Job":
		return clusterKindOptions()[kindJobs].icon
	case "CronJob":
		return clusterKindOptions()[kindCronJobs].icon
	}
	return clusterKindOptions()[kindDeployments].icon
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
