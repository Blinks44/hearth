package ui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

// The pod list's tabs.
const (
	podsAll = iota
	podsTrouble
	podsRestarted
	podsDone
)

// podShows reports whether the pod list's tab shows p.
func podShows(tab int, p kube.Pod) bool {
	switch tab {
	case podsTrouble:
		return podTrouble(p)
	case podsRestarted:
		return p.Restarts > 0
	case podsDone:
		return p.Phase == "Succeeded" || p.Phase == "Failed"
	}
	return true
}

// matches reports whether a pod matches every word of the query: node:x
// its node, key=value a label, any other word part of its name, its
// owner's or its namespace.
func matches(query string, p kube.Pod) bool {
	for word := range strings.FieldsSeq(strings.ToLower(query)) {
		if node, ok := strings.CutPrefix(word, "node:"); ok {
			if !strings.Contains(strings.ToLower(p.Node), node) {
				return false
			}
			continue
		}
		if k, v, ok := strings.Cut(word, "="); ok {
			found := false
			for key, value := range p.Labels {
				found = found || strings.ToLower(key) == k && strings.ToLower(value) == v
			}
			if !found {
				return false
			}
			continue
		}
		if !strings.Contains(strings.ToLower(p.Namespace+"/"+p.Name+" "+p.Owner.Name), word) {
			return false
		}
	}
	return true
}

// podGroup is the pods of one owner, those in trouble first.
type podGroup struct {
	owner     kube.Owner
	namespace string
	pods      []kube.Pod
	trouble   int
}

func (g podGroup) key() string { return g.namespace + "/" + g.owner.Kind + "/" + g.owner.Name }

// podGroups groups the pods by owner, groups in trouble first; a pod
// nothing owns is a group of its own.
func podGroups(pods []kube.Pod) []podGroup {
	var groups []podGroup
	index := map[string]int{}
	for _, p := range pods {
		g := podGroup{owner: p.Owner, namespace: p.Namespace}
		if g.owner == (kube.Owner{}) {
			g.owner = kube.Owner{Name: p.Name}
		}
		i, ok := index[g.key()]
		if !ok {
			i = len(groups)
			index[g.key()] = i
			groups = append(groups, g)
		}
		groups[i].pods = append(groups[i].pods, p)
		if podTrouble(p) {
			groups[i].trouble++
		}
	}
	for i := range groups {
		slices.SortFunc(groups[i].pods, troubleFirst)
	}
	slices.SortStableFunc(groups, func(x, y podGroup) int {
		if (x.trouble > 0) != (y.trouble > 0) {
			if x.trouble > 0 {
				return -1
			}
			return 1
		}
		return cmp.Compare(x.key(), y.key())
	})
	return groups
}

func troubleFirst(x, y kube.Pod) int {
	if tx, ty := podTrouble(x), podTrouble(y); tx != ty {
		if tx {
			return -1
		}
		return 1
	}
	return strings.Compare(x.Name, y.Name)
}

// groupOpen reports whether the group's pods show: a group in trouble
// shows open, others closed, unless toggled; all show open while the
// list is searched or shows restarted or completed pods, which healthy
// groups would hide.
func (a *App) groupOpen(g podGroup) bool {
	s := &a.cluster
	open := g.trouble > 0 || s.query != "" || s.podFilter == podsRestarted || s.podFilter == podsDone
	return open != s.toggled[g.key()]
}

func (a *App) toggleGroup(g podGroup) {
	s := &a.cluster
	if s.toggled == nil {
		s.toggled = map[string]bool{}
	}
	s.toggled[g.key()] = !s.toggled[g.key()]
}

