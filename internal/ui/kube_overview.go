package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) clusterOverview(c *ui.Context, cl *kube.Cluster) {
	o := cl.Overview
	var readyNodes, readyPods, podProblems int
	var requested, allocatable kube.Resources
	versions := make(map[string]bool)
	for _, n := range o.Nodes {
		if n.Ready {
			readyNodes++
		}
		if n.Version != "" {
			versions[n.Version] = true
		}
		requested.CPU += n.Requested.CPU
		requested.Memory += n.Requested.Memory
		allocatable.CPU += n.Allocatable.CPU
		allocatable.Memory += n.Allocatable.Memory
	}
	for _, p := range cl.Pods {
		if podTrouble(p) {
			podProblems++
		} else if p.Phase == "Running" {
			readyPods++
		}
	}
	pal := Colors(c.Theme())
	ui.Row(c).Gap(12).Children(func() {
		nodes, caption := locales.KubeGroupReady(readyNodes, len(o.Nodes)), ""
		if o.NodesErr != nil {
			nodes, caption = "–", a.accessWords(o.NodesErr)
		} else if len(versions) == 1 {
			for v := range versions {
				caption = v
			}
		}
		overviewStat(c, locales.KubeOverviewNodes(), nodes, func() {
			overviewCaption(c, caption, pal.TextFaint)
		})
		overviewStat(c, "Pods", locales.KubeGroupReady(readyPods, len(cl.Pods)), func() {
			if podProblems > 0 {
				overviewCaption(c, locales.KubeOverviewPodProblems(podProblems), pal.DangerText)
			}
		})
		err := o.NodesErr
		if err == nil {
			err = o.RequestsErr
		}
		a.overviewRequest(c, locales.KubeOverviewCPU(), requested.CPU, allocatable.CPU, err, locales.KubeOverviewCores)
		a.overviewRequest(c, locales.KubeOverviewMemory(), requested.Memory, allocatable.Memory, err, func(req, total float64) string {
			return locales.KubeOverviewGiB(req/(1<<30), total/(1<<30))
		})
	})
	a.metricsNotice(c, cl.MetricsErr)
	columns(c, 14, func() { a.overviewAttention(c, cl) }, func() {
		a.overviewNodes(c, o)
		a.overviewWarnings(c, o)
	})
}

func overviewStat(c *ui.Context, label, value string, caption func()) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(92).Gap(3).Padding(10, 13).Radius(10).
		BorderWidth(1).BorderColor(pal.Line).Background(pal.Subtle).Children(func() {
		clipped(c, label).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, value).FontSize(19).FontWeight(600).Wrap()
		caption()
	})
}

func overviewCaption(c *ui.Context, label string, color ui.Color) {
	if label != "" {
		clipped(c, label).FontSize(11.5).TextColor(color)
	}
}

func (a *App) overviewRequest(c *ui.Context, label string, requested, total float64, err error, amount func(float64, float64) string) {
	pal := Colors(c.Theme())
	if err != nil || total <= 0 {
		caption := locales.KubeOverviewNoCapacity()
		if err != nil {
			caption = a.accessWords(err)
		}
		overviewStat(c, label, "–", func() { overviewCaption(c, caption, pal.TextFaint) })
		return
	}
	share := requested / total
	overviewStat(c, label, requestPercent(share), func() {
		requestBar(c, share, Accent)
		overviewCaption(c, amount(requested, total), pal.TextFaint)
	})
}

func requestPercent(share float64) string {
	if share > 0 && share < 0.005 {
		return "<1%"
	}
	return strconv.FormatFloat(math.Round(share*100), 'f', 0, 64) + "%"
}

func requestBar(c *ui.Context, share float64, color ui.Color) {
	ui.Column(c).Height(6).Radius(3).Background(Colors(c.Theme()).Line).Children(func() {
		if share > 0 {
			ui.Box(c).WidthPercent(float32(min(share, 1) * 100)).Height(6).Radius(3).Background(color)
		}
	})
}

func overviewPanel(c *ui.Context, title, caption string, rows func()) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Background(t.Background).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(10, 13).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(500).Wrap()
			overviewCaption(c, caption, pal.TextFaint)
		})
		rows()
	})
}

