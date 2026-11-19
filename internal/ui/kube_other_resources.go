package ui

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func otherResourceKind(kind int) kube.ResourceKind {
	switch kind {
	case kindStatefulSets:
		return kube.StatefulSets
	case kindDaemonSets:
		return kube.DaemonSets
	case kindReplicaSets:
		return kube.ReplicaSets
	case kindIngresses:
		return kube.Ingresses
	case kindEndpoints:
		return kube.Endpoints
	case kindNetworkPolicies:
		return kube.NetworkPolicies
	case kindConfigMaps:
		return kube.ConfigMaps
	case kindPVCs:
		return kube.PersistentVolumeClaims
	case kindPVs:
		return kube.PersistentVolumes
	case kindStorageClasses:
		return kube.StorageClasses
	}
	return ""
}

type otherResourceView struct {
	uid  string
	tab  int
	yaml *resourceYAML
}

type resourceYAML struct {
	text string
	err  error
	stop func()
	done bool
}

func (v *otherResourceView) clearYAML() {
	if v.yaml != nil && v.yaml.stop != nil {
		v.yaml.stop()
	}
	v.yaml = nil
}

func (v *otherResourceView) reset() {
	v.clearYAML()
	v.uid, v.tab = "", 0
}

func (s *kubeClusterState) otherResource() *kube.Resource {
	if s.other.uid == "" || s.data == nil || s.data.Resources == nil || s.data.Resources.Kind != otherResourceKind(s.kind) {
		return nil
	}
	items := s.data.Resources.Items
	if i := slices.IndexFunc(items, func(r kube.Resource) bool { return r.UID == s.other.uid }); i >= 0 {
		return &items[i]
	}
	return nil
}

func (a *App) syncResourceYAML() {
	v := &a.cluster.other
	if v.tab != 2 {
		v.clearYAML()
		return
	}
	item := a.cluster.otherResource()
	if v.yaml != nil || item == nil || a.ReadResourceYAML == nil || a.router.Path() != kubeClusterPage {
		return
	}
	in, ok := cliCopy(a.cliState(), "kubectl")
	if !ok {
		return
	}
	request := &resourceYAML{}
	v.yaml = request
	target := kube.Target{Kubectl: in.Path, Context: a.router.Query("context")}
	request.stop = a.ReadResourceYAML(target, *item, func(text string, err error) {
		if v.yaml == request {
			request.text, request.err, request.done = text, err, true
		}
	})
}

func (a *App) otherResourcesView(c *ui.Context, cl *kube.Cluster) {
	s := &a.cluster
	v := &s.other
	pal := Colors(c.Theme())
	claims := cl.Resources.Claims
	visible := func(item kube.Resource) bool {
		return matchesOtherResource(item, resourceQuery(s.query), claims) && claimShows(s.claimFilter, item, claims)
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
		if cl.Resources.Kind == kube.PersistentVolumeClaims {
			claimFilters(c, s, cl.Resources)
		}
		ui.Box(c).Grow(1)
		placeholder := locales.KubeDeploymentsSearch()
		if cl.Resources.Kind == kube.PersistentVolumeClaims {
			placeholder = locales.KubeStorageSearch()
		}
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).
			Placeholder(placeholder).Width(240).OnChange(func() {
			if item := s.otherResource(); item != nil && !visible(*item) {
				v.reset()
			}
		})
	})
	var shown []kube.Resource
	for _, r := range cl.Resources.Items {
		if visible(r) {
			shown = append(shown, r)
		}
	}
	slices.SortFunc(shown, func(a, b kube.Resource) int {
		if a.Kind == kube.PersistentVolumeClaims && claimProblem(a) != claimProblem(b) {
			if claimProblem(a) {
				return -1
			}
			return 1
		}
		if order := strings.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.Namespace, b.Namespace)
	})
	if len(shown) == 0 {
		message := locales.KubeResourcesEmpty()
		if cl.Resources.Kind == kube.PersistentVolumes || cl.Resources.Kind == kube.StorageClasses {
			message = locales.KubeResourcesEmptyCluster()
		}
		if resourceQuery(s.query) != "" {
			message = locales.SearchNoResults(s.query)
		} else if len(cl.Resources.Items) > 0 {
			message = locales.KubeStorageFilterEmpty()
		}
		toolNote(c, message)
		return
	}
	selected := s.otherResource()
	uid := ""
	if selected != nil && visible(*selected) {
		uid = selected.UID
	}
	tracks := []ui.Track{ui.Fr(1), ui.Fixed(92), ui.Fixed(66)}
	if uid == "" {
		tracks[1] = ui.Fixed(166)
	}
	workloadPanes(c, func() {
		resourceHead(c, tracks, locales.KubeClusterName(), locales.KubeClusterStatus(), locales.KubeClusterAge())
	}, func() {
		for _, r := range shown {
			row := resourceGrid(ui.ButtonBase(c.Key("other-resource-"+r.UID)), tracks).
				Label(r.Name).Padding(7, 16).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).
				OnClick(func() { s.toggleWorkload(&v.uid, r.UID); v.clearYAML(); v.tab = 0; s.storageLink = storageLink{} })
			if r.UID == uid {
				row.Background(pal.ActiveRow)
			} else if row.Hovered() {
				row.Background(pal.Subtle)
			}
			s.revealWorkloadRow(row, r.UID)
			row.Children(func() {
				ui.Column(c).Gap(2).MinWidth(0).Children(func() {
					clipped(c, r.Name).Font(FontMono).FontSize(12).FontWeight(500)
					if r.Namespace != "" && len(a.clusterNamespaces(a.router.Query("context"))) != 1 {
						clipped(c, r.Namespace).FontSize(10.5).TextColor(pal.TextFaint)
					}
				})
				label, bg, fg := otherResourceStatus(c, r, claims)
				workloadStatus(c, label, bg, fg)
				created := otherResourceAge(c, r)
				clipped(c, created).FontSize(11.5).TextColor(pal.TextFaint)
			})
		}
	}, uid, func() { a.otherResourceDetail(c, cl, *selected) })
}

