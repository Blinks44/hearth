package ui

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type kubeApplyDialog struct {
	open, busy       bool
	root, title, cli string
	detected         []string
	source           ops.KubeManifestSource
	err              string
}

func (a *App) openProjectApply(p projects.Project, cli, target, namespace string) {
	d := &kubeApplyDialog{
		open: true, root: p.Dir, title: filepath.Base(p.Dir), cli: cli,
		source: ops.KubeManifestSource{Context: target, Namespace: namespace},
	}
	if p.Dir == "" {
		d.title = target
	}
	for _, entry := range p.Kube {
		switch entry {
		case "Chart.yaml", "helmfile.yaml", "charts/", "helm/", "Tiltfile", "skaffold.yaml":
			continue
		case "kustomization.yaml", "kustomization.yml", "Kustomization":
			d.detected = append(d.detected, p.Dir)
		default:
			d.detected = append(d.detected, filepath.Join(p.Dir, entry))
		}
	}
	if len(d.detected) > 0 {
		d.source.Path = d.detected[0]
	}
	a.kubeApply = d
}

func (a *App) kubeApplyView(c *ui.Context) {
	d := a.kubeApply
	if d == nil {
		return
	}
	t, pal := c.Theme(), Colors(c.Theme())
	dialogBase(c.Key("kube-apply-source"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Children(func() {
			ui.Column(c).Gap(8).Padding(20).Children(func() {
				ui.Text(c, locales.KubeApplyTitle(d.title)).FontSize(18).FontWeight(600)
				ui.Text(c, locales.KubeLinkName(d.source.Context, d.source.Namespace)).FontSize(12).TextColor(t.TextMuted).Wrap()
				sources := slices.Clone(d.detected)
				if d.source.Path != "" && !slices.Contains(sources, d.source.Path) {
					sources = append(sources, d.source.Path)
				}
				if len(sources) > 0 {
					ui.Select(c.Key("kube-apply-detected"), &d.source.Path, sources).Label(locales.KubeApplySource()).Disabled(d.busy)
				}
				ui.Row(c).Gap(8).Children(func() {
					if a.PickManifest != nil {
						ui.Button(c.Key("kube-apply-file"), locales.KubeApplyChooseFile()).Disabled(d.busy).OnClick(func() { a.pickManifest(d, false) })
						ui.Button(c.Key("kube-apply-folder"), locales.KubeApplyChooseFolder()).Disabled(d.busy).OnClick(func() { a.pickManifest(d, true) })
					}
				})
				if d.busy {
					ui.Text(c, locales.KubeApplyChecking()).FontSize(12).TextColor(t.TextMuted)
				} else if d.err != "" {
					ui.Text(c, d.err).FontSize(12).TextColor(pal.DangerText).Wrap()
				}
			})
			ui.Row(c).Gap(8).Padding(12, 20).Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Button(c, locales.Cancel()).OnClick(func() { d.open = false })
				ui.PrimaryButton(c.Key("kube-apply-review"), locales.KubeResourceCheck()).Disabled(d.busy || d.source.Path == "").
					OnClick(func() { a.reviewManifests(d) })
			})
		})
	})
}

func (a *App) pickManifest(d *kubeApplyDialog, folder bool) {
	a.PickManifest(d.root, folder, func(path string, err error) {
		if a.kubeApply != d || !d.open {
			return
		}
		if err != nil {
			d.err = err.Error()
		} else if path != "" {
			d.source.Path, d.err = path, ""
		}
	})
}

func (a *App) reviewManifests(d *kubeApplyDialog) {
	source, cli, title := d.source, d.cli, d.title
	d.busy = true
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		return ops.PlanKubeManifests(ctx, run.Exec{}, env, cli, title, source)
	}, func(p ops.Plan, err error) {
		d.busy = false
		if a.kubeApply != d || !d.open {
			return
		}
		if err != nil {
			d.err = err.Error()
			return
		}
		d.open = false
		if a.op.running() {
			a.op.open = true
			return
		}
		a.Propose(p)
		a.op.after = func() { a.cluster.refresh++ }
		a.op.chooseManifest = func() {
			a.op.open = false
			d.err, d.open = "", true
		}
	})
}

func (a *App) manifestPreview(c *ui.Context, op *operation, p *ops.KubeManifestPreview) {
	pal := Colors(c.Theme())
	flag := "-f"
	if p.Kustomize {
		flag = "-k"
	}
	path := tilde(p.Path, a.Home)
	ui.Row(c).Gap(8).Padding(0, 20, 12, 20).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).Padding(6, 8).AlignItems(ui.Center).Radius(6).BorderWidth(1).BorderColor(pal.Line).Children(func() {
			ui.Icon(c, Icon("folder-tree")).Size(14, 14)
			clipped(c, flag+" "+path).FontSize(12).Grow(1).MinWidth(0).Tooltip(p.Path)
		})
		if op.chooseManifest != nil {
			ui.Button(c.Key("kube-apply-other"), locales.KubeApplyOther()).OnClick(op.chooseManifest)
		}
	})
	dialogSection(c, strings.ToUpper(locales.KubeApplySummary(len(p.Changed), len(p.Created), len(p.Unchanged))))
	for _, change := range p.Changed {
		if len(change.Lines) > 0 {
			operationDiffCard(c, change.Resource, change.Lines)
		} else {
			listRow(c, change.Resource, true, locales.KubeApplySecretDiff(), func() {})
		}
	}
	if len(p.Created) > 0 {
		dialogSection(c, strings.ToUpper(locales.KubeApplyCreates()))
		for _, label := range p.Created {
			ui.Text(c, label).Font(FontMono).FontSize(12).Wrap().Padding(2, 20)
		}
	}
	ui.Column(c).Gap(4).Padding(12, 20, 14, 20).Children(func() {
		ui.Text(c, locales.KubeApplyNoPrune()).FontSize(12).Wrap()
		ui.Text(c, locales.KubeApplyVerification()).FontSize(11.5).TextColor(pal.TextFaint).Wrap()
	})
}
