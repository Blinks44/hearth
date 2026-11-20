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
	sortOptions := []string{locales.KubeServicesProblemsFirst(), locales.KubeClusterName()}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
		segments(c, "service-filter", &s.serviceFilter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[serviceAll])},
			{locales.KubeServicesNoPods(), strconv.Itoa(counts[serviceNoPods])},
			{locales.KubeServicesForwarded(), strconv.Itoa(counts[serviceForwarded])},
			{locales.KubeServicesExternal(), strconv.Itoa(counts[serviceExternal])},
		})
		ui.Box(c).Grow(1)
		sort := sortOptions[0]
		if s.serviceSort == "name" {
			sort = sortOptions[1]
		}
		ui.Select(c.Key("service-sort"), &sort, sortOptions).Label(locales.KubeServicesSort()).Width(158).OnChange(func() {
			if sort == sortOptions[1] {
				s.serviceSort = "name"
			} else {
				s.serviceSort = ""
			}
		})
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(locales.KubeServicesSearch()).Width(215).MinWidth(100)
	})
	if len(shown) == 0 {
		toolNote(c, locales.CleanupNone())
		return
	}
	slices.SortStableFunc(shown, func(a, b kube.Service) int {
		if s.serviceSort != "name" && serviceNeedsAttention(a) != serviceNeedsAttention(b) {
			if serviceNeedsAttention(a) {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Namespace, b.Namespace))
	})
	t := c.Theme()
	pal := Colors(t)
	tracks := []ui.Track{ui.Fr(1.4), ui.Fr(0.7), ui.Fr(1), ui.Fr(1.1), ui.Fr(1), ui.Fixed(150)}
	headers := []string{locales.KubeClusterName(), locales.KubeServicesType(), "Cluster IP", locales.PortsTitle(), locales.KubeServicesPods(), ""}
	compact := selected.UID != ""
	if compact {
		tracks = []ui.Track{ui.Fr(1.4), ui.Fr(1)}
		headers = []string{locales.KubeClusterName(), locales.KubeServicesPods()}
	}
	workloadPanes(c.Key("service-panes"), func() {
		resourceHead(c, tracks, headers...)
	}, func() {
		for _, service := range shown {
			row := resourceGrid(ui.Box(c.Key(service.UID)), tracks).Padding(8, 16).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
			switch {
			case service.UID == selected.UID:
				row.Background(pal.AccentSoft)
			case serviceNeedsAttention(service):
				row.Background(pal.DangerSoft.Alpha(0.4))
			}
			row.Children(func() {
				button := ui.ButtonBase(c.Key("select-service")).MinWidth(0).Label(service.Namespace + "/" + service.Name).
					OnClick(func() { s.toggleWorkload(&s.service, service.UID) })
				button.Children(func() {
					ui.Column(c).MinWidth(0).Gap(1).Children(func() {
						clipped(c, service.Name).FontSize(13).FontWeight(500)
						clipped(c, service.Namespace).FontSize(10.5).TextColor(pal.TextFaint)
					})
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
				if !compact {
					a.serviceActions(c, service)
				}
			})
			s.revealWorkloadRow(row, service.UID)
		}
	}, selected.UID, func() {
		detailFrame(c, 0, false, func() {
			ui.Column(c).Padding(16).Gap(8).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, Icon("network")).Size(16, 16).Shrink(0)
					clipped(c, selected.Name).FontSize(15).FontWeight(600).Grow(1).MinWidth(0)
					iconButton(c, "x", locales.KubeCloseDetails()).OnClick(func() { s.toggleWorkload(&s.service, selected.UID) })
				})
				ui.Text(c, strings.Join([]string{"Service", selected.Namespace, selected.Type}, " · ")).
					FontSize(12).TextColor(t.TextMuted).Wrap()
				a.serviceActions(c, selected)
			})
		}, func() {
			detailSection(c, locales.KubeFacts())
			factRows(c, [][2]string{
				{"Cluster IP", selected.ClusterIP},
				{locales.PortsTitle(), servicePortLabels(selected)},
				{locales.KubeServicesPods(), serviceReadyLabel(selected)},
			})
			d := relationshipsFor(cl, selected.Namespace, selected.UID)
			a.resourceRelationships(c, d.Links, d.Errors)
		})
	})
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

func (a *App) serviceActions(c *ui.Context, service kube.Service) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		if service.HasSelector {
			var ports []forwardPort
			for _, port := range service.Ports {
				if port.Protocol == "TCP" {
					ports = append(ports, forwardPort{port.Port, cmp.Or(parseForwardPort(port.Target), port.Port), port.String()})
				}
			}
			if len(ports) > 0 {
				a.forwardButton(c, service.UID, "service", service.Namespace, service.Name, ports)
			}
		}
		a.resourceMenu(c, a.resourceRef("Service", service.Namespace, service.Name, service.UID))
	})
}
