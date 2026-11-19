package ui

import (
	"context"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) deploymentRevisions(c *ui.Context, ref ops.KubeResource, d kube.Deployment) {
	t := c.Theme()
	pal := Colors(t)
	detailSection(c, locales.KubeRevisions(len(d.Revisions)))
	if len(d.Revisions) == 0 {
		ui.Text(c, locales.KubeNoRevisions()).FontSize(12).TextColor(t.TextMuted).Padding(4, 16)
		return
	}
	tracks := []ui.Track{ui.Fixed(30), ui.Fr(1), ui.Fixed(72), ui.Fixed(100)}
	ui.Column(c.Key("deployment-revisions")).Margin(0, 16, 6, 16).Radius(10).BorderWidth(1).BorderColor(pal.Line).Children(func() {
		for i, revision := range d.Revisions {
			row := resourceGrid(ui.Box(c.Key("revision-"+revision.Name)), tracks).Padding(4, 10).MinHeight(36)
			if i+1 < len(d.Revisions) {
				row.BorderWidth(0, 0, 1, 0).BorderColor(pal.Line)
			}
			row.Children(func() {
				ui.Text(c, "#"+strconv.Itoa(revision.Number)).Font(FontMono).FontSize(12).TextColor(pal.TextFaint).SingleLine()
				clipped(c, strings.Join(revision.Images, ", ")).Font(FontMono).FontSize(12)
				clipped(c, age(c, revision.Created)).FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Justify(ui.End).AlignItems(ui.Center).Children(func() {
					switch {
					case revision.Number > 0 && revision.Number == d.Revision:
						aiTag(c, locales.KubeCurrent(), pal.SuccessSoft, pal.SuccessText)
					case a.BuildPlan != nil && ref.UID != "" && revision.UID != "" && revision.Number > 0:
						a.deploymentRollbackButton(c, ref, revision, d.Paused)
					default:
						ui.Text(c, locales.KubePodCount(revision.Pods)).FontSize(12).TextColor(pal.TextFaint).SingleLine()
					}
				})
			})
		}
	})
}

func (a *App) deploymentRollbackButton(c *ui.Context, ref ops.KubeResource, revision kube.Revision, paused bool) {
	f := a.cluster.resources[ref]
	button := ui.Button(c.Key("deployment-rollback"), "").Height(24).Label(locales.KubeRollbackRevision(revision.Number)).
		Disabled(paused || (f != nil && f.busy) || a.op.running()).
		OnClick(func() { a.planRollback(a.resourceForm(ref), revision) })
	button.Children(func() { ui.Text(c, locales.KubeRollbackOpen()).FontSize(12).SingleLine() })
	if paused {
		button.Tooltip(locales.KubeRollbackPaused())
	}
}

func (a *App) planRollback(f *kubeResourceForm, revision kube.Revision) {
	if f.busy || a.op.running() {
		return
	}
	f.busy = true
	ref := f.ref
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanKubeRollback(context.Background(), run.Exec{}, env, ref, revision.Name, revision.UID, revision.Number)
	}, func(p ops.Plan, err error) {
		f.busy = false
		s := &a.cluster
		if !a.resourceFormCurrent(f) || s.kind != kindDeployments || s.deployment != ref.UID || s.tab != 0 {
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
