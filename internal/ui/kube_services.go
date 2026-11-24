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

const (
	serviceAll = iota
	serviceNoPods
	serviceForwarded
	serviceExternal
)

func serviceHasNoPods(s kube.Service) bool { return s.PodsKnown && s.Pods == 0 }

func serviceNeedsAttention(s kube.Service) bool {
	return s.PodsKnown && (s.Pods == 0 || s.Ready < s.Pods)
}

func (a *App) serviceForwarded(s kube.Service, context string) bool {
	return slices.ContainsFunc(a.forwards, func(f *portForward) bool {
		t := f.target
		return f.pod.Name != "" && !f.stopping && !f.ended && t.Kind == "service" && t.Context == context && t.Namespace == s.Namespace && t.UID == s.UID
	})
}

func (a *App) servicesView(c *ui.Context, cl *kube.Cluster) {
	list := cl.Services
	s := &a.cluster
	context := a.router.Query("context")
	var counts [4]int
	var shown []kube.Service
	var selected kube.Service
	for _, service := range list {
		filters := [...]bool{true, serviceHasNoPods(service), a.serviceForwarded(service, context), service.External}
		for i, match := range filters {
			if match {
				counts[i]++
			}
		}
		if filters[s.serviceFilter] && strings.Contains(strings.ToLower(service.Namespace+"/"+service.Name), strings.ToLower(strings.TrimSpace(s.query))) {
			shown = append(shown, service)
			if service.UID == s.service {
				selected = service
			}
		}
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
		segments(c, "service-filter", &s.serviceFilter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[serviceAll])},
			{locales.KubeServicesNoPods(), strconv.Itoa(counts[serviceNoPods])},
			{locales.KubeServicesForwarded(), strconv.Itoa(counts[serviceForwarded])},
			{locales.KubeServicesExternal(), strconv.Itoa(counts[serviceExternal])},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(locales.KubeServicesSearch()).Height(controlHeight).Width(270).MinWidth(100)
	})
	if len(shown) == 0 {
		toolNote(c, locales.CleanupNone())
		return
	}
	slices.SortStableFunc(shown, func(a, b kube.Service) int {
		if serviceNeedsAttention(a) != serviceNeedsAttention(b) {
			if serviceNeedsAttention(a) {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Namespace, b.Namespace))
	})
	sort := s.sortOf(kindServices)
	sortRows(*sort, shown, map[string]func(x, y kube.Service) int{
		"name":  func(x, y kube.Service) int { return cmp.Compare(x.Name, y.Name) },
		"type":  func(x, y kube.Service) int { return cmp.Compare(x.Type, y.Type) },
		"ip":    func(x, y kube.Service) int { return cmp.Compare(x.ClusterIP, y.ClusterIP) },
		"ports": func(x, y kube.Service) int { return cmp.Compare(servicePortLabels(x), servicePortLabels(y)) },
		"pods": func(x, y kube.Service) int {
			return cmp.Or(cmp.Compare(serviceReadyShare(x), serviceReadyShare(y)), cmp.Compare(x.Pods, y.Pods))
		},
	})
	t := c.Theme()
	pal := Colors(t)
	tracks := []ui.Track{ui.Fr(1.4), ui.Fr(0.7), ui.Fr(1), ui.Fr(1.1), ui.Fr(1), ui.Fixed(150)}
	headers := []sortColumn{{locales.KubeClusterName(), "name"}, {locales.KubeServicesType(), "type"}, {"Cluster IP", "ip"},
		{locales.PortsTitle(), "ports"}, {locales.KubeServicesPods(), "pods"}, {"", ""}}
	compact := selected.UID != ""
	if compact {
		tracks = []ui.Track{ui.Fr(1.4), ui.Fr(1)}
		headers = []sortColumn{{locales.KubeClusterName(), "name"}, {locales.KubeServicesPods(), "pods"}}
	}
	dataTracks := tracks
	if !compact {
		dataTracks = tracks[:len(tracks)-1]
	}
	workloadPanes(c.Key("service-panes"), func() {
		sortHead(c, tracks, nil, sort, headers...)
	}, func() {
		for _, service := range shown {
			line := workloadSelectionRow(c.Key(service.UID), pal.Line)
			line.Children(func() {
				row := resourceGrid(ui.ButtonBase(c.Key("select-service")), dataTracks).
					Padding(8, 0).MinHeight(44).Grow(1).MinWidth(0).Label(service.Namespace + "/" + service.Name).
					OnClick(func() { s.toggleWorkload(&s.service, service.UID) })
				switch {
				case service.UID == selected.UID:
					line.Background(pal.ActiveRow)
				case row.Hovered():
					line.Background(pal.Subtle)
				case serviceNeedsAttention(service):
					line.Background(pal.DangerSoft.Alpha(0.4))
				}
				a.resourceContextMenu(c, row, a.resourceRef("Service", service.Namespace, service.Name, service.UID), nil)
				s.revealWorkloadRow(row, service.UID)
				row.Children(func() {
					ui.Column(c).MinWidth(0).Gap(1).Children(func() {
						clipped(c, service.Name).FontSize(13).FontWeight(500).Tooltip(service.Name)
						clipped(c, service.Namespace).FontSize(10.5).TextColor(pal.TextFaint)
					})
					if !compact {
						workloadStatus(c, service.Type, pal.Muted, t.TextMuted)
						clipped(c, cmp.Or(service.ClusterIP, "—")).Font(FontMono).FontSize(12).TextColor(t.TextMuted)
						clipped(c, cmp.Or(servicePortLabels(service), "—")).Font(FontMono).FontSize(12).TextColor(t.TextMuted)
					}
					text, color := serviceReadyLabel(service), t.TextMuted
					if serviceNeedsAttention(service) {
						color = pal.WarningText
					}
					clipped(c, text).FontSize(12).TextColor(color)
				})
				if !compact {
					ui.Row(c).Width(150).Shrink(0).AlignItems(ui.Center).Justify(ui.End).Children(func() {
						a.serviceActions(c, service)
					})
				}
			})
		}
	}, selected.UID, func() {
		a.serviceDetail(c, cl, selected)
	})
}

