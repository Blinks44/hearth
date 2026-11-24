package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

type clusterKind struct {
	label, icon, alias string
}

func clusterKindOptions() [kindCustom + 1]clusterKind {
	return [...]clusterKind{
		kindPods:            {"Pods", "box", "po"},
		kindDeployments:     {"Deployments", "boxes", "deploy"},
		kindOverview:        {locales.KubeOverview(), "layout-dashboard", "overview"},
		kindJobs:            {"Jobs", "package", "job"},
		kindCronJobs:        {"CronJobs", "history", "cj"},
		kindServices:        {"Services", "network", "svc"},
		kindSecrets:         {"Secrets", "key-round", "secret"},
		kindHelm:            {locales.KubeHelmReleases(), "layers", "helm"},
		kindStatefulSets:    {"StatefulSets", "boxes", "sts"},
		kindDaemonSets:      {"DaemonSets", "boxes", "ds"},
		kindReplicaSets:     {"ReplicaSets", "boxes", "rs"},
		kindIngresses:       {"Ingresses", "globe", "ing"},
		kindEndpoints:       {"Endpoints", "network", "ep"},
		kindNetworkPolicies: {"NetworkPolicies", "network", "netpol"},
		kindConfigMaps:      {"ConfigMaps", "file-text", "cm"},
		kindPVCs:            {"PersistentVolumeClaims", "database", "pvc"},
		kindPVs:             {"PersistentVolumes", "database", "pv"},
		kindStorageClasses:  {"StorageClasses", "layers", "sc"},
		kindNodes:           {"Nodes", "cpu", "no"},
		kindNamespaces:      {locales.KubeNamespaces(), "folder-tree", "ns"},
		kindPolicies:        {locales.KubePolicies(), "gauge", "hpa"},
		kindAccess:          {locales.KubeAccessResources(), "key-round", "rbac"},
		kindServiceAccounts: {"ServiceAccounts", "user", "sa"},
		kindCustom:          {locales.KubeCustomDefinitions(), "boxes", "crd"},
	}
}

func isBatchKind(kind int) bool { return kind == kindJobs || kind == kindCronJobs }

func clusterKindCounts(cl *kube.Cluster, kind int) (count, trouble int, known bool) {
	if cl == nil {
		return 0, 0, false
	}
	if cl.Access != nil {
		return len(cl.Access.Rules), 0, kind == kindAccess && cl.Access.RulesErr == nil
	}
	if cl.Policies != nil {
		if kind == kindPolicies {
			for _, list := range cl.Policies {
				if list.Err != nil {
					return 0, 0, false
				}
				count += len(list.Items)
			}
		}
		return count, 0, kind == kindPolicies
	}
	if cl.Resources != nil {
		return len(cl.Resources.Items), 0, cl.Resources.Custom == nil && cl.Resources.Kind == otherResourceKind(kind)
	}
	workloadsRead := !cl.Taken.IsZero() && cl.HelmReleases == nil && cl.Secrets == nil && cl.Services == nil &&
		(cl.Batch == nil || cl.Overview != nil)
	switch kind {
	case kindPods:
		for _, p := range cl.Pods {
			if podTrouble(p) {
				trouble++
			}
		}
		return len(cl.Pods), trouble, workloadsRead || cl.Pods != nil
	case kindDeployments:
		for _, d := range cl.Deployments {
			if deploymentTrouble(d) {
				trouble++
			}
		}
		return len(cl.Deployments), trouble, workloadsRead || cl.Deployments != nil
	case kindSecrets:
		return len(cl.Secrets), 0, cl.Secrets != nil
	case kindHelm:
		for _, r := range cl.HelmReleases {
			if r.Status == "failed" || r.Pending() {
				trouble++
			}
		}
		return len(cl.HelmReleases), trouble, cl.HelmReleases != nil
	case kindServices:
		for _, service := range cl.Services {
			if service.PodsKnown && service.Pods == 0 {
				trouble++
			}
		}
		return len(cl.Services), trouble, cl.Services != nil
	case kindJobs:
		if cl.Batch != nil && cl.Batch.JobsErr == nil {
			for _, j := range cl.Batch.Jobs {
				if j.State == kube.JobFailed {
					trouble++
				}
			}
			return len(cl.Batch.Jobs), trouble, true
		}
	case kindCronJobs:
		if cl.Batch != nil && cl.Batch.CronJobsErr == nil {
			for _, j := range cl.Batch.CronJobs {
				if lastCronFailed(j) {
					trouble++
				}
			}
			return len(cl.Batch.CronJobs), trouble, true
		}
	}
	return 0, 0, false
}

