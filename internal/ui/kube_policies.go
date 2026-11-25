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

func (s *kubeClusterState) activeResourceList(cl *kube.Cluster) *kube.ResourceList {
	if s.kind == kindPolicies {
		if s.policyTab < len(cl.Policies) {
			return &cl.Policies[s.policyTab]
		}
		return nil
	}
	return cl.Resources
}

func policyKindName(kind kube.ResourceKind) string {
	switch kind {
	case kube.Autoscalers:
		return "HorizontalPodAutoscaler"
	case kube.DisruptionBudgets:
		return "PodDisruptionBudget"
	case kube.ResourceQuotas:
		return "ResourceQuota"
	case kube.LimitRanges:
		return "LimitRange"
	}
	return ""
}

func policyTabs(c *ui.Context, tab *int, lists []kube.ResourceList) {
	tabs := []segment{{"HPA", ""}, {"PDB", ""}, {locales.KubePolicyQuotas(), ""}, {"LimitRange", ""}}
	for i, list := range lists {
		if list.Err == nil {
			tabs[i].count = strconv.Itoa(len(list.Items))
		}
	}
	segments(c, "policy-tabs", tab, tabs)
}

func autoscalerCondition(h *kube.Autoscaler, kind string) kube.CustomCondition {
	for _, condition := range h.Conditions {
		if condition.Type == kind {
			return condition
		}
	}
	return kube.CustomCondition{}
}

func autoscalerProblem(h *kube.Autoscaler) (string, kube.CustomCondition) {
	if c := autoscalerCondition(h, "AbleToScale"); c.Status == "False" {
		return locales.KubeAutoscalerInactive(), c
	}
	if c := autoscalerCondition(h, "ScalingActive"); c.Status == "False" {
		if c.Reason == "ScalingDisabled" {
			return locales.KubeAutoscalerInactive(), c
		}
		if len(h.Metrics) > 0 && !slices.ContainsFunc(h.Metrics, func(m kube.AutoscalerMetric) bool { return m.Current != "" }) {
			return locales.KubeAutoscalerNoMetrics(), c
		}
		return locales.KubeAutoscalerInactive(), c
	}
	if c := autoscalerCondition(h, "ScalingLimited"); c.Status == "True" && c.Reason == "TooManyReplicas" {
		return locales.KubeAutoscalerAtMax(), c
	}
	return "", kube.CustomCondition{}
}

func policyPriority(item kube.Resource) int {
	if item.Deleting {
		return 2
	}
	if h := item.Autoscaler; h != nil {
		if label, _ := autoscalerProblem(h); label != "" {
			return 2
		}
		if h.Current != nil && h.Desired != nil && *h.Current != *h.Desired {
			return 1
		}
	}
	if b := item.Budget; b != nil && b.CurrentStatus && b.DisruptionsAllowed == 0 {
		return 1
	}
	return 0
}

func policyStatus(c *ui.Context, item kube.Resource) (string, ui.Color, ui.Color) {
	pal := Colors(c.Theme())
	switch {
	case item.Autoscaler != nil:
		h := item.Autoscaler
		if label, _ := autoscalerProblem(h); label != "" {
			return label, pal.DangerSoft, pal.DangerText
		}
		if h.Current != nil {
			if h.Desired != nil && *h.Current != *h.Desired {
				return locales.KubeAutoscalerScaling(*h.Current, *h.Desired), pal.WarningSoft, pal.WarningText
			}
			return locales.KubeAutoscalerReplicas(*h.Current, h.Min, h.Max), pal.SuccessSoft, pal.SuccessText
		}
	case item.Budget != nil:
		if b := item.Budget; b.CurrentStatus {
			label := locales.KubePolicyAllowed(b.DisruptionsAllowed)
			if b.DisruptionsAllowed == 0 {
				return label, pal.WarningSoft, pal.WarningText
			}
			return label, pal.SuccessSoft, pal.SuccessText
		}
	case item.Quota != nil:
		if n := len(item.Quota.Hard); n > 0 {
			return locales.KubePolicyCount(n), pal.Muted, c.Theme().TextMuted
		}
	default:
		return locales.KubePolicyCount(len(item.Limits)), pal.Muted, c.Theme().TextMuted
	}
	return locales.KubePolicyPending(), pal.Muted, c.Theme().TextMuted
}

func (a *App) policyOverview(c *ui.Context, item kube.Resource, lists []kube.ResourceList) {
	switch {
	case item.Autoscaler != nil:
		a.autoscalerOverview(c, item, lists)
	case item.Budget != nil:
		b := item.Budget
		var facts [][2]string
		if b.MinAvailable != nil {
			facts = append(facts, [2]string{"minAvailable", strings.Trim(string(b.MinAvailable), `"`)})
		}
		if b.MaxUnavailable != nil {
			facts = append(facts, [2]string{"maxUnavailable", strings.Trim(string(b.MaxUnavailable), `"`)})
		}
		if b.CurrentStatus {
			facts = append(facts, [][2]string{
				{locales.KubePolicyHealthy(), fmt.Sprintf("%d / %d", b.CurrentHealthy, b.DesiredHealthy)},
				{locales.KubePolicyExpected(), strconv.Itoa(b.ExpectedPods)},
				{locales.KubePolicyDisruptions(), strconv.Itoa(b.DisruptionsAllowed)},
			}...)
		} else {
			storageReadNote(c, locales.KubePolicyPending())
		}
		factRows(c, facts)
		if b.CurrentStatus {
			policyConditions(c, b.Conditions)
		}
	case item.Quota != nil:
		if len(item.Quota.Hard) == 0 {
			storageReadNote(c, locales.KubePolicyPending())
			return
		}
		detailSection(c, locales.KubePolicyUsedOfLimit())
		var facts [][2]string
		for _, key := range slices.Sorted(maps.Keys(item.Quota.Hard)) {
			facts = append(facts, [2]string{key, quotaUsage(item.Quota, key)})
		}
		factRows(c, facts)
	default:
		for _, limit := range item.Limits {
			detailSection(c, limit.Type)
			var facts [][2]string
			for _, group := range []struct {
				name   string
				values map[string]string
			}{
				{"min", limit.Min}, {"max", limit.Max},
				{"default", limit.Default}, {"defaultRequest", limit.DefaultRequest},
				{"maxLimitRequestRatio", limit.MaxLimitRequestRatio},
			} {
				for _, key := range slices.Sorted(maps.Keys(group.values)) {
					facts = append(facts, [2]string{group.name + " · " + key, group.values[key]})
				}
			}
			factRows(c, facts)
		}
	}
}

