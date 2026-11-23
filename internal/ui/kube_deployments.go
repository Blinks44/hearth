package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

// The Deployment list's tabs.
const (
	deploymentsAll = iota
	deploymentsTrouble
	deploymentsRolling
	deploymentsStopped
)

// rolling reports a Deployment whose pods are not all of its current
// revision yet.
func rolling(d kube.Deployment) bool {
	return !d.Paused && d.Replicas > 0 && (d.Updated < d.Replicas || d.Pods > d.Replicas)
}

func deploymentShows(tab int, d kube.Deployment) bool {
	switch tab {
	case deploymentsTrouble:
		return deploymentTrouble(d)
	case deploymentsRolling:
		return rolling(d)
	case deploymentsStopped:
		return d.Replicas == 0
	}
	return true
}

// deploymentTag is a Deployment's status as its tag says it, and the
// tag's colors.
func deploymentTag(c *ui.Context, d kube.Deployment) (label string, bg, fg ui.Color) {
	t := c.Theme()
	pal := Colors(t)
	switch {
	case d.Replicas == 0:
		return locales.KubeClusterScaledDown(), pal.Muted, t.TextMuted
	case d.Paused:
		return locales.KubePaused(), pal.WarningSoft, pal.WarningText
	case rolling(d) && d.Stalled == "":
		return locales.KubeRollingOut(d.Updated, d.Replicas), pal.WarningSoft, pal.WarningText
	case deploymentTrouble(d):
		return locales.KubeGroupReady(d.Ready, d.Replicas), pal.DangerSoft, pal.DangerText
	}
	return locales.KubeGroupReady(d.Ready, d.Replicas), pal.SuccessSoft, pal.SuccessText
}

// deploymentsView is the Deployments tab: the list, and the chosen one's
// details beside it.
func (a *App) deploymentsView(c *ui.Context, cl *kube.Cluster) {
	s := &a.cluster
	counts := make([]int, 4)
	var shown []kube.Deployment
	var ids []string
	var selected []kubeBulkTarget
	query := strings.ToLower(strings.TrimSpace(s.query))
	for _, d := range cl.Deployments {
		if s.bulk.selected[d.UID] {
			selected = append(selected, kubeBulkTarget{a.resourceRef("Deployment", d.Namespace, d.Name, d.UID), !d.Paused})
		}
		for tab := range counts {
			if deploymentShows(tab, d) {
				counts[tab]++
			}
		}
		if deploymentShows(s.deploymentFilter, d) && strings.Contains(strings.ToLower(d.Namespace+"/"+d.Name), query) {
			shown = append(shown, d)
			if d.UID != "" {
				ids = append(ids, d.UID)
			}
		}
	}
	slices.SortStableFunc(shown, func(x, y kube.Deployment) int {
		if tx, ty := deploymentTrouble(x), deploymentTrouble(y); tx != ty {
			if tx {
				return -1
			}
			return 1
		}
		return strings.Compare(x.Namespace+"/"+x.Name, y.Namespace+"/"+y.Name)
	})
	sort := s.sortOf(kindDeployments)
	sortRows(*sort, shown, map[string]func(x, y kube.Deployment) int{
		"name":   func(x, y kube.Deployment) int { return cmp.Compare(x.Name, y.Name) },
		"status": func(x, y kube.Deployment) int { return cmp.Compare(deploymentReadyShare(x), deploymentReadyShare(y)) },
		"ready":  func(x, y kube.Deployment) int { return cmp.Compare(x.Ready, y.Ready) },
		"updated": func(x, y kube.Deployment) int {
			return cmp.Compare(x.Updated, y.Updated)
		},
		"image": func(x, y kube.Deployment) int { return cmp.Compare(imageTag(x.Images), imageTag(y.Images)) },
		"age":   func(x, y kube.Deployment) int { return y.Created.Compare(x.Created) },
	})
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		segments(c, "deployment-filter", &s.deploymentFilter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[deploymentsAll])},
			{locales.KubePodsTrouble(), strconv.Itoa(counts[deploymentsTrouble])},
			{locales.KubeDeploymentsRolling(), strconv.Itoa(counts[deploymentsRolling])},
			{locales.KubeDeploymentsStopped(), strconv.Itoa(counts[deploymentsStopped])},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(locales.KubeDeploymentsSearch()).Height(controlHeight).Width(270)
	})
	chosen := slices.IndexFunc(cl.Deployments, func(d kube.Deployment) bool { return d.UID == s.deployment })
	if s.deployment != "" && chosen < 0 {
		toolNote(c, locales.KubeDeploymentGone())
	}
	if len(cl.Deployments) == 0 {
		toolNote(c, locales.KubeClusterNoDeployments())
		return
	}
	var uid string
	var detail func()
	if chosen >= 0 {
		resource := cl.Deployments[chosen]
		uid = resource.UID
		detail = func() { a.deploymentDetail(c, cl, resource) }
	}
	compact := chosen >= 0
	columns := workloadTracks(c, compact)
	if !compact {
		if narrow(c) {
			columns = []ui.Track{ui.Fr(1), ui.Fixed(106), ui.Fixed(46), ui.Fixed(64), ui.Fixed(110), ui.Fixed(50)}
		} else {
			columns = []ui.Track{ui.Fr(1), ui.Fixed(170), ui.Fixed(54), ui.Fixed(74), ui.Fixed(110), ui.Fixed(60)}
		}
	}
	_, hasKubectl := cliCopy(a.cliState(), "kubectl")
	tracks := append([]ui.Track{ui.Fixed(18)}, columns...)
	workloadPanes(c.Key("deployments-panes"), func() {
		headers := []sortColumn{{locales.KubeClusterName(), "name"}, {"", ""}}
		if !compact {
			headers = []sortColumn{
				{locales.KubeClusterName(), "name"}, {locales.KubeClusterStatus(), "status"},
				{locales.KubeClusterReady(), "ready"}, {locales.KubeClusterUpdated(), "updated"},
				{locales.KubeImage(), "image"}, {locales.KubeClusterAge(), "age"},
			}
		}
		sortHead(c, append(tracks, ui.Fixed(podRowActionsWidth(compact))), func() {
			if hasKubectl {
				s.bulk.checkbox(c, "deployments-select-all", locales.KubeBulkSelectAll(), ids)
			}
		}, sort, headers...)
	}, func() {
		for _, d := range shown {
			a.deploymentRow(c, d, compact, tracks)
		}
	}, uid, detail)
	a.kubeBulkBar(c, selected)
}