func resourceQuery(query string) string { return strings.ToLower(strings.TrimSpace(query)) }

func otherResourceAge(c *ui.Context, r kube.Resource) string {
	if r.Created.IsZero() {
		return ""
	}
	return age(c, r.Created)
}

func matchesOtherResource(r kube.Resource, query string, claims *kube.ClaimDetails) bool {
	match := func(text string) bool { return strings.Contains(strings.ToLower(text), query) }
	if claims != nil && slices.ContainsFunc(claims.Consumers[claimKey(r)], func(p kube.ClaimConsumer) bool { return match(p.Name) }) {
		return true
	}
	return match(r.Namespace+"/"+r.Name) || match(r.Storage.Class) || match(r.Storage.Provisioner) || slices.ContainsFunc(r.Keys, match) ||
		slices.ContainsFunc(r.Routes, func(route kube.IngressRoute) bool { return match(route.Host) || match(route.Service) })
}

func otherResourceStatus(c *ui.Context, item kube.Resource, claims *kube.ClaimDetails) (string, ui.Color, ui.Color) {
	pal := Colors(c.Theme())
	if item.Deleting {
		return locales.KubeResourceTerminating(), pal.WarningSoft, pal.WarningText
	}
	switch item.Kind {
	case kube.PersistentVolumeClaims, kube.PersistentVolumes, kube.StorageClasses:
		return storageStatus(c, item, claims)
	case kube.StatefulSets, kube.DaemonSets, kube.ReplicaSets, kube.Endpoints:
		label := fmt.Sprintf("%d/%d", item.Ready, item.Desired)
		switch {
		case item.Desired == 0:
			return label, pal.Muted, c.Theme().TextMuted
		case item.Ready < item.Desired:
			return label, pal.WarningSoft, pal.WarningText
		}
		return label, pal.SuccessSoft, pal.SuccessText
	case kube.Ingresses:
		if len(item.Addresses) == 0 {
			return locales.KubeResourceNoAddress(), pal.WarningSoft, pal.WarningText
		}
		return item.Addresses[0], pal.SuccessSoft, pal.SuccessText
	case kube.NetworkPolicies:
		return strings.Join(item.PolicyTypes, " · "), pal.Muted, c.Theme().TextMuted
	}
	return locales.KubeResourceKeyCount(len(item.Keys)), pal.Muted, c.Theme().TextMuted
}

func (a *App) otherResourceDetail(c *ui.Context, cl *kube.Cluster, item kube.Resource) {
	s := &a.cluster
	v := &s.other
	t := c.Theme()
	label, bg, fg := otherResourceStatus(c, item, cl.Resources.Claims)
	kind := clusterKindOptions()[s.kind]
	events, read := clusterEvents(cl, item.UID)
	detailFrame(c, v.tab, false, func() {
		ui.Column(c).Padding(16).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon(kind.icon)).Size(16, 16).Shrink(0)
				clipped(c, item.Name).FontSize(15).FontWeight(600).Grow(1).MinWidth(0)
				aiTag(c, label, bg, fg).MaxWidth(110).Tooltip(label)
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(v.reset)
			})
			meta := []string{kind.label}
			for _, value := range []string{item.Namespace, item.Storage.Requested, strings.Join(item.Storage.AccessModes, ", ")} {
				if value != "" {
					meta = append(meta, value)
				}
			}
			if created := otherResourceAge(c, item); created != "" {
				meta = append(meta, locales.KubeCreatedAgo(created))
			}
			ui.Text(c, strings.Join(meta, " · ")).
				FontSize(12).TextColor(t.TextMuted).Wrap()
			count := ""
			if read && cl.Resources.EventsErr == nil {
				count = strconv.Itoa(len(events))
			}
			segments(c, "resource-tabs", &v.tab, []segment{{locales.KubeTabOverview(), ""}, {locales.KubeTabEvents(), count}, {locales.KubeResourceYaml(), ""}})
		})
	}, func() {
		switch v.tab {
		case 1:
			if read && cl.Resources.EventsErr != nil {
				warningRow(c, cl.Resources.EventsErr.Error())
			} else {
				eventList(c, events, read, 0)
			}
		case 2:
			otherResourceYAML(c, v)
		default:
			switch item.Kind {
			case kube.PersistentVolumeClaims, kube.PersistentVolumes, kube.StorageClasses:
				a.storageOverview(c, item, cl.Resources.Claims, events)
			default:
				resourceOverview(c, item)
			}
		}
	})
}

