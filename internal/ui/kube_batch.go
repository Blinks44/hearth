package ui

import (
	"cmp"
	"context"
	"fmt"
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

func cronStatusTag(c *ui.Context, j kube.CronJob) (text string, bg, fg ui.Color) {
	t := c.Theme()
	pal := Colors(t)
	switch {
	case j.Suspended:
		return locales.KubeJobSuspended(), pal.Muted, t.TextMuted
	case j.Active > 0:
		return locales.KubeJobRunning(), pal.InfoSoft, pal.InfoText
	case lastCronFailed(j):
		return locales.KubeCronScheduled(), pal.DangerSoft, pal.DangerText
	default:
		return locales.KubeCronScheduled(), pal.SuccessSoft, pal.SuccessText
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
		ui.SearchField(c.Key("cluster-query"), &s.query).Bind(&s.queryField).Placeholder(placeholder).Height(controlHeight).Width(240).MinWidth(100)
	})
}

func (a *App) openJob(j kube.Job) {
	s := &a.cluster
	s.kind, s.query, s.jobFilter, s.resourceLink = kindJobs, "", batchAll, resourceLink{}
	s.job, s.tab, s.revealWorkload = j.UID, 0, j.UID
}

func (a *App) openCronJob(cj kube.CronJob) {
	s := &a.cluster
	s.kind, s.query, s.cronJobFilter, s.resourceLink = kindCronJobs, "", batchAll, resourceLink{}
	s.cronJob, s.tab, s.revealWorkload = cj.UID, 0, cj.UID
}

func (a *App) jobsView(c *ui.Context, cl *kube.Cluster) {
	batch := cl.Batch
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
	var selected kube.Job
	for _, j := range batch.Jobs {
		if j.UID != "" && j.UID == s.job {
			selected = j
			break
		}
	}
	compact := selected.UID != ""
	tracks := []ui.Track{ui.Fr(1.35), ui.Fr(1), ui.Fr(0.85), ui.Fr(0.75), ui.Fixed(28)}
	headers := []sortColumn{
		{locales.KubeClusterName(), "name"}, {locales.KubeClusterStatus(), "status"},
		{locales.KubeJobCompleted(), "completed"}, {locales.KubeClusterAge(), "age"}, {"", ""},
	}
	if compact {
		tracks = []ui.Track{ui.Fr(1.35), ui.Fr(0.85)}
		headers = []sortColumn{{locales.KubeClusterName(), "name"}, {locales.KubeClusterStatus(), "status"}}
	}
	pal := Colors(c.Theme())
	dataTracks := tracks
	if !compact {
		dataTracks = tracks[:len(tracks)-1]
	}
	workloadPanes(c.Key("jobs-panes"), func() {
		sortHead(c, tracks, nil, sort, headers...)
	}, func() {
		for _, j := range shown {
			ref := a.resourceRef("Job", j.Namespace, j.Name, j.UID)
			var issue *rowIssue
			if failure := jobFailure(j); failure != "" {
				title := locales.KubeJobFailed()
				if j.Reason != "" && j.Message != "" {
					title = j.Reason
				}
				issue = &rowIssue{
					key:      "kube-job:" + j.UID,
					label:    locales.Details() + " · " + j.Name,
					severity: issueDanger,
					title:    title,
					body:     failure,
				}
			}
			line := workloadSelectionRow(c.Key(j.UID), pal.Line)
			line.Children(func() {
				row := resourceGrid(ui.ButtonBase(c.Key("select-job")), dataTracks).
					Padding(6, 0).MinHeight(44).Grow(1).MinWidth(0).Label(j.Name).
					OnClick(func() { s.toggleWorkload(&s.job, j.UID) })
				switch {
				case j.UID == selected.UID:
					line.Background(pal.ActiveRow)
				case row.Hovered():
					line.Background(pal.Subtle)
				case j.State == kube.JobFailed:
					line.Background(pal.DangerSoft.Alpha(0.35))
				}
				a.resourceContextMenu(c, row, ref, func(m *ui.Menu) {
					if a.StreamLogs != nil && a.WatchCluster != nil && ref.Kubectl != "" && m.Item(locales.KubeLogsOpen()).Chosen() {
						a.openOwnerLogs(ref)
					}
				})
				s.revealWorkloadRow(row, j.UID)
				row.Children(func() {
					ui.Column(c).MinWidth(0).Gap(1).Children(func() {
						clipped(c, j.Name).FontSize(13).FontWeight(500).Tooltip(j.Name)
						overviewCaption(c, j.Namespace, pal.TextFaint)
					})
					ui.Row(c).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
						text, bg, fg := jobState(c, j.State)
						aiTag(c, text, bg, fg).Shrink(1).MinWidth(0).MaxWidthPercent(100).Tooltip(text)
						if !compact {
							a.rowIssueBadge(c, issue)
						}
					})
					if !compact {
						clipped(c, fmt.Sprintf("%d/%d", j.Completed, j.Want)).FontSize(12).TextColor(c.Theme().TextMuted)
						clipped(c, age(c, j.Created)).FontSize(12).TextColor(c.Theme().TextMuted)
					}
				})
				if !compact {
					ui.Row(c).AlignItems(ui.Center).Shrink(0).Children(func() {
						a.batchResourceMenu(c, ref)
					})
				}
			})
		}
	}, selected.UID, func() {
		a.jobDetail(c, cl, selected)
	})
}

