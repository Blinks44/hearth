package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func nodePriority(n *kube.Node) int {
	switch {
	case !n.Ready:
		return 3
	case n.Pressured():
		return 2
	case n.Cordoned:
		return 1
	}
	return 0
}

func nodeShows(filter int, n *kube.Node) bool {
	if n == nil {
		return true
	}
	switch filter {
	case 1:
		return !n.Ready
	case 2:
		return n.Cordoned
	case 3:
		return n.Pressured()
	}
	return true
}

func nodeFilters(c *ui.Context, s *kubeClusterState, items []kube.Resource) {
	labels := []string{locales.TabAll(), locales.KubePodsTrouble(), locales.KubeNodeCordoned(), locales.KubeNodePressure()}
	tabs := make([]segment, 0, len(labels))
	for i, label := range labels {
		count := 0
		for _, item := range items {
			if nodeShows(i, item.Node) {
				count++
			}
		}
		tabs = append(tabs, segment{label, strconv.Itoa(count)})
	}
	segments(c, "nodes-filter", &s.nodeFilter, tabs)
}

func nodeStatus(c *ui.Context, n *kube.Node) (string, ui.Color, ui.Color) {
	pal := Colors(c.Theme())
	switch nodePriority(n) {
	case 3:
		return locales.KubeNodeNotReady(), pal.DangerSoft, pal.DangerText
	case 2:
		return locales.KubeNodePressure(), pal.WarningSoft, pal.WarningText
	case 1:
		return locales.KubeNodeCordoned(), pal.WarningSoft, pal.WarningText
	}
	return locales.KubeNodeReady(), pal.SuccessSoft, pal.SuccessText
}

func nodeOverview(c *ui.Context, n *kube.Node, list *kube.ResourceList, read bool) {
	if index := slices.IndexFunc(n.Conditions, func(cond kube.NodeCondition) bool { return cond.Type == "Ready" && cond.Status != "True" }); index >= 0 {
		if message := n.Conditions[index].Message; message != "" {
			ui.Column(c).Padding(8, 16).Children(func() { warningRow(c, message) })
		}
	}
	detailSection(c, locales.KubeFacts())
	facts := [][2]string{
		{locales.KubeResourceAddresses(), strings.Join(n.Addresses, ", ")},
		{locales.KubeNodeKubelet(), n.Version},
	}
	if n.Cordoned {
		facts = append(facts, [2]string{locales.KubeClusterStatus(), locales.KubeNodeCordoned()})
	}
	if read && list.NodePodsErr == nil {
		count := strconv.Itoa(len(list.NodePods))
		if n.PodCapacity > 0 {
			count += " / " + strconv.Itoa(n.PodCapacity)
		}
		facts = append(facts, [2]string{"Pods", count})
		if n.Allocatable.CPU > 0 {
			facts = append(facts, [2]string{locales.KubeOverviewCPU(), locales.KubeOverviewCores(n.Requested.CPU, n.Allocatable.CPU)})
		}
		if n.Allocatable.Memory > 0 {
			facts = append(facts, [2]string{locales.KubeOverviewMemory(), locales.KubeOverviewGiB(n.Requested.Memory/(1<<30), n.Allocatable.Memory/(1<<30))})
		}
	}
	if n.Usage != nil {
		facts = append(facts, [2]string{locales.KubeMetricsUsage(), metricsCPU(n.Usage.CPU) + " · " + metricsMemory(n.Usage.Memory)})
	}
	factRows(c, facts)
	if list.NodePodsErr != nil {
		ui.Column(c).Padding(4, 16).Children(func() { warningRow(c, list.NodePodsErr.Error()) })
	}
	detailSection(c, locales.KubeNodeConditions())
	for _, cond := range n.Conditions {
		value := strings.Join(slices.DeleteFunc([]string{cond.Status, cond.Reason, cond.Message}, func(part string) bool { return part == "" }), " · ")
		nodeFact(c.Key(cond.Type), cond.Type, value)
	}
	for i, taint := range n.Taints {
		ui.Text(c.Key("taint-"+strconv.Itoa(i)), taint.String()).Font(FontMono).FontSize(12).Wrap().Selectable().
			Padding(7, 16).BorderWidth(0, 0, 1, 0).BorderColor(Colors(c.Theme()).Line)
	}
}