func (a *App) toggleClusterKinds() {
	s := &a.cluster
	s.kindsOpen = !s.kindsOpen
	if s.kindsOpen {
		s.kindQuery, s.kindSelected, s.kindShowAll = "", 0, false
		s.contextOpen, s.namespaceOpen = false, false
		a.loadCustomDefinitions(a.router.Query("context"))
	}
}

func clusterKindAllowed(access *kube.ListAccess, kind int) bool {
	if access == nil {
		return true
	}
	switch kind {
	case kindOverview, kindAccess:
		return true
	case kindPods:
		return access.CanList("", "pods")
	case kindDeployments:
		return access.CanList("apps", "deployments")
	case kindStatefulSets:
		return access.CanList("apps", "statefulsets")
	case kindDaemonSets:
		return access.CanList("apps", "daemonsets")
	case kindReplicaSets:
		return access.CanList("apps", "replicasets")
	case kindJobs:
		return access.CanList("batch", "jobs")
	case kindCronJobs:
		return access.CanList("batch", "cronjobs")
	case kindServices:
		return access.CanList("", "services")
	case kindIngresses:
		return access.CanList("networking.k8s.io", "ingresses")
	case kindEndpoints:
		return access.CanList("", "endpoints")
	case kindNetworkPolicies:
		return access.CanList("networking.k8s.io", "networkpolicies")
	case kindSecrets:
		return access.CanList("", "secrets")
	case kindConfigMaps:
		return access.CanList("", "configmaps")
	case kindHelm:
		return access.CanList("", "secrets") || access.CanList("", "configmaps")
	case kindPVCs:
		return access.CanList("", "persistentvolumeclaims")
	case kindPVs:
		return access.CanList("", "persistentvolumes")
	case kindStorageClasses:
		return access.CanList("storage.k8s.io", "storageclasses")
	case kindNodes:
		return access.CanList("", "nodes")
	case kindNamespaces:
		return access.CanList("", "namespaces")
	case kindServiceAccounts:
		return access.CanList("", "serviceaccounts")
	case kindPolicies:
		return access.CanList("autoscaling", "horizontalpodautoscalers") ||
			access.CanList("policy", "poddisruptionbudgets") ||
			access.CanList("", "resourcequotas") ||
			access.CanList("", "limitranges")
	case kindCustom:
		return access.CanList("apiextensions.k8s.io", "customresourcedefinitions")
	}
	return true
}