func (a *App) jobDetail(c *ui.Context, cl *kube.Cluster, j kube.Job) {
	s := &a.cluster
	t := c.Theme()
	events, read := clusterEvents(cl, j.UID)
	label, bg, fg := jobState(c, j.State)
	ref := a.resourceRef("Job", j.Namespace, j.Name, j.UID)
	detailFrame(c, s.tab, s.tab == 2, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon(ownerIcon("Job"))).Size(18, 18).Shrink(0).TextColor(t.TextMuted)
				clipped(c, j.Name).FontSize(16).FontWeight(500).Shrink(1).MinWidth(0)
				aiTag(c, label, bg, fg).Shrink(0)
				ui.Box(c).Grow(1)
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(func() { s.toggleWorkload(&s.job, j.UID) })
			})
			meta := []string{"Job", j.Namespace}
			meta = append(meta, j.Images...)
			if !j.Created.IsZero() {
				meta = append(meta, locales.KubeCreatedAgo(age(c, j.Created)))
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(t.TextMuted).Wrap()
			if j.CronJob != "" {
				ui.Box(c).Height(2)
				a.ownerChip(c, j.Namespace, kube.Owner{Kind: "CronJob", Name: j.CronJob})
			}
		})
		if ref.Kubectl != "" && (a.BuildPlan != nil || a.StreamLogs != nil && a.WatchCluster != nil) {
			ui.Row(c).Gap(8).Wrap().Padding(0, 16, 12, 16).Children(func() {
				a.ownerLogsButton(c, ref)
				a.resourceActions(c, ref, 2)
			})
		}
		ui.Row(c).Padding(0, 16, 12, 16).Children(func() {
			count := ""
			if read && cl.EventsErr == nil {
				count = strconv.Itoa(len(events))
			}
			tabs := []segment{{locales.KubeTabOverview(), ""}, {locales.KubeTabEvents(), count}}
			hasYAML := false
			if f := s.resources[ref]; f != nil && (f.editor != nil || s.tab == 2) {
				tabs = append(tabs, segment{locales.KubeResourceYaml(), ""})
				hasYAML = true
			}
			clampDetailTab(&s.tab, 2, hasYAML)
			segments(c, "job-tab", &s.tab, tabs)
		})
	}, func() {
		switch s.tab {
		case 2:
			if s.resources[ref] != nil {
				a.resourceEditor(c, s.resources[ref])
				return
			}
		case 1:
			workloadEventList(c, events, read, cl.EventsErr, 0)
			return
		}
		if failure := jobFailure(j); failure != "" {
			storyCard(c, podStory{title: cmp.Or(j.Reason, locales.KubeJobFailed()), detail: failure, known: true}, nil)
		}
		detailSection(c, locales.KubeFacts())
		facts := [][2]string{
			{locales.KubeClusterStatus(), label},
			{locales.KubeJobCompleted(), fmt.Sprintf("%d/%d", j.Completed, j.Want)},
		}
		if j.Active > 0 {
			facts = append(facts, [2]string{locales.KubeJobActive(), strconv.Itoa(j.Active)})
		}
		if j.Failed > 0 {
			facts = append(facts, [2]string{locales.KubeJobFailed(), strconv.Itoa(j.Failed)})
		}
		if !j.Started.IsZero() {
			facts = append(facts, [2]string{locales.KubeJobStarted(), locales.KubeCreatedAgo(age(c, j.Started))})
		}
		if !j.Started.IsZero() && !j.CompletedAt.IsZero() && !j.CompletedAt.Before(j.Started) {
			facts = append(facts, [2]string{locales.KubeJobDuration(), span(j.CompletedAt.Sub(j.Started))})
		}
		if j.Parallelism > 0 {
			facts = append(facts, [2]string{locales.KubeJobParallelism(), strconv.Itoa(j.Parallelism)})
		}
		if j.BackoffLimit != nil {
			facts = append(facts, [2]string{locales.KubeJobBackoffLimit(), strconv.Itoa(*j.BackoffLimit)})
		}
		factRows(c, facts)
		relations := relationshipsFor(cl, j.Namespace, j.UID)
		a.resourceRelationships(c, relations.Links, relations.Errors)
	})
}