func overviewMessage(c *ui.Context, message string) {
	ui.Text(c, message).FontSize(12).TextColor(c.Theme().TextMuted).Padding(10, 13).Wrap()
}

type overviewProblem struct {
	key, name, detail string
	waiting           bool
	open              func()
}

func (a *App) overviewAttention(c *ui.Context, cl *kube.Cluster) {
	s := &a.cluster
	var problems []overviewProblem
	for _, p := range cl.Pods {
		if story, ok := storyOf(p, nil, c.Now()); ok {
			problems = append(problems, overviewProblem{"pod-" + p.UID, "Pod · " + p.Name, p.Namespace + " · " + story.line(), story.waiting, func() {
				a.openPod(p)
			}})
		}
	}
	for _, d := range cl.Deployments {
		if deploymentTrouble(d) || d.Stalled != "" {
			label, _, _ := deploymentTag(c, d)
			if d.Stalled != "" {
				label = d.Stalled
			}
			problems = append(problems, overviewProblem{"deployment-" + d.UID, "Deployment · " + d.Name, d.Namespace + " · " + label, rolling(d) && d.Stalled == "", func() {
				s.kind, s.deployment, s.tab, s.deploymentFilter, s.query = kindDeployments, d.UID, 0, deploymentsAll, ""
			}})
		}
	}
	if batch := cl.Batch; batch != nil {
		for _, j := range batch.Jobs {
			if j.State == kube.JobFailed && (j.CronJob == "" || batch.CronJobsErr != nil) {
				problems = append(problems, overviewProblem{"job-" + j.UID, "Job · " + j.Name, j.Namespace + " · " + jobFailure(j), false, func() {
					s.kind, s.jobFilter, s.query = kindJobs, batchAll, j.Namespace+"/"+j.Name
				}})
			}
		}
		for _, j := range batch.CronJobs {
			if lastCronFailed(j) {
				problems = append(problems, overviewProblem{"cronjob-" + j.UID, "CronJob · " + j.Name, j.Namespace + " · " + jobFailure(j.Runs[0]), false, func() {
					s.kind, s.cronJobFilter, s.query = kindCronJobs, batchAll, j.Namespace+"/"+j.Name
				}})
			}
		}
	}
	for _, n := range cl.Overview.Nodes {
		if !n.Ready {
			name := n.Name
			problems = append(problems, overviewProblem{
				key:    "node-" + name,
				name:   locales.KubeOverviewNode(name),
				detail: locales.KubeOverviewNodeNoReady(),
				open:   func() { a.openResourceLink(resourceLink{kind: kindNodes, name: name}) },
			})
		}
	}
	overviewPanel(c, locales.KubeOverviewAttention(), strconv.Itoa(len(problems)), func() {
		if len(problems) == 0 {
			if cl.Overview.NodesErr != nil || cl.Batch != nil && (cl.Batch.JobsErr != nil || cl.Batch.CronJobsErr != nil) {
				overviewMessage(c, locales.KubeOverviewNoWorkloadProblems())
			} else {
				overviewMessage(c, locales.KubeOverviewNoProblems())
			}
		}
		for i, p := range problems {
			a.overviewProblemRow(c, p, i > 0)
		}
	})
}

func (a *App) overviewProblemRow(c *ui.Context, p overviewProblem, divider bool) {
	pal := Colors(c.Theme())
	icon, color := "circle-alert", pal.DangerText
	if p.waiting {
		icon, color = "triangle-alert", pal.WarningText
	}
	row := ui.Row(c.Key(p.key)).MinHeight(52).Gap(10).AlignItems(ui.Center).Padding(7, 13)
	if divider {
		row.BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	}
	row.Children(func() {
		ui.Icon(c, Icon(icon)).Size(15, 15).TextColor(color).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, p.name).FontSize(13.5).FontWeight(500)
			clipped(c, p.detail).FontSize(12).TextColor(c.Theme().TextMuted)
		})
		if p.open != nil {
			ui.Button(c, locales.KubeClusterOpen()).Label(locales.KubeClusterOpen() + " " + p.name).Height(24).OnClick(p.open)
		}
	})
}