func (a *App) deploymentRow(c *ui.Context, d kube.Deployment, compact bool, tracks []ui.Track) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	label, bg, fg := deploymentTag(c, d)
	_, hasKubectl := cliCopy(a.cliState(), "kubectl")
	line := workloadSelectionRow(c.Key("deployment-select-row-"+d.UID), pal.Line)
	line.Children(func() {
		var ids []string
		if d.UID != "" {
			ids = []string{d.UID}
		}
		ui.Column(c).Width(18).Shrink(0).AlignItems(ui.Start).Justify(ui.Center).Children(func() {
			if hasKubectl {
				s.bulk.checkbox(c, "deployment-select", locales.KubeBulkSelect(d.Name), ids)
			}
		})
		row := resourceGrid(ui.ButtonBase(c.Key("deployment-"+d.UID)), tracks[1:]).
			Padding(8, 0).MinHeight(40).Grow(1).MinWidth(0).Label(d.Name).
			OnClick(func() { s.toggleWorkload(&s.deployment, d.UID) })
		ref := a.resourceRef("Deployment", d.Namespace, d.Name, d.UID)
		var logs func()
		if ref.Kubectl != "" && a.StreamLogs != nil && a.WatchCluster != nil {
			logs = func() { a.openOwnerLogs(ref) }
		}
		menu := func(m *ui.Menu) {
			if logs != nil && m.Item(locales.KubeOwnerLogsAll()).Chosen() {
				logs()
			}
			if m.Item(locales.KubeCopyName()).Chosen() {
				copyName(c, ref.Name)
			}
			a.resourceMenuItems(m, ref)
		}
		row.ContextMenu(menu)
		s.revealWorkloadRow(row, d.UID)
		switch {
		case s.deployment == d.UID:
			line.Background(pal.ActiveRow)
		case row.Hovered():
			line.Background(pal.Subtle)
		}
		row.Children(func() {
			ui.Column(c).MinWidth(0).Gap(1).Children(func() {
				clipped(c, d.Name).FontSize(13).Tooltip(d.Name)
				if !compact {
					ui.Text(c, d.Namespace).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
				}
			})
			if compact {
				a.compactStatus(c, label, bg, fg, nil)
				return
			}
			workloadStatus(c, label, bg, fg)
			ui.Text(c, fmt.Sprintf("%d/%d", d.Ready, d.Replicas)).Font(FontMono).FontSize(12).SingleLine()
			ui.Text(c, strconv.Itoa(d.Updated)).Font(FontMono).FontSize(12).SingleLine()
			clipped(c, imageTag(d.Images)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).Tooltip(strings.Join(d.Images, "\n"))
			ui.Text(c, age(c, d.Created)).FontSize(12).TextColor(t.TextMuted).SingleLine()
		})
		podRowActions(c.Key("deployment-actions-"+d.UID), d.Name, locales.KubeOwnerLogsAll(), compact, logs, menu)
	})
}

