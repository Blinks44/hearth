package ui

import (
	"context"
	"maps"
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

const namespaceQuotaWarning = 0.9

func namespacePriority(item kube.Resource) int {
	switch {
	case item.Deleting:
		return 3
	case item.NamespaceInfo.Quota >= namespaceQuotaWarning:
		return 2
	case item.NamespaceInfo.Pods != nil && *item.NamespaceInfo.Pods == 0:
		return 1
	}
	return 0
}

func namespaceShows(filter int, item kube.Resource) bool {
	if item.NamespaceInfo == nil {
		return true
	}
	priority := namespacePriority(item)
	switch filter {
	case 1:
		return priority >= 2
	case 2:
		return item.Deleting
	case 3:
		return priority == 1
	}
	return true
}

func (a *App) namespaceFilters(c *ui.Context, items []kube.Resource) {
	var tabs []segment
	for i, label := range []string{locales.TabAll(), locales.KubePodsTrouble(), locales.KubeNamespaceRemoving(), locales.KubeNamespaceWithoutPods()} {
		count := 0
		for _, item := range items {
			if namespaceShows(i, item) {
				count++
			}
		}
		tabs = append(tabs, segment{label, strconv.Itoa(count)})
	}
	segments(c, "namespace-status-filter", &a.cluster.namespaceFilter, tabs)
}

func namespaceStatus(c *ui.Context, item kube.Resource) (string, ui.Color, ui.Color) {
	pal := Colors(c.Theme())
	ns := item.NamespaceInfo
	switch namespacePriority(item) {
	case 3:
		label := locales.KubeResourceTerminating()
		if !ns.DeletingAt.IsZero() {
			label += " · " + age(c, ns.DeletingAt)
		}
		return label, pal.DangerSoft, pal.DangerText
	case 2:
		return locales.KubeNamespaceQuota(int(ns.Quota * 100)), pal.WarningSoft, pal.WarningText
	case 1:
		return locales.KubeNamespaceWithoutPods(), pal.WarningSoft, pal.WarningText
	}
	if ns.Pods != nil {
		return locales.KubeNamespacePodCount(*ns.Pods), pal.SuccessSoft, pal.SuccessText
	}
	return ns.Phase, pal.Muted, c.Theme().TextMuted
}

func (a *App) namespaceHeaderActions(c *ui.Context, item kube.Resource, v *otherResourceView) {
	ref := a.resourceRef("Namespace", "", item.Name, item.UID)
	f := a.cluster.resources[ref]
	ui.Row(c).Gap(6).Wrap().Children(func() {
		kubeActionButton(c.Key("namespace-events"), "scroll-text", locales.KubeTabEvents(), podActionHeight).
			OnClick(func() { v.tab = 2 })
		command := run.TerminalCommand([]string{ref.Kubectl, "--context=" + ref.Context, "get", "namespace", item.Name})
		kubeActionButton(c.Key("namespace-copy-command"), "copy", locales.KubeShellCopy(), podActionHeight).
			OnClick(func() { c.WriteClipboard(command) })
		ui.Box(c).Grow(1)
		if a.BuildPlan != nil && ref.Kubectl != "" && ref.UID != "" && !item.Deleting {
			kubeActionButton(c.Key("namespace-delete"), "trash-2", locales.KubeResourceDelete(), podActionHeight).
				TextColor(c.Theme().Danger).
				Disabled(f != nil && f.busy || a.op.running()).
				OnClick(func() { a.planResourceChange(a.resourceForm(ref), true) })
		}
	})
}

func namespaceOverview(c *ui.Context, item kube.Resource, list *kube.ResourceList, read bool, tab *int) {
	ns := item.NamespaceInfo
	if item.Deleting {
		pal := Colors(c.Theme())
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Padding(10).Margin(8, 16).Radius(9).Background(pal.DangerSoft).Children(func() {
			ui.Icon(c, Icon("triangle-alert")).Size(16, 16).Shrink(0).TextColor(c.Theme().Danger)
			ui.Text(c, locales.KubeNamespaceTerminatingHelp()).Grow(1).MinWidth(140).FontSize(12).TextColor(pal.DangerText).Wrap()
			ui.Button(c.Key("namespace-show-members"), locales.KubeNamespaceContents()).Height(podActionHeight).
				OnClick(func() { *tab = 1 })
		})
	}
	detailSection(c, locales.KubeFacts())
	facts := [][2]string{{locales.KubeClusterStatus(), ns.Phase}}
	if ns.Pods != nil {
		facts = append(facts, [2]string{"Pods", strconv.Itoa(*ns.Pods)})
	}
	var labels []string
	for _, key := range slices.Sorted(maps.Keys(ns.Labels)) {
		labels = append(labels, key+"="+ns.Labels[key])
	}
	if len(labels) != 0 {
		facts = append(facts, [2]string{locales.KubeLabels(), strings.Join(labels, ", ")})
	}
	if len(ns.Finalizers) != 0 {
		facts = append(facts, [2]string{"Finalizers", strings.Join(ns.Finalizers, ", ")})
	}
	factRows(c, facts)
	if !item.Deleting {
		return
	}
	detailSection(c, locales.KubeNamespaceDeletionBlockers())
	for _, cond := range ns.Conditions {
		if cond.Status == "True" {
			detail := cond.Message
			if cond.Reason != "" {
				detail = strings.TrimSuffix(cond.Reason+" · "+detail, " · ")
			}
			ui.Column(c.Key(cond.Type)).Gap(3).Padding(7, 16).
				BorderWidth(0, 0, 1, 0).BorderColor(Colors(c.Theme()).Line).Children(func() {
				ui.Text(c, cond.Type).Font(FontMono).FontSize(12).Wrap().Selectable()
				ui.Text(c, detail).FontSize(11.5).TextColor(c.Theme().TextMuted).Wrap().Selectable()
			})
		}
	}
	namespaceMembersView(c, list, read, true)
}

func namespaceMembersView(c *ui.Context, list *kube.ResourceList, read, blockersOnly bool) {
	switch {
	case !read:
		toolNote(c, locales.KubeNamespaceReadingContents())
	case list.NamespaceMembersErr != nil:
		warningRow(c, locales.KubeNamespaceInventoryFailed()+" "+list.NamespaceMembersErr.Error())
	case len(list.NamespaceMembers) == 0 && !blockersOnly:
		toolNote(c, locales.KubeNamespaceEmpty())
	default:
		for _, member := range list.NamespaceMembers {
			if !blockersOnly || member.Finalizers != "" {
				nodeFact(c.Key("namespace-member-"+member.UID), member.Kind, strings.TrimSuffix(member.Name+" · "+member.Finalizers, " · "))
			}
		}
	}
}

type namespaceCreateForm struct {
	context, kubectl, name string
	err                    error
	open, busy             bool
}

func (a *App) namespaceCreate(c *ui.Context) {
	f := a.cluster.namespaceCreate
	if f == nil || a.cluster.kind != kindNamespaces || f.context != a.router.Query("context") {
		return
	}
	dialogBase(c.Key("namespace-create"), &f.open, func(_ ui.Element, panel ui.Element) {
		pal := Colors(c.Theme())
		panel.Width(420).Background(c.Theme().Background).Radius(12).Padding(20).Gap(12).Children(func() {
			ui.Text(c, locales.KubeNamespaceNew()).FontSize(17).FontWeight(600)
			ui.Text(c, f.context).Font(FontMono).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
			ui.TextInput(c.Key("new-namespace-name"), &f.name).Label(locales.KubeNamespaceLabel()).
				Placeholder(locales.KubeNamespaceLabel()).Disabled(f.busy).AutoFocus().
				OnSubmit(func() { a.planNamespaceCreate(f) })
			if f.name != "" && !ops.KubeNamespaceValid(f.name) {
				ui.Text(c, locales.KubeNamespaceInvalid()).FontSize(12).TextColor(pal.WarningText).Wrap()
			}
			if f.err != nil {
				ui.Text(c, f.err.Error()).FontSize(12).TextColor(pal.DangerText).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if f.busy {
					ui.Spinner(c).Size(14, 14)
				}
				ui.Button(c, locales.Cancel()).Height(controlHeight).OnClick(func() { f.open = false })
				ui.PrimaryButton(c.Key("new-namespace-review"), locales.KubeResourceCheck()).Height(controlHeight).
					Disabled(f.busy || !ops.KubeNamespaceValid(f.name)).OnClick(func() { a.planNamespaceCreate(f) })
			})
		})
	})
}

func (a *App) planNamespaceCreate(f *namespaceCreateForm) {
	if f.busy || !f.open || !ops.KubeNamespaceValid(f.name) || a.op.running() {
		return
	}
	f.busy, f.err = true, nil
	cli, contextName, name := f.kubectl, f.context, f.name
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanKubeNamespaceCreate(context.Background(), run.Exec{}, env, cli, contextName, name)
	}, func(p ops.Plan, err error) {
		f.busy, f.err = false, err
		s := &a.cluster
		if err != nil || s.namespaceCreate != f || !f.open || s.kind != kindNamespaces ||
			a.router.Path() != kubeClusterPage || a.router.Query("context") != contextName {
			return
		}
		f.open = false
		a.proposeThen(func() {
			if a.router.Query("context") == contextName {
				s.refresh++
			}
		})(p, nil)
	})
}
