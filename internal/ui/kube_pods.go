package ui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

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

// podGroups groups the pods by owner, groups in trouble first, then by
// name; a pod nothing owns is a group of its own. A sorted column orders
// the groups by their pods' totals, and the pods within them.
func podGroups(pods []kube.Pod, sort tableSort) []podGroup {
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
		sortRows(sort, groups[i].pods, podColumns)
	}
	slices.SortStableFunc(groups, func(x, y podGroup) int {
		if (x.trouble > 0) != (y.trouble > 0) {
			if x.trouble > 0 {
				return -1
			}
			return 1
		}
		return cmp.Compare(x.owner.Name+"/"+x.namespace, y.owner.Name+"/"+y.namespace)
	})
	sortRows(sort, groups, map[string]func(x, y podGroup) int{
		"name":     func(x, y podGroup) int { return cmp.Compare(x.owner.Name, y.owner.Name) },
		"status":   func(x, y podGroup) int { return cmp.Compare(y.trouble, x.trouble) },
		"ready":    func(x, y podGroup) int { return cmp.Compare(x.readyShare(), y.readyShare()) },
		"restarts": func(x, y podGroup) int { return cmp.Compare(x.total(podRestarts), y.total(podRestarts)) },
		"cpu":      func(x, y podGroup) int { return cmp.Compare(x.total(podCPU), y.total(podCPU)) },
		"memory":   func(x, y podGroup) int { return cmp.Compare(x.total(podMemory), y.total(podMemory)) },
	})
	return groups
}

// podColumns order pods by the columns of their table.
var podColumns = map[string]func(x, y kube.Pod) int{
	"name": func(x, y kube.Pod) int { return cmp.Compare(x.Name, y.Name) },
	"status": func(x, y kube.Pod) int {
		tx, ty := podTrouble(x), podTrouble(y)
		if tx != ty {
			if tx {
				return -1
			}
			return 1
		}
		return cmp.Compare(x.Reason+x.Phase, y.Reason+y.Phase)
	},
	"ready":    func(x, y kube.Pod) int { return cmp.Compare(podReadyShare(x), podReadyShare(y)) },
	"restarts": func(x, y kube.Pod) int { return cmp.Compare(podRestarts(x), podRestarts(y)) },
	"cpu":      func(x, y kube.Pod) int { return cmp.Compare(podCPU(x), podCPU(y)) },
	"memory":   func(x, y kube.Pod) int { return cmp.Compare(podMemory(x), podMemory(y)) },
}

func podRestarts(p kube.Pod) float64 { return float64(p.Restarts) }

// podCPU and podMemory are a pod's use, -1 without a sample: those sort
// below any that has one.
func podCPU(p kube.Pod) float64 {
	if p.Usage == nil {
		return -1
	}
	return p.Usage.CPU
}

func podMemory(p kube.Pod) float64 {
	if p.Usage == nil {
		return -1
	}
	return p.Usage.Memory
}

func podReadyShare(p kube.Pod) float64 {
	if len(p.Containers) == 0 {
		return 1
	}
	return float64(p.Ready) / float64(len(p.Containers))
}

func (g podGroup) total(value func(kube.Pod) float64) float64 {
	var sum float64
	for _, p := range g.pods {
		sum += max(value(p), 0)
	}
	return sum
}