// deploymentDetail is the chosen Deployment: what keeps it from being
// ready, its replicas and revisions, its pods and events.
func (a *App) deploymentDetail(c *ui.Context, cl *kube.Cluster, d kube.Deployment) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	events, read := clusterEvents(cl, d.UID)
	var pods []kube.Pod
	for _, p := range cl.Pods {
		if p.Namespace == d.Namespace && p.Owner == (kube.Owner{Kind: "Deployment", Name: d.Name}) {
			pods = append(pods, p)
		}
	}
	slices.SortFunc(pods, troubleFirst)
	label, bg, fg := deploymentTag(c, d)
	ref := a.resourceRef("Deployment", d.Namespace, d.Name, d.UID)
	detailFrame(c, s.tab, s.tab == 3, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon("boxes")).Size(18, 18).Shrink(0).TextColor(t.TextMuted)
				clipped(c, d.Name).FontSize(16).FontWeight(500).Shrink(1).MinWidth(0)
				aiTag(c, label, bg, fg).Shrink(0)
				ui.Box(c).Grow(1)
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(func() { s.toggleWorkload(&s.deployment, d.UID) })
			})
			meta := []string{"Deployment", d.Namespace}
			meta = append(meta, d.Images...)
			if d.Revision > 0 {
				meta = append(meta, locales.KubeRevision(d.Revision))
			}
			meta = append(meta, locales.KubeCreatedAgo(age(c, d.Created)))
			ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		if ref.Kubectl != "" && (a.BuildPlan != nil || a.StreamLogs != nil && a.WatchCluster != nil) {
			ui.Row(c).Gap(8).Wrap().Padding(0, 16, 12, 16).Children(func() {
				a.ownerLogsButton(c, ref)
				if a.BuildPlan != nil {
					a.deploymentRestart(c, ref, d.Paused)
				}
				a.resourceActions(c, ref, 3)
			})
		}
		ui.Row(c).Padding(0, 16, 12, 16).Children(func() {
			count := ""
			if read && cl.EventsErr == nil {
				count = strconv.Itoa(len(events))
			}
			tabs := []segment{{locales.KubeTabOverview(), ""}, {"Pods", strconv.Itoa(len(pods))}, {locales.KubeTabEvents(), count}}
			if f := s.resources[ref]; f != nil && (f.editor != nil || s.tab == 3) {
				tabs = append(tabs, segment{locales.KubeResourceYaml(), ""})
			}
			segments(c, "deployment-tab", &s.tab, tabs)
		})
	}, func() {
		switch s.tab {
		case 3:
			a.resourceEditor(c, s.resources[ref])
			return
		case 1:
			for _, p := range pods {
				a.ownedPodRow(c, p)
			}
			if len(pods) == 0 {
				ui.Text(c, locales.KubeNoOwnedPods()).FontSize(12).TextColor(t.TextMuted).Padding(4, 16)
			}
			return
		case 2:
			workloadEventList(c, events, read, cl.EventsErr, 0)
			return
		}
		if d.Stalled != "" {
			storyCard(c, podStory{title: locales.KubeStalled(), detail: d.Stalled, known: true}, nil)
		}
		trouble := slices.DeleteFunc(slices.Clone(pods), func(p kube.Pod) bool { return !podTrouble(p) })
		if len(trouble) > 0 {
			ui.Column(c).Gap(4).Margin(0, 16, 6, 16).Padding(10, 14).Radius(10).Background(pal.DangerSoft).Children(func() {
				ui.Text(c, locales.KubePodsNotReady(len(trouble), len(pods))).FontSize(13).FontWeight(500).TextColor(pal.DangerText)
				for _, p := range trouble[:min(len(trouble), 3)] {
					story, _ := storyOf(p, nil, c.Now())
					link := ui.ButtonBase(c.Key("trouble-" + p.UID)).AlignSelf(ui.Start).Label(p.Name).OnClick(func() { a.openPod(p) })
					link.Children(func() {
						ui.Text(c, p.Name+": "+story.line()).FontSize(12).TextColor(t.Text).Wrap()
					})
				}
			})
		}
		a.deploymentScale(c, ref, d)
		a.deploymentPause(c, ref, d.Paused)
		a.deploymentRevisions(c, ref, d)
		detailSection(c, locales.KubeReplicas())
		facts := [][2]string{
			{locales.KubeWanted(), strconv.Itoa(d.Replicas)},
			{locales.KubeReadyCount(), strconv.Itoa(d.Ready)},
			{locales.KubeUpdatedCount(), strconv.Itoa(d.Updated)},
			{locales.KubeAvailableCount(), strconv.Itoa(d.Available)},
		}
		if d.Pods != d.Replicas {
			facts = append(facts, [2]string{locales.KubeAllPods(), strconv.Itoa(d.Pods)})
		}
		facts = append(facts, [2]string{locales.KubeStrategy(), d.Strategy})
		factRows(c, facts)
		relations := relationshipsFor(cl, d.Namespace, d.UID)
		a.resourceRelationships(c, relations.Links, relations.Errors)
	})
}

