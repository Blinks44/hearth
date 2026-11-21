package ui

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type kubeResourceForm struct {
	ref      ops.KubeResource
	draft    ops.KubeDraft
	editor   *editor.State
	previous string
	readAt   time.Time
	busy     bool
	open     bool
	err      error
}

func (a *App) resourceRef(kind, namespace, name, uid string) ops.KubeResource {
	in, _ := cliCopy(a.cliState(), "kubectl")
	return ops.KubeResource{Kubectl: in.Path, Context: a.router.Query("context"), Kind: kind, Namespace: namespace, Name: name, UID: uid}
}

func (a *App) resourceForm(ref ops.KubeResource) *kubeResourceForm {
	s := &a.cluster
	if s.resources == nil {
		s.resources = make(map[ops.KubeResource]*kubeResourceForm)
	}
	form := s.resources[ref]
	if form == nil {
		form = &kubeResourceForm{ref: ref}
		s.resources[ref] = form
	}
	return form
}

func (a *App) resourceFormCurrent(f *kubeResourceForm) bool {
	return a.router.Path() == kubeClusterPage && a.router.Query("context") == f.ref.Context && a.cluster.resources[f.ref] == f
}

func (a *App) openResourceEditor(ref ops.KubeResource, tab int) {
	f := a.resourceForm(ref)
	if tab < 0 {
		f.open = true
		a.cluster.resourceDialog = f
	} else {
		a.cluster.tab = tab
	}
	if !f.busy && (f.editor == nil || !f.editor.Buffer.Modified()) {
		a.loadResourceEditor(f)
	}
}

func (a *App) loadResourceEditor(f *kubeResourceForm) {
	f.busy, f.err = true, nil
	if f.editor != nil {
		f.editor.ReadOnly = true
	}
	ref := f.ref
	var draft ops.KubeDraft
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		var err error
		draft, err = ops.LoadKubeDraft(context.Background(), run.Exec{}, env, ref, true)
		return ops.Plan{}, err
	}, func(_ ops.Plan, err error) {
		f.busy, f.err = false, err
		if f.editor != nil {
			f.editor.ReadOnly = false
		}
		if err != nil {
			return
		}
		if f.editor != nil && f.editor.Buffer.Modified() {
			f.previous = f.editor.Buffer.String()
		}
		f.draft, f.readAt = draft, time.Now()
		buf := editor.NewBuffer(draft.YAML)
		buf.SetLanguage(editor.LanguageFor("resource.yaml"))
		f.editor = &editor.State{Buffer: buf, DiffBase: editor.NewBuffer(draft.YAML),
			Label: locales.KubeResourceYaml(), Font: ui.Font{Family: FontMono, Size: 12}, NoFolding: true, NoScrollPastEnd: true}
	})
}

func (a *App) planResourceChange(f *kubeResourceForm, remove bool) {
	if f.busy {
		return
	}
	if a.op.running() {
		a.op.open = true
		return
	}
	f.busy, f.err = true, nil
	ref, draft := f.ref, f.draft
	var yaml string
	if !remove {
		yaml = f.editor.Buffer.String()
		f.editor.ReadOnly = true
	}
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		ctx := context.Background()
		if remove {
			var err error
			draft, err = ops.LoadKubeDraft(ctx, run.Exec{}, env, ref, false)
			if err != nil {
				return ops.Plan{}, err
			}
		}
		return ops.PlanKubeChange(ctx, run.Exec{}, env, draft, yaml, remove)
	}, func(p ops.Plan, err error) {
		f.busy = false
		if !remove {
			f.editor.ReadOnly = false
		}
		if !a.resourceFormCurrent(f) {
			return
		}
		if err != nil {
			if remove {
				a.proposeOrSay(p, err)
			} else {
				f.err = err
			}
			return
		}
		wasRunning := a.op.running()
		a.Propose(p)
		if !wasRunning {
			a.op.after = func() {
				if a.resourceFormCurrent(f) {
					a.cluster.refresh++
					if !remove {
						f.editor.Buffer.MarkSaved()
						a.loadResourceEditor(f)
					}
				}
			}
		}
	})
}

func (a *App) resourceActions(c *ui.Context, ref ops.KubeResource, tab int) {
	if a.BuildPlan == nil || ref.UID == "" {
		return
	}
	busy := a.cluster.resources[ref] != nil && a.cluster.resources[ref].busy
	kubeActionButton(c.Key("resource-edit"), "file-text", locales.KubeResourceEdit(), podActionHeight).
		OnClick(func() { a.openResourceEditor(ref, tab) })
	ui.Box(c).Grow(1)
	kubeActionButton(c.Key("resource-delete"), "trash-2", locales.KubeResourceDelete(), podActionHeight).
		TextColor(c.Theme().Danger).Disabled(busy).OnClick(func() { a.planResourceChange(a.resourceForm(ref), true) })
}

