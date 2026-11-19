package ui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

type clusterKind struct {
	label, icon, alias string
}

func clusterKindOptions() [kindConfigMaps + 1]clusterKind {
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
	}
}

func isBatchKind(kind int) bool { return kind == kindJobs || kind == kindCronJobs }

func clusterKindCounts(cl *kube.Cluster, kind int) (count, trouble int, known bool) {
	if cl == nil {
		return 0, 0, false
	}
	if cl.Resources != nil {
		return len(cl.Resources.Items), 0, cl.Resources.Kind == otherResourceKind(kind)
	}
	switch kind {
	case kindPods:
		for _, p := range cl.Pods {
			if podTrouble(p) {
				trouble++
			}
		}
		return len(cl.Pods), trouble, cl.HelmReleases == nil && cl.Secrets == nil && (!cl.Taken.IsZero() || cl.Pods != nil)
	case kindDeployments:
		for _, d := range cl.Deployments {
			if deploymentTrouble(d) {
				trouble++
			}
		}
		return len(cl.Deployments), trouble, cl.HelmReleases == nil && cl.Secrets == nil && (!cl.Taken.IsZero() || cl.Deployments != nil)
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
		s.kindQuery, s.kindSelected = "", 0
		s.contextOpen, s.namespaceOpen = false, false
	}
}

func (a *App) clusterKindPicker(c *ui.Context) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	options := clusterKindOptions()
	query := strings.ToLower(strings.TrimSpace(s.kindQuery))
	var exact, partial []int
	for _, i := range []int{kindOverview, kindPods, kindDeployments, kindStatefulSets, kindDaemonSets, kindReplicaSets,
		kindJobs, kindCronJobs, kindServices, kindIngresses, kindEndpoints, kindNetworkPolicies, kindSecrets, kindConfigMaps, kindHelm} {
		kind := options[i]
		label := strings.ToLower(kind.label)
		switch {
		case label == query || kind.alias == query:
			exact = append(exact, i)
		case strings.Contains(label, query) || strings.Contains(kind.alias, query):
			partial = append(partial, i)
		}
	}
	results := append(exact, partial...)
	selected := min(s.kindSelected, max(len(results)-1, 0))
	choose := func(i int) { s.kind, s.logs, s.kindsOpen = i, nil, false }
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
		c.OnShortcut(0, ui.KeyDown, func() { move(1) })
		c.OnShortcut(0, ui.KeyUp, func() { move(-1) })
		if len(results) == 0 {
			overviewMessage(c, locales.SearchNoResults(s.kindQuery))
		}
		ui.Scroll(c.Key("kind-results")).MaxHeight(435).Children(func() {
			for index, id := range results {
				kind := options[id]
				row := ui.ButtonBase(c.Key("pick-kind-"+kind.label)).Height(32).Radius(7).Padding(0, 10).Gap(9).AlignItems(ui.Center).
					Label(kind.label).OnClick(func() { choose(id) })
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
					if id == s.kind {
						fg = pal.AccentInk
					}
					ui.Icon(c, Icon(kind.icon)).Size(15, 15).TextColor(fg)
					ui.Text(c, kind.label).FontSize(13).FontWeight(500)
					if id != kindOverview {
						ui.Text(c, kind.alias).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
					}
					ui.Box(c).Grow(1)
					count, trouble, known := clusterKindCounts(s.data, id)
					if trouble > 0 {
						aiTag(c, strconv.Itoa(trouble), pal.DangerSoft, pal.DangerText)
					}
					if known && ((!s.batchData && s.data.Services == nil) || id != kindPods && id != kindDeployments) {
						ui.Text(c, strconv.Itoa(count)).FontSize(11).TextColor(pal.TextFaint)
					}
				})
			}
		})
	})
}