// podTag is a pod's status as its tag says it, and the tag's colors.
func podTag(c *ui.Context, p kube.Pod) (label string, bg, fg ui.Color) {
	pal := Colors(c.Theme())
	label = p.Phase
	if p.Reason != "" {
		label = p.Reason
	}
	switch {
	case p.Phase == "Pending" && (p.Reason == "" || p.Reason == "Unschedulable"):
		return label, pal.WarningSoft, pal.WarningText
	case podTrouble(p):
		return label, pal.DangerSoft, pal.DangerText
	case p.Phase == "Succeeded":
		return label, pal.Muted, c.Theme().TextMuted
	}
	return label, pal.SuccessSoft, pal.SuccessText
}

// podsView is the Pods tab: the list by owner, and the chosen pod's
// details beside it.
func (a *App) podsView(c *ui.Context, cl *kube.Cluster) {
	s := &a.cluster
	counts := make([]int, 4)
	var shown []kube.Pod
	var ids []string
	var selected []kubeBulkTarget
	for _, p := range cl.Pods {
		if s.bulk.selected[p.UID] {
			restartable := p.Phase != "Succeeded" && p.Phase != "Failed"
			switch p.Owner.Kind {
			case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet", "ReplicationController":
			default:
				restartable = false
			}
			selected = append(selected, kubeBulkTarget{a.resourceRef("Pod", p.Namespace, p.Name, p.UID), restartable})
		}
		for tab := range counts {
			if podShows(tab, p) {
				counts[tab]++
			}
		}
		if podShows(s.podFilter, p) && matches(s.query, p) {
			shown = append(shown, p)
			if p.UID != "" {
				ids = append(ids, p.UID)
			}
		}
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		segments(c, "pod-filter", &s.podFilter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[podsAll])},
			{locales.KubePodsTrouble(), strconv.Itoa(counts[podsTrouble])},
			{locales.KubePodsRestarted(), strconv.Itoa(counts[podsRestarted])},
			{locales.KubePodsDone(), strconv.Itoa(counts[podsDone])},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(locales.KubePodsSearch()).Width(270)
	})
	chosen := slices.IndexFunc(cl.Pods, func(p kube.Pod) bool { return p.UID == s.pod })
	if s.pod != "" && chosen < 0 {
		toolNote(c, locales.KubePodGone())
	}
	if len(cl.Pods) == 0 {
		toolNote(c, locales.KubeClusterNoPods())
		return
	}
	groups := podGroups(shown)
	var uid string
	var detail func()
	if chosen >= 0 {
		resource := cl.Pods[chosen]
		uid = resource.UID
		detail = func() { a.podDetail(c, cl, resource) }
	}
	compact := chosen >= 0
	tracks := append([]ui.Track{ui.Fixed(18)}, workloadTracks(c, compact)...)
	workloadPanes(c.Key("pods-panes"), func() {
		headers := []string{locales.KubeClusterName(), locales.KubeClusterStatus()}
		if !compact {
			headers = append(headers, locales.KubeClusterReady(), locales.KubeClusterRestarts(), locales.KubeClusterNode(), locales.KubeClusterAge())
		}
		workloadHead(c, tracks, func() {
			s.bulk.checkbox(c, "pods-select-all", locales.KubeBulkSelectAll(), ids)
		}, headers...)
	}, func() {
		for _, g := range groups {
			a.podGroupRows(c, g, compact, tracks)
		}
		a.podFooter(c, groups, len(shown))
	}, uid, detail)
	a.kubeBulkBar(c, selected)
}