func (a *App) cronJobsView(c *ui.Context, cl *kube.Cluster) {
	batch := cl.Batch
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
	var selected kube.CronJob
	for _, j := range batch.CronJobs {
		if j.UID != "" && j.UID == s.cronJob {
			selected = j
			break
		}
	}
	compact := selected.UID != ""
	var actionWidth float32 = 28
	actionHeader := ""
	if _, hasKubectl := cliCopy(a.cliState(), "kubectl"); hasKubectl && a.BuildPlan != nil {
		actionWidth, actionHeader = cronActionsWidth, locales.KubeCronOn()
	}
	tracks := []ui.Track{ui.Fr(1.35), ui.Fr(1), ui.Fr(1), ui.Fr(0.9), ui.Fixed(actionWidth)}
	headers := []sortColumn{
		{locales.KubeClusterName(), "name"}, {locales.KubeCronSchedule(), "schedule"},
		{locales.KubeCronRuns(), "runs"}, {locales.KubeClusterStatus(), "status"}, {actionHeader, ""},
	}
	if compact {
		tracks = []ui.Track{ui.Fr(1.35), ui.Fr(0.85)}
		headers = []sortColumn{{locales.KubeClusterName(), "name"}, {locales.KubeClusterStatus(), "status"}}
	}
	pal := Colors(c.Theme())
	dataTracks := tracks
	if !compact {
		dataTracks = tracks[:len(tracks)-1]
	}
	workloadPanes(c.Key("cronjobs-panes"), func() {
		sortHead(c, tracks, nil, sort, headers...)
	}, func() {
		for _, j := range shown {
			var issue *rowIssue
			if lastCronFailed(j) {
				issue = &rowIssue{
					key:      "kube-cron:" + j.UID,
					label:    locales.Details() + " · " + j.Name,
					severity: issueDanger,
					title:    j.Runs[0].Name,
					body:     jobFailure(j.Runs[0]),
				}
			}
			ref := a.resourceRef("CronJob", j.Namespace, j.Name, j.UID)
			line := workloadSelectionRow(c.Key(j.UID), pal.Line)
			line.Children(func() {
				row := resourceGrid(ui.ButtonBase(c.Key("select-cronjob")), dataTracks).
					Padding(6, 0).MinHeight(44).Grow(1).MinWidth(0).Label(j.Name).
					OnClick(func() { s.toggleWorkload(&s.cronJob, j.UID) })
				switch {
				case j.UID == selected.UID:
					line.Background(pal.ActiveRow)
				case row.Hovered():
					line.Background(pal.Subtle)
				case lastCronFailed(j):
					line.Background(pal.DangerSoft.Alpha(0.35))
				}
				a.resourceContextMenu(c, row, ref, nil)
				s.revealWorkloadRow(row, j.UID)
				row.Children(func() {
					ui.Column(c).MinWidth(0).Gap(1).Children(func() {
						clipped(c, j.Name).FontSize(13).FontWeight(500).Tooltip(j.Name)
						overviewCaption(c, j.Namespace, pal.TextFaint)
					})
					if compact {
						text, bg, fg := cronStatusTag(c, j)
						ui.Row(c).MinWidth(0).AlignItems(ui.Center).Children(func() {
							aiTag(c, text, bg, fg).Shrink(1).MinWidth(0).MaxWidthPercent(100).Tooltip(text)
						})
						return
					}
					ui.Column(c).MinWidth(0).Gap(1).Children(func() {
						clipped(c, j.Schedule).Font(FontMono).FontSize(12)
						caption := cronCaption(j.Schedule)
						if j.TimeZone != "" {
							caption = strings.TrimPrefix(caption+" · "+j.TimeZone, " · ")
						}
						overviewCaption(c, caption, pal.TextFaint)
					})
					a.cronHistory(c, j.Runs, batch.JobsErr)
					ui.Row(c).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
						clipped(c, cronStatus(j)).FontSize(12).TextColor(c.Theme().TextMuted).MinWidth(0)
						a.rowIssueBadge(c, issue)
					})
				})
				if !compact {
					ui.Row(c).AlignItems(ui.Center).Shrink(0).Children(func() {
						a.cronJobActions(c, j, actionWidth)
					})
				}
			})
		}
	}, selected.UID, func() {
		a.cronJobDetail(c, cl, selected)
	})
}