func (a *App) overviewNodes(c *ui.Context, o *kube.Overview) {
	caption := ""
	if o.NodesErr == nil {
		caption = strconv.Itoa(len(o.Nodes))
	}
	overviewPanel(c, locales.KubeOverviewNodes(), caption, func() {
		switch {
		case o.NodesErr != nil:
			overviewMessage(c, a.accessWords(o.NodesErr))
		case len(o.Nodes) == 0:
			overviewMessage(c, locales.KubeOverviewNoNodes())
		default:
			for _, n := range o.Nodes {
				a.overviewNodeRow(c, n, o.RequestsErr == nil)
			}
		}
	})
}

func (a *App) overviewNodeRow(c *ui.Context, n kube.Node, requests bool) {
	t := c.Theme()
	pal := Colors(t)
	dot := t.Success
	if !n.Ready {
		dot = t.Danger
	}
	var shares [2]float64
	labels := [2]string{"–", "–"}
	if requests && n.Allocatable.CPU > 0 {
		shares[0] = n.Requested.CPU / n.Allocatable.CPU
		labels[0] = requestPercent(shares[0])
	}
	if requests && n.Allocatable.Memory > 0 {
		shares[1] = n.Requested.Memory / n.Allocatable.Memory
		labels[1] = requestPercent(shares[1])
	}
	name := n.Name
	row := ui.ButtonBase(c.Key("overview-node-"+name)).MinHeight(48).Padding(8, 13).
		AlignItems(ui.Stretch).Label(locales.KubeClusterOpen() + " " + name).
		OnClick(func() { a.openResourceLink(resourceLink{kind: kindNodes, name: name}) })
	if row.Hovered() {
		row.Background(pal.Subtle)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(4).Background(dot).Shrink(0)
				clipped(c, n.Name).Font(FontMono).FontSize(12).Grow(1).MinWidth(0)
				overviewCaption(c, locales.KubeOverviewNodeRequests(labels[0], labels[1]), pal.TextFaint)
			})
			if n.Usage != nil {
				overviewCaption(c, locales.KubeMetricsNodeUse(metricsCPU(n.Usage.CPU), metricsMemory(n.Usage.Memory)), pal.TextFaint)
			}
			if requests {
				ui.Row(c).Gap(8).Children(func() {
					for _, share := range shares {
						ui.Column(c).Grow(1).Basis(0).Children(func() {
							color := pal.Info
							if share >= .7 {
								color = t.Warning
							}
							requestBar(c, share, color)
						})
					}
				})
			}
		})
	})
}

func (a *App) overviewWarnings(c *ui.Context, o *kube.Overview) {
	t := c.Theme()
	pal := Colors(t)
	caption := ""
	if o.WarningsErr == nil {
		caption = locales.KubeOverviewLastHour(len(o.Warnings))
	}
	overviewPanel(c, locales.KubeOverviewWarnings(), caption, func() {
		if o.WarningsErr != nil {
			overviewMessage(c, a.accessWords(o.WarningsErr))
			return
		}
		if len(o.Warnings) == 0 {
			overviewMessage(c, locales.KubeOverviewNoWarnings())
		}
		for _, e := range o.Warnings[:min(3, len(o.Warnings))] {
			ui.Row(c).Gap(8).Padding(6, 13).AlignItems(ui.Center).Children(func() {
				aiTag(c, e.Reason, pal.WarningSoft, pal.WarningText).Shrink(0)
				object := e.Object.Name
				if e.Namespace != "" {
					object = e.Namespace + "/" + object
				}
				if e.Object.Kind != "" {
					object = strings.ToLower(e.Object.Kind) + "/" + object
				}
				if object == "" {
					object = e.Message
				}
				ui.Text(c, object).SingleLine().Tooltip(e.Message).Font(FontMono).FontSize(11.5).Grow(1).MinWidth(0)
				count := locales.KubeEventCount(e.Count)
				if !e.Last.IsZero() {
					count = locales.KubeEventRepeated(e.Count, age(c, e.Last))
				}
				ui.Text(c, count).FontSize(11).TextColor(pal.TextFaint).Shrink(0)
			})
		}
	})
}
