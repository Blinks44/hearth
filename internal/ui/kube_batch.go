package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

// Batch filters use the conditions reported by the controller. A retry
// does not make a Job failed until its Failed condition is true.
const (
	batchAll = iota
	batchFailed
	batchRunning
	batchSuspended
)

func lastCronFailed(j kube.CronJob) bool { return len(j.Runs) > 0 && j.Runs[0].State == kube.JobFailed }

func jobState(c *ui.Context, state kube.JobState) (text string, bg, fg ui.Color) {
	t := c.Theme()
	pal := Colors(t)
	switch state {
	case kube.JobFailed:
		return locales.KubeJobFailed(), pal.DangerSoft, pal.DangerText
	case kube.JobSucceeded:
		return locales.KubeJobSucceeded(), pal.SuccessSoft, pal.SuccessText
	case kube.JobRunning:
		return locales.KubeJobRunning(), pal.InfoSoft, pal.InfoText
	case kube.JobSuspended:
		return locales.KubeJobSuspended(), pal.Muted, t.TextMuted
	default:
		return locales.KubeJobPending(), pal.WarningSoft, pal.WarningText
	}
}

func batchMatches(query, name, namespace string) bool {
	return strings.Contains(strings.ToLower(namespace+"/"+name), strings.ToLower(strings.TrimSpace(query)))
}

func batchKindFilter(selected int, failed, running, suspended bool) bool {
	switch selected {
	case batchFailed:
		return failed
	case batchRunning:
		return running
	case batchSuspended:
		return suspended
	default:
		return true
	}
}

func (a *App) batchFilters(c *ui.Context, filter *int, counts [4]int, placeholder string) {
	s := &a.cluster
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		segments(c, "batch-filter", filter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[batchAll])},
			{locales.KubeJobFilterFailed(), strconv.Itoa(counts[batchFailed])},
			{locales.KubeJobRunning(), strconv.Itoa(counts[batchRunning])},
			{locales.KubeJobSuspended(), strconv.Itoa(counts[batchSuspended])},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(placeholder).Width(240).MinWidth(100)
	})
}

func (a *App) jobsView(c *ui.Context, batch *kube.Batch) {
	if batch.JobsErr != nil {
		toolNote(c, locales.KubeBatchReadError("Jobs", a.accessWords(batch.JobsErr)))
		return
	}
	s := &a.cluster
	var counts [4]int
	var shown []kube.Job
	for _, j := range batch.Jobs {
		failed, running, suspended := j.State == kube.JobFailed, j.State == kube.JobRunning, j.State == kube.JobSuspended
		for i := range counts {
			if batchKindFilter(i, failed, running, suspended) {
				counts[i]++
			}
		}
		if batchKindFilter(s.jobFilter, failed, running, suspended) && batchMatches(s.query, j.Name, j.Namespace) {
			shown = append(shown, j)
		}
	}
	a.batchFilters(c.Key("jobs"), &s.jobFilter, counts, locales.KubeJobSearch())
	if len(shown) == 0 {
		toolNote(c, locales.KubeBatchEmpty("Jobs"))
		return
	}
	slices.SortStableFunc(shown, func(a, b kube.Job) int {
		return cmp.Compare(batchIssueOrder(a.State == kube.JobFailed), batchIssueOrder(b.State == kube.JobFailed))
	})
	sort := s.sortOf(kindJobs)
	sortRows(*sort, shown, map[string]func(x, y kube.Job) int{
		"name":      func(x, y kube.Job) int { return cmp.Compare(x.Name, y.Name) },
		"status":    func(x, y kube.Job) int { return cmp.Compare(x.State, y.State) },
		"completed": func(x, y kube.Job) int { return cmp.Compare(x.Completed, y.Completed) },
		"age":       func(x, y kube.Job) int { return y.Created.Compare(x.Created) },
	})
	headers := []sortColumn{{locales.KubeClusterName(), "name"}, {locales.KubeClusterStatus(), "status"},
		{locales.KubeJobCompleted(), "completed"}, {locales.KubeClusterAge(), "age"}}
	batchTable(c, sort, headers, 28, "", func() {
		for _, j := range shown {
			ref := a.resourceRef("Job", j.Namespace, j.Name, j.UID)
			a.batchRow(c.Key(j.UID), ref, j.State == kube.JobFailed, func() {
				batchName(c, j.Name, j.Namespace)
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Start).Children(func() {
					text, bg, fg := jobState(c, j.State)
					aiTag(c, text, bg, fg)
				})
				batchText(c, fmt.Sprintf("%d/%d", j.Completed, j.Want))
				batchText(c, age(c, j.Created))
				a.batchResourceMenu(c, a.resourceRef("Job", j.Namespace, j.Name, j.UID))
			}, jobFailure(j))
		}
	})
}