func (g podGroup) readyShare() float64 {
	if len(g.pods) == 0 {
		return 1
	}
	return float64(len(g.pods)-g.trouble) / float64(len(g.pods))
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

// groupsForced reports whether every group shows open: while the list
// is searched or shows restarted or completed pods, which closed groups
// would hide.
func (a *App) groupsForced() bool {
	s := &a.cluster
	return s.query != "" || s.podFilter == podsRestarted || s.podFilter == podsDone
}

// groupOpen reports whether the group's pods show: groups show closed,
// those in trouble with their count in red, until opened.
func (a *App) groupOpen(g podGroup) bool {
	return a.groupsForced() || a.cluster.toggled[g.key()]
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
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(locales.KubePodsSearch()).Height(controlHeight).Width(270)
	})
	chosen := slices.IndexFunc(cl.Pods, func(p kube.Pod) bool { return p.UID == s.pod })
	if s.pod != "" && chosen < 0 {
		toolNote(c, locales.KubePodGone())
	}
	if len(cl.Pods) == 0 {
		toolNote(c, locales.KubeClusterNoPods())
		return
	}
	metricsNotice(c, cl.MetricsErr)
	sort := s.sortOf(kindPods)
	groups := podGroups(shown, *sort)
	var uid string
	var detail func()
	if chosen >= 0 {
		resource := cl.Pods[chosen]
		uid = resource.UID
		detail = func() { a.podDetail(c, cl, resource) }
	}
	compact := chosen >= 0
	columns := workloadTracks(c, compact)
	if compact && narrow(c) {
		columns[1] = ui.Fixed(92)
	}
	if !compact {
		// Reserve the actions inside the shared workload grid. The
		// name and status keep their alignment with Deployments.
		if narrow(c) {
			columns = []ui.Track{ui.Fr(1), ui.Fixed(106), ui.Fixed(42), ui.Fixed(72), ui.Fixed(72), ui.Fixed(84)}
		} else {
			columns = []ui.Track{ui.Fr(1), ui.Fixed(170), ui.Fixed(54), ui.Fixed(80), ui.Fixed(84), ui.Fixed(80)}
		}
	}
	tracks := append([]ui.Track{ui.Fixed(18)}, columns...)
	workloadPanes(c.Key("pods-panes"), func() {
		headers := []sortColumn{{locales.KubeClusterName(), "name"}, {locales.KubeClusterStatus(), "status"}}
		if !compact {
			headers = append(headers, sortColumn{locales.KubeClusterReady(), "ready"}, sortColumn{locales.KubeClusterRestarts(), "restarts"},
				sortColumn{"CPU", "cpu"}, sortColumn{locales.KubeMetricsMemory(), "memory"})
		}
		sortHead(c, append(tracks, ui.Fixed(podRowActionsWidth)), func() {
			s.bulk.checkbox(c, "pods-select-all", locales.KubeBulkSelectAll(), ids)
		}, sort, headers...)
	}, func() {
		for _, g := range groups {
			a.podGroupRows(c, cl, g, compact, tracks)
		}
		a.podFooter(c, groups, len(shown))
	}, uid, detail)
	a.kubeBulkBar(c, selected)
}

