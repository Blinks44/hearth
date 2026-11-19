package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

// logPeriods are the log's periods: the whole run, or its last minutes.
var logPeriods = []time.Duration{0, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour}

// logView is the log of a pod's container, shown in place of the list.
type logView struct {
	pod, container string
	previous       bool
	// since is the period's, in logPeriods.
	since       int
	times, wrap bool
	query       string
	queryField  ui.Handle
	// match is the current one among the query's.
	match int
	list  ui.ListState
	// lines are those read, the last kube.MaxLogLines; dropped how many
	// went before them.
	lines   []kube.LogLine
	dropped int
	err     error
	// ended is true once kubectl ended without an error: a previous run
	// read, or the run followed stopped.
	ended bool
	// found caches the rows matching found.query among found.n lines.
	found struct {
		query      string
		n, dropped int
		rows       []int
	}
}

// openLogs shows the log of the pod's container, its previous run's or
// its current one's.
func (a *App) openLogs(p kube.Pod, container string, previous bool) {
	a.cluster.logs = &logView{pod: p.UID, container: container, previous: previous, times: true}
}

// closeLogs goes back to the pod's details.
func (a *App) closeLogs() {
	if l := a.cluster.logs; l != nil {
		a.cluster.kind, a.cluster.pod = kindPods, l.pod
	}
	a.cluster.logs = nil
}

// logPod is the pod and container of the log shown.
func (a *App) logPod() (kube.Pod, kube.Container, bool) {
	s := &a.cluster
	if s.logs == nil || s.data == nil {
		return kube.Pod{}, kube.Container{}, false
	}
	for _, p := range s.data.Pods {
		if p.UID != s.logs.pod {
			continue
		}
		for _, ct := range p.Containers {
			if ct.Name == s.logs.container {
				return p, ct, true
			}
		}
	}
	return kube.Pod{}, kube.Container{}, false
}

// syncLogs keeps kubectl logs reading the log shown while the cluster
// page shows it, and stops it as the page or the log closes, as
// syncCluster does the watch. A container that starts again is read
// again: its new run, or the run that just ended.
func (a *App) syncLogs() {
	s := &a.cluster
	var spec kube.Logs
	running := ""
	if p, ct, ok := a.logPod(); ok && a.router.Path() == kubeClusterPage && a.StreamLogs != nil {
		if in, found := cliCopy(a.cliState(), "kubectl"); found {
			spec = kube.Logs{Kubectl: in.Path, Context: a.router.Query("context"), Namespace: p.Namespace, Pod: p.Name,
				Container: ct.Name, Previous: s.logs.previous, Since: logPeriods[s.logs.since]}
			running = fmt.Sprintf("%q", []string{spec.Kubectl, spec.Context, spec.Namespace, spec.Pod, spec.Container,
				strconv.FormatBool(spec.Previous), spec.Since.String(), strconv.Itoa(ct.Restarts)})
		}
	}
	if running == s.logRunning {
		return
	}
	if s.logStop != nil {
		s.logStop()
		s.logStop = nil
	}
	s.logRunning = running
	if running == "" {
		return
	}
	l := s.logs
	l.lines, l.dropped, l.err, l.ended, l.match = nil, 0, nil, false, 0
	l.list = ui.ListState{}
	s.logStop = a.StreamLogs(spec, func(lines []kube.LogLine) {
		if s.logRunning != running {
			return
		}
		l.lines = append(l.lines, lines...)
		if over := len(l.lines) - kube.MaxLogLines; over > 0 {
			l.lines, l.dropped = slices.Clone(l.lines[over:]), l.dropped+over
		}
	}, func(err error) {
		if s.logRunning != running {
			return
		}
		l.err, l.ended = err, err == nil
	})
}

// stopLogs ends the log's read.
func (a *App) stopLogs() {
	if a.cluster.logStop != nil {
		a.cluster.logStop()
		a.cluster.logStop, a.cluster.logRunning = nil, ""
	}
}

