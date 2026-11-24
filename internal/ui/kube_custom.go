package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

type customResourceView struct {
	context, name, label string
	catalog              *customCatalog
}

type customCatalog struct {
	scope     string
	result    *kube.CustomDefinitions
	presence  map[string]bool
	probing   bool
	taken     time.Time
	stop      func()
	probeStop func()
}

func (c *customCatalog) stopAll() {
	if c == nil {
		return
	}
	if c.stop != nil {
		c.stop()
	}
	if c.probeStop != nil {
		c.probeStop()
	}
}

func (c *customCatalog) invalidatePresence() {
	if c == nil {
		return
	}
	if c.probeStop != nil {
		c.probeStop()
		c.probeStop = nil
	}
	c.probing = false
	c.presence = nil
}

func (c *customCatalog) isEmpty(name string) bool {
	if c == nil || c.presence == nil {
		return false
	}
	has, known := c.presence[name]
	return known && !has
}

func (v *customResourceView) reset(context string) {
	v.catalog.stopAll()
	*v = customResourceView{context: context}
}

func (v *customResourceView) list(cl *kube.Cluster) *kube.ResourceList {
	if cl != nil && cl.Resources != nil && cl.Resources.Custom != nil && cl.Resources.Custom.Name() == v.name {
		return cl.Resources
	}
	return nil
}

func (a *App) customCatalogScope(name string) string {
	return strings.Join(a.clusterNamespaces(name), ",") + "|" + a.accessNamespace(name)
}

func (a *App) syncCustomDefinitions() {
	s := &a.cluster
	if a.router.Path() != kubeClusterPage {
		if s.custom.context != "" {
			s.custom.reset("")
		}
		return
	}
	if s.kind == kindCustom && s.custom.name == "" && (s.custom.catalog == nil || s.custom.catalog.scope != a.customCatalogScope(s.custom.context)) {
		a.loadCustomDefinitions(s.custom.context)
	}
}

func (a *App) loadCustomDefinitions(name string) {
	if name == "" || a.ListCustomDefinitions == nil {
		return
	}
	in, _ := cliCopy(a.cliState(), "kubectl")
	ns := a.clusterNamespaces(name)
	accessNS := a.accessNamespace(name)
	scope := strings.Join(ns, ",") + "|" + accessNS
	v := &a.cluster.custom
	request := &customCatalog{scope: scope}
	if v.catalog != nil {
		if v.catalog.scope == scope {
			request.result, request.taken, request.presence = v.catalog.result, v.catalog.taken, v.catalog.presence
		}
		v.catalog.stopAll()
	}
	v.catalog = request
	target := kube.Target{Kubectl: in.Path, Context: name, Namespaces: ns, AccessNamespace: accessNS}
	request.stop = a.ListCustomDefinitions(target, func(result *kube.CustomDefinitions) {
		if a.router.Path() == kubeClusterPage && a.router.Query("context") == name && v.catalog == request {
			request.result, request.taken = result, time.Now()
			if !a.cluster.kindShowEmpty && request.presence == nil && !request.probing {
				a.startCustomPresenceProbe(name, request)
			}
		}
	})
}

func (a *App) ensureCustomPresence(name string) {
	catalog := a.cluster.custom.catalog
	if catalog == nil || catalog.result == nil || catalog.presence != nil || catalog.probing {
		return
	}
	a.startCustomPresenceProbe(name, catalog)
}

func (a *App) startCustomPresenceProbe(name string, catalog *customCatalog) {
	if name == "" || a.ProbeCustomPresence == nil || catalog == nil || catalog.result == nil || catalog.result.Err != nil {
		return
	}
	access := catalog.result.Access
	defs := make([]kube.CustomDefinition, 0, len(catalog.result.Items))
	for _, d := range catalog.result.Items {
		if access == nil || access.CanList(d.Group, d.Names.Plural) {
			defs = append(defs, d)
		}
	}
	if len(defs) == 0 {
		return
	}
	if catalog.probeStop != nil {
		catalog.probeStop()
	}
	catalog.probing = true
	target := kube.Target{Context: name, Namespaces: a.clusterNamespaces(name), AccessNamespace: a.accessNamespace(name)}
	catalog.probeStop = a.ProbeCustomPresence(target, defs, func(presence map[string]bool) {
		if a.router.Path() == kubeClusterPage && a.router.Query("context") == name && a.cluster.custom.catalog == catalog {
			catalog.probing = false
			catalog.presence = presence
		}
	})
}

