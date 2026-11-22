package ui

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const (
	helmOverview = iota
	helmValues
	helmHistory
	helmManifest
)

type helmDetails struct {
	release kube.HelmRelease
	target  kube.HelmTarget
	history []kube.HelmRevision
	readErr error
	tab     int
	content *helmContent
	stop    func()
	busy    bool
	planErr error
}

type helmContent struct {
	text     string
	manifest bool
	err      error
	stop     func()
	done     bool
}

func (d *helmDetails) clearContent() {
	if d.content != nil && d.content.stop != nil {
		d.content.stop()
	}
	d.content = nil
}

func (s *kubeClusterState) clearHelmDetails() {
	if d := s.helmDetail; d != nil {
		if d.stop != nil {
			d.stop()
		}
		d.clearContent()
		s.helmDetail = nil
	}
}

func (a *App) selectHelm(r kube.HelmRelease) {
	helm, ok := cliCopy(a.cliState(), "helm")
	if !ok || a.ReadHelmHistory == nil {
		return
	}
	d := &helmDetails{release: r, target: kube.HelmTarget{
		Helm: helm.Path, Context: a.router.Query("context"), Namespace: r.Namespace, Name: r.Name,
	}}
	a.cluster.helmDetail = d
	d.stop = a.ReadHelmHistory(d.target, func(history []kube.HelmRevision, err error) {
		if a.cluster.helmDetail != d {
			return
		}
		if err == nil && (len(history) == 0 || strconv.Itoa(history[0].Revision) != r.Revision || history[0].Status != r.Status) {
			err = errors.New(locales.KubeHelmChanged())
		}
		d.history, d.readErr = history, err
	})
}

func (a *App) syncHelmRelease(releases []kube.HelmRelease) {
	d := a.cluster.helmDetail
	if d == nil {
		return
	}
	i := slices.IndexFunc(releases, func(r kube.HelmRelease) bool { return helmKey(r) == helmKey(d.release) })
	if i == -1 {
		a.cluster.clearHelmDetails()
	} else if r := releases[i]; r.Revision != d.release.Revision || r.Status != d.release.Status || r.Updated != d.release.Updated {
		a.cluster.clearHelmDetails()
		a.selectHelm(r)
	}
}

func (a *App) syncHelmContent() {
	d := a.cluster.helmDetail
	if d == nil {
		return
	}
	if d.tab != helmValues && d.tab != helmManifest {
		d.clearContent()
		return
	}
	manifest := d.tab == helmManifest
	if d.content != nil && d.content.manifest != manifest {
		d.clearContent()
	}
	if d.content != nil || len(d.history) == 0 || d.readErr != nil || a.ReadHelmContent == nil {
		return
	}
	request := &helmContent{manifest: manifest}
	d.content = request
	request.stop = a.ReadHelmContent(d.target, d.history[0].Revision, manifest, func(text string, err error) {
		if a.cluster.helmDetail == d && d.content == request {
			request.text, request.err, request.done = text, err, true
		}
	})
}

func (a *App) planHelm(d *helmDetails, target int) {
	kubectl, ok := cliCopy(a.cliState(), "kubectl")
	if !ok || a.BuildPlan == nil || len(d.history) == 0 || d.busy || a.op.running() {
		return
	}
	d.busy, d.planErr = true, nil
	ref, current := d.target, d.history[0]
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		return ops.PlanKubeHelm(ctx, run.Exec{}, env, kubectl.Path, ref, current, target)
	}, func(p ops.Plan, err error) {
		d.busy, d.planErr = false, err
		if err != nil || a.cluster.helmDetail != d || a.op.running() {
			return
		}
		a.Propose(p)
		a.op.after = func() { a.cluster.refresh++ }
	})
}

func (a *App) helmDetailsView(c *ui.Context, d *helmDetails) {
	t := c.Theme()
	pal := Colors(t)
	r := d.release
	_, hasKubectl := cliCopy(a.cliState(), "kubectl")
	blocked := !hasKubectl || a.BuildPlan == nil || a.cluster.err != nil || a.op.running() || d.busy || d.readErr != nil ||
		len(d.history) == 0 || r.Pending() || r.Status == "uninstalling"
	var previous int
	if len(d.history) != 0 {
		for _, v := range d.history[1:] {
			if v.CanRollback(d.history[0]) {
				previous = v.Revision
				break
			}
		}
	}
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Column(c).Gap(7).Padding(13, 16).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, Icon("layers")).Size(18, 18).TextColor(t.TextMuted).Shrink(0)
				clipped(c, r.Name).FontSize(17).FontWeight(600).Tooltip(r.Name)
				helmStatus(c, r.Status)
			})
			subtitle := strings.Join([]string{r.Namespace, r.Chart, cmp.Or(r.AppVersion, "—"), locales.KubeResourceVersion(r.Revision)}, " · ")
			clipped(c, subtitle).FontSize(12).TextColor(t.TextMuted).Tooltip(subtitle)
			ui.Row(c).Gap(6).Wrap().Children(func() {
				kubeActionButton(c.Key("helm-rollback"), "history", locales.KubeRollbackOpen(), podActionHeight).
					Disabled(blocked || previous == 0).Tooltip(locales.KubeHelmRollbackTo(previous)).
					OnClick(func() { a.planHelm(d, previous) })
				ui.Box(c).Grow(1)
				kubeActionButton(c.Key("helm-uninstall"), "trash-2", locales.KubeHelmDeleteOpen(), podActionHeight).
					TextColor(t.Danger).Disabled(blocked).OnClick(func() { a.planHelm(d, 0) })
			})
		})
		ui.Row(c).Padding(10, 16, 0, 16).Shrink(0).Children(func() {
			count := ""
			if len(d.history) > 0 {
				count = strconv.Itoa(len(d.history))
			}
			segments(c, "helm-tabs", &d.tab, []segment{
				{locales.KubeTabOverview(), ""}, {locales.KubeHelmValues(), ""}, {locales.KubeHelmHistory(), count},
				{locales.KubeHelmManifest(), ""},
			})
		})
		ui.Scroll(c.Key("helm-detail-tab-" + strconv.Itoa(d.tab))).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(10).Padding(12, 16, 16, 16).Children(func() {
				if d.planErr != nil {
					toolNote(c, d.planErr.Error())
				}
				switch {
				case d.readErr != nil:
					toolNote(c, a.helmReadError(d.readErr))
					ui.Button(c.Key("helm-retry"), locales.KubeResourceReload()).OnClick(func() {
						a.cluster.clearHelmDetails()
						a.selectHelm(r)
					})
				case len(d.history) == 0:
					ui.Spinner(c).Size(16, 16)
				case d.tab == helmValues || d.tab == helmManifest:
					a.helmContentView(c, d)
				default:
					if d.tab == helmOverview {
						a.helmOverviewView(c, d, previous, blocked)
					}
					revisions := d.history
					if d.tab == helmOverview {
						revisions = revisions[:min(3, len(revisions))]
					}
					a.helmRevisions(c, d, revisions, blocked)
				}
			})
		})
	})
}