func (a *App) cronJobDetail(c *ui.Context, cl *kube.Cluster, cj kube.CronJob) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	events, read := clusterEvents(cl, cj.UID)
	label, bg, fg := cronStatusTag(c, cj)
	ref := a.resourceRef("CronJob", cj.Namespace, cj.Name, cj.UID)
	detailFrame(c, s.tab, s.tab == 3, func() {
		ui.Column(c).Gap(6).Padding(16, 16, 14, 16).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon(ownerIcon("CronJob"))).Size(18, 18).Shrink(0).TextColor(t.TextMuted)
				clipped(c, cj.Name).FontSize(16).FontWeight(500).Shrink(1).MinWidth(0)
				aiTag(c, label, bg, fg).Shrink(0)
				ui.Box(c).Grow(1)
				iconButton(c, "x", locales.KubeCloseDetails()).OnClick(func() { s.toggleWorkload(&s.cronJob, cj.UID) })
			})
			meta := []string{"CronJob", cj.Namespace}
			meta = append(meta, cj.Images...)
			if !cj.Created.IsZero() {
				meta = append(meta, locales.KubeCreatedAgo(age(c, cj.Created)))
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		if ref.Kubectl != "" && a.BuildPlan != nil {
			f := a.cluster.resources[ref]
			busy := (f != nil && f.busy) || a.op.running()
			toggleLabel := locales.KubeCronPauseAction()
			toggleIcon := "pause"
			if cj.Suspended {
				toggleLabel = locales.KubeCronResumeAction()
				toggleIcon = "play"
			}
			ui.Row(c).Gap(8).Wrap().Padding(0, 16, 12, 16).Children(func() {
				kubeActionButton(c.Key("cron-detail-toggle"), toggleIcon, toggleLabel+"…", podActionHeight).Disabled(busy).
					OnClick(func() {
						a.planCronAction(ref, func(ctx context.Context, r run.Runner, env sysenv.Env, ref ops.KubeResource) (ops.Plan, error) {
							return ops.PlanKubeCronSuspend(ctx, r, env, ref, !cj.Suspended)
						})
					})
				kubeActionButton(c.Key("cron-detail-run"), "play", locales.KubeCronRunAction()+"…", podActionHeight).Disabled(busy).
					OnClick(func() { a.planCronAction(ref, ops.PlanKubeCronRun) })
				a.resourceActions(c, ref, 3)
			})
		}
		ui.Row(c).Padding(0, 16, 12, 16).Children(func() {
			count := ""
			if read && cl.EventsErr == nil {
				count = strconv.Itoa(len(events))
			}
			tabs := []segment{
				{locales.KubeTabOverview(), ""},
				{"Jobs", strconv.Itoa(len(cj.Runs))},
				{locales.KubeTabEvents(), count},
			}
			hasYAML := false
			if f := s.resources[ref]; f != nil && (f.editor != nil || s.tab == 3) {
				tabs = append(tabs, segment{locales.KubeResourceYaml(), ""})
				hasYAML = true
			}
			clampDetailTab(&s.tab, 3, hasYAML)
			segments(c, "cronjob-tab", &s.tab, tabs)
		})
	}, func() {
		switch s.tab {
		case 3:
			if s.resources[ref] != nil {
				a.resourceEditor(c, s.resources[ref])
				return
			}
		case 1:
			if cl.Batch != nil && cl.Batch.JobsErr != nil {
				storageReadNote(c, a.accessWords(cl.Batch.JobsErr))
				return
			}
			if len(cj.Runs) == 0 {
				storageReadNote(c, locales.KubeCronNoRuns())
				return
			}
			for _, run := range cj.Runs {
				a.ownedJobRow(c, run)
			}
			return
		case 2:
			workloadEventList(c, events, read, cl.EventsErr, 0)
			return
		}
		if lastCronFailed(cj) {
			failed := cj.Runs[0]
			ui.Column(c).Gap(4).Margin(0, 16, 6, 16).Padding(10, 14).Radius(10).Background(pal.DangerSoft).Children(func() {
				ui.Text(c, locales.KubeJobFailed()).FontSize(13).FontWeight(500).TextColor(pal.DangerText)
				link := ui.ButtonBase(c.Key("cron-failed-" + failed.UID)).AlignSelf(ui.Start).Label(failed.Name).OnClick(func() { a.openJob(failed) })
				link.Children(func() {
					ui.Text(c, failed.Name+": "+jobFailure(failed)).FontSize(12).TextColor(t.Text).Wrap()
				})
			})
		}
		detailSection(c, locales.KubeFacts())
		schedule := cj.Schedule
		if caption := cronCaption(cj.Schedule); caption != "" {
			schedule += " · " + caption
		}
		facts := [][2]string{
			{locales.KubeClusterStatus(), cronStatus(cj)},
			{locales.KubeCronSchedule(), schedule},
			{locales.KubeCronTimeZone(), cj.TimeZone},
			{locales.KubeCronConcurrency(), cj.ConcurrencyPolicy},
			{locales.KubeJobActive(), strconv.Itoa(cj.Active)},
		}
		if !cj.LastSchedule.IsZero() {
			facts = append(facts, [2]string{locales.KubeCronLastSchedule(), locales.KubeCreatedAgo(age(c, cj.LastSchedule))})
		}
		factRows(c, facts)
		relations := relationshipsFor(cl, cj.Namespace, cj.UID)
		a.resourceRelationships(c, relations.Links, relations.Errors)
	})
}