func nodeFact(c *ui.Context, label, detail string) {
	resourceGrid(ui.Row(c), []ui.Track{ui.Fixed(110), ui.Fr(1)}).
		Padding(7, 16).BorderWidth(0, 0, 1, 0).BorderColor(Colors(c.Theme()).Line).Children(func() {
		ui.Text(c, label).Font(FontMono).FontSize(12).Wrap()
		ui.Text(c, detail).FontSize(11.5).TextColor(c.Theme().TextMuted).MinWidth(0).Wrap().Selectable()
	})
}

func nodePodsView(c *ui.Context, list *kube.ResourceList, read bool) {
	switch {
	case !read:
		toolNote(c, locales.KubeNodeReadingPods())
	case list.NodePodsErr != nil:
		warningRow(c, list.NodePodsErr.Error())
	case len(list.NodePods) == 0:
		toolNote(c, locales.KubeNodeNoPods())
	default:
		tracks := []ui.Track{ui.Fr(1), ui.Fixed(76)}
		resourceHead(c, tracks, locales.KubeClusterName(), locales.KubeClusterStatus())
		for _, pod := range list.NodePods {
			resourceGrid(ui.Row(c.Key(pod.Namespace+"/"+pod.Name)), tracks).
				Padding(5, 16).BorderWidth(0, 0, 1, 0).BorderColor(Colors(c.Theme()).Line).Children(func() {
				ui.Column(c).MinWidth(0).Children(func() {
					clipped(c, pod.Name).Font(FontMono).FontSize(12)
					clipped(c, pod.Namespace).FontSize(11).TextColor(c.Theme().TextMuted)
				})
				clipped(c, pod.Phase).FontSize(11.5)
			})
		}
	}
}

func (a *App) nodeActions(c *ui.Context, item kube.Resource) {
	ref := a.resourceRef("Node", "", item.Name, item.UID)
	if a.BuildPlan == nil || ref.UID == "" || item.Deleting {
		return
	}
	f := a.cluster.resources[ref]
	busy := (f != nil && f.busy) || a.op.running()
	label, icon := locales.KubeNodeCordon(), "scroll-text"
	if item.Node.Cordoned {
		label, icon = locales.KubeNodeUncordon(), "play"
	}
	ui.Row(c).Gap(6).Wrap().Children(func() {
		kubeActionButton(c.Key("node-cordon"), icon, label, podActionHeight).Disabled(busy).OnClick(func() {
			a.planNodeAction(ref, func(ctx context.Context, r run.Runner, env sysenv.Env, ref ops.KubeResource) (ops.Plan, error) {
				return ops.PlanKubeCordon(ctx, r, env, ref, !item.Node.Cordoned)
			})
		})
		kubeActionButton(c.Key("node-drain"), "refresh-cw", locales.KubeNodeDrain(), podActionHeight).Disabled(busy).
			OnClick(func() { a.planNodeAction(ref, ops.PlanKubeDrain) })
	})
}

func (a *App) planNodeAction(ref ops.KubeResource, plan func(context.Context, run.Runner, sysenv.Env, ops.KubeResource) (ops.Plan, error)) {
	f := a.resourceForm(ref)
	if f.busy || a.op.running() {
		return
	}
	f.busy = true
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return plan(context.Background(), run.Exec{}, env, ref)
	}, func(p ops.Plan, err error) {
		f.busy = false
		s := &a.cluster
		if !a.resourceFormCurrent(f) || s.kind != kindNodes || s.other.uid != ref.UID {
			return
		}
		if err != nil {
			s.refresh++
		}
		a.proposeThen(func() {
			if a.resourceFormCurrent(f) {
				s.refresh++
			}
		})(p, err)
	})
}