func (a *App) serviceDetail(c *ui.Context, cl *kube.Cluster, selected kube.Service) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	events, read := clusterEvents(cl, selected.UID)
	ref := a.resourceRef("Service", selected.Namespace, selected.Name, selected.UID)
	detailFrame(c, s.tab, s.tab == 2, func() {
		ui.Column(c).Padding(16, 16, 14, 16).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon("network")).Size(18, 18).Shrink(0).TextColor(t.TextMuted)
				clipped(c, selected.Name).FontSize(16).FontWeight(500).Shrink(1).MinWidth(0)
				if selected.Type != "" {
					aiTag(c, selected.Type, pal.Muted, t.TextMuted).Shrink(0)
				}
				ui.Box(c).Grow(1)
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(func() { s.toggleWorkload(&s.service, selected.UID) })
			})
			meta := []string{"Service", selected.Namespace}
			if !selected.Created.IsZero() {
				meta = append(meta, locales.KubeCreatedAgo(age(c, selected.Created)))
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		ports := serviceForwardPorts(selected)
		hasForward := len(ports) > 0
		hasManage := ref.Kubectl != "" && a.BuildPlan != nil
		if hasForward || hasManage {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Padding(0, 16, 12, 16).Children(func() {
				if hasForward {
					a.forwardButton(c, selected.UID, "service", selected.Namespace, selected.Name, ports, podActionHeight)
				}
				if hasManage {
					a.resourceActions(c, ref, 2)
				}
			})
		}
		ui.Row(c).Padding(0, 16, 12, 16).Children(func() {
			count := ""
			if read && cl.EventsErr == nil {
				count = strconv.Itoa(len(events))
			}
			tabs := []segment{{locales.KubeTabOverview(), ""}, {locales.KubeTabEvents(), count}}
			hasYAML := false
			if f := s.resources[ref]; f != nil && (f.editor != nil || s.tab == 2) {
				tabs = append(tabs, segment{locales.KubeResourceYaml(), ""})
				hasYAML = true
			}
			clampDetailTab(&s.tab, 2, hasYAML)
			segments(c, "service-tab", &s.tab, tabs)
		})
	}, func() {
		switch s.tab {
		case 2:
			if s.resources[ref] != nil {
				a.resourceEditor(c, s.resources[ref])
				return
			}
		case 1:
			workloadEventList(c, events, read, cl.EventsErr, 0)
			return
		}
		detailSection(c, locales.KubeFacts())
		var facts [][2]string
		add := func(label, value string) {
			if value != "" {
				facts = append(facts, [2]string{label, value})
			}
		}
		add(locales.KubeServicesType(), selected.Type)
		add("Cluster IP", selected.ClusterIP)
		if selected.Namespace != "" && selected.Name != "" {
			add("DNS", selected.Name+"."+selected.Namespace+".svc.cluster.local")
		}
		add("ExternalName", selected.ExternalName)
		add("LoadBalancer", strings.Join(selected.LoadBalancer, ", "))
		add(locales.KubeServicesExternalIPs(), strings.Join(selected.ExternalIPs, ", "))
		add(locales.PortsTitle(), serviceDetailPortLabels(selected))
		if selected.PodsKnown {
			add(locales.KubeServicesPods(), serviceReadyLabel(selected))
		}
		add(locales.KubeServicesSessionAffinity(), selected.SessionAffinity)
		var traffic []string
		if selected.ExternalTrafficPolicy != "" {
			traffic = append(traffic, locales.KubeServiceExternal(selected.ExternalTrafficPolicy))
		}
		if selected.InternalTrafficPolicy != "" {
			traffic = append(traffic, locales.KubeServiceInternal(selected.InternalTrafficPolicy))
		}
		add(locales.KubeServicesTrafficPolicy(), strings.Join(traffic, " · "))
		add(locales.KubeResourceSelector(), formatLabels(selected.Selector))
		add(locales.KubeLabels(), formatLabels(selected.Labels))
		factRows(c, facts)
		d := relationshipsFor(cl, selected.Namespace, selected.UID)
		a.resourceRelationships(c, d.Links, d.Errors)
	})
}

