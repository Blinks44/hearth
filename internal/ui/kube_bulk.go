package ui

import (
	"cmp"
	"context"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type resourceSelection struct {
	kind     int
	selected map[string]bool
	busy     bool
	revision int
}

func (s *resourceSelection) clear() {
	s.selected, s.busy = nil, false
	s.revision++
}

func (s *resourceSelection) retain(cl kube.Cluster) {
	if len(s.selected) == 0 {
		return
	}
	visible := make(map[string]bool, len(s.selected))
	switch s.kind {
	case kindPods:
		for _, p := range cl.Pods {
			if s.selected[p.UID] {
				visible[p.UID] = true
			}
		}
	case kindDeployments:
		for _, d := range cl.Deployments {
			if s.selected[d.UID] {
				visible[d.UID] = true
			}
		}
	}
	if len(visible) != len(s.selected) {
		s.selected, s.busy = visible, false
		s.revision++
	}
}

func (s *resourceSelection) checkbox(c *ui.Context, key, label string, ids []string) {
	state, count := ui.CheckOff, 0
	for _, id := range ids {
		if s.selected[id] {
			count++
		}
	}
	switch {
	case count == 0:
	case count == len(ids):
		state = ui.CheckOn
	default:
		state = ui.CheckMixed
	}
	ui.CheckboxState(c.Key(key), &state, "").Label(label).Disabled(len(ids) == 0).OnClick(func() {
		if s.selected == nil {
			s.selected = make(map[string]bool)
		}
		for _, id := range ids {
			if state == ui.CheckOn {
				s.selected[id] = true
			} else {
				delete(s.selected, id)
			}
		}
		s.busy = false
		s.revision++
	})
}

type kubeBulkTarget struct {
	ref     ops.KubeResource
	restart bool
}

func (a *App) kubeBulkBar(c *ui.Context, targets []kubeBulkTarget) {
	if len(targets) == 0 || a.BuildPlan == nil {
		return
	}
	t := c.Theme()
	ns := make(map[string]bool)
	refs := make([]ops.KubeResource, 0, len(targets))
	restartable := true
	for _, target := range targets {
		ns[target.ref.Namespace] = true
		refs = append(refs, target.ref)
		restartable = restartable && target.restart
	}
	slices.SortFunc(refs, func(x, y ops.KubeResource) int {
		if n := cmp.Compare(x.Namespace, y.Namespace); n != 0 {
			return n
		}
		return cmp.Compare(x.Name, y.Name)
	})
	restart := locales.KubeBulkRestart()
	if refs[0].Kind == "Pod" {
		restart = locales.KubeBulkRecreate()
	}
	s := &a.cluster.bulk
	blocked := s.busy || a.op.running()
	caption := locales.KubeBulkSelected(len(refs), len(ns))
	bg, fg := ui.RGB(31, 29, 26), ui.RGB(250, 249, 247)
	ui.Row(c.Key("bulk-bar-position")).Absolute().Left(0).Right(0).Bottom(8).Justify(ui.Center).Children(func() {
		ui.Row(c.Key("bulk-bar")).MinHeight(40).Padding(4, 8, 4, 14).Gap(6).AlignItems(ui.Center).
			Radius(12).Background(bg).Shadow(0, 5, 18, 0, ui.RGBA(0, 0, 0, 0.18)).Children(func() {
			ui.Text(c, caption).FontSize(12.5).TextColor(fg).SingleLine()
			ui.Box(c).Size(1, 18).Background(fg.Alpha(0.2)).Margin(0, 3)
			bulkAction(c.Key("bulk-restart"), "refresh-cw", restart+"…", fg, ui.Transparent, blocked || !restartable, func() {
				a.planKubeBulk(refs, true)
			})
			bulkAction(c.Key("bulk-delete"), "trash-2", locales.KubeBulkDelete()+"…", ui.RGB(255, 151, 140),
				t.Danger.Alpha(0.18), blocked, func() { a.planKubeBulk(refs, false) })
			bulkAction(c.Key("bulk-clear"), "", locales.KubeBulkClear(), fg, ui.Transparent, false, s.clear)
		})
	})
}

func bulkAction(c *ui.Context, icon, label string, fg, bg ui.Color, disabled bool, click func()) {
	button := ui.ButtonBase(c).Height(28).Padding(0, 8).Radius(6).Gap(6).AlignItems(ui.Center).
		Label(label).Disabled(disabled).OnClick(click)
	if button.Hovered() && !disabled {
		bg = fg.Alpha(0.16)
	}
	if disabled {
		fg = fg.Alpha(0.5)
	}
	button.Background(bg).Children(func() {
		if icon == "" {
			ui.Text(c, "Esc").FontSize(11).TextColor(fg.Alpha(0.6))
			return
		}
		ui.Icon(c, Icon(icon)).Size(14, 14).TextColor(fg)
		ui.Text(c, label).FontSize(12).TextColor(fg).SingleLine()
	})
	if icon == "" {
		button.Tooltip(label)
	}
}

func (a *App) planKubeBulk(refs []ops.KubeResource, restart bool) {
	s := &a.cluster.bulk
	s.busy = true
	revision := s.revision
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanKubeBulk(context.Background(), run.Exec{}, env, refs, restart)
	}, func(p ops.Plan, err error) {
		if s.revision != revision || a.router.Path() != kubeClusterPage || a.router.Query("context") != refs[0].Context {
			return
		}
		s.busy = false
		a.proposeOrSay(p, err)
	})
}