func (a *App) cronJobsView(c *ui.Context, batch *kube.Batch) {
	if batch.CronJobsErr != nil {
		toolNote(c, locales.KubeBatchReadError("CronJobs", a.accessWords(batch.CronJobsErr)))
		return
	}
	s := &a.cluster
	var counts [4]int
	var shown []kube.CronJob
	for _, j := range batch.CronJobs {
		for i := range counts {
			if batchKindFilter(i, lastCronFailed(j), j.Active > 0, j.Suspended) {
				counts[i]++
			}
		}
		if batchKindFilter(s.cronJobFilter, lastCronFailed(j), j.Active > 0, j.Suspended) && batchMatches(s.query, j.Name, j.Namespace) {
			shown = append(shown, j)
		}
	}
	a.batchFilters(c.Key("cronjobs"), &s.cronJobFilter, counts, locales.KubeCronSearch())
	if len(shown) == 0 {
		toolNote(c, locales.KubeBatchEmpty("CronJobs"))
		return
	}
	slices.SortStableFunc(shown, func(a, b kube.CronJob) int {
		return cmp.Compare(batchIssueOrder(lastCronFailed(a)), batchIssueOrder(lastCronFailed(b)))
	})
	sort := s.sortOf(kindCronJobs)
	sortRows(*sort, shown, map[string]func(x, y kube.CronJob) int{
		"name":     func(x, y kube.CronJob) int { return cmp.Compare(x.Name, y.Name) },
		"schedule": func(x, y kube.CronJob) int { return cmp.Compare(x.Schedule, y.Schedule) },
		"runs": func(x, y kube.CronJob) int {
			return cmp.Compare(batchIssueOrder(lastCronFailed(x)), batchIssueOrder(lastCronFailed(y)))
		},
		"status": func(x, y kube.CronJob) int { return cmp.Compare(cronStatus(x), cronStatus(y)) },
	})
	var actionWidth float32 = 28
	actionHeader := ""
	if a.BuildPlan != nil {
		actionWidth, actionHeader = cronActionsWidth, locales.KubeCronOn()
	}
	headers := []sortColumn{{locales.KubeClusterName(), "name"}, {locales.KubeCronSchedule(), "schedule"},
		{locales.KubeCronRuns(), "runs"}, {locales.KubeClusterStatus(), "status"}}
	batchTable(c, sort, headers, actionWidth, actionHeader, func() {
		for _, j := range shown {
			failure := ""
			if lastCronFailed(j) {
				failure = j.Runs[0].Name + " · " + jobFailure(j.Runs[0])
			}
			ref := a.resourceRef("CronJob", j.Namespace, j.Name, j.UID)
			a.batchRow(c.Key(j.UID), ref, lastCronFailed(j), func() {
				batchName(c, j.Name, j.Namespace)
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
					clipped(c, j.Schedule).Font(FontMono).FontSize(12)
					caption := cronCaption(j.Schedule)
					if j.TimeZone != "" {
						caption = strings.TrimPrefix(caption+" · "+j.TimeZone, " · ")
					}
					overviewCaption(c, caption, Colors(c.Theme()).TextFaint)
				})
				a.cronHistory(c, j.Runs, batch.JobsErr)
				batchText(c, cronStatus(j))
				a.cronJobActions(c, j, actionWidth)
			}, failure)
		}
	})
}

// cronStatus is the CronJob's state in words.
func cronStatus(j kube.CronJob) string {
	switch {
	case j.Suspended:
		return locales.KubeJobSuspended()
	case j.Active > 0:
		return locales.KubeJobRunning()
	}
	return locales.KubeCronScheduled()
}

func (a *App) batchResourceMenu(c *ui.Context, ref ops.KubeResource) {
	ui.Row(c).Width(28).Shrink(0).Justify(ui.End).Children(func() { a.resourceMenu(c, ref) })
}

func batchIssueOrder(failed bool) int {
	if failed {
		return 0
	}
	return 1
}

func jobFailure(j kube.Job) string {
	if j.State != kube.JobFailed {
		return ""
	}
	if j.Message != "" {
		return j.Message
	}
	if j.Reason != "" {
		return j.Reason
	}
	return locales.KubeJobFailed()
}

func batchTable(c *ui.Context, sort *tableSort, headers []sortColumn, actionWidth float32, actionHeader string, rows func()) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(10).Padding(8, 16).Background(pal.Subtle).Children(func() {
			for i, col := range headers {
				on := sort.column == col.key
				b := ui.ButtonBase(c.Key("sort-" + col.key)).Grow(batchColumnWeight(i)).Basis(0).MinWidth(0).Gap(3).AlignItems(ui.Center).
					Justify(ui.Start).Label(locales.KubeSortBy(col.label)).OnClick(func() { *sort = sort.next(col.key) })
				color := pal.TextFaint
				if on || b.Hovered() {
					color = t.TextMuted
				}
				b.Children(func() {
					ui.Text(c, strings.ToUpper(col.label)).FontSize(10.5).TextColor(color).MinWidth(0).SingleLine()
					if on {
						icon := "chevron-up"
						if sort.desc {
							icon = "chevron-down"
						}
						ui.Icon(c, Icon(icon)).Size(11, 11).Shrink(0).TextColor(color)
					}
				})
			}
			ui.Row(c).Width(actionWidth).Shrink(0).Children(func() {
				if actionHeader != "" {
					ui.Box(c).Width(32).Center().Children(func() {
						ui.Text(c, strings.ToUpper(actionHeader)).FontSize(10.5).TextColor(pal.TextFaint).SingleLine()
					})
				}
			})
		})
		rows()
	})
}