// podGroupRows are a group's header, and its pods while it is open; a
// lone pod nothing owns has no header. The header sits on the table's
// subtle tone and its pods hang under it from a guide line, so the two
// do not read as one.
func (a *App) podGroupRows(c *ui.Context, cl *kube.Cluster, g podGroup, compact bool, tracks []ui.Track) {
	if g.owner.Kind == "" {
		a.podRow(c, cl, g.pods[0], false, compact, tracks)
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
		head := resourceGrid(ui.ButtonBase(c.Key(key)), tracks[1:]).Grow(1).MinWidth(0).Padding(9, 0).
			Label(g.owner.Kind + " " + g.owner.Name).OnClick(func() { a.toggleGroup(g) })
		ref := a.podGroupRef(cl, g)
		var logs func()
		if ref.UID != "" && a.StreamLogs != nil && a.WatchCluster != nil {
			logs = func() { a.openOwnerLogs(ref) }
		}
		menu := a.ownerMenu(c, g, ref, logs)
		head.ContextMenu(menu)
		bg := t.Background.Alpha(0)
		switch {
		case head.Hovered():
			bg = pal.Muted
		case open:
			bg = pal.Subtle
		}
		line.Background(line.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
		head.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
				turn := float32(0)
				if open {
					turn = 90
				}
				chevron := ui.Box(c).Size(14, 14).Shrink(0).Center()
				chevron.Rotate(chevron.AnimateTo("turn", turn, ui.Animation{Duration: 160 * time.Millisecond})).Children(func() {
					ui.Icon(c, Icon("chevron-right")).Size(14, 14).TextColor(pal.TextFaint)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					metadata := g.namespace + " · " + locales.KubePodCount(len(g.pods))
					if narrow(c) {
						clipped(c, g.owner.Name).FontSize(13).FontWeight(600)
						clipped(c, g.owner.Kind+" · "+metadata).FontSize(11).TextColor(pal.TextFaint)
					} else {
						ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
							aiTag(c, g.owner.Kind, t.Background, t.TextMuted).Shrink(0).BorderWidth(1).BorderColor(pal.Line)
							clipped(c, g.owner.Name).FontSize(13).FontWeight(600).MinWidth(0)
						})
						if !compact {
							clipped(c, metadata).FontSize(11).TextColor(pal.TextFaint)
						}
					}
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
			if compact {
				return
			}
			// The group's totals, faint beside its pods' own values.
			ready := 0
			var usage *kube.Resources
			var limits kube.Resources
			sampled := true
			for _, p := range g.pods {
				if !podTrouble(p) {
					ready++
				}
				limits.CPU += p.Limits.CPU
				limits.Memory += p.Limits.Memory
				if p.Usage == nil {
					sampled = false
					continue
				}
				if usage == nil {
					usage = &kube.Resources{}
				}
				usage.CPU += p.Usage.CPU
				usage.Memory += p.Usage.Memory
			}
			if !sampled {
				usage = nil
			}
			// A share of the limits says something only when every pod has one.
			for _, p := range g.pods {
				if p.Limits.CPU == 0 {
					limits.CPU = 0
				}
				if p.Limits.Memory == 0 {
					limits.Memory = 0
				}
			}
			ui.Text(c, fmt.Sprintf("%d/%d", ready, len(g.pods))).Font(FontMono).FontSize(12).TextColor(t.TextMuted).SingleLine().
				Tooltip(locales.KubeGroupReady(ready, len(g.pods)))
			restarts := ui.Text(c, strconv.Itoa(int(g.total(podRestarts)))).Font(FontMono).FontSize(12).TextColor(t.TextMuted).SingleLine()
			if g.trouble > 0 && g.total(podRestarts) > 0 {
				restarts.TextColor(t.Danger)
			}
			podMetricCell(c, usage, limits, false)
			podMetricCell(c, usage, limits, true)
		})
		podRowActions(c.Key(key+"-actions"), g.owner.Name, locales.KubeOwnerLogsAll(), logs, menu)
	})
	if !open {
		return
	}
	// The pods unfold under their header as one block.
	ui.Column(c.Key(key + "-pods")).Transition(rowMotion).Children(func() {
		for _, p := range g.pods {
			a.podRow(c, cl, p, true, compact, tracks)
		}
	})
}

