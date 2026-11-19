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

func (a *App) servicesView(c *ui.Context, list []kube.Service) {
	s := &a.cluster
	context := a.router.Query("context")
	var counts [4]int
	var shown []kube.Service
	for _, service := range list {
		filters := [...]bool{true, serviceHasNoPods(service), a.serviceForwarded(service, context), service.External}
		for i, match := range filters {
			if match {
				counts[i]++
			}
		}
		if filters[s.serviceFilter] && strings.Contains(strings.ToLower(service.Namespace+"/"+service.Name), strings.ToLower(strings.TrimSpace(s.query))) {
			shown = append(shown, service)
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
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		resourceHead(c, tracks, locales.KubeClusterName(), locales.KubeServicesType(), "Cluster IP", locales.PortsTitle(), locales.KubeServicesPods(), "")
		for _, service := range shown {
			row := resourceGrid(ui.Box(c.Key(service.UID)), tracks).Padding(8, 16).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
			if serviceNeedsAttention(service) {
				row.Background(pal.DangerSoft.Alpha(0.4))
			}
			row.Children(func() {
				ui.Column(c).MinWidth(0).Gap(1).Children(func() {
					clipped(c, service.Name).FontSize(13).FontWeight(500)
					clipped(c, service.Namespace).FontSize(10.5).TextColor(pal.TextFaint)
				})
				workloadStatus(c, service.Type, pal.Muted, t.TextMuted)
				clipped(c, cmp.Or(service.ClusterIP, "—")).Font(FontMono).FontSize(12).TextColor(t.TextMuted)
				var labels []string
				var ports []forwardPort
				for _, p := range service.Ports {
					label := p.String()
					labels = append(labels, label)
					if p.Protocol == "TCP" {
						ports = append(ports, forwardPort{p.Port, cmp.Or(parseForwardPort(p.Target), p.Port), label})
					}
				}
				clipped(c, cmp.Or(strings.Join(labels, ", "), "—")).Font(FontMono).FontSize(12).TextColor(t.TextMuted)
				text, color := "—", t.TextMuted
				if service.PodsKnown {
					if service.Pods == 0 {
						text = locales.KubeServicesNoPods()
					} else {
						text = locales.KubeServicesReady(strconv.Itoa(service.Ready), strconv.Itoa(service.Pods))
					}
					if serviceNeedsAttention(service) {
						color = pal.WarningText
					}
				}
				clipped(c, text).FontSize(12).TextColor(color)
				ui.Row(c).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
					if service.HasSelector && len(ports) > 0 {
						a.forwardButton(c, service.UID, "service", service.Namespace, service.Name, ports)
					}
					a.resourceMenu(c, a.resourceRef("Service", service.Namespace, service.Name, service.UID))
				})
			})
		}
	})
}