// podGroupRows are a group's header, and its pods while it is open; a
// lone pod nothing owns has no header.
func (a *App) podGroupRows(c *ui.Context, g podGroup, compact bool, tracks []ui.Track) {
	if g.owner.Kind == "" {
		a.podRow(c, g.pods[0], false, compact, tracks)
		return
	}
	t := c.Theme()
	pal := Colors(t)
	open := a.groupOpen(g)
	key := "group-" + g.key()
	line := workloadSelectionRow(c.Key(key+"-select-row"), pal.Line)
	line.Children(func() {
		var ids []string
		for _, p := range g.pods {
			if p.UID != "" {
				ids = append(ids, p.UID)
			}
		}
		ui.Column(c).Width(18).Shrink(0).AlignItems(ui.Start).Justify(ui.Center).Children(func() {
			a.cluster.bulk.checkbox(c, "group-select", locales.KubeBulkSelect(g.owner.Name), ids)
		})
		head := resourceGrid(ui.ButtonBase(c.Key(key)), tracks[1:]).Grow(1).MinWidth(0).Padding(8, 0).
			Label(g.owner.Name).OnClick(func() { a.toggleGroup(g) })
		if head.Hovered() {
			line.Background(pal.Muted)
		} else if open {
			line.Background(pal.Subtle)
		}
		head.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
				chevron := "chevron-right"
				if open {
					chevron = "chevron-down"
				}
				ui.Icon(c, Icon(chevron)).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					clipped(c, g.owner.Name).FontSize(13).FontWeight(600)
					meta := g.owner.Kind
					if !compact {
						meta += " · " + g.namespace + " · " + locales.KubePodCount(len(g.pods))
					}
					clipped(c, meta).FontSize(10.5).TextColor(pal.TextFaint)
				})
			})
			switch {
			case g.trouble > 0:
				workloadStatus(c, locales.KubeGroupNotReady(g.trouble, len(g.pods)), pal.DangerSoft, pal.DangerText)
			case g.owner.Kind == "Job" && !slices.ContainsFunc(g.pods, func(p kube.Pod) bool { return p.Phase != "Succeeded" }):
				workloadStatus(c, locales.KubeGroupDone(), pal.Muted, t.TextMuted)
			default:
				workloadStatus(c, locales.KubeGroupReady(len(g.pods), len(g.pods)), pal.SuccessSoft, pal.SuccessText)
			}
		})
	})
	if !open {
		return
	}
	for _, p := range g.pods {
		a.podRow(c, p, true, compact, tracks)
	}
}

// podRow is a pod in the list: in full, its columns and what is going on
// with it; compact beside its details, its name and status.
func (a *App) podRow(c *ui.Context, p kube.Pod, owned, compact bool, tracks []ui.Track) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	label, bg, fg := podTag(c, p)
	story, trouble := storyOf(p, nil, c.Now())
	line := workloadSelectionRow(c.Key("pod-select-row-"+p.UID), pal.Line)
	line.Children(func() {
		var ids []string
		if p.UID != "" {
			ids = []string{p.UID}
		}
		ui.Column(c).Width(18).Shrink(0).AlignItems(ui.Start).Padding(8, 0, 0, 0).Children(func() {
			s.bulk.checkbox(c, "pod-select", locales.KubeBulkSelect(p.Name), ids)
		})
		row := ui.ButtonBase(c.Key("pod-"+p.UID)).Column().Grow(1).MinWidth(0).Gap(5).Padding(8, 0).
			AlignItems(ui.Stretch).Label(p.Name).OnClick(func() { s.toggleWorkload(&s.pod, p.UID) })
		s.revealWorkloadRow(row, p.UID)
		switch {
		case s.pod == p.UID:
			line.Background(pal.ActiveRow)
		case trouble && story.waiting:
			line.Background(pal.WarningSoft.Alpha(0.4))
		case trouble:
			line.Background(pal.DangerSoft.Alpha(0.4))
		case row.Hovered():
			line.Background(pal.Subtle)
		}
		row.Children(func() {
			resourceGrid(ui.Box(c), tracks[1:]).Children(func() {
				cell := ui.Column(c).MinWidth(0).Gap(1)
				if owned {
					cell.Padding(0, 0, 0, 22)
				}
				cell.Children(func() {
					name := clipped(c, p.Name).Font(FontMono).FontSize(12.5)
					if compact {
						name.TruncateStart()
					}
					if !owned && !compact {
						ui.Text(c, p.Namespace).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
					}
				})
				workloadStatus(c, label, bg, fg)
				if compact {
					return
				}
				ui.Text(c, fmt.Sprintf("%d/%d", p.Ready, len(p.Containers))).Font(FontMono).FontSize(12).SingleLine()
				restarts := ui.Text(c, strconv.Itoa(p.Restarts)).Font(FontMono).FontSize(12).SingleLine()
				if p.Restarts > 0 && trouble {
					restarts.TextColor(t.Danger)
				}
				node := p.Node
				if node == "" {
					node = "–"
				}
				clipped(c, node).Font(FontMono).FontSize(12)
				ui.Text(c, age(c, p.Created)).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
			if trouble && !compact {
				ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
					icon, color := "circle-alert", pal.DangerText
					if story.waiting {
						icon, color = "triangle-alert", pal.WarningText
					}
					ui.Icon(c, Icon(icon)).Size(13, 13).Shrink(0).TextColor(color).Margin(1, 0, 0, 0)
					clipped(c, story.line()).FontSize(12).TextColor(color).Grow(1).MinWidth(0)
				})
			}
		})
	})
}