func batchColumnWeight(i int) float32 {
	if i == 0 {
		return 1.35
	}
	return 1
}

func (a *App) batchRow(c *ui.Context, ref ops.KubeResource, failed bool, cells func(), message string) {
	pal := Colors(c.Theme())
	row := ui.Column(c).Transition(rowMotion).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Gap(4).Padding(8, 16)
	if failed {
		row.Background(pal.DangerSoft.Alpha(0.35))
	}
	a.resourceContextMenu(c, row, ref, func(m *ui.Menu) {
		if ref.Kind == "Job" && a.StreamLogs != nil && a.WatchCluster != nil && m.Item(locales.KubeLogsOpen()).Chosen() {
			a.openOwnerLogs(ref)
		}
	})
	row.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(cells)
		if message != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon("circle-alert")).Size(13, 13).TextColor(pal.DangerText).Shrink(0)
				clipped(c, message).FontSize(11.5).TextColor(pal.DangerText).Grow(1).MinWidth(0)
			})
		}
	})
}

func batchName(c *ui.Context, name, namespace string) {
	ui.Column(c).Grow(batchColumnWeight(0)).Basis(0).MinWidth(0).Gap(1).Children(func() {
		clipped(c, name).FontSize(13).FontWeight(500)
		overviewCaption(c, namespace, Colors(c.Theme()).TextFaint)
	})
}

func batchText(c *ui.Context, text string) {
	clipped(c, text).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1).Basis(0).MinWidth(0)
}

func (a *App) cronHistory(c *ui.Context, runs []kube.Job, err error) {
	pal := Colors(c.Theme())
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
		if err != nil {
			overviewCaption(c, a.accessWords(err), pal.WarningText)
			return
		}
		if len(runs) == 0 {
			overviewCaption(c, locales.KubeCronNoRuns(), pal.TextFaint)
			return
		}
		ui.Row(c).Gap(3).Children(func() {
			for range 10 - len(runs) {
				ui.Box(c).Size(6, 14).Radius(2).Background(pal.Line)
			}
			for _, j := range slices.Backward(runs) {
				label, _, fg := jobState(c, j.State)
				ui.Box(c.Key(j.UID)).Size(6, 14).Radius(2).Background(fg).Tooltip(j.Name + " · " + label + " · " + age(c, j.Created))
			}
		})
		last := runs[0]
		label, _, fg := jobState(c, last.State)
		overviewCaption(c, label+" · "+age(c, last.Created), fg)
	})
}

// cronCaption describes common schedules. The table always shows the exact
// expression, including schedules with lists, ranges or day-of-month rules.
func cronCaption(schedule string) string {
	switch schedule {
	case "@hourly":
		return locales.KubeCronHourly()
	case "@daily", "@midnight":
		return locales.KubeCronDaily("00:00")
	case "@weekly":
		return locales.KubeCronWeekly("0", "00:00")
	case "@monthly":
		return locales.KubeCronMonthly()
	case "@yearly", "@annually":
		return locales.KubeCronYearly()
	}
	f := strings.Fields(schedule)
	if len(f) != 5 || f[2] != "*" || f[3] != "*" {
		return ""
	}
	minute, hour := cronNumber(f[0], 59), cronNumber(f[1], 23)
	if f[4] == "*" {
		if n, ok := strings.CutPrefix(f[0], "*/"); ok && f[1] == "*" {
			if step := cronNumber(n, 59); step > 0 && 60%step == 0 {
				return locales.KubeCronMinutes(step)
			}
		}
		if minute == 0 && f[1] == "*" {
			return locales.KubeCronHourly()
		}
		if n, ok := strings.CutPrefix(f[1], "*/"); ok && minute == 0 {
			if step := cronNumber(n, 23); step > 0 {
				return locales.KubeCronHours(step)
			}
		}
	}
	if minute < 0 || hour < 0 {
		return ""
	}
	at := fmt.Sprintf("%02d:%02d", hour, minute)
	if f[4] == "*" {
		return locales.KubeCronDaily(at)
	}
	day := cronNumber(f[4], 6)
	if day < 0 {
		day = slices.Index([]string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}, strings.ToUpper(f[4]))
	}
	if day >= 0 {
		return locales.KubeCronWeekly(strconv.Itoa(day), at)
	}
	return ""
}

func cronNumber(s string, maxValue int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > maxValue {
		return -1
	}
	return n
}
