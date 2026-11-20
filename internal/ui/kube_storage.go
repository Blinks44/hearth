package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

type resourceLink struct {
	kind, policyTab      int
	namespace, name, uid string
	direction            string
	missing              bool
}

func (a *App) openResourceLink(link resourceLink) {
	s := &a.cluster
	s.kind, s.query, s.podFilter, s.claimFilter = link.kind, "", 0, 0
	if link.kind == kindPolicies {
		s.policyTab = link.policyTab
	}
	if link.kind == kindDeployments {
		s.deployment, s.deploymentFilter, s.tab = "", 0, 0
	}
	if link.kind == kindServices {
		s.service, s.serviceFilter = "", 0
	}
	if link.kind == kindSecrets {
		s.secret.reset()
		s.secret.filter = 0
	}
	s.resourceLink = link
	s.other.reset()
	if link.kind == kindPods {
		s.pod, s.tab = "", 0
	}
	// A link within the same list (or a retry) still needs a read.
	s.refresh++
}

func (s *kubeClusterState) followResourceLink(cl kube.Cluster) {
	link := s.resourceLink
	if link.name == "" || link.kind != s.kind || link.missing {
		return
	}
	matches := func(namespace, name, uid string) bool {
		return namespace == link.namespace && name == link.name && (link.uid == "" || uid == link.uid)
	}
	if link.kind == kindPods {
		for _, pod := range cl.Pods {
			if matches(pod.Namespace, pod.Name, pod.UID) {
				s.pod, s.tab, s.revealWorkload = pod.UID, 0, pod.UID
				if pod.Owner.Kind != "" {
					group := podGroup{namespace: pod.Namespace, owner: pod.Owner}
					if s.toggled == nil {
						s.toggled = map[string]bool{}
					}
					s.toggled[group.key()] = !slices.ContainsFunc(cl.Pods, func(p kube.Pod) bool {
						return p.Namespace == pod.Namespace && p.Owner == pod.Owner && podTrouble(p)
					})
				}
				s.resourceLink = resourceLink{}
				return
			}
		}
	}
	if link.kind == kindDeployments {
		for _, deployment := range cl.Deployments {
			if matches(deployment.Namespace, deployment.Name, deployment.UID) {
				s.deployment, s.tab, s.revealWorkload = deployment.UID, 0, deployment.UID
				s.resourceLink = resourceLink{}
				return
			}
		}
	}
	if link.kind == kindServices {
		for _, service := range cl.Services {
			if matches(service.Namespace, service.Name, service.UID) {
				s.service, s.revealWorkload = service.UID, service.UID
				s.resourceLink = resourceLink{}
				return
			}
		}
	}
	if link.kind == kindSecrets {
		for _, secret := range cl.Secrets {
			if matches(secret.Namespace, secret.Name, secret.UID) {
				s.secret.uid, s.secret.scope, s.revealWorkload = secret.UID, s.target, secret.UID
				s.resourceLink = resourceLink{}
				return
			}
		}
	}
	if list := s.activeResourceList(&cl); list != nil && (link.kind == kindPolicies || list.Kind == otherResourceKind(link.kind)) {
		if list.Err != nil {
			return
		}
		for _, item := range list.Items {
			if matches(item.Namespace, item.Name, item.UID) {
				s.other.uid, s.other.tab, s.revealWorkload = item.UID, 0, item.UID
				s.resourceLink = resourceLink{}
				return
			}
		}
	}
	s.resourceLink.missing = true
}

func claimKey(item kube.Resource) kube.ClaimKey {
	return kube.ClaimKey{Namespace: item.Namespace, Name: item.Name}
}

func claimProblem(item kube.Resource) bool {
	return item.Deleting || item.Storage.Phase == "Pending" || item.Storage.Phase == "Lost"
}

func claimShows(filter int, item kube.Resource, claims *kube.ClaimDetails) bool {
	if item.Kind != kube.PersistentVolumeClaims {
		return true
	}
	switch filter {
	case 1:
		return claimProblem(item)
	case 2:
		return claims != nil && claims.ConsumersErr == nil && len(claims.Consumers[claimKey(item)]) == 0
	}
	return true
}

func claimFilters(c *ui.Context, s *kubeClusterState, list *kube.ResourceList) {
	labels := []string{locales.TabAll(), locales.KubePodsTrouble()}
	if list.Claims != nil && list.Claims.ConsumersErr == nil {
		labels = append(labels, locales.KubeStorageUnused())
	}
	tabs := make([]segment, 0, len(labels))
	for i, label := range labels {
		count := 0
		for _, item := range list.Items {
			if claimShows(i, item, list.Claims) {
				count++
			}
		}
		tabs = append(tabs, segment{label, strconv.Itoa(count)})
	}
	segments(c, "claim-filter", &s.claimFilter, tabs)
}