// podFooter counts what the list shows and opens or closes every group.
func (a *App) podFooter(c *ui.Context, groups []podGroup, pods int) {
	s := &a.cluster
	pal := Colors(c.Theme())
	owned := slices.DeleteFunc(slices.Clone(groups), func(g podGroup) bool { return g.owner.Kind == "" })
	allOpen := !slices.ContainsFunc(owned, func(g podGroup) bool { return !a.groupOpen(g) })
	ui.Row(c).Gap(10).Padding(8, 16).AlignItems(ui.Center).Background(pal.Subtle).Children(func() {
		ui.Text(c, locales.KubePodsFooter(len(owned), pods)).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1).MinWidth(0).SingleLine()
		if len(owned) == 0 {
			return
		}
		label := locales.KubeOpenAll()
		if allOpen {
			label = locales.KubeCloseAll()
		}
		ui.Button(c.Key("groups-toggle"), label).OnClick(func() {
			s.toggled = map[string]bool{}
			for _, g := range owned {
				if a.groupOpen(g) == allOpen {
					s.toggled[g.key()] = true
				}
			}
		})
	})
}

// clusterEvents are the events of the object whose details show, when
// they were read for it.
func clusterEvents(cl *kube.Cluster, uid string) ([]kube.Event, bool) {
	if uid == "" || cl.EventsOf.UID != uid {
		return nil, false
	}
	return cl.Events, true
}

