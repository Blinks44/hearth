package ui

import (
	"cmp"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

type storageLink struct {
	kind                 int
	namespace, name, uid string
}

func (a *App) openStorageLink(link storageLink) {
	s := &a.cluster
	s.kind, s.query, s.podFilter, s.claimFilter = link.kind, "", 0, 0
	s.storageLink = link
	s.other.reset()
	if link.kind == kindPods {
		s.pod, s.tab = "", 0
	}
	// A link within the same list (or a retry) still needs a read.
	s.refresh++
}

func (s *kubeClusterState) followStorageLink(cl kube.Cluster) {
	link := s.storageLink
	if link.name == "" || link.kind != s.kind {
		return
	}
	if link.kind == kindPods {
		for _, pod := range cl.Pods {
			if pod.UID == link.uid && pod.Namespace == link.namespace {
				s.pod, s.tab, s.revealWorkload = pod.UID, 0, pod.UID
				s.storageLink = storageLink{}
				return
			}
		}
	}
	if cl.Resources != nil && cl.Resources.Kind == otherResourceKind(link.kind) {
		for _, item := range cl.Resources.Items {
			if item.Namespace == link.namespace && item.Name == link.name && (link.uid == "" || item.UID == link.uid) {
				s.other.uid, s.other.tab, s.revealWorkload = item.UID, 0, item.UID
				s.storageLink = storageLink{}
				return
			}
		}
	}
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
		a.storageLinkRow(c.Key("claim-pod-"+pod.UID), pod.Name, pod.Phase, locales.KubeClusterOpen(),
			storageLink{kind: kindPods, uid: pod.UID, namespace: pod.Namespace, name: pod.Name})
	}
	switch {
	case claims.ConsumersErr != nil:
		storageReadNote(c, claims.ConsumersErr.Error())
	case len(consumers) == 0:
		storageReadNote(c, locales.KubeStorageNoConsumers())
	}
	if s.Volume != "" {
		detailSection(c, locales.KubeStorageVolume())
		a.storageLinkRow(c.Key("claim-volume"), s.Volume, "", locales.KubeClusterOpen(),
			storageLink{kind: kindPVs, name: s.Volume})
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
		a.storageLinkRow(c.Key("claim-class-"+class.UID), class.Name, description, locales.KubeClusterOpen(),
			storageLink{kind: kindStorageClasses, name: class.Name, uid: class.UID})
	}
	if claims.ClassesErr == nil && len(claims.Classes) == 0 {
		storageReadNote(c, locales.KubeResourcesEmptyCluster())
	}
}

func storageReadNote(c *ui.Context, text string) {
	ui.Text(c, text).Padding(4, 16).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
}

func (a *App) storageLinkRow(c *ui.Context, name, description, button string, link storageLink) {
	pal := Colors(c.Theme())
	ui.Row(c).Padding(5, 16).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			clipped(c, name).Font(FontMono).FontSize(12)
			if description != "" {
				clipped(c, description).FontSize(11.5).TextColor(pal.TextFaint)
			}
		})
		kubeActionButton(c, "arrow-right", button, podActionHeight).Tooltip(name).OnClick(func() { a.openStorageLink(link) })
	})
}