func resourceOverview(c *ui.Context, item kube.Resource) {
	var facts [][2]string
	add := func(label, value string) {
		if value != "" {
			facts = append(facts, [2]string{label, value})
		}
	}
	if item.Owner.Kind != "" {
		add(locales.KubeResourceOwner(), item.Owner.Kind+" "+item.Owner.Name)
	}
	if len(item.Selector) > 0 {
		var labels []string
		for _, key := range slices.Sorted(maps.Keys(item.Selector)) {
			labels = append(labels, key+"="+item.Selector[key])
		}
		add(locales.KubeResourceSelector(), strings.Join(labels, ", "))
	}
	switch item.Kind {
	case kube.StatefulSets, kube.DaemonSets, kube.ReplicaSets:
		add(locales.KubeReplicas(), fmt.Sprintf("%d/%d", item.Ready, item.Desired))
		if item.Kind != kube.ReplicaSets {
			add(locales.KubeResourceUpdated(), strconv.Itoa(item.Updated))
		}
		add(locales.KubeResourceImages(), strings.Join(item.Images, " · "))
		add(locales.KubeResourceService(), item.Service)
		add(locales.KubeResourceStrategy(), item.Strategy)
		add(locales.KubeResourcePodOrder(), item.PodOrder)
	case kube.Ingresses:
		add(locales.KubeResourceClass(), item.IngressClass)
		add(locales.KubeResourceAddresses(), strings.Join(item.Addresses, ", "))
		add("TLS", strings.Join(item.TLSSecrets, ", "))
	case kube.Endpoints:
		add(locales.KubeResourceAddresses(), strings.Join(item.Addresses, ", "))
		var ports []string
		for _, p := range item.Ports {
			ports = append(ports, p.String())
		}
		add(locales.KubeResourcePorts(), strings.Join(ports, ", "))
	case kube.NetworkPolicies:
		add(locales.KubeClusterStatus(), strings.Join(item.PolicyTypes, " · "))
		add(locales.KubeResourceIngressRules(), strconv.Itoa(item.IngressRules))
		add(locales.KubeResourceEgressRules(), strconv.Itoa(item.EgressRules))
	case kube.ConfigMaps:
		add(locales.KubeSecretKeys(), strconv.Itoa(len(item.Keys)))
		if item.Immutable {
			add(locales.KubeResourceImmutable(), "true")
		}
	}
	detailSection(c, locales.KubeFacts())
	factRows(c, facts)
	if len(item.Routes) > 0 {
		detailSection(c, locales.KubeResourceIngressRules())
		for i, route := range item.Routes {
			if route.Host == "" && len(item.Addresses) != 0 {
				route.Host = item.Addresses[0]
			}
			address := route.URL()
			ui.Row(c.Key("route-"+strconv.Itoa(i))).Padding(5, 16).Gap(8).AlignItems(ui.Center).Children(func() {
				text := route.Host + route.Path
				if route.Service != "" {
					text += " → " + route.Service + ":" + route.Port
				}
				ui.Text(c, text).Font(FontMono).FontSize(11.5).Wrap().Grow(1).MinWidth(0)
				if address != "" {
					kubeActionButton(c, "external-link", locales.KubeClusterOpen(), podActionHeight).
						Tooltip(address).OnClick(func() { c.OpenURL(address) })
				}
			})
		}
	}
	if len(item.Keys) > 0 {
		detailSection(c, locales.KubeSecretKeys())
		t := c.Theme()
		for _, key := range item.Keys {
			ui.Text(c, key).Padding(5, 16).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).Wrap().Selectable()
		}
	}
}

func otherResourceYAML(c *ui.Context, v *otherResourceView) {
	request := v.yaml
	ui.Column(c).Padding(8, 16).Gap(8).Children(func() {
		switch {
		case request == nil || !request.done:
			ui.Spinner(c).Size(13, 13)
		case request.err != nil:
			warningRow(c, request.err.Error())
			ui.Button(c, locales.KubeClusterRetry()).OnClick(v.clearYAML)
		default:
			kubeActionButton(c.Key("resource-copy-yaml"), "copy", locales.KubeResourceCopyYAML(), podActionHeight).
				AlignSelf(ui.End).OnClick(func() { c.WriteClipboard(request.text) })
			ui.Text(c, request.text).Font(FontMono).FontSize(11.5).Wrap().Selectable()
		}
	})
}