// podDetail is the chosen pod: what is going on, its containers, facts
// and events.
func (a *App) podDetail(c *ui.Context, cl *kube.Cluster, p kube.Pod) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	events, read := clusterEvents(cl, p.UID)
	story, trouble := storyOf(p, events, c.Now())
	label, bg, fg := podTag(c, p)
	ref := a.resourceRef("Pod", p.Namespace, p.Name, p.UID)
	detailFrame(c, s.tab, s.tab == 2, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if trouble {
					ui.Icon(c, Icon("circle-x")).Size(18, 18).Shrink(0).TextColor(fg)
				}
				clipped(c, p.Name).Font(FontMono).FontSize(15).FontWeight(500).Shrink(1).MinWidth(0)
				aiTag(c, label, bg, fg).Shrink(0)
				ui.Box(c).Grow(1)
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(func() { s.toggleWorkload(&s.pod, p.UID) })
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Wrap().Children(func() {
				meta := []string{"Pod", p.Namespace}
				if p.Node != "" {
					meta = append(meta, locales.KubeOnNode(p.Node))
				}
				meta = append(meta, locales.KubeCreatedAgo(age(c, p.Created)))
				ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(t.TextMuted)
				if p.Owner.Kind != "" {
					ui.Text(c, "·").FontSize(12).TextColor(t.TextMuted)
					owner := locales.KubeOwner(p.Owner.Kind, p.Owner.Name)
					d := slices.IndexFunc(cl.Deployments, func(d kube.Deployment) bool {
						return p.Owner.Kind == "Deployment" && d.Namespace == p.Namespace && d.Name == p.Owner.Name
					})
					if d < 0 {
						ui.Text(c, owner).FontSize(12).TextColor(t.TextMuted)
						return
					}
					uid := cl.Deployments[d].UID
					link := ui.ButtonBase(c.Key("pod-owner")).Label(owner).OnClick(func() {
						s.kind, s.deployment, s.tab = kindDeployments, uid, 0
					})
					link.Children(func() {
						ui.Text(c, owner+" ›").FontSize(12).TextColor(pal.AccentInk)
					})
				}
			})
		})
		if a.StreamLogs != nil && len(p.Containers) > 0 || a.RunForward != nil && p.Phase == "Running" || a.BuildPlan != nil {
			ui.Row(c).Gap(8).Wrap().Padding(0, 16, 12, 16).Children(func() {
				if a.StreamLogs != nil && len(p.Containers) > 0 {
					container := p.Containers[0].Name
					if story.crashed != "" {
						container = story.crashed
					}
					kubeActionButton(c.Key("pod-logs"), "scroll-text", locales.KubeLogsOpen(), podActionHeight).
						OnClick(func() { a.openLogs(p, container, false) })
				}
				if a.RunForward != nil && p.Phase == "Running" {
					var ports []forwardPort
					for _, port := range p.TCPPorts {
						ports = append(ports, forwardPort{port, port, strconv.Itoa(port) + "/TCP"})
					}
					a.forwardButton(c, p.UID, "pod", p.Namespace, p.Name, ports)
				}
				a.resourceActions(c, ref, 2)
			})
		}
		ui.Row(c).Padding(0, 16, 12, 16).Children(func() {
			count := ""
			if read {
				count = strconv.Itoa(len(events))
			}
			tabs := []segment{{locales.KubeTabOverview(), ""}, {locales.KubeTabEvents(), count}}
			if f := s.resources[ref]; f != nil && (f.editor != nil || s.tab == 2) {
				tabs = append(tabs, segment{locales.KubeResourceYaml(), ""})
			}
			segments(c, "pod-tab", &s.tab, tabs)
		})
	}, func() {
		if s.tab == 2 {
			a.resourceEditor(c, s.resources[ref])
			return
		}
		if s.tab == 1 {
			eventList(c, events, read, 0)
			return
		}
		if trouble {
			var actions func()
			if story.crashed != "" {
				tail := cl.LastLineOf
				if tail.Pod == p.Name && tail.Container == story.crashed && tail.Previous == !story.ended && cl.LastLine != "" {
					story.facts = append(story.facts, locales.KubeWhyLastLine(cl.LastLine))
				}
				label := locales.KubeLogsOpenPrevious()
				if story.ended {
					label = locales.KubeLogsOpenEnded()
				}
				actions = func() {
					ui.Button(c.Key("story-logs"), label).Background(t.Text).TextColor(t.Background).BorderColor(t.Text).
						OnClick(func() { a.openLogs(p, story.crashed, !story.ended) })
				}
			}
			storyCard(c, story, actions)
		}
		detailSection(c, locales.KubeContainers(len(p.Containers)))
		for _, ct := range p.Containers {
			containerRow(c, ct)
		}
		detailSection(c, locales.KubeFacts())
		facts := [][2]string{{locales.KubePodIP(), p.IP}, {"QoS", p.QoS}, {"Service account", p.Account}, {locales.KubeNode(), p.Node}}
		if len(p.Labels) > 0 {
			keys := slices.Sorted(maps.Keys(p.Labels))
			labels := keys[0] + "=" + p.Labels[keys[0]]
			if len(keys) > 1 {
				labels += " +" + strconv.Itoa(len(keys)-1)
			}
			facts = append(facts, [2]string{locales.KubeLabels(), labels})
		}
		if len(p.Volumes) > 0 {
			facts = append(facts, [2]string{locales.KubeVolumes(), strings.Join(p.Volumes, " · ")})
		}
		factRows(c, facts)
		ui.Row(c).Padding(14, 16, 0, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			ui.Text(c, strings.ToUpper(locales.KubeRecentEvents())).FontSize(11).FontWeight(500).TextColor(pal.TextFaint).Grow(1)
			if read && len(events) > 3 {
				more := ui.ButtonBase(c.Key("all-events")).Label(locales.KubeAllEvents()).OnClick(func() { s.tab = 1 })
				more.Children(func() { ui.Text(c, locales.KubeAllEvents()+" ›").FontSize(12).TextColor(pal.AccentInk) })
			}
		})
		ui.Box(c).Height(6)
		eventList(c, events, read, 3)
	})
}