func (a *App) selectClusterKind(id int, def *kube.CustomDefinition) {
	s := &a.cluster
	name, label := "", ""
	if def != nil {
		name, label = def.Name(), def.Names.Plural+" · "+def.Group
	}
	if name != s.custom.name {
		s.other.reset()
		s.err = nil
	}
	if id != s.kind {
		s.tab = 0
	}
	s.custom.name, s.custom.label = name, label
	s.kind, s.logs, s.kindsOpen = id, nil, false
	s.resourceLink = resourceLink{}
}

func (a *App) customCatalogNotice(c *ui.Context) {
	s := &a.cluster
	if s.custom.catalog == nil || s.custom.catalog.result == nil {
		ui.Spinner(c).Size(14, 14)
	} else if err := s.custom.catalog.result.Err; err != nil {
		label := a.accessWords(err)
		if label == locales.KubeClusterForbidden() {
			label = locales.KubeCustomForbiddenTitle()
		}
		warningRow(c, label)
	}
}

func (a *App) nonEmptyCRDToggleRow(c *ui.Context, key string, catalog *customCatalog, hiddenEmpty int) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	nonEmpty := !s.kindShowEmpty
	apply := func(next bool) {
		s.kindShowEmpty = !next
		s.kindSelected = 0
		if next {
			a.ensureCustomPresence(a.router.Query("context"))
		}
	}
	ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(2, 10, 4, 10).Children(func() {
		ui.ButtonBase(c.Key(key + "-label")).AlignItems(ui.Center).Gap(8).Grow(1).MinWidth(0).
			Label(locales.KubeKindNonEmptyOnly()).OnClick(func() { apply(!nonEmpty) }).Children(func() {
			ui.Text(c, locales.KubeKindNonEmptyOnly()).FontSize(12).TextColor(t.TextMuted).SingleLine()
			if catalog.probing {
				ui.Spinner(c).Size(12, 12)
			} else if hiddenEmpty > 0 {
				ui.Text(c, locales.KubeKindHiddenEmpty(hiddenEmpty)).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			}
		})
		toggle(c, key, &nonEmpty).Size(32, 18).Shrink(0).Label(locales.KubeKindNonEmptyOnly()).OnChange(func() {
			apply(nonEmpty)
		})
	})
}

func (a *App) customDefinitionsView(c *ui.Context) {
	s := &a.cluster
	catalog := s.custom.catalog
	if catalog == nil || catalog.result == nil || catalog.result.Err != nil {
		a.customCatalogNotice(c)
		return
	}
	access := catalog.result.Access
	var defs []kube.CustomDefinition
	hiddenEmpty := 0
	hiddenNoAccess := 0
	for _, d := range catalog.result.Items {
		if access != nil && !access.CanList(d.Group, d.Names.Plural) {
			hiddenNoAccess++
			if !s.kindShowAll {
				continue
			}
		} else if !s.kindShowEmpty && catalog.isEmpty(d.Name()) {
			hiddenEmpty++
			continue
		}
		defs = append(defs, d)
	}
	showToggle := len(catalog.result.Items) > 0 && (a.ProbeCustomPresence != nil || catalog.presence != nil)
	renderHiddenBar := func() {
		if hiddenNoAccess == 0 {
			return
		}
		t := c.Theme()
		pal := Colors(t)
		ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(2, 10, 4, 10).Children(func() {
			ui.Text(c, locales.KubeKindHiddenNoAccess(hiddenNoAccess)).FontSize(11).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
			toggleLabel := locales.KubeKindShowHidden()
			if s.kindShowAll {
				toggleLabel = locales.KubeKindHideNoAccess()
			}
			b := ui.ButtonBase(c.Key("crd-table-toggle-hidden")).Padding(2, 6).Radius(5).Label(toggleLabel).
				OnClick(func() { s.kindShowAll = !s.kindShowAll })
			if b.Hovered() {
				b.Background(pal.Muted)
			}
			b.Children(func() {
				ui.Text(c, toggleLabel).FontSize(11).FontWeight(500).TextColor(t.TextMuted)
			})
		})
	}
	if len(defs) == 0 {
		if showToggle {
			a.nonEmptyCRDToggleRow(c, "crd-table-non-empty", catalog, hiddenEmpty)
		}
		renderHiddenBar()
		if hiddenEmpty > 0 {
			toolNote(c, locales.KubeCustomAllEmpty())
		} else {
			toolNote(c, locales.KubeResourcesEmptyCluster())
		}
		return
	}
	pal := Colors(c.Theme())
	tracks := []ui.Track{ui.Fr(2), ui.Fr(2), ui.Fr(1), ui.Fr(1)}
	if showToggle {
		a.nonEmptyCRDToggleRow(c, "crd-table-non-empty", catalog, hiddenEmpty)
	}
	renderHiddenBar()
	workloadPanes(c, func() {
		resourceHead(c, tracks, locales.KubeClusterName(), locales.KubeCustomGroup(), locales.KubeCustomVersions(), locales.KubeCustomScope())
	}, func() {
		for _, d := range defs {
			var versions []string
			for _, v := range d.Versions {
				if v.Served {
					versions = append(versions, v.Name)
				}
			}
			scope := locales.KubeCustomClusterScoped()
			if d.Namespaced() {
				scope = locales.KubeCustomNamespaced()
			}
			row := resourceGrid(ui.ButtonBase(c.Key(d.Name())), tracks).Label(d.Names.Plural).
				Padding(8, 16).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).
				OnClick(func() { a.selectClusterKind(kindCustom, &d) })
			if row.Hovered() {
				row.Background(pal.Subtle)
			}
			row.Children(func() {
				clipped(c, d.Names.Plural).FontSize(12.5).FontWeight(500)
				clipped(c, d.Group).FontSize(12)
				clipped(c, strings.Join(versions, ", ")).Font(FontMono).FontSize(11.5)
				clipped(c, scope).FontSize(11.5)
			})
		}
	}, "", nil)
}