// ownedPodRow is a Deployment's pod in its details, opening the pod's.
func (a *App) ownedPodRow(c *ui.Context, p kube.Pod) {
	pal := Colors(c.Theme())
	label, bg, fg := podTag(c, p)
	row := ui.ButtonBase(c.Key("owned-"+p.UID)).Gap(10).Padding(7, 16).AlignItems(ui.Center).Label(p.Name).OnClick(func() { a.openPod(p) })
	if row.Hovered() {
		row.Background(pal.Subtle)
	}
	row.Children(func() {
		clipped(c, p.Name).Font(FontMono).FontSize(12.5).Grow(1).MinWidth(0)
		ui.Text(c, fmt.Sprintf("%d/%d", p.Ready, len(p.Containers))).Font(FontMono).FontSize(12).Shrink(0)
		aiTag(c, label, bg, fg).Shrink(0)
	})
}

// openPod shows the pod's details on the Pods tab, its group open and its
// row in view.
func (a *App) openPod(p kube.Pod) {
	s := &a.cluster
	s.kind, s.query, s.podFilter, s.resourceLink = kindPods, "", podsAll, resourceLink{}
	s.pod, s.tab, s.revealWorkload = p.UID, 0, p.UID
	if p.Owner.Kind != "" {
		if s.toggled == nil {
			s.toggled = map[string]bool{}
		}
		s.toggled[podGroup{namespace: p.Namespace, owner: p.Owner}.key()] = true
	}
}

func deploymentReadyShare(d kube.Deployment) float64 {
	if d.Replicas == 0 {
		return 1
	}
	return float64(d.Ready) / float64(d.Replicas)
}

// imageTag is the Deployment's image as its column shows it: the last
// part of the first image's path, with its tag, and how many more.
func imageTag(images []string) string {
	if len(images) == 0 {
		return "—"
	}
	tag := images[0]
	if i := strings.LastIndex(tag, "/"); i >= 0 {
		tag = tag[i+1:]
	}
	if len(images) > 1 {
		tag += " +" + strconv.Itoa(len(images)-1)
	}
	return tag
}