// detailFrame keeps actions and tabs in reach while the tab's contents scroll.
func detailFrame(c *ui.Context, tab int, editorTab bool, header, body func()) {
	t := c.Theme()
	line := Colors(t).Divider
	ui.Column(c).Shrink(0).Radius(12, 12, 0, 0).Clip().Background(t.Background).
		BorderWidth(1, 1, 0, 1).BorderColor(line).Children(header)
	if editorTab {
		ui.Column(c).Grow(1).MinHeight(0).Radius(0, 0, 12, 12).Clip().Background(t.Background).
			BorderWidth(0, 1, 1, 1).BorderColor(line).Children(body)
		return
	}
	ui.Scroll(c.Key("detail-tab-" + strconv.Itoa(tab))).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Radius(0, 0, 12, 12).Clip().Background(t.Background).
			BorderWidth(0, 1, 1, 1).BorderColor(line).Padding(0, 0, 12, 0).Children(body)
	})
}

// storyCard says what is going on with a pod: Kubernetes' facts, what
// Hearth infers from them apart, and Kubernetes' own words; then the
// actions, when there are any.
func storyCard(c *ui.Context, s podStory, actions func()) {
	t := c.Theme()
	pal := Colors(t)
	bg, fg, icon := pal.DangerSoft, pal.DangerText, "circle-alert"
	if s.waiting {
		bg, fg, icon = pal.WarningSoft, pal.WarningText, "triangle-alert"
	}
	ui.Column(c).Gap(6).Margin(0, 16, 6, 16).Padding(12, 14).Radius(10).Background(bg).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, Icon(icon)).Size(15, 15).Shrink(0).TextColor(fg).Margin(1, 0, 0, 0)
			ui.Text(c, s.title).FontSize(13.5).FontWeight(500).TextColor(fg).Grow(1).MinWidth(0).Wrap()
		})
		for _, f := range s.facts {
			ui.Text(c, "• "+f).FontSize(12.5).TextColor(t.Text).Wrap().Padding(0, 0, 0, 23)
		}
		for _, g := range s.guesses {
			ui.Text(c, "• "+g).FontSize(12.5).TextColor(t.Text).Wrap().Padding(0, 0, 0, 23)
		}
		if len(s.guesses) > 0 {
			ui.Text(c, locales.KubeWhyGuessNote()).FontSize(11.5).TextColor(t.TextMuted).Wrap().Padding(0, 0, 0, 23)
		}
		if !s.known {
			ui.Text(c, locales.KubeWhyUnknownNote()).FontSize(12).TextColor(t.TextMuted).Wrap().Padding(0, 0, 0, 23)
		}
		if s.detail != "" {
			ui.Text(c, s.detail).Font(FontMono).FontSize(11).Padding(7, 10).Margin(2, 0, 0, 23).Radius(7).
				Background(t.Background.Alpha(0.6)).Wrap().Selectable()
		}
		if actions != nil {
			ui.Row(c).Gap(8).Padding(4, 0, 0, 23).Children(actions)
		}
	})
}