func (a *App) helmReadError(err error) string {
	if _, ok := errors.AsType[*kube.Error](err); ok {
		return a.accessWords(err)
	}
	if errors.Is(err, kube.ErrHelmReleaseMissing) {
		return locales.KubeHelmCheckMissing()
	}
	return err.Error()
}

func (a *App) helmOverviewView(c *ui.Context, d *helmDetails, previous int, blocked bool) {
	current := d.history[0]
	pal := Colors(c.Theme())
	if current.Status == "failed" && current.Description != "" {
		ui.Row(c).Padding(10).Radius(8).Background(pal.DangerSoft).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Icon(c, Icon("triangle-alert")).Size(15, 15).TextColor(pal.DangerText)
			ui.Text(c, current.Description).FontSize(12).TextColor(pal.DangerText).Grow(1).MinWidth(110).Wrap().Selectable()
			if previous != 0 {
				kubeActionButton(c.Key("helm-previous"), "history", locales.KubeHelmRollbackTo(previous), podActionHeight).
					Disabled(blocked).OnClick(func() { a.planHelm(d, previous) })
			}
		})
	}
	nodeFact(c, locales.KubeHelmChart(), current.Chart)
	nodeFact(c, locales.KubeHelmAppVersion(), cmp.Or(current.AppVersion, "—"))
	nodeFact(c, locales.KubeHelmUpdated(), helmUpdated(current.Updated))
}

func (a *App) helmRevisions(c *ui.Context, d *helmDetails, rows []kube.HelmRevision, blocked bool) {
	t := c.Theme()
	pal := Colors(t)
	dialogSection(c, strings.ToUpper(locales.KubeHelmHistory()))
	ui.Column(c).Radius(8).BorderWidth(1).BorderColor(pal.Line).Clip().Children(func() {
		for _, v := range rows {
			ui.Row(c.Key(strconv.Itoa(v.Revision))).AlignItems(ui.Center).Gap(8).Padding(6, 8).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, "#"+strconv.Itoa(v.Revision)).Width(30).Font(FontMono).FontSize(11).TextColor(t.TextMuted)
				ui.Column(c).Gap(2).MinWidth(0).Grow(1).Children(func() {
					text := v.Status + " · " + v.Chart
					clipped(c, text).FontSize(12).Tooltip(text)
					text = strings.Trim(strings.Join([]string{helmUpdated(v.Updated), v.Description}, " · "), " ·")
					clipped(c, text).FontSize(11).TextColor(t.TextMuted).Tooltip(text)
				})
				if v.CanRollback(d.history[0]) {
					kubeActionButton(c.Key("helm-revision-rollback"), "history", locales.KubeRollbackOpen(), podActionHeight).
						Tooltip(locales.KubeHelmRollbackTo(v.Revision)).Disabled(blocked).OnClick(func() { a.planHelm(d, v.Revision) })
				}
			})
		}
	})
}

func helmUpdated(updated string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, updated); err == nil {
		return parsed.Local().Format("2006-01-02 15:04")
	}
	return cmp.Or(updated, "—")
}

func (a *App) helmContentView(c *ui.Context, d *helmDetails) {
	part := d.content
	switch {
	case a.ReadHelmContent == nil:
		return
	case part == nil || !part.done:
		ui.Spinner(c).Size(16, 16)
	case part.err != nil:
		toolNote(c, a.helmReadError(part.err))
		ui.Button(c.Key("helm-content-retry"), locales.KubeResourceReload()).OnClick(d.clearContent)
	default:
		text := strings.TrimRight(part.text, "\r\n")
		if text == "" || d.tab == helmValues && (text == "null" || text == "{}") {
			toolNote(c, locales.KubeHelmContentEmpty())
			return
		}
		ui.Text(c, text).Font(FontMono).FontSize(12).Wrap().Selectable()
	}
}
