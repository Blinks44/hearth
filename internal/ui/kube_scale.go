package ui

import (
	"context"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type scaleForm struct {
	ref  ops.KubeResource
	want int
	busy bool
}

func (a *App) planScale(f *scaleForm, from int) {
	if f.busy || a.op.running() {
		return
	}
	f.busy = true
	ref, to := f.ref, f.want
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanKubeScale(context.Background(), run.Exec{}, env, ref, from, to)
	}, func(p ops.Plan, err error) {
		f.busy = false
		s := &a.cluster
		if s.scale != f || a.router.Path() != kubeClusterPage || a.router.Query("context") != ref.Context ||
			s.kind != kindDeployments || s.deployment != ref.UID || s.tab != 0 {
			return
		}
		if err != nil {
			s.refresh++
		}
		a.proposeThen(func() {
			if s.scale == f {
				s.scale = nil
			}
			if a.router.Path() == kubeClusterPage && a.router.Query("context") == ref.Context {
				s.refresh++
			}
		})(p, err)
	})
}

func (a *App) deploymentScale(c *ui.Context, ref ops.KubeResource, d kube.Deployment) {
	if a.BuildPlan == nil || ref.UID == "" {
		return
	}
	s := &a.cluster
	want, busy := d.Replicas, false
	f := s.scale
	if f != nil && f.ref == ref {
		want, busy = f.want, f.busy
	}
	disabled := busy || a.op.running()
	set := func(n int) { s.scale = &scaleForm{ref: ref, want: n} }
	t := c.Theme()
	pal := Colors(t)
	previous := locales.KubeScaleBefore(d.Replicas)
	previousWidth, _ := c.MeasureText(0, ui.Span{Text: previous, Size: 11})
	detailSection(c, locales.KubeScaleSection())
	ui.Row(c.Key("deployment-scale")).Gap(10).Wrap().AlignItems(ui.Center).Margin(0, 16, 6, 16).Padding(10, 12).
		Radius(10).BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(84).Gap(2).Children(func() {
			ui.Text(c, locales.KubeReplicas()).FontSize(13).FontWeight(500)
			ui.Text(c, locales.KubeScaleNow(d.Replicas)).FontSize(12).TextColor(t.TextMuted).Wrap()
		})
		ui.Row(c).Gap(9).Shrink(0).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Height(podActionHeight).Radius(6).Clip().BorderWidth(1).BorderColor(pal.ControlBorder).AlignItems(ui.Center).
				Children(func() {
					step := func(key, icon, label string, off bool, to int) {
						b := ui.ButtonBase(c.Key(key)).Size(28, podActionHeight).Center().Label(label).Tooltip(label).
							Disabled(disabled || off).OnClick(func() { set(to) })
						color := t.Text
						switch {
						case disabled || off:
							color = pal.TextFaint
						case b.Hovered():
							b.Background(pal.Muted)
						}
						b.Children(func() { ui.Icon(c, Icon(icon)).Size(13, 13).TextColor(color) })
					}
					step("replicas-less", "minus", locales.KubeScaleLess(), want <= 0, want-1)
					ui.Row(c).Height(podActionHeight).MinWidth(36).Padding(0, 8).AlignItems(ui.Center).Justify(ui.Center).
						BorderWidth(0, 1, 0, 1).BorderColor(pal.ControlBorder).Children(func() {
						ui.Text(c, strconv.Itoa(want)).Font(FontMono).FontSize(12.5).SingleLine()
					})
					step("replicas-more", "plus", locales.KubeScaleMore(), want >= ops.KubeMaxReplicas, want+1)
				})
			ui.Row(c).Width(previousWidth + 2).Shrink(0).Children(func() {
				if want != d.Replicas {
					ui.Text(c, previous).FontSize(11).TextColor(pal.TextFaint).SingleLine()
				}
			})
			ui.PrimaryButton(c.Key("replicas-plan"), locales.KubeScaleApply()).Height(podActionHeight).
				Disabled(disabled || want == d.Replicas).OnClick(func() { a.planScale(f, d.Replicas) })
		})
	})
}