// matches are the rows of the lines holding the query, any case.
func (l *logView) matches() []int {
	q := strings.ToLower(strings.TrimSpace(l.query))
	f := &l.found
	if f.query == q && f.n == len(l.lines) && f.dropped == l.dropped {
		return f.rows
	}
	f.query, f.n, f.dropped, f.rows = q, len(l.lines), l.dropped, nil
	if q == "" {
		return nil
	}
	for i, ln := range l.lines {
		if strings.Contains(strings.ToLower(ln.Text), q) {
			f.rows = append(f.rows, i)
		}
	}
	return f.rows
}

// step goes to the next match, or the one before, round the ends.
func (l *logView) step(by int) {
	rows := l.matches()
	if len(rows) == 0 {
		return
	}
	l.match = (l.match + by + len(rows)) % len(rows)
	l.list.ScrollTo(rows[l.match], ui.Center)
}

// text is the lines as the log shows them, for the clipboard.
func (l *logView) text() string {
	var b strings.Builder
	for _, ln := range l.lines {
		if l.times && !ln.At.IsZero() {
			b.WriteString(ln.At.Local().Format("15:04:05.000") + " ")
		}
		if word := levelWord(ln.Level); word != "" {
			b.WriteString(word + " ")
		}
		b.WriteString(ln.Text + "\n")
	}
	return b.String()
}

func levelWord(l kube.Level) string {
	return [...]string{"", "DEBUG", "INFO", "WARN", "ERROR"}[l]
}

// logsCrumb is the header's end while the log shows: the pod, going back
// to its details, and a tag saying it is its log.
func (a *App) logsCrumb(c *ui.Context) {
	p, _, ok := a.logPod()
	if !ok {
		return
	}
	pal := Colors(c.Theme())
	pathSlash(c)
	b := ui.ButtonBase(c.Key("logs-pod")).Height(30).Padding(0, 8).Radius(7).AlignItems(ui.Center).Gap(7).Label(p.Name).
		Tooltip(locales.KubeLogsBack())
	if b.Hovered() {
		b.Background(pal.Muted)
	}
	b.OnClick(a.closeLogs).Children(func() {
		clipped(c, p.Name).Font(FontMono).FontSize(13).MaxWidth(320)
	})
	aiTag(c, locales.KubeLogsTag(), pal.Muted, c.Theme().TextMuted)
}