func (a *App) resourceMenu(c *ui.Context, ref ops.KubeResource) {
	logs := ref.Kind == "Job" && a.StreamLogs != nil && a.WatchCluster != nil
	if ref.UID == "" || a.BuildPlan == nil && !logs {
		return
	}
	iconButton(c.Key("resource-actions"), "ellipsis", locales.MoreActions()).Menu(func(m *ui.Menu) {
		if logs && m.Item(locales.KubeLogsOpen()).Chosen() {
			a.openOwnerLogs(ref)
		}
		if a.BuildPlan != nil {
			if m.Item(locales.KubeResourceEdit()).Chosen() {
				a.openResourceEditor(ref, -1)
			}
			if m.Item(locales.KubeResourceDelete()).Chosen() {
				a.planResourceChange(a.resourceForm(ref), true)
			}
		}
	})
}

func (a *App) resourceEditor(c *ui.Context, f *kubeResourceForm) {
	if f == nil {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c.Key(f.ref.UID)).Grow(1).MinHeight(0).Gap(8).Padding(0, 16, 12, 16).Children(func() {
		if f.editor == nil {
			if f.err != nil {
				ui.Text(c, f.err.Error()).FontSize(12).TextColor(pal.DangerText).Wrap()
			}
			if f.busy {
				ui.Spinner(c).Size(16, 16)
			} else {
				ui.Button(c.Key("yaml-reload"), locales.KubeResourceReload()).OnClick(func() { a.loadResourceEditor(f) })
			}
			return
		}
		changed := f.editor.Buffer.Modified()
		bg, fg := pal.Subtle, t.TextMuted
		title := locales.KubeResourceYaml()
		if changed {
			bg, fg = pal.ActiveRow, pal.AccentInk
			title = locales.KubeResourceDraft()
		}
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Padding(8, 10).Radius(8).Background(bg).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, title).FontSize(12).FontWeight(500).TextColor(fg)
				ui.Text(c, locales.KubeResourceVersion(f.draft.Version)+" · "+locales.KubeResourceReadAge(age(c, f.readAt))).
					FontSize(11).TextColor(t.TextMuted)
			})
			if f.previous != "" {
				iconButton(c.Key("yaml-previous"), "copy", locales.KubeResourceCopyDraft()).Size(controlHeight, controlHeight).Disabled(f.busy).
					OnClick(func() { c.WriteClipboard(f.previous) })
			}
			ui.Button(c.Key("yaml-reload"), locales.KubeResourceReload()).Height(controlHeight).Disabled(f.busy).
				OnClick(func() { a.loadResourceEditor(f) })
			ui.PrimaryButton(c.Key("yaml-check"), locales.KubeResourceCheck()).Height(controlHeight).Disabled(f.busy || !changed).
				OnClick(func() { a.planResourceChange(f, false) })
		})
		if f.err != nil {
			ui.Scroll(c.Key("yaml-error")).MaxHeight(72).Children(func() {
				ui.Text(c, f.err.Error()).Font(FontMono).FontSize(11.5).TextColor(pal.DangerText).Wrap()
			})
		}
		editor.View(c.Key("resource-yaml"), f.editor).Grow(1).MinHeight(0).BorderWidth(1).BorderColor(pal.Line).Radius(8).Clip()
	})
}

func (a *App) resourceEditorDialog(c *ui.Context) {
	f := a.cluster.resourceDialog
	if f == nil || !a.resourceFormCurrent(f) {
		return
	}
	dialogBase(c.Key("kube-resource-editor"), &f.open, func(backdrop, panel ui.Element) {
		t := c.Theme()
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Height(484).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(Colors(t).Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18)).Children(func() {
			ui.Column(c).Fill().Gap(6).Children(func() {
				ui.Row(c).Padding(14, 16).AlignItems(ui.Center).Children(func() {
					ui.Text(c, f.ref.Kind+" "+f.ref.Namespace+"/"+f.ref.Name).FontSize(16).Grow(1).Wrap()
					iconButton(c, "x", locales.OpClose()).OnClick(func() { f.open = false })
				})
				a.resourceEditor(c, f)
			})
		})
	})
}