// containerRow is a container: whether it is ready, its image, restarts
// and state.
func containerRow(c *ui.Context, ct kube.Container) {
	t := c.Theme()
	pal := Colors(t)
	restartsWidth, stateWidth := float32(136), float32(164)
	if narrow(c) {
		restartsWidth, stateWidth = 88, 112
	}
	tracks := []ui.Track{ui.Fixed(7), ui.Fr(1), ui.Fixed(restartsWidth), ui.Fixed(stateWidth)}
	resourceGrid(ui.Box(c), tracks).Padding(7, 16).Children(func() {
		dot := t.Success
		if !ct.Ready && ct.Reason != "Completed" {
			dot = t.Danger
		}
		ui.Box(c).Size(7, 7).Radius(4).Shrink(0).Background(dot)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			clipped(c, ct.Name).FontSize(13).FontWeight(500)
			clipped(c, ct.Image).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
		})
		restarts := locales.KubeRestartCount(ct.Restarts)
		if ct.Last != nil {
			restarts += " · " + locales.KubeExitCode(ct.Last.ExitCode)
		}
		color := t.TextMuted
		if ct.Restarts > 0 && !ct.Ready {
			color = pal.DangerText
		}
		clipped(c, restarts).FontSize(12).TextColor(color)
		state := ct.Reason
		if state == "" {
			state = ct.State
		}
		switch {
		case state == "":
		case ct.Ready || state == "Completed":
			workloadStatus(c, state, pal.SuccessSoft, pal.SuccessText)
		case ct.State == "running":
			workloadStatus(c, state, pal.WarningSoft, pal.WarningText)
		default:
			workloadStatus(c, state, pal.DangerSoft, pal.DangerText)
		}
	})
}

// factRows are a resource's facts, a name and a value each, skipping
// those without a value.
func factRows(c *ui.Context, facts [][2]string) {
	t := c.Theme()
	for _, f := range facts {
		if f[1] == "" {
			continue
		}
		ui.Row(c).Gap(12).Padding(3, 16).Children(func() {
			ui.Text(c, f[0]).FontSize(12.5).TextColor(t.TextMuted).Width(130).Shrink(0).SingleLine()
			clipped(c, f[1]).Font(FontMono).FontSize(12).Grow(1).MinWidth(0).Selectable()
		})
	}
}

// eventList lists an object's events, newest first, at most limit of
// them unless limit is 0; read is false while they are being read.
func eventList(c *ui.Context, events []kube.Event, read bool, limit int) {
	t := c.Theme()
	pal := Colors(t)
	switch {
	case !read:
		ui.Row(c).Gap(8).Padding(4, 16).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, locales.KubeReadingEvents()).FontSize(12).TextColor(t.TextMuted)
		})
		return
	case len(events) == 0:
		ui.Text(c, locales.KubeNoEvents()).FontSize(12).TextColor(t.TextMuted).Padding(4, 16).Wrap()
		return
	}
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	reasonWidth, timeWidth := float32(112), float32(88)
	if narrow(c) {
		reasonWidth, timeWidth = 88, 72
	}
	tracks := []ui.Track{ui.Fixed(reasonWidth), ui.Fr(1), ui.Fixed(timeWidth)}
	for i, e := range events {
		row := resourceGrid(ui.Box(c), tracks).Padding(6, 16).AlignItems(ui.Start)
		if i < len(events)-1 {
			row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
		}
		row.Children(func() {
			if e.Type == "Warning" {
				workloadStatus(c, e.Reason, pal.WarningSoft, pal.WarningText)
			} else {
				workloadStatus(c, e.Reason, pal.Muted, t.TextMuted)
			}
			ui.Text(c, e.Message).FontSize(12.5).MinWidth(0).Wrap().Selectable()
			when := age(c, e.Last)
			if e.Count > 1 {
				when = "×" + strconv.Itoa(e.Count) + " · " + when
			}
			clipped(c, when).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).TextAlign(ui.End)
		})
	}
}