// logsView is the log of a pod's container (frame 86): the container,
// its run and period, finding in it, then the lines.
func (a *App) logsView(c *ui.Context) {
	s := &a.cluster
	l := s.logs
	t := c.Theme()
	pal := Colors(t)
	p, ct, ok := a.logPod()
	if !ok {
		ui.Column(c).Gap(12).Padding(22, 28).Children(func() {
			toolNote(c, locales.KubeLogsGone())
			ui.Button(c.Key("logs-back"), locales.KubeLogsBack()).OnClick(a.closeLogs)
		})
		return
	}
	rows := l.matches()
	if l.match >= len(rows) {
		l.match = 0
	}
	ui.Column(c).Grow(1).MinHeight(0).Gap(18).Padding(22, 28, 24, 28).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			logSelect(c, "logs-container", locales.KubeLogsContainer(), l.container).PopoverMenuButton(func(m *ui.Menu) {
				for _, other := range p.Containers {
					if m.Item(other.Name).Radio(other.Name == l.container).Chosen() {
						l.container = other.Name
						if other.Last == nil {
							l.previous = false
						}
					}
				}
			})
			if ct.Last != nil {
				run := 0
				if l.previous {
					run = 1
				}
				segments(c, "logs-run", &run, []segment{{locales.KubeLogsCurrent(), ""}, {locales.KubeLogsPrevious(ct.Last.ExitCode), ""}})
				l.previous = run == 1
			}
			if !l.previous {
				period := locales.KubeLogsWholeRun()
				if l.since > 0 {
					period = locales.KubeLogsLast(span(logPeriods[l.since]))
				}
				logSelect(c, "logs-since", "", period).PopoverMenuButton(func(m *ui.Menu) {
					for i, d := range logPeriods {
						label := locales.KubeLogsWholeRun()
						if d > 0 {
							label = locales.KubeLogsLast(span(d))
						}
						if m.Item(label).Radio(l.since == i).Chosen() {
							l.since = i
						}
					}
				})
			}
			ui.Box(c).Grow(1)
			ui.SearchField(c.Key("logs-query"), &l.query).Bind(&l.queryField).Placeholder(locales.KubeLogsFind()).Width(210).
				OnChange(func() {
					l.match = 0
					if rows := l.matches(); len(rows) > 0 {
						l.list.ScrollTo(rows[0], ui.Center)
					}
				}).OnSubmit(func() { l.step(1) })
			if strings.TrimSpace(l.query) != "" {
				count := locales.KubeLogsNoMatches()
				if len(rows) > 0 {
					count = locales.KubeLogsMatches(l.match+1, len(rows))
				}
				ui.Text(c, count).FontSize(11.5).TextColor(pal.TextFaint).Shrink(0).SingleLine()
				iconButton(c, "chevron-up", locales.KubeLogsPreviousMatch()).OnClick(func() { l.step(-1) })
				iconButton(c, "chevron-down", locales.KubeLogsNextMatch()).OnClick(func() { l.step(1) })
			}
			logToggle(c, &l.times, "logs-times", "", locales.KubeLogsTime())
			logToggle(c, &l.wrap, "logs-wrap", "wrap-text", locales.KubeLogsWrap())
		})
		ui.Column(c).Grow(1).MinHeight(0).Radius(10).Clip().BorderWidth(1).BorderColor(pal.Divider).Background(pal.Subtle.Alpha(0.5)).Children(func() {
			a.logHead(c, ct)
			switch {
			case l.err != nil:
				ui.Column(c).Gap(8).Padding(16).Children(func() {
					ui.Text(c, locales.KubeLogsFailed()).FontSize(13).FontWeight(500).TextColor(pal.DangerText)
					ui.Text(c, l.err.Error()).Font(FontMono).FontSize(11.5).Padding(9, 12).Radius(8).Background(t.Background).
						BorderWidth(1).BorderColor(pal.Line).Wrap().Selectable()
				})
			case len(l.lines) == 0 && !l.ended:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(14, 16).Children(func() {
					ui.Spinner(c).Size(14, 14)
					ui.Text(c, locales.KubeLogsReading()).FontSize(12.5).TextColor(t.TextMuted)
				})
			case len(l.lines) == 0:
				ui.Text(c, locales.KubeLogsEmpty()).FontSize(12.5).TextColor(t.TextMuted).Padding(14, 16)
			default:
				a.logLines(c, rows)
			}
		})
	})
}

// logHead says which run the log is of and how it went, and how many
// lines it has.
func (a *App) logHead(c *ui.Context, ct kube.Container) {
	l := a.cluster.logs
	t := c.Theme()
	pal := Colors(t)
	tag, text := locales.KubeLogsTagCurrent(), ""
	at := func(tm time.Time) string { return tm.Local().Format("15:04:05") }
	switch {
	case l.previous && ct.Last != nil:
		tag = locales.KubeLogsTagPrevious()
		last := ct.Last
		text = locales.KubeLogsPreviousRun(ct.Name, span(last.Finished.Sub(last.Started)), at(last.Started), at(last.Finished), last.ExitCode, ct.Restarts+1)
	case ct.State == "running":
		text = locales.KubeLogsRunning(ct.Name, at(ct.Started))
	case ct.State == "terminated" && ct.Ended != nil:
		text = locales.KubeLogsEnded(ct.Name, ct.Ended.ExitCode)
	default:
		text = locales.KubeLogsWaiting(ct.Name, ct.Reason)
	}
	ui.Row(c).Gap(8).Padding(10, 14, 10, 16).AlignItems(ui.Center).Background(t.Background).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		aiTag(c, tag, pal.Muted, t.TextMuted).Shrink(0)
		ui.Text(c, text).FontSize(12.5).Grow(1).MinWidth(0).Wrap()
		if l.dropped > 0 {
			ui.Text(c, locales.KubeLogsDropped()).FontSize(11).TextColor(pal.TextFaint).Shrink(0).SingleLine()
		}
		ui.Text(c, locales.KubeLogsLines(len(l.lines))).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Shrink(0).SingleLine()
	})
}