func (a *App) ownedJobRow(c *ui.Context, j kube.Job) {
	pal := Colors(c.Theme())
	label, bg, fg := jobState(c, j.State)
	row := ui.ButtonBase(c.Key("owned-job-"+j.UID)).Gap(10).Padding(7, 16).AlignItems(ui.Center).Label(j.Name).OnClick(func() { a.openJob(j) })
	if row.Hovered() {
		row.Background(pal.Subtle)
	}
	row.Children(func() {
		clipped(c, j.Name).Font(FontMono).FontSize(12.5).Grow(1).MinWidth(0)
		ui.Text(c, fmt.Sprintf("%d/%d", j.Completed, j.Want)).Font(FontMono).FontSize(12).TextColor(c.Theme().TextMuted).Shrink(0)
		if !j.Created.IsZero() {
			ui.Text(c, age(c, j.Created)).FontSize(12).TextColor(c.Theme().TextMuted).Shrink(0)
		}
		aiTag(c, label, bg, fg).Shrink(0)
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

func batchColumnWeight(i int) float32 {
	if i == 0 {
		return 1.35
	}
	return 1
}

func batchName(c *ui.Context, name, namespace string) {
	ui.Column(c).Grow(batchColumnWeight(0)).Basis(0).MinWidth(0).Gap(1).Children(func() {
		clipped(c, name).FontSize(13).FontWeight(500)
		overviewCaption(c, namespace, Colors(c.Theme()).TextFaint)
	})
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

func (a *App) cronHistory(c *ui.Context, runs []kube.Job, err error) {
	pal := Colors(c.Theme())
	ui.Column(c).MinWidth(0).Gap(3).Children(func() {
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
// expression alongside any caption.
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
	if len(f) != 5 {
		return ""
	}
	minute, hour := cronNumber(f[0], 59), cronNumber(f[1], 23)
	if f[2] == "*" && f[3] == "*" && f[4] == "*" {
		if f[0] == "*" && f[1] == "*" {
			return locales.KubeCronEveryMinute()
		}
		if n, ok := strings.CutPrefix(f[0], "*/"); ok && f[1] == "*" {
			if step := cronNumber(n, 59); step == 1 {
				return locales.KubeCronEveryMinute()
			} else if step > 1 {
				if 60%step == 0 {
					return locales.KubeCronMinutes(step)
				}
				return locales.KubeCronMinutesOfHour(step)
			}
		}
		if minute >= 0 && f[1] == "*" {
			if minute == 0 {
				return locales.KubeCronHourly()
			}
			return locales.KubeCronHourlyAt(fmt.Sprintf("%02d", minute))
		}
		if n, ok := strings.CutPrefix(f[1], "*/"); ok && minute >= 0 {
			if step := cronNumber(n, 23); step == 1 {
				if minute == 0 {
					return locales.KubeCronHourly()
				}
				return locales.KubeCronHourlyAt(fmt.Sprintf("%02d", minute))
			} else if step > 1 {
				if minute == 0 {
					return locales.KubeCronHours(step)
				}
				return locales.KubeCronHoursAt(step, fmt.Sprintf("%02d", minute))
			}
		}
		if minute >= 0 && strings.Contains(f[1], ",") {
			if times, ok := cronHourList(f[1], minute); ok {
				return locales.KubeCronDaily(times)
			}
		}
	}
	if minute < 0 || hour < 0 {
		return ""
	}
	at := fmt.Sprintf("%02d:%02d", hour, minute)
	if f[2] == "*" && f[3] == "*" {
		if f[4] == "*" {
			return locales.KubeCronDaily(at)
		}
		if dow := strings.ToUpper(f[4]); dow == "1-5" || dow == "MON-FRI" {
			return locales.KubeCronWeekdays(at)
		}
		if day := cronWeekday(f[4]); day >= 0 {
			return locales.KubeCronWeekly(strconv.Itoa(day), at)
		}
		return ""
	}
	if f[4] != "*" {
		return ""
	}
	dom := cronNumber(f[2], 31)
	if dom < 1 {
		return ""
	}
	if f[3] == "*" {
		return locales.KubeCronMonthlyOn(dom, at)
	}
	if month := cronMonth(f[3]); month >= 1 {
		return locales.KubeCronYearlyOn(strconv.Itoa(month), dom, at)
	}
	return ""
}

func cronHourList(field string, minute int) (string, bool) {
	parts := strings.Split(field, ",")
	if len(parts) < 2 || len(parts) > 6 {
		return "", false
	}
	times := make([]string, 0, len(parts))
	for _, p := range parts {
		h := cronNumber(p, 23)
		if h < 0 {
			return "", false
		}
		times = append(times, fmt.Sprintf("%02d:%02d", h, minute))
	}
	return strings.Join(times, ", "), true
}

func cronWeekday(field string) int {
	if day := cronNumber(field, 7); day >= 0 {
		if day == 7 {
			return 0
		}
		return day
	}
	return slices.Index([]string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}, strings.ToUpper(field))
}

func cronMonth(field string) int {
	if m := cronNumber(field, 12); m >= 1 {
		return m
	}
	if i := slices.Index([]string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}, strings.ToUpper(field)); i >= 0 {
		return i + 1
	}
	return -1
}

func cronNumber(s string, maxValue int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > maxValue {
		return -1
	}
	return n
}
