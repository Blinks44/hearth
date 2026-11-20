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
	case kindNamespaces:
		return kube.NamespaceKind
	case kindNodes:
		return kube.Nodes
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
	case kindServiceAccounts:
		return kube.ServiceAccounts
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
	kind kube.ResourceKind
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
	if s.other.uid == "" || s.data == nil {
		return nil
	}
	list := s.activeResourceList(s.data)
	if list == nil {
		return nil
	}
	if s.kind == kindCustom {
		if s.custom.list(s.data) == nil {
			return nil
		}
	} else if s.kind != kindPolicies && list.Kind != otherResourceKind(s.kind) {
		return nil
	}
	items := list.Items
	if i := slices.IndexFunc(items, func(r kube.Resource) bool { return r.UID == s.other.uid }); i >= 0 {
		return &items[i]
	}
	return nil
}

func (a *App) syncResourceYAML() {
	v := &a.cluster.other
	yamlTab := 2
	if item := a.cluster.otherResource(); a.cluster.kind == kindNodes || a.cluster.kind == kindNamespaces || a.cluster.kind == kindCustom || item != nil && item.Autoscaler != nil {
		yamlTab = 3
	}
	if v.tab != yamlTab {
		v.clearYAML()
		return
	}
	item := a.cluster.otherResource()
	if v.yaml != nil && (item == nil || v.yaml.kind != item.Kind) {
		v.clearYAML()
	}
	if v.yaml != nil || item == nil || a.ReadResourceYAML == nil || a.router.Path() != kubeClusterPage {
		return
	}
	in, ok := cliCopy(a.cliState(), "kubectl")
	if !ok {
		return
	}
	request := &resourceYAML{kind: item.Kind}
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
		return matchesOtherResource(item, resourceQuery(s.query), claims) && claimShows(s.claimFilter, item, claims) && nodeShows(s.nodeFilter, item.Node) && namespaceShows(s.namespaceFilter, item)
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
		if s.kind == kindPolicies {
			policyTabs(c, &s.policyTab, cl.Policies)
		}
		if cl.Resources.Kind == kube.PersistentVolumeClaims {
			claimFilters(c, s, cl.Resources)
		}
		if cl.Resources.Kind == kube.Nodes {
			nodeFilters(c, s, cl.Resources.Items)
		}
		if s.kind == kindNamespaces {
			a.namespaceFilters(c, cl.Resources.Items)
		}
		ui.Box(c).Grow(1)
		if s.kind == kindPolicies {
			policySort(c, &s.policySort)
		}
		if s.kind == kindNamespaces && a.BuildPlan != nil {
			name := a.router.Query("context")
			cli, ok := cliCopy(a.cliState(), "kubectl")
			kubeActionButton(c.Key("new-namespace"), "plus", locales.KubeNamespaceNew(), podActionHeight).
				Disabled(!ok || a.op.running()).OnClick(func() {
				s.namespaceCreate = &namespaceCreateForm{context: name, kubectl: cli.Path, open: true}
			})
		}
		width := float32(240)
		if (s.kind == kindPolicies || s.kind == kindNamespaces) && narrow(c) {
			width = 180
		}
		placeholder := locales.KubeDeploymentsSearch()
		if s.kind == kindNamespaces {
			placeholder = locales.KubeNamespaceSearch()
		}
		if cl.Resources.Kind == kube.PersistentVolumeClaims {
			placeholder = locales.KubeStorageSearch()
		}
		if cl.Resources.Kind == kube.Nodes {
			placeholder = locales.KubeNodeSearch()
		}
		if cl.Resources.Kind == kube.Autoscalers {
			placeholder = locales.KubeAutoscalerSearch()
		}
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).
			Placeholder(placeholder).Label(placeholder).Width(width).OnChange(func() {
			if item := s.otherResource(); item != nil && !visible(*item) {
				v.reset()
			}
		})
	})
	if cl.Resources.Err != nil {
		a.clusterError(c, a.router.Query("context"), cl.Resources.Err)
		return
	}
	var shown []kube.Resource
	for _, r := range cl.Resources.Items {
		if visible(r) {
			shown = append(shown, r)
		}
	}
	slices.SortFunc(shown, func(a, b kube.Resource) int {
		if s.kind == kindPolicies && !s.policySort {
			if priority := policyPriority(b) - policyPriority(a); priority != 0 {
				return priority
			}
		}
		if a.NamespaceInfo != nil && b.NamespaceInfo != nil {
			if priority := namespacePriority(b) - namespacePriority(a); priority != 0 {
				return priority
			}
		}
		if a.Node != nil && b.Node != nil {
			if priority := nodePriority(b.Node) - nodePriority(a.Node); priority != 0 {
				return priority
			}
		}
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
		if s.kind == kindNamespaces || cl.Resources.Kind == kube.Nodes || cl.Resources.Kind == kube.PersistentVolumes || cl.Resources.Kind == kube.StorageClasses {
			message = locales.KubeResourcesEmptyCluster()
		}
		if resourceQuery(s.query) != "" {
			message = locales.SearchNoResults(s.query)
		} else if len(cl.Resources.Items) > 0 {
			if s.kind == kindNamespaces {
				message = locales.KubeNamespaceFilterEmpty()
			} else if cl.Resources.Kind == kube.Nodes {
				message = locales.KubeNodeFilterEmpty()
			} else {
				message = locales.KubeStorageFilterEmpty()
			}
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
	headers := []string{locales.KubeClusterName(), locales.KubeClusterStatus(), locales.KubeClusterAge()}
	if uid == "" {
		tracks[1] = ui.Fixed(166)
	}
	if s.kind == kindPolicies {
		tracks = []ui.Track{ui.Fr(1), ui.Fixed(124)}
		headers = headers[:2]
		if cl.Resources.Kind == kube.Autoscalers {
			headers[1] = locales.KubeReplicas()
		}
	}
	if s.kind == kindNamespaces {
		tracks = []ui.Track{ui.Fr(1), ui.Fixed(140)}
		headers = headers[:2]
	}
	if s.kind == kindServiceAccounts {
		tracks = []ui.Track{ui.Fr(1), ui.Fixed(66)}
		headers = []string{locales.KubeClusterName(), locales.KubeClusterAge()}
	}
	if cl.Resources.Kind == kube.Nodes {
		metricsNotice(c, cl.MetricsErr)
		tracks = []ui.Track{ui.Fr(1), ui.Fixed(116)}
		headers = headers[:2]
		if uid == "" {
			tracks = append(tracks, ui.Fixed(110), ui.Fixed(110))
			headers = append(headers, "CPU", locales.KubeMetricsMemory())
		}
	}
	workloadPanes(c, func() {
		resourceHead(c, tracks, headers...)
	}, func() {
		for _, r := range shown {
			row := resourceGrid(ui.ButtonBase(c.Key("other-resource-"+r.UID)), tracks).
				Label(r.Name).Padding(7, 16).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).
				OnClick(func() { s.toggleWorkload(&v.uid, r.UID); v.clearYAML(); v.tab = 0; s.resourceLink = resourceLink{} })
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
				if s.kind != kindServiceAccounts {
					label, bg, fg := otherResourceStatus(c, r, claims)
					workloadStatus(c, label, bg, fg)
				}
				if r.Node == nil && s.kind != kindPolicies && s.kind != kindNamespaces {
					created := otherResourceAge(c, r)
					clipped(c, created).FontSize(11.5).TextColor(pal.TextFaint)
				} else if r.Node != nil && uid == "" {
					podMetricCell(c, r.Node.Usage, kube.Resources{}, false)
					podMetricCell(c, r.Node.Usage, kube.Resources{}, true)
				}
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
	if r.NamespaceInfo != nil {
		for key, value := range r.NamespaceInfo.Labels {
			if match(key + "=" + value) {
				return true
			}
		}
	}
	if r.Node != nil {
		for key, value := range r.Node.Labels {
			if match(key + "=" + value) {
				return true
			}
		}
		for _, taint := range r.Node.Taints {
			if match(taint.String()) {
				return true
			}
		}
	}
	if claims != nil && slices.ContainsFunc(claims.Consumers[claimKey(r)], func(p kube.ClaimConsumer) bool { return match(p.Name) }) {
		return true
	}
	return match(r.Namespace+"/"+r.Name) || r.Autoscaler != nil && match(r.Autoscaler.Target.Kind+"/"+r.Autoscaler.Target.Name) || match(r.Storage.Class) || match(r.Storage.Provisioner) || slices.ContainsFunc(r.Keys, match) ||
		slices.ContainsFunc(r.Routes, func(route kube.IngressRoute) bool { return match(route.Host) || match(route.Service) })
}

func otherResourceStatus(c *ui.Context, item kube.Resource, claims *kube.ClaimDetails) (string, ui.Color, ui.Color) {
	pal := Colors(c.Theme())
	if item.NamespaceInfo != nil {
		return namespaceStatus(c, item)
	}
	if item.Deleting {
		return locales.KubeResourceTerminating(), pal.WarningSoft, pal.WarningText
	}
	switch item.Kind {
	case kube.Autoscalers, kube.DisruptionBudgets, kube.ResourceQuotas, kube.LimitRanges:
		return policyStatus(c, item)
	case kube.Nodes:
		return nodeStatus(c, item.Node)
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
	case kube.ServiceAccounts:
		return "", pal.Muted, c.Theme().TextMuted
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
	if item.NamespaceInfo != nil {
		kind.label = locales.KubeNamespaceLabel()
	}
	if s.kind == kindPolicies {
		kind.label = policyKindName(item.Kind)
	}
	events, read := clusterEvents(cl, item.UID)
	detailFrame(c, v.tab, false, func() {
		ui.Column(c).Padding(16).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon(kind.icon)).Size(16, 16).Shrink(0)
				clipped(c, item.Name).FontSize(15).FontWeight(600).Grow(1).MinWidth(0)
				width := float32(110)
				if s.kind == kindPolicies || s.kind == kindNamespaces {
					width = 144
				}
				if label != "" {
					aiTag(c, label, bg, fg).MaxWidth(width).Tooltip(label)
				}
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(v.reset)
			})
			meta := []string{kind.label}
			if n := item.Node; n != nil {
				meta = append(meta, n.Roles...)
				for _, value := range []string{n.Version, n.OS, n.Runtime} {
					if value != "" {
						meta = append(meta, value)
					}
				}
			}
			for _, value := range []string{item.Namespace, item.Storage.Requested, strings.Join(item.Storage.AccessModes, ", ")} {
				if value != "" {
					meta = append(meta, value)
				}
			}
			if hpa := item.Autoscaler; hpa != nil {
				meta = append(meta, hpa.Target.Kind+" "+hpa.Target.Name)
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
			tabs := []segment{{locales.KubeTabOverview(), ""}}
			if item.Node != nil {
				a.nodeActions(c, item)
				pods := ""
				if read && cl.Resources.NodePodsErr == nil {
					pods = strconv.Itoa(len(cl.Resources.NodePods))
				}
				tabs = append(tabs, segment{"Pods", pods})
			}
			if item.NamespaceInfo != nil {
				a.namespaceHeaderActions(c, item, v)
				members := ""
				if read && cl.Resources.NamespaceMembersErr == nil {
					members = strconv.Itoa(len(cl.Resources.NamespaceMembers))
				}
				tabs = append(tabs, segment{locales.KubeNamespaceContents(), members})
			}
			if item.Autoscaler != nil {
				tabs = append(tabs, segment{locales.KubeAutoscalerMetrics(), strconv.Itoa(len(item.Autoscaler.Metrics))})
			}
			if item.Kind == kube.StatefulSets {
				a.ownerLogsButton(c, a.resourceRef("StatefulSet", item.Namespace, item.Name, item.UID))
			}
			tabs = append(tabs, segment{locales.KubeTabEvents(), count}, segment{locales.KubeResourceYaml(), ""})
			segments(c, "resource-tabs", &v.tab, tabs)
		})
	}, func() {
		tab := v.tab
		if item.Node != nil && tab > 0 {
			if tab == 1 {
				nodePodsView(c, cl.Resources, read)
				return
			}
			tab--
		}
		if item.NamespaceInfo != nil && tab > 0 {
			if tab == 1 {
				namespaceMembersView(c, cl.Resources, read, false)
				return
			}
			tab--
		}
		if item.Autoscaler != nil && tab > 0 {
			if tab == 1 {
				autoscalerMetricsView(c, item.Autoscaler.Metrics)
				return
			}
			tab--
		}
		switch tab {
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
			case kube.NamespaceKind:
				namespaceOverview(c, item, cl.Resources, read, &v.tab)
			case kube.Autoscalers, kube.DisruptionBudgets, kube.ResourceQuotas, kube.LimitRanges:
				a.policyOverview(c, item, cl.Policies)
			case kube.Nodes:
				nodeOverview(c, item.Node, cl.Resources, read)
			case kube.PersistentVolumeClaims, kube.PersistentVolumes, kube.StorageClasses:
				a.storageOverview(c, item, cl.Resources.Claims, events)
			default:
				resourceOverview(c, item)
				d := relationshipsFor(cl, item.Namespace, item.UID)
				a.resourceRelationships(c, d.Links, d.Errors)
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
	case kube.ServiceAccounts:
		add("UID", item.UID)
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