// podRow is a pod in the list: in full, its columns and what is going on
// with it; compact beside its details, its name and status. A pod of a
// group hangs from the group's guide line.
func (a *App) podRow(c *ui.Context, cl *kube.Cluster, p kube.Pod, owned, compact bool, tracks []ui.Track) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	label, tagBg, tagFg := podTag(c, p)
	story, trouble := storyOf(p, nil, c.Now())
	line := workloadSelectionRow(c.Key("pod-select-row-"+p.UID), pal.Line).MinHeight(44)
	line.Children(func() {
		var ids []string
		if p.UID != "" {
			ids = []string{p.UID}
		}
		ui.Column(c).Width(18).Shrink(0).AlignItems(ui.Start).Justify(ui.Center).Children(func() {
			s.bulk.checkbox(c, "pod-select", locales.KubeBulkSelect(p.Name), ids)
		})
		if owned {
			// The guide line from the group's chevron down its pods.
			ui.Box(c).Width(2).Shrink(0).Margin(0, 4, 0, 6).Background(pal.ControlBorder)
		}
		row := ui.ButtonBase(c.Key("pod-"+p.UID)).Column().Grow(1).MinWidth(0).Padding(6, 0).
			AlignItems(ui.Stretch).Justify(ui.Center).Label(p.Name).OnClick(func() { s.toggleWorkload(&s.pod, p.UID) })
		var logs func()
		if container := podLogContainer(p, story.crashed); a.StreamLogs != nil && container != "" {
			logs = func() { a.openLogs(p, container, false) }
		}
		menu := a.podMenu(c, cl, p, logs, story.crashed)
		row.ContextMenu(menu)
		s.revealWorkloadRow(row, p.UID)
		var bg ui.Color
		switch {
		case s.pod == p.UID:
			bg = pal.ActiveRow
		case row.Hovered():
			bg = pal.Subtle
		case trouble && story.waiting:
			bg = pal.WarningSoft.Alpha(0.35)
		case trouble:
			bg = pal.DangerSoft.Alpha(0.35)
		default:
			bg = t.Background.Alpha(0)
		}
		line.Background(line.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
		var issue *rowIssue
		if trouble {
			sev := issueDanger
			if story.waiting {
				sev = issueWarning
			}
			title, body := podIssueText(label, story)
			issue = &rowIssue{
				key:      "kube-pod:" + p.UID,
				label:    locales.Details() + " · " + p.Name,
				severity: sev,
				title:    title,
				body:     body,
			}
		}
		row.Children(func() {
			resourceGrid(ui.Box(c), tracks[1:]).Children(func() {
				ui.Column(c).MinWidth(0).Gap(1).Children(func() {
					name := clipped(c, p.Name).Font(FontMono).FontSize(12.5)
					if compact {
						name.TruncateStart()
					}
					if !owned && !compact {
						ui.Text(c, p.Namespace).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
					}
				})
				a.workloadStatusIssue(c, label, tagBg, tagFg, issue)
				if compact {
					return
				}
				ui.Text(c, fmt.Sprintf("%d/%d", p.Ready, len(p.Containers))).Font(FontMono).FontSize(12).SingleLine()
				restarts := ui.Text(c, strconv.Itoa(p.Restarts)).Font(FontMono).FontSize(12).SingleLine()
				if p.Restarts > 0 && trouble {
					restarts.TextColor(t.Danger)
				}
				podMetricCell(c, p.Usage, p.Limits, false)
				podMetricCell(c, p.Usage, p.Limits, true)
			})
		})
		podRowActions(c.Key("pod-actions-"+p.UID), p.Name, locales.KubeLogsOpen(), logs, menu)
	})
}

func podIssueText(status string, story podStory) (title, body string) {
	lines := make([]string, 0, len(story.facts)+len(story.guesses)+1)
	lines = append(lines, story.facts...)
	lines = append(lines, story.guesses...)
	if len(lines) == 0 && story.detail != "" {
		lines = append(lines, story.detail)
	}
	if len(lines) > 0 {
		return story.title, strings.Join(lines, "\n")
	}
	return status, story.title
}

// podFooter counts what the list shows and opens or closes every group.
const podRowActionsWidth = 52

func podRowActions(c *ui.Context, name, logLabel string, log func(), menu func(*ui.Menu)) {
	ui.Row(c).Width(podRowActionsWidth).Shrink(0).AlignSelf(ui.Center).Gap(4).Children(func() {
		if log != nil {
			iconButton(c, "scroll-text", logLabel+" · "+name).OnClick(log)
		} else {
			ui.Box(c).Width(24).Shrink(0)
		}
		iconButton(c, "ellipsis", locales.MoreActions()+" · "+name).Menu(menu)
	})
}