func (a *App) customResourcesView(c *ui.Context, cl *kube.Cluster) {
	s := &a.cluster
	list := cl.Resources
	columns := list.Custom.Version().Columns
	pal := Colors(c.Theme())
	tracks := []ui.Track{ui.Fr(2)}
	headers := []sortColumn{{locales.KubeClusterName(), "name"}}
	var indices []int
	for i := range columns {
		indices = append(indices, i)
	}
	slices.SortStableFunc(indices, func(i, j int) int { return cmp.Compare(columns[i].Priority, columns[j].Priority) })
	// In the split view, the full set of columns is available in the overview.
	maxColumns := 5
	if narrow(c) {
		maxColumns = 3
	}
	if s.other.uid != "" {
		maxColumns = 1
	}
	indices = indices[:min(len(indices), maxColumns)]
	if len(columns) == 0 {
		headers = append(headers, sortColumn{locales.KubeClusterAge(), "age"})
		tracks = append(tracks, ui.Fixed(66))
	}
	for _, i := range indices {
		headers = append(headers, sortColumn{columns[i].Name, "column-" + strconv.Itoa(i)})
		tracks = append(tracks, ui.Fr(1))
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		kubeActionButton(c.Key("custom-definitions"), "layers", locales.KubeCustomDefinitions(), controlHeight).
			OnClick(func() { a.selectClusterKind(kindCustom, nil) })
		if len(columns) > 0 && !narrow(c) {
			ui.Text(c, locales.KubeCustomColumns()).FontSize(11.5).TextColor(pal.TextFaint)
		}
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).
			Placeholder(locales.KubeCustomSearch()).Label(locales.KubeCustomSearch()).Height(controlHeight).Width(240).OnChange(s.other.reset)
	})
	var items []kube.Resource
	query := resourceQuery(s.query)
	for _, item := range list.Items {
		if strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strings.ToLower(item.Namespace), query) ||
			slices.ContainsFunc(item.Custom.Cells, func(value string) bool { return strings.Contains(strings.ToLower(value), query) }) {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b kube.Resource) int {
		if cmp := strings.Compare(a.Name, b.Name); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Namespace, b.Namespace)
	})
	sort := s.sortOf(kindCustom)
	by := map[string]func(x, y kube.Resource) int{
		"name": func(x, y kube.Resource) int { return cmp.Compare(x.Name, y.Name) },
		"age":  func(x, y kube.Resource) int { return y.Created.Compare(x.Created) },
	}
	for _, i := range indices {
		cell := func(r kube.Resource) string {
			if i < len(r.Custom.Cells) {
				return r.Custom.Cells[i]
			}
			return ""
		}
		by["column-"+strconv.Itoa(i)] = func(x, y kube.Resource) int { return cmp.Compare(cell(x), cell(y)) }
	}
	sortRows(*sort, items, by)
	if len(items) == 0 {
		message := locales.KubeResourcesEmpty()
		if !list.Custom.Namespaced() {
			message = locales.KubeResourcesEmptyCluster()
		}
		if query != "" {
			message = locales.SearchNoResults(s.query)
		}
		toolNote(c, message)
		return
	}
	selected := s.otherResource()
	uid := s.other.uid
	if selected == nil {
		uid = ""
	}
	workloadPanes(c, func() { sortHead(c, tracks, nil, sort, headers...) }, func() {
		for _, item := range items {
			row := resourceGrid(ui.ButtonBase(c.Key("custom-"+item.UID)), tracks).Label(item.Name).
				Padding(7, 16).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).OnClick(func() {
				s.toggleWorkload(&s.other.uid, item.UID)
				s.other.clearYAML()
				s.other.tab = 0
			})
			if item.UID == uid {
				row.Background(pal.ActiveRow)
			} else if row.Hovered() {
				row.Background(pal.Subtle)
			}
			s.revealWorkloadRow(row, item.UID)
			row.Children(func() {
				ui.Column(c).Gap(2).MinWidth(0).Children(func() {
					clipped(c, item.Name).Font(FontMono).FontSize(12).FontWeight(500)
					if item.Namespace != "" && len(a.clusterNamespaces(s.custom.context)) != 1 {
						clipped(c, item.Namespace).FontSize(10.5).TextColor(pal.TextFaint)
					}
				})
				if len(columns) == 0 {
					clipped(c, otherResourceAge(c, item)).FontSize(11.5).TextColor(pal.TextFaint)
				}
				for _, i := range indices {
					value := item.Custom.Cells[i]
					clipped(c, customColumnValue(c, columns[i], value)).FontSize(11.5).Tooltip(value)
				}
			})
		}
	}, uid, func() { a.customResourceDetail(c, cl, *selected) })
}