// logLines are the lines, the end followed as they come unless a search
// shows its matches; the rows holding one are marked, the current one
// framed.
func (a *App) logLines(c *ui.Context, rows []int) {
	l := a.cluster.logs
	t := c.Theme()
	pal := Colors(t)
	current := -1
	if len(rows) > 0 {
		current = rows[l.match]
	}
	end := ""
	switch {
	case l.previous && l.ended:
		end = locales.KubeLogsPreviousEnd()
	case l.ended:
		end = locales.KubeLogsStreamEnded()
	}
	n := len(l.lines)
	if end != "" {
		n++ // the end says why the lines end, after the last
	}
	l.list.FollowEnd = strings.TrimSpace(l.query) == ""
	ui.List(c.Key("log-lines"), &l.list, n, func(i int) {
		if i == len(l.lines) {
			ui.Text(c, end).FontSize(12).TextColor(t.TextMuted).Padding(12, 16, 6, 140).Wrap()
			return
		}
		ln := l.lines[i]
		bg, fg := ui.Transparent, pal.TextFaint
		switch ln.Level {
		case kube.LevelWarn:
			bg, fg = pal.WarningSoft.Alpha(0.55), pal.WarningText
		case kube.LevelError:
			bg, fg = pal.DangerSoft.Alpha(0.6), pal.DangerText
		}
		row := ui.Row(c).Gap(14).Padding(3, 16).AlignItems(ui.Start).Background(bg)
		if i == current {
			row.BorderWidth(1, 0, 1, 0).BorderColor(t.Danger)
		}
		row.Children(func() {
			ui.Row(c).Width(40).Height(17).Shrink(0).AlignItems(ui.Center).Children(func() {
				if _, found := slices.BinarySearch(rows, i); found {
					ui.Box(c).Size(7, 7).Radius(4).Background(t.Danger)
				} else {
					ui.Text(c, strconv.Itoa(l.dropped+i+1)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
				}
			})
			if l.times {
				at := ""
				if !ln.At.IsZero() {
					at = ln.At.Local().Format("15:04:05.000")
				}
				ui.Text(c, at).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).Width(92).Shrink(0).SingleLine()
			}
			ui.Text(c, levelWord(ln.Level)).Font(FontMono).FontSize(11.5).TextColor(fg).Width(42).Shrink(0).SingleLine()
			text := ui.Text(c, ln.Text).Font(FontMono).FontSize(11.5).Grow(1).MinWidth(0).Selectable()
			if ln.Level == kube.LevelError {
				text.TextColor(pal.DangerText)
			}
			if l.wrap {
				text.Wrap()
			} else {
				text.SingleLine()
			}
		})
	}).Grow(1).Padding(6, 0)
}

// logSelect is a select of the log's controls: a faint label, the value
// and a chevron.
func logSelect(c *ui.Context, key, label, value string) ui.Element {
	t := c.Theme()
	pal := Colors(t)
	b := ui.ButtonBase(c.Key(key)).Height(26).Padding(0, 8, 0, 9).Radius(7).AlignItems(ui.Center).Gap(5).
		BorderWidth(1).BorderColor(pal.ControlBorder).Label(strings.TrimSpace(label + " " + value))
	if b.Hovered() {
		b.Background(pal.Muted)
	}
	return b.Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(12).TextColor(pal.TextFaint).SingleLine()
		}
		ui.Text(c, value).FontSize(12).FontWeight(500).SingleLine()
		ui.Icon(c, Icon("chevron-down")).Size(11, 11).TextColor(pal.TextFaint)
	})
}

// logToggle is a toggle of the log's controls, dark while on.
func logToggle(c *ui.Context, on *bool, key, icon, label string) {
	t := c.Theme()
	pal := Colors(t)
	b := ui.ToggleBase(c.Key(key), on).Height(26).Padding(0, 10).Radius(13).AlignItems(ui.Center).Gap(6).
		BorderWidth(1).BorderColor(pal.ControlBorder).Label(label)
	fg := t.Text
	switch {
	case *on:
		b.Background(t.Text).BorderColor(t.Text)
		fg = t.Background
	case b.Hovered():
		b.Background(pal.Muted)
	}
	b.Children(func() {
		if icon != "" {
			ui.Icon(c, Icon(icon)).Size(13, 13).TextColor(fg)
		}
		ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(fg).SingleLine()
	})
}