func (a *App) podFooter(c *ui.Context, groups []podGroup, pods int) {
	s := &a.cluster
	pal := Colors(c.Theme())
	owned := slices.DeleteFunc(slices.Clone(groups), func(g podGroup) bool { return g.owner.Kind == "" })
	allOpen := !slices.ContainsFunc(owned, func(g podGroup) bool { return !a.groupOpen(g) })
	ui.Row(c).Gap(10).Padding(8, 16).AlignItems(ui.Center).Background(pal.Subtle).Children(func() {
		ui.Text(c, locales.KubePodsFooter(len(owned), pods)).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1).MinWidth(0).SingleLine()
		if !narrow(c) && s.pod == "" {
			ui.Text(c, locales.KubeRowMenuHint()).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
		}
		if len(owned) == 0 || a.groupsForced() {
			return
		}
		label := locales.KubeOpenAll()
		if allOpen {
			label = locales.KubeCloseAll()
		}
		ui.Button(c.Key("groups-toggle"), label).OnClick(func() {
			s.toggled = map[string]bool{}
			if !allOpen {
				for _, g := range owned {
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
			meta := []string{"Pod", p.Namespace}
			if p.Node != "" {
				meta = append(meta, locales.KubeOnNode(p.Node))
			}
			meta = append(meta, locales.KubeCreatedAgo(age(c, p.Created)))
			ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(t.TextMuted).Wrap()
			if p.Owner.Kind != "" {
				ui.Box(c).Height(2)
				a.ownerChip(c, p.Namespace, p.Owner)
			}
		})
		if (a.StreamLogs != nil || a.OpenKubeShell != nil) && len(p.Containers) > 0 || a.RunForward != nil && p.Phase == "Running" || a.BuildPlan != nil {
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
				a.shellButton(c, p, story.crashed)
				a.resourceActions(c, ref, 2)
			})
		}
		ui.Row(c).Padding(0, 16, 12, 16).Children(func() {
			count := ""
			if read && cl.EventsErr == nil {
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
			workloadEventList(c, events, read, cl.EventsErr, 0)
			return
		}
		if trouble {
			var actions func()
			if story.crashed != "" {
				tail := kube.Tail{Namespace: p.Namespace, Pod: p.Name, Container: story.crashed, Previous: !story.ended}
				if cl.LastLineOf == tail && cl.LastLine != "" {
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
		detailSection(c, locales.KubeMetricsUsage())
		workloadMetrics(c, p.Usage, p.Requests, p.Limits)
		detailSection(c, locales.KubeContainers(len(p.Containers)))
		for _, ct := range p.Containers {
			containerRow(c, ct)
			containerMetrics(c, ct)
		}
		if len(p.InitContainers) > 0 {
			detailSection(c, locales.KubeMetricsInitContainers())
			for _, ct := range p.InitContainers {
				containerRow(c, ct)
				containerMetrics(c, ct)
			}
		}
		a.podEnv(c, p)
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
		a.resourceRelationships(c, p.References, nil)
		ui.Row(c).Padding(14, 16, 0, 16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			ui.Text(c, strings.ToUpper(locales.KubeRecentEvents())).FontSize(11).FontWeight(500).TextColor(pal.TextFaint).Grow(1)
			if read && len(events) > 3 {
				more := ui.ButtonBase(c.Key("all-events")).Label(locales.KubeAllEvents()).OnClick(func() { s.tab = 1 })
				more.Children(func() { ui.Text(c, locales.KubeAllEvents()+" ›").FontSize(12).TextColor(pal.AccentInk) })
			}
		})
		ui.Box(c).Height(6)
		workloadEventList(c, events, read, cl.EventsErr, 3)
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

func workloadEventList(c *ui.Context, events []kube.Event, read bool, err error, limit int) {
	if read && err != nil {
		ui.Column(c).Padding(4, 16).Children(func() { warningRow(c, err.Error()) })
		return
	}
	eventList(c, events, read, limit)
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
	reasonWidth, timeWidth := float32(112), float32(104)
	if narrow(c) {
		reasonWidth, timeWidth = 88, 92
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
				when = locales.KubeEventRepeated(e.Count, when)
			}
			clipped(c, when).FontSize(11.5).TextColor(pal.TextFaint).TextAlign(ui.End).Tooltip(e.Last.Local().Format(time.DateTime))
		})
	}
}