func customColumnValue(c *ui.Context, col kube.CustomColumn, value string) string {
	if value == "" {
		return "—"
	}
	if col.Type == "date" {
		if date, err := time.Parse(time.RFC3339, value); err == nil {
			return age(c, date)
		}
	}
	return value
}

func (a *App) customResourceDetail(c *ui.Context, cl *kube.Cluster, item kube.Resource) {
	v := &a.cluster.other
	custom := item.Custom
	def := custom.Definition
	events, read := clusterEvents(cl, item.UID)
	ready := slices.IndexFunc(custom.Conditions, func(condition kube.CustomCondition) bool { return condition.Type == "Ready" })
	pal := Colors(c.Theme())
	detailFrame(c, v.tab, false, func() {
		ui.Column(c).Padding(16).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon("boxes")).Size(16, 16)
				clipped(c, item.Name).FontSize(15).FontWeight(600).Grow(1)
				if ready >= 0 && custom.Conditions[ready].Status == "False" {
					aiTag(c, "Ready: False", pal.DangerSoft, pal.DangerText).Shrink(0)
				}
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(v.reset)
			})
			meta := []string{def.Names.Kind, def.Group + "/" + custom.Version}
			if item.Namespace != "" {
				meta = append(meta, item.Namespace)
			}
			if created := otherResourceAge(c, item); created != "" {
				meta = append(meta, locales.KubeCreatedAgo(created))
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
			count := ""
			if read && cl.Resources.EventsErr == nil {
				count = strconv.Itoa(len(events))
			}
			segments(c, "resource-tabs", &v.tab, []segment{
				{locales.KubeTabOverview(), ""}, {locales.KubeCustomConditions(), strconv.Itoa(len(custom.Conditions))},
				{locales.KubeTabEvents(), count}, {locales.KubeResourceYaml(), ""},
			})
		})
	}, func() {
		switch v.tab {
		case 1:
			if len(custom.Conditions) == 0 {
				toolNote(c, locales.KubeCustomNoConditions())
			}
			ui.Column(c).Gap(10).Padding(8, 16).Children(func() {
				for _, condition := range custom.Conditions {
					parts := []string{condition.Type + ": " + condition.Status}
					if condition.Reason != "" {
						parts = append(parts, condition.Reason)
					}
					if !condition.LastTransitionTime.IsZero() {
						parts = append(parts, age(c, condition.LastTransitionTime))
					}
					ui.Text(c, strings.Join(parts, " · ")).FontSize(12).FontWeight(600).Wrap()
					if condition.Message != "" {
						ui.Text(c, condition.Message).FontSize(12).Wrap().Selectable()
					}
				}
			})
		case 2:
			if read && cl.Resources.EventsErr != nil {
				warningRow(c, cl.Resources.EventsErr.Error())
			} else {
				eventList(c, events, read, 0)
			}
		case 3:
			otherResourceYAML(c, v)
		default:
			ui.Column(c).Padding(8, 16).Gap(10).Children(func() {
				if ready >= 0 {
					if condition := custom.Conditions[ready]; condition.Status == "False" && condition.Message != "" {
						message := condition.Message
						if condition.Reason != "" {
							message = condition.Reason + " · " + message
						}
						warningRow(c, message)
					}
				}
				for i, col := range def.Version().Columns {
					ui.Row(c).Gap(8).Children(func() {
						clipped(c, col.Name).Width(96).Shrink(0).FontSize(12).TextColor(c.Theme().TextMuted)
						ui.Text(c, customColumnValue(c, col, custom.Cells[i])).Tooltip(custom.Cells[i]).Grow(1).Font(FontMono).FontSize(12).Wrap().Selectable()
					})
				}
			})
		}
	})
}
