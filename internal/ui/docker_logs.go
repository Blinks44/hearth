package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/locales"
)

type dockerLogState struct {
	logView
	name    string
	targets []docker.LogTarget
	key     string
	stop    func()
}

func (a *App) openDockerLogs(name string, targets []docker.LogTarget, combined bool) {
	s := &a.engine.containers
	s.closeLogs()
	s.detail.secret.reset()
	if s.detail.stop != nil {
		s.detail.stop()
		s.detail.stop, s.detail.request, s.detail.key = nil, nil, ""
	}
	s.logs = &dockerLogState{name: name, targets: targets}
	s.logs.times, s.logs.combined = true, combined
}

func (s *dockerContainersState) closeLogs() {
	if l := s.logs; l != nil && l.stop != nil {
		l.stop()
	}
	s.logs = nil
}

func (a *App) syncDockerLogs() {
	s := &a.engine.containers
	l := s.logs
	if l == nil || a.StreamDockerLogs == nil || s.watching == "" {
		return
	}
	key := fmt.Sprintf("%s/%d", s.watching, l.since)
	if l.key == key {
		return
	}
	if l.stop != nil {
		l.stop()
	}
	l.key = key
	l.lines, l.dropped, l.err, l.ended, l.match = nil, 0, nil, false, 0
	l.list = ui.ListState{}
	l.run++
	l.revision++
	generation := l.run
	spec := docker.Logs{Target: s.target, Containers: l.targets, Since: logPeriods[l.since]}
	l.stop = a.StreamDockerLogs(spec, func(lines []containerlog.LogLine) {
		if s.logs == l && l.run == generation {
			l.addLines(lines)
		}
	}, func(err error) {
		if s.logs == l && l.run == generation {
			l.err, l.ended = err, err == nil
		}
	})
}

func (a *App) dockerLogsView(c *ui.Context, l *dockerLogState) {
	rows := l.matches()
	if l.match >= len(rows) {
		l.match = 0
	}
	ui.Column(c).Grow(1).MinHeight(0).Gap(18).Padding(22, 28, 24, 28).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			l.periodControl(c)
			l.searchControls(c, rows)
		})
		a.logPanel(c, &l.logView, rows, locales.DockerLogsFailed(), func() {
			t := c.Theme()
			pal := Colors(t)
			ui.Row(c).Gap(8).Padding(10, 14, 10, 16).AlignItems(ui.Center).Background(t.Background).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, l.name).FontSize(12.5).Grow(1).MinWidth(0).Wrap()
				if l.dropped > 0 {
					ui.Text(c, locales.KubeLogsDropped()).FontSize(11).TextColor(pal.TextFaint).Shrink(0).SingleLine()
				}
				ui.Text(c, locales.KubeLogsLines(len(l.lines))).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).Shrink(0).SingleLine()
			})
		})
	})
}