func (a *App) clusterKindPicker(c *ui.Context) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	options := clusterKindOptions()
	query := strings.ToLower(strings.TrimSpace(s.kindQuery))
	var access *kube.ListAccess
	if catalog := s.custom.catalog; catalog != nil && catalog.result != nil {
		access = catalog.result.Access
	}
	hasAccessibleCRD := true
	if catalog := s.custom.catalog; catalog != nil && catalog.result != nil && catalog.result.Err == nil && access != nil && len(catalog.result.Items) > 0 {
		hasAccessibleCRD = false
		for i := range catalog.result.Items {
			d := &catalog.result.Items[i]
			if access.CanList(d.Group, d.Names.Plural) {
				hasAccessibleCRD = true
				break
			}
		}
	}
	type entry struct {
		id    int
		kind  clusterKind
		group string
		def   *kube.CustomDefinition
	}
	var exact, partial []entry
	add := func(e entry, aliases ...string) {
		terms := append(aliases, strings.ToLower(e.kind.label), e.kind.alias, strings.ToLower(e.group))
		for _, term := range terms {
			if term == query && query != "" {
				exact = append(exact, e)
				return
			}
		}
		for _, term := range terms {
			if strings.Contains(term, query) {
				partial = append(partial, e)
				return
			}
		}
	}
	hidden := 0
	for _, i := range []int{kindOverview, kindPods, kindDeployments, kindStatefulSets, kindDaemonSets, kindReplicaSets,
		kindJobs, kindCronJobs, kindServices, kindIngresses, kindEndpoints, kindNetworkPolicies, kindSecrets, kindConfigMaps, kindHelm,
		kindPVCs, kindPVs, kindStorageClasses, kindNodes, kindNamespaces, kindPolicies, kindServiceAccounts, kindAccess, kindCustom} {
		allowed := clusterKindAllowed(access, i) && (i != kindCustom || hasAccessibleCRD)
		if !allowed {
			hidden++
			if !s.kindShowAll {
				continue
			}
		}
		group := ""
		switch i {
		case kindPVCs, kindPVs, kindStorageClasses:
			group = locales.KubeCustomStorage()
		case kindNodes, kindNamespaces, kindPolicies, kindServiceAccounts, kindAccess, kindCustom:
			group = locales.KubeCustomCluster()
		}
		switch i {
		case kindPolicies:
			add(entry{id: i, kind: options[i], group: group}, "horizontalpodautoscalers", "pdb", "poddisruptionbudgets", "resourcequota", "resourcequotas", "quota", "limitrange", "limitranges", "limits")
		case kindAccess:
			add(entry{id: i, kind: options[i], group: group}, "roles", "clusterroles", "rolebindings", "clusterrolebindings", "bindings", "can-i")
		default:
			add(entry{id: i, kind: options[i], group: group})
		}
	}
	hiddenEmpty := 0
	if catalog := s.custom.catalog; catalog != nil && catalog.result != nil {
		for i := range catalog.result.Items {
			d := &catalog.result.Items[i]
			if access != nil && !access.CanList(d.Group, d.Names.Plural) {
				hidden++
				if !s.kindShowAll {
					continue
				}
			} else if !s.kindShowEmpty && query == "" && catalog.isEmpty(d.Name()) {
				hiddenEmpty++
				continue
			}
			alias := d.Names.Singular
			if len(d.Names.ShortNames) > 0 {
				alias = d.Names.ShortNames[0]
			}
			kind := clusterKind{d.Names.Plural, "boxes", alias}
			add(entry{id: kindCustom, kind: kind, group: d.Group, def: d}, append([]string{d.Name(), d.Names.Singular}, d.Names.ShortNames...)...)
		}
	}
	results := append(exact, partial...)
	selected := min(s.kindSelected, max(len(results)-1, 0))
	choose := func(e entry) {
		if e.id == kindPolicies {
			switch query {
			case "hpa", "horizontalpodautoscalers":
				s.policyTab = 0
			case "pdb", "poddisruptionbudgets":
				s.policyTab = 1
			case "quota", "resourcequota", "resourcequotas":
				s.policyTab = 2
			case "limitrange", "limitranges":
				s.policyTab = 3
			}
		}
		if e.id == kindAccess {
			s.access.tab = accessRulesTab
			s.access.resetSelection()
			switch query {
			case "roles", "clusterroles":
				s.access.tab = accessRolesTab
			case "rolebindings", "clusterrolebindings", "bindings":
				s.access.tab = accessBindingsTab
			}
		}
		a.selectClusterKind(e.id, e.def)
	}
	move := func(by int) {
		if n := len(results); n > 0 {
			s.kindSelected = (selected + by + n) % n
			s.kindScroll = true
		}
	}
	ui.Column(c).Width(365).Padding(6).Gap(2).Children(func() {
		ui.SearchField(c.Key("kind-query"), &s.kindQuery).Placeholder(locales.KubeKindSearch()).Label(locales.KubeKindSearch()).AutoFocus().Margin(0, 0, 5, 0).
			OnChange(func() { s.kindSelected, s.kindScroll = 0, true }).OnSubmit(func() {
			if len(results) > 0 {
				choose(results[selected])
			}
		})
		if catalog := s.custom.catalog; catalog != nil && catalog.result != nil && len(catalog.result.Items) > 0 && (a.ProbeCustomPresence != nil || catalog.presence != nil) {
			a.nonEmptyCRDToggleRow(c, "kind-non-empty", catalog, hiddenEmpty)
		}
		c.OnShortcut(0, ui.KeyDown, func() { move(1) })
		c.OnShortcut(0, ui.KeyUp, func() { move(-1) })
		if len(results) == 0 {
			overviewMessage(c, locales.SearchNoResults(s.kindQuery))
		}
		ui.Scroll(c.Key("kind-results")).MaxHeight(435).Children(func() {
			group := ""
			for index, entry := range results {
				id, kind := entry.id, entry.kind
				if entry.group != group && entry.group != "" {
					ui.Text(c, strings.ToUpper(entry.group)).FontSize(10.5).TextColor(pal.TextFaint).Margin(8, 0, 4, 10)
				}
				group = entry.group
				key := kind.label
				if entry.def != nil {
					key = entry.def.Name()
				}
				row := ui.ButtonBase(c.Key("pick-kind-"+key)).Height(32).Radius(7).Padding(0, 10).Gap(9).AlignItems(ui.Center).
					Label(kind.label).OnClick(func() { choose(entry) })
				if index == selected {
					row.Background(pal.AccentSoft)
					if s.kindScroll {
						row.ScrollIntoView()
						s.kindScroll = false
					}
				} else if row.Hovered() {
					row.Background(pal.Muted)
				}
				row.Children(func() {
					fg := t.TextMuted
					if id == s.kind && (id != kindCustom || s.custom.name == "" && entry.def == nil || entry.def != nil && entry.def.Name() == s.custom.name) {
						fg = pal.AccentInk
					}
					ui.Icon(c, Icon(kind.icon)).Size(15, 15).TextColor(fg)
					clipped(c, kind.label).FontSize(13).FontWeight(500).MinWidth(0)
					if id != kindOverview {
						ui.Text(c, kind.alias).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
					}
					ui.Box(c).Grow(1)
					if entry.def != nil {
						if list := s.custom.list(s.data); id == s.kind && entry.def.Name() == s.custom.name {
							if s.err != nil {
								clipped(c, a.accessWords(s.err)).FontSize(11).TextColor(pal.TextFaint)
							} else if list != nil {
								ui.Text(c, strconv.Itoa(len(list.Items))).FontSize(11).TextColor(pal.TextFaint)
							}
						}
						return
					}
					count, trouble, known := clusterKindCounts(s.data, id)
					if trouble > 0 {
						aiTag(c, strconv.Itoa(trouble), pal.DangerSoft, pal.DangerText)
					}
					if known && ((!s.batchData && s.data.Services == nil) || id != kindPods && id != kindDeployments) {
						ui.Text(c, strconv.Itoa(count)).FontSize(11).TextColor(pal.TextFaint)
					}
					if id != kindOverview {
						a.pinButton(c, id)
					}
				})
			}
		})
		if catalog := s.custom.catalog; catalog != nil && (catalog.result == nil || (catalog.result.Err != nil && (access == nil || s.kindShowAll))) {
			a.customCatalogNotice(c)
		}
		if hidden > 0 {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(6, 10, 4, 10).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, locales.KubeKindHiddenNoAccess(hidden)).FontSize(11).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
				toggleLabel := locales.KubeKindShowHidden()
				if s.kindShowAll {
					toggleLabel = locales.KubeKindHideNoAccess()
				}
				b := ui.ButtonBase(c.Key("kind-toggle-hidden")).Padding(2, 6).Radius(5).Label(toggleLabel).
					OnClick(func() { s.kindShowAll = !s.kindShowAll; s.kindSelected = 0 })
				if b.Hovered() {
					b.Background(pal.Muted)
				}
				b.Children(func() {
					ui.Text(c, toggleLabel).FontSize(11).FontWeight(500).TextColor(t.TextMuted)
				})
			})
		}
		ui.Row(c).Gap(14).Padding(8, 10, 4, 10).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			for _, hint := range []string{locales.KubeKindHintMove(), locales.KubeKindHintOpen(), locales.KubeKindHintPin()} {
				ui.Text(c, hint).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			}
		})
	})
}

// pinButton pins the kind above the cluster's tables, or unpins it.
func (a *App) pinButton(c *ui.Context, kind int) {
	t := c.Theme()
	pal := Colors(t)
	pinned := slices.Contains(a.pinnedKinds(), kind)
	label := locales.KubeKindPin(clusterKindOptions()[kind].label)
	icon, color := "star", pal.TextFaint
	if pinned {
		label = locales.KubeKindUnpin(clusterKindOptions()[kind].label)
		icon, color = "star-filled", t.Accent
	}
	b := ui.ButtonBase(c.Key("pin-kind-"+strconv.Itoa(kind))).Size(22, 22).Radius(5).Center().Label(label).Tooltip(label).
		OnClick(func() { a.togglePin(kind) })
	if b.Hovered() {
		b.Background(pal.Muted)
		if !pinned {
			color = t.TextMuted
		}
	}
	b.Children(func() { ui.Icon(c, Icon(icon)).Size(14, 14).TextColor(color) })
}