func (a *App) autoscalerOverview(c *ui.Context, item kube.Resource, lists []kube.ResourceList) {
	h := item.Autoscaler
	if label, condition := autoscalerProblem(h); label != "" {
		pal := Colors(c.Theme())
		ui.Column(c).Gap(4).Margin(0, 16, 6, 16).Padding(10, 12).Radius(9).Background(pal.DangerSoft).Children(func() {
			ui.Text(c, label).FontSize(12.5).FontWeight(600).TextColor(pal.DangerText).Wrap()
			for _, line := range []string{condition.Message, condition.Type + " · " + condition.Reason} {
				if line != "" {
					ui.Text(c, line).FontSize(11.5).TextColor(pal.DangerText).Wrap().Selectable()
				}
			}
		})
	}
	detailSection(c, locales.KubeTabOverview())
	facts := [][2]string{
		{locales.KubeAutoscalerCurrent(), replicaValue(h.Current)},
		{locales.KubeWanted(), replicaValue(h.Desired)},
		{locales.KubeAutoscalerRange(), fmt.Sprintf("%d–%d", h.Min, h.Max)},
	}
	if h.DownscaleWindow != nil {
		facts = append(facts, [2]string{locales.KubeAutoscalerDownscale(), locales.KubeAgeSeconds(*h.DownscaleWindow)})
	}
	if !h.LastScale.IsZero() {
		facts = append(facts, [2]string{locales.KubeAutoscalerLastScale(), locales.KubeCreatedAgo(age(c, h.LastScale))})
	}
	for _, metric := range h.Metrics {
		facts = append(facts, [2]string{metric.Name, metricSummary(metric)})
	}
	factRows(c, facts)
	detailSection(c, locales.KubePolicyRelated(item.Namespace))
	if h.Target.TargetsDeployment(h.Target.Name) {
		a.resourceLinkRow(c.Key("hpa-target"), h.Target.Kind+" "+h.Target.Name, "", locales.KubeClusterOpen(),
			resourceLink{kind: kindDeployments, namespace: item.Namespace, name: h.Target.Name})
	} else {
		storageReadNote(c, h.Target.Kind+" "+h.Target.Name)
	}
	for i, list := range lists {
		if list.Kind == kube.Autoscalers {
			continue
		}
		if list.Err != nil {
			storageReadNote(c, policyKindName(list.Kind)+" · "+a.accessWords(list.Err))
			continue
		}
		for _, related := range list.Items {
			if related.Namespace != item.Namespace {
				continue
			}
			label := policyLinkSummary(c, related)
			a.resourceLinkRow(c.Key("policy-related-"+related.UID), policyKindName(related.Kind)+" "+related.Name, label, locales.KubeClusterOpen(),
				resourceLink{kind: kindPolicies, policyTab: i, namespace: related.Namespace, name: related.Name, uid: related.UID})
		}
	}
	policyConditions(c, h.Conditions)
}

func quotaUsage(quota *kube.ResourceQuota, key string) string {
	return cmp.Or(quota.Used[key], "—") + " / " + quota.Hard[key]
}

func policyLinkSummary(c *ui.Context, item kube.Resource) string {
	var values []string
	switch {
	case item.Quota != nil:
		for _, key := range slices.Sorted(maps.Keys(item.Quota.Hard)) {
			values = append(values, key+" "+quotaUsage(item.Quota, key))
		}
	case item.Kind == kube.LimitRanges:
		for _, limit := range item.Limits {
			for _, key := range slices.Sorted(maps.Keys(limit.Default)) {
				values = append(values, key+" "+limit.Default[key])
			}
		}
	}
	if len(values) > 0 {
		return strings.Join(values, " · ")
	}
	label, _, _ := policyStatus(c, item)
	return label
}

func replicaValue(value *int) string {
	if value != nil {
		return strconv.Itoa(*value)
	}
	return "—"
}

func metricSummary(metric kube.AutoscalerMetric) string {
	return cmp.Or(metric.Current, "—") + " / " + cmp.Or(metric.Target, "—")
}

func autoscalerMetricsView(c *ui.Context, metrics []kube.AutoscalerMetric) {
	if len(metrics) == 0 {
		storageReadNote(c, locales.KubeAutoscalerNoMetrics())
		return
	}
	detailSection(c, locales.KubeAutoscalerCurrentOfTarget())
	var facts [][2]string
	for _, metric := range metrics {
		facts = append(facts, [2]string{metric.Name + " · " + metric.Source, metricSummary(metric)})
	}
	factRows(c, facts)
}

func policyConditions(c *ui.Context, conditions []kube.CustomCondition) {
	if len(conditions) == 0 {
		return
	}
	detailSection(c, locales.KubeCustomConditions())
	for _, condition := range conditions {
		detailSection(c, condition.Type+" · "+condition.Status)
		storageReadNote(c, strings.TrimSpace(condition.Reason+" "+condition.Message))
	}
}
