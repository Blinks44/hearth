package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) helmView(c *ui.Context, releases []kube.HelmRelease, version string) {
	s := &a.cluster
	var counts [3]int
	var shown []kube.HelmRelease
	query := strings.ToLower(strings.TrimSpace(s.query))
	for _, r := range releases {
		filters := [...]bool{true, r.Status == "failed", r.Pending()}
		for i, match := range filters {
			if match {
				counts[i]++
			}
		}
		if filters[s.helmFilter] && strings.Contains(strings.ToLower(r.Namespace+"/"+r.Name+" "+r.Chart), query) {
			shown = append(shown, r)
		}
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
		segments(c, "helm-filter", &s.helmFilter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[0])},
			{locales.KubeHelmFailed(), strconv.Itoa(counts[1])},
			{locales.KubeHelmPending(), strconv.Itoa(counts[2])},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(locales.KubeHelmSearch()).Height(controlHeight).Width(270).MinWidth(100)
	})
	if len(shown) == 0 && s.helmDetail == nil {
		empty := locales.KubeHelmNoMatches()
		if len(releases) == 0 {
			empty = locales.KubeHelmEmpty()
		}
		toolNote(c, empty)
		return
	}
	slices.SortStableFunc(shown, func(a, b kube.HelmRelease) int {
		if order := cmp.Compare(helmStatusOrder(a), helmStatusOrder(b)); order != 0 {
			return order
		}
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Namespace, b.Namespace))
	})
	sort := s.sortOf(kindHelm)
	sortRows(*sort, shown, map[string]func(x, y kube.HelmRelease) int{
		"name":     func(x, y kube.HelmRelease) int { return cmp.Compare(x.Name, y.Name) },
		"chart":    func(x, y kube.HelmRelease) int { return cmp.Compare(x.Chart, y.Chart) },
		"app":      func(x, y kube.HelmRelease) int { return cmp.Compare(x.AppVersion, y.AppVersion) },
		"revision": func(x, y kube.HelmRelease) int { return cmp.Compare(helmRevision(x), helmRevision(y)) },
		"status":   func(x, y kube.HelmRelease) int { return cmp.Compare(helmStatusOrder(x), helmStatusOrder(y)) },
	})
	detail := s.helmDetail
	selected := ""
	tracks := []ui.Track{ui.Fr(1.1), ui.Fr(1.15), ui.Fr(0.65), ui.Fixed(62), ui.Fixed(135)}
	headers := []sortColumn{{locales.KubeHelmRelease(), "name"}, {locales.KubeHelmChart(), "chart"}, {locales.KubeHelmAppVersion(), "app"},
		{locales.KubeHelmRevision(), "revision"}, {locales.KubeClusterStatus(), "status"}}
	if detail != nil {
		selected = helmKey(detail.release)
		tracks = []ui.Track{ui.Fr(1), ui.Fixed(132)}
		headers = []sortColumn{{locales.KubeHelmRelease(), "name"}, {locales.KubeClusterStatus(), "status"}}
	}
	workloadPanes(c, func() {
		sortHead(c, tracks, nil, sort, headers...)
	}, func() {
		for _, r := range shown {
			a.helmRow(c, tracks, r, selected)
		}
		if len(shown) == 0 {
			toolNote(c, locales.KubeHelmNoMatches())
		}
		ui.Text(c, locales.KubeHelmSource(strings.TrimSpace("Helm "+version))).FontSize(11.5).
			TextColor(c.Theme().TextMuted).Padding(8, 16)
	}, selected, func() { a.helmDetailsView(c, detail) })
}

func helmStatusOrder(r kube.HelmRelease) int {
	switch {
	case r.Status == "failed":
		return 0
	case r.Pending():
		return 1
	default:
		return 2
	}
}

func helmRevision(r kube.HelmRelease) int {
	n, _ := strconv.Atoi(r.Revision)
	return n
}

func helmKey(r kube.HelmRelease) string { return r.Namespace + "/" + r.Name }

func (a *App) helmRow(c *ui.Context, tracks []ui.Track, r kube.HelmRelease, selected string) {
	t := c.Theme()
	pal := Colors(t)
	key := helmKey(r)
	ui.Column(c.Key(key)).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		row := resourceGrid(ui.ButtonBase(c.Key("select-helm")), tracks).Label(key).Padding(8, 16).
			OnClick(func() {
				a.cluster.revealWorkload = key
				a.cluster.clearHelmDetails()
				if key != selected {
					a.selectHelm(r)
				}
			})
		switch {
		case key == selected:
			row.Background(pal.ActiveRow)
		case row.Hovered():
			row.Background(pal.Subtle)
		case r.Status == "failed":
			row.Background(pal.DangerSoft.Alpha(0.35))
		}
		a.resourceContextMenu(c, row, ops.KubeResource{Name: r.Name}, nil)
		a.cluster.revealWorkloadRow(row, key)
		row.Children(func() {
			batchName(c, r.Name, r.Namespace)
			if selected == "" {
				for _, value := range []string{r.Chart, r.AppVersion, r.Revision} {
					clipped(c, cmp.Or(value, "—")).Font(FontMono).FontSize(12).TextColor(t.TextMuted).Tooltip(value)
				}
			}
			ui.Column(c).MinWidth(0).AlignItems(ui.Start).Children(func() {
				helmStatus(c, r.Status)
			})
		})
		if selected == "" && r.Description != "" {
			clipped(c, strings.Join(strings.Fields(r.Description), " ")).FontSize(12).TextColor(pal.DangerText).
				Padding(0, 16, 8, 16).Tooltip(r.Description)
		}
	})
}

func helmStatus(c *ui.Context, status string) {
	t := c.Theme()
	pal := Colors(t)
	dot, bg, fg := pal.TextFaint, pal.Muted, t.TextMuted
	switch {
	case status == "failed":
		dot, bg, fg = t.Danger, pal.DangerSoft, pal.DangerText
	case strings.HasPrefix(status, "pending-"):
		dot, bg, fg = t.Warning, pal.WarningSoft, pal.WarningText
	case status == "deployed":
		dot, bg, fg = t.Success, pal.SuccessSoft, pal.SuccessText
	}
	chip(c, dot, bg, fg, cmp.Or(status, "—"))
}
