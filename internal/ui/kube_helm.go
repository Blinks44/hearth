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
		sorts := []string{locales.KubeServicesProblemsFirst(), locales.KubeClusterName()}
		if s.helmSort == "" {
			s.helmSort = sorts[0]
		}
		ui.Select(c.Key("helm-sort"), &s.helmSort, sorts).Label(locales.KubeHelmSort()).Width(158)
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(locales.KubeHelmSearch()).Width(215).MinWidth(100)
	})
	if len(shown) == 0 {
		empty := locales.KubeHelmNoMatches()
		if len(releases) == 0 {
			empty = locales.KubeHelmEmpty()
		}
		toolNote(c, empty)
		return
	}
	slices.SortStableFunc(shown, func(a, b kube.HelmRelease) int {
		if s.helmSort != locales.KubeClusterName() {
			if order := cmp.Compare(helmStatusOrder(a), helmStatusOrder(b)); order != 0 {
				return order
			}
		}
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Namespace, b.Namespace))
	})
	t := c.Theme()
	pal := Colors(t)
	tracks := []ui.Track{ui.Fr(1.1), ui.Fr(1.15), ui.Fr(0.65), ui.Fixed(62), ui.Fixed(135)}
	ui.Column(c).MinHeight(0).Shrink(1).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		resourceHead(c, tracks, locales.KubeHelmRelease(), locales.KubeHelmChart(), locales.KubeHelmAppVersion(), locales.KubeHelmRevision(), locales.KubeClusterStatus())
		ui.Scroll(c.Key("helm-list")).MinHeight(0).Shrink(1).Children(func() {
			for _, r := range shown {
				helmRow(c, tracks, r)
			}
		})
	})
	label := strings.TrimSpace("Helm " + version)
	ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(9, 12).Radius(8).Background(pal.InfoSoft).Shrink(0).Children(func() {
		ui.Icon(c, Icon("info")).Size(14, 14).Shrink(0).TextColor(pal.Info)
		ui.Text(c, locales.KubeHelmReadOnly(label)).FontSize(11.5).TextColor(pal.InfoText).Wrap()
	})
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

func helmRow(c *ui.Context, tracks []ui.Track, r kube.HelmRelease) {
	t := c.Theme()
	pal := Colors(t)
	dot, bg, fg := pal.TextFaint, pal.Muted, t.TextMuted
	switch {
	case r.Status == "failed":
		dot, bg, fg = t.Danger, pal.DangerSoft, pal.DangerText
	case r.Pending():
		dot, bg, fg = t.Warning, pal.WarningSoft, pal.WarningText
	case r.Status == "deployed":
		dot, bg, fg = t.Success, pal.SuccessSoft, pal.SuccessText
	}
	row := ui.Column(c.Key(r.Namespace+"/"+r.Name)).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
	if r.Status == "failed" {
		row.Background(bg.Alpha(0.35))
	}
	row.Children(func() {
		resourceGrid(ui.Box(c), tracks).Padding(8, 16).Children(func() {
			batchName(c, r.Name, r.Namespace)
			for _, value := range []string{r.Chart, r.AppVersion, r.Revision} {
				clipped(c, cmp.Or(value, "—")).Font(FontMono).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Column(c).MinWidth(0).AlignItems(ui.Start).Children(func() {
				chip(c, dot, bg, fg, cmp.Or(r.Status, "—"))
			})
		})
		if r.Description != "" {
			clipped(c, strings.Join(strings.Fields(r.Description), " ")).FontSize(12).TextColor(fg).Padding(0, 16, 8, 16)
		}
	})
}