// serviceReadyShare is the share of the Service's pods ready, -1 when
// unknown and 0 without pods: those sort first.
func serviceReadyShare(service kube.Service) float64 {
	if !service.PodsKnown {
		return -1
	}
	if service.Pods == 0 {
		return 0
	}
	return float64(service.Ready) / float64(service.Pods)
}

func serviceReadyLabel(service kube.Service) string {
	if !service.PodsKnown {
		return "—"
	}
	if service.Pods == 0 {
		return locales.KubeServicesNoPods()
	}
	return locales.KubeServicesReady(strconv.Itoa(service.Ready), strconv.Itoa(service.Pods))
}

func servicePortLabels(service kube.Service) string {
	labels := make([]string, 0, len(service.Ports))
	for _, port := range service.Ports {
		labels = append(labels, port.String())
	}
	return strings.Join(labels, ", ")
}

func serviceDetailPortLabels(service kube.Service) string {
	labels := make([]string, 0, len(service.Ports))
	for _, port := range service.Ports {
		labels = append(labels, port.DetailString())
	}
	return strings.Join(labels, ", ")
}

func serviceForwardPorts(service kube.Service) []forwardPort {
	if !service.HasSelector {
		return nil
	}
	var ports []forwardPort
	for _, port := range service.Ports {
		if port.Protocol == "" || port.Protocol == "TCP" {
			ports = append(ports, forwardPort{port.Port, cmp.Or(parseForwardPort(port.Target), port.Port), port.String()})
		}
	}
	return ports
}

func (a *App) serviceActions(c *ui.Context, service kube.Service) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		if ports := serviceForwardPorts(service); len(ports) > 0 {
			a.forwardButton(c, service.UID, "service", service.Namespace, service.Name, ports, 24)
		}
		a.resourceMenu(c, a.resourceRef("Service", service.Namespace, service.Name, service.UID))
	})
}