func storageStatus(c *ui.Context, item kube.Resource, claims *kube.ClaimDetails) (string, ui.Color, ui.Color) {
	pal := Colors(c.Theme())
	s := item.Storage
	if item.Kind == kube.StorageClasses {
		if s.Default {
			return locales.KubeStorageDefault(), pal.SuccessSoft, pal.SuccessText
		}
		return s.Provisioner, pal.Muted, c.Theme().TextMuted
	}
	if claims.MissingClass(item) {
		return locales.KubeStoragePendingClass(), pal.DangerSoft, pal.DangerText
	}
	label := s.Phase
	if size := cmp.Or(s.Capacity, s.Requested); size != "" {
		label += " · " + size
	}
	switch s.Phase {
	case "Bound", "Available":
		return label, pal.SuccessSoft, pal.SuccessText
	case "Lost", "Failed":
		return label, pal.DangerSoft, pal.DangerText
	case "Pending", "Released":
		return label, pal.WarningSoft, pal.WarningText
	}
	return label, pal.Muted, c.Theme().TextMuted
}

func (a *App) storageOverview(c *ui.Context, item kube.Resource, claims *kube.ClaimDetails, events []kube.Event) {
	s := item.Storage
	if item.Kind == kube.PersistentVolumeClaims && s.Phase == "Pending" {
		switch {
		case claims.MissingClass(item):
			ui.Column(c).Padding(8, 16).Children(func() { warningRow(c, locales.KubeStorageClassMissing(s.Class)) })
		case kube.NoVolumeEvent(events) != "":
			ui.Column(c).Padding(8, 16).Children(func() { warningRow(c, locales.KubeStorageNoVolume()) })
		}
	}
	detailSection(c, locales.KubeFacts())
	facts := [][2]string{
		{locales.KubeClusterStatus(), s.Phase},
		{locales.KubeStorageRequested(), s.Requested},
		{locales.KubeStorageCapacity(), s.Capacity},
		{"StorageClass", s.Class},
		{locales.KubeStorageVolume(), s.Volume},
		{locales.KubeStorageAccessModes(), strings.Join(s.AccessModes, ", ")},
		{locales.KubeStorageVolumeMode(), s.VolumeMode},
		{locales.KubeStorageReclaimPolicy(), s.ReclaimPolicy},
		{locales.KubeStorageProvisioner(), s.Provisioner},
		{locales.KubeStorageBindingMode(), s.BindingMode},
	}
	if s.Expandable {
		facts = append(facts, [2]string{locales.KubeStorageExpansion(), locales.KubeStorageAllowed()})
	}
	factRows(c, facts)
	if item.Kind == kube.PersistentVolumes && s.ClaimName != "" {
		// The claim can be in a namespace outside the user's current filter.
		factRows(c, [][2]string{{locales.KubeStorageClaims(), s.ClaimNamespace + "/" + s.ClaimName}})
	}
	if item.Kind != kube.PersistentVolumeClaims || claims == nil {
		return
	}
	detailSection(c, locales.KubeStorageConsumers())
	consumers := claims.Consumers[claimKey(item)]
	for _, pod := range consumers {
		a.resourceLinkRow(c.Key("claim-pod-"+pod.UID), pod.Name, pod.Phase, locales.KubeClusterOpen(),
			resourceLink{kind: kindPods, uid: pod.UID, namespace: pod.Namespace, name: pod.Name})
	}
	switch {
	case claims.ConsumersErr != nil:
		storageReadNote(c, claims.ConsumersErr.Error())
	case len(consumers) == 0:
		storageReadNote(c, locales.KubeStorageNoConsumers())
	}
	if s.Volume != "" {
		detailSection(c, locales.KubeStorageVolume())
		a.resourceLinkRow(c.Key("claim-volume"), s.Volume, "", locales.KubeClusterOpen(),
			resourceLink{kind: kindPVs, name: s.Volume})
	}
	detailSection(c, locales.KubeStorageClasses())
	if claims.ClassesErr != nil {
		storageReadNote(c, claims.ClassesErr.Error())
	}
	for _, class := range claims.Classes {
		description := class.Storage.Provisioner
		if class.Storage.Default {
			description += " · " + locales.KubeStorageDefault()
		}
		a.resourceLinkRow(c.Key("claim-class-"+class.UID), class.Name, description, locales.KubeClusterOpen(),
			resourceLink{kind: kindStorageClasses, name: class.Name, uid: class.UID})
	}
	if claims.ClassesErr == nil && len(claims.Classes) == 0 {
		storageReadNote(c, locales.KubeResourcesEmptyCluster())
	}
}

func storageReadNote(c *ui.Context, text string) {
	ui.Text(c, text).Padding(4, 16).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
}

func (a *App) resourceLinkRow(c *ui.Context, name, description, button string, link resourceLink) {
	pal := Colors(c.Theme())
	padding := float32(16)
	if link.direction != "" {
		padding = 12
	}
	ui.Row(c).Padding(5, padding).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		if link.direction != "" {
			ui.Text(c, link.direction).Width(16).Shrink(0).FontSize(12).TextColor(pal.TextFaint)
			ui.Row(c).Gap(10).Grow(1).MinWidth(0).AlignItems(ui.Center).Children(func() {
				clipped(c, name).Font(FontMono).FontSize(12).Shrink(1).MinWidth(0)
				if description != "" {
					clipped(c, description).FontSize(11.5).TextColor(pal.TextFaint).Shrink(1).MinWidth(0)
				}
			})
		} else {
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
				clipped(c, name).Font(FontMono).FontSize(12)
				if description != "" {
					clipped(c, description).FontSize(11.5).TextColor(pal.TextFaint)
				}
			})
		}
		kubeActionButton(c, "arrow-right", button, podActionHeight).Tooltip(name).OnClick(func() { a.openResourceLink(link) })
	})
}
